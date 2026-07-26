// identity.go — ตรวจชื่อผู้เล่นกับ Mojang profile API (control-plane มี egress ผ่าน edge network)
// แปลง 32-hex id เป็น dashed uuid.UUID ให้ตรงกับที่ whitelist.json ของ Minecraft ต้องการ
//
// ความรู้เฉพาะ Minecraft ทั้งก้อน — ชั้น httpapi เห็นแค่ games.PlayerSpec.Lookup
package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/game-manager/control-plane/internal/games"
)

const (
	profileURL             = "https://api.mojang.com/users/profiles/minecraft/"
	lookupTimeout          = 5 * time.Second
	maxProfileResponseSize = 1 << 16
)

// lookupProfile query Mojang profile ของ username. คืน games.ErrPlayerNotFound เมื่อไม่มีตัวตน,
// error อื่น (network/timeout/non-2xx) = upstream ไม่พร้อม (handler map เป็น 502)
func lookupProfile(ctx context.Context, username string) (games.Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL+username, nil)
	if err != nil {
		return games.Profile{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return games.Profile{}, err
	}
	defer resp.Body.Close()

	// 204/404 = ไม่มี username นี้ (Mojang เคยตอบทั้งสองแบบ)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return games.Profile{}, games.ErrPlayerNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return games.Profile{}, errors.New("minecraft: mojang unexpected status " + resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProfileResponseSize))
	if err != nil {
		return games.Profile{}, err
	}
	// body ว่าง (บาง edge case ตอบ 200 body ว่างแทน 204)
	if len(body) == 0 {
		return games.Profile{}, games.ErrPlayerNotFound
	}

	var raw struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return games.Profile{}, err
	}
	if raw.ID == "" {
		return games.Profile{}, games.ErrPlayerNotFound
	}

	// Mojang ส่ง id เป็น 32 hex ไม่มี dash — uuid.Parse รับได้ทั้งสองแบบ
	id, err := uuid.Parse(raw.ID)
	if err != nil {
		return games.Profile{}, err
	}
	return games.Profile{UUID: id, Username: raw.Name}, nil
}
