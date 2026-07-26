// versions.go — proxy รายชื่อ MC version จาก upstream (Mojang/PaperMC/Fabric/Forge)
// พร้อม cache in-memory 10 นาที — คืนรายการเรียงใหม่ -> เก่าเสมอ
// (นี่คือ implementation ของ games.VersionSpec.List สำหรับเกมนี้)
package minecraft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/game-manager/control-plane/internal/games"
)

const (
	cacheTTL        = 10 * time.Minute
	maxResponseSize = 16 << 20 // manifest ของ Mojang ใหญ่หลัก MB

	vanillaManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"
	// PaperMC ปิด api.papermc.io/v2 แล้ว (410 Gone) — ต้องใช้ Fill API v3 เท่านั้น
	paperFillProjectURL = "https://fill.papermc.io/v3/projects/"
	fabricGameURL       = "https://meta.fabricmc.net/v2/versions/game"
	forgePromotionsURL  = "https://files.minecraftforge.net/maven/net/minecraftforge/forge/promotions_slim.json"
)

// versionService ถือ cache ของรายการเวอร์ชันต่อ variant — มี state จึงต้องสร้างครั้งเดียว
// ต่อ process แล้วผูกเข้า Definition (ดู New)
type versionService struct {
	client *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	fetchedAt time.Time
	versions  []string
}

func newVersionService() *versionService {
	return &versionService{
		client: &http.Client{Timeout: 15 * time.Second},
		cache:  make(map[string]cacheEntry),
	}
}

// list = games.VersionSpec.List — variant ที่ไม่รู้จักคืน games.ErrUnknownVariant
// (caller เช็คกับ catalog ของ Definition มาก่อนแล้ว นี่เป็นด่านสำรอง)
func (s *versionService) list(ctx context.Context, variant string) ([]string, error) {
	switch variant {
	case "vanilla", "paper", "velocity", "fabric", "forge":
	default:
		return nil, games.ErrUnknownVariant
	}

	s.mu.Lock()
	if e, ok := s.cache[variant]; ok && time.Since(e.fetchedAt) < cacheTTL {
		s.mu.Unlock()
		return e.versions, nil
	}
	s.mu.Unlock()

	var (
		versions []string
		err      error
	)
	switch variant {
	case "vanilla":
		versions, err = s.fetchVanilla(ctx)
	case "paper", "velocity":
		versions, err = s.fetchPaperProject(ctx, variant)
	case "fabric":
		versions, err = s.fetchFabric(ctx)
	case "forge":
		versions, err = s.fetchForge(ctx)
	}
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[variant] = cacheEntry{fetchedAt: time.Now(), versions: versions}
	s.mu.Unlock()
	return versions, nil
}

func (s *versionService) fetchJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "game-manager/control-plane")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream %s returned status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(dst)
}

func (s *versionService) fetchVanilla(ctx context.Context) ([]string, error) {
	var manifest struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := s.fetchJSON(ctx, vanillaManifestURL, &manifest); err != nil {
		return nil, fmt.Errorf("fetch vanilla manifest: %w", err)
	}
	// manifest เรียงใหม่ -> เก่าอยู่แล้ว เอาเฉพาะ release (ตัด snapshot/beta)
	versions := make([]string, 0, len(manifest.Versions))
	for _, v := range manifest.Versions {
		if v.Type == "release" {
			versions = append(versions, v.ID)
		}
	}
	return versions, nil
}

func (s *versionService) fetchPaperProject(ctx context.Context, project string) ([]string, error) {
	// Fill v3 คืน versions เป็น map family -> รายการ version (ลำดับ family หายใน Go map
	// ต้อง flatten แล้ว sort เอง)
	var out struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := s.fetchJSON(ctx, paperFillProjectURL+project, &out); err != nil {
		return nil, fmt.Errorf("fetch %s versions: %w", project, err)
	}
	// paper: ตัด pre-release/rc ออกให้รายการสะอาด
	// velocity: ต้องเก็บ "-SNAPSHOT" ไว้ เพราะ release ปกติของ velocity ใช้ชื่อแบบนั้น
	keepPrerelease := project == "velocity"
	var versions []string
	for _, list := range out.Versions {
		for _, v := range list {
			if !keepPrerelease && strings.Contains(v, "-") {
				continue
			}
			versions = append(versions, v)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	return versions, nil
}

func (s *versionService) fetchFabric(ctx context.Context) ([]string, error) {
	var out []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := s.fetchJSON(ctx, fabricGameURL, &out); err != nil {
		return nil, fmt.Errorf("fetch fabric versions: %w", err)
	}
	versions := make([]string, 0, len(out))
	for _, v := range out {
		if v.Stable {
			versions = append(versions, v.Version)
		}
	}
	return versions, nil
}

func (s *versionService) fetchForge(ctx context.Context) ([]string, error) {
	var out struct {
		Promos map[string]string `json:"promos"`
	}
	if err := s.fetchJSON(ctx, forgePromotionsURL, &out); err != nil {
		return nil, fmt.Errorf("fetch forge promotions: %w", err)
	}
	// key เป็น "<mcver>-recommended" / "<mcver>-latest" — เอาเฉพาะ mc version
	// ที่มี promoted build เพราะ build อื่นเสถียรภาพไม่การันตี
	seen := make(map[string]bool)
	var versions []string
	for key := range out.Promos {
		mcVer, suffix, ok := strings.Cut(key, "-")
		if !ok || (suffix != "recommended" && suffix != "latest") {
			continue
		}
		if !seen[mcVer] {
			seen[mcVer] = true
			versions = append(versions, mcVer)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	return versions, nil
}

// compareVersions เทียบเลขเวอร์ชันแบบ numeric ต่อส่วน ("1.10" > "1.9")
// ส่วนที่ parse ไม่ได้นับเป็น 0
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var an, bn int
		if i < len(as) {
			an, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bn, _ = strconv.Atoi(bs[i])
		}
		if an != bn {
			if an > bn {
				return 1
			}
			return -1
		}
	}
	return 0
}
