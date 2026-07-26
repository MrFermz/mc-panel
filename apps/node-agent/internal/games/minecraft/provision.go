// provision.go — ที่มาของ artifact ต่อ variant (official source เท่านั้น + verify checksum
// เมื่อ upstream ให้มา). ทุกขั้นตอนต้อง idempotent — job โดน redeliver ซ้ำได้เสมอ
package minecraft

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/mc-panel/node-agent/internal/games"
)

// provision = games.Definition.Provision — เลือกที่มาของ artifact ตาม variant
func provision(ctx context.Context, env games.ProvisionEnv) (string, error) {
	switch env.Variant {
	case "vanilla":
		return provisionVanilla(ctx, env)
	case "paper":
		return provisionPaperProject(ctx, env, "paper", "server.jar")
	case "velocity":
		return provisionPaperProject(ctx, env, "velocity", "velocity.jar")
	case "fabric":
		return provisionFabric(ctx, env)
	case "forge":
		return provisionForge(ctx, env)
	default:
		return "", fmt.Errorf("unsupported server_type %q", env.Variant)
	}
}

// ---------- vanilla (Mojang launchermeta) ----------

const vanillaManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"

func provisionVanilla(ctx context.Context, env games.ProvisionEnv) (string, error) {
	var manifest struct {
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}
	if err := env.FetchJSON(ctx, vanillaManifestURL, &manifest); err != nil {
		return "", err
	}
	versionURL := ""
	for _, v := range manifest.Versions {
		if v.ID == env.Version {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return "", fmt.Errorf("vanilla version %q not found in mojang manifest", env.Version)
	}

	var version struct {
		Downloads struct {
			Server struct {
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"server"`
		} `json:"downloads"`
	}
	if err := env.FetchJSON(ctx, versionURL, &version); err != nil {
		return "", err
	}
	if version.Downloads.Server.URL == "" {
		return "", fmt.Errorf("vanilla version %q has no server download", env.Version)
	}
	dest := filepath.Join(env.Dir, "server.jar")
	if err := env.Download(ctx, version.Downloads.Server.URL, dest, "sha1", version.Downloads.Server.SHA1); err != nil {
		return "", err
	}
	return "vanilla " + env.Version + " sha1=" + version.Downloads.Server.SHA1, nil
}

// ---------- paper / velocity (PaperMC Fill API v3) ----------
// api.papermc.io/v2 ถูกปิดถาวรแล้ว (410 Gone) — ห้ามย้อนกลับไปใช้

const paperFillAPIBase = "https://fill.papermc.io/v3"

func provisionPaperProject(ctx context.Context, env games.ProvisionEnv, project, jarName string) (string, error) {
	var builds []struct {
		ID        int    `json:"id"`
		Channel   string `json:"channel"`
		Downloads map[string]struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			Checksums struct {
				SHA256 string `json:"sha256"`
			} `json:"checksums"`
		} `json:"downloads"`
	}
	buildsURL := fmt.Sprintf("%s/projects/%s/versions/%s/builds", paperFillAPIBase, project, url.PathEscape(env.Version))
	if err := env.FetchJSON(ctx, buildsURL, &builds); err != nil {
		return "", err
	}
	if len(builds) == 0 {
		return "", fmt.Errorf("no %s builds found for version %q", project, env.Version)
	}
	// Fill v3 เรียงใหม่→เก่า — เอา build STABLE ตัวแรก
	// fallback build ใหม่สุดไม่ว่า channel ไหนสำหรับ version ที่ยังมีแต่ experimental
	pick := 0
	for i, b := range builds {
		if b.Channel == "STABLE" {
			pick = i
			break
		}
	}
	b := builds[pick]
	dl, ok := b.Downloads["server:default"]
	if !ok || dl.URL == "" || dl.Checksums.SHA256 == "" {
		return "", fmt.Errorf("%s build %d has no server:default download", project, b.ID)
	}
	dest := filepath.Join(env.Dir, jarName)
	if err := env.Download(ctx, dl.URL, dest, "sha256", dl.Checksums.SHA256); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s build %d sha256=%s", project, env.Version, b.ID, dl.Checksums.SHA256), nil
}

// ---------- fabric (FabricMC meta) ----------

const fabricMetaBase = "https://meta.fabricmc.net/v2"

func provisionFabric(ctx context.Context, env games.ProvisionEnv) (string, error) {
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
			Stable  bool   `json:"stable"`
		} `json:"loader"`
	}
	if err := env.FetchJSON(ctx, fabricMetaBase+"/versions/loader/"+url.PathEscape(env.Version), &loaders); err != nil {
		return "", err
	}
	if len(loaders) == 0 {
		return "", fmt.Errorf("fabric has no loader for mc version %q", env.Version)
	}
	loaderVersion := loaders[0].Loader.Version
	for _, l := range loaders {
		if l.Loader.Stable {
			loaderVersion = l.Loader.Version
			break
		}
	}

	var installers []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := env.FetchJSON(ctx, fabricMetaBase+"/versions/installer", &installers); err != nil {
		return "", err
	}
	if len(installers) == 0 {
		return "", fmt.Errorf("fabric installer list is empty")
	}
	installerVersion := installers[0].Version
	for _, in := range installers {
		if in.Stable {
			installerVersion = in.Version
			break
		}
	}

	// fabric meta ไม่แจก checksum ของ server launcher jar — โหลดจาก official host ตรง ๆ
	jarURL := fmt.Sprintf("%s/versions/loader/%s/%s/%s/server/jar",
		fabricMetaBase, url.PathEscape(env.Version), loaderVersion, installerVersion)
	dest := filepath.Join(env.Dir, "server.jar")
	if err := env.Download(ctx, jarURL, dest, "", ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("fabric %s loader=%s installer=%s", env.Version, loaderVersion, installerVersion), nil
}
