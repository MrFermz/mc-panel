// Package mojang ตรวจ username กับ Mojang profile API (control-plane มี egress ผ่าน edge network)
// แปลง 32-hex id เป็น dashed uuid.UUID ให้ตรงกับที่ whitelist.json ต้องการ
package mojang

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound: username ไม่มีตัวตน (Mojang ตอบ 404 หรือ body ว่าง) — แยกจาก error อื่น
// เพื่อให้ handler map เป็น 404 player_not_found ไม่ใช่ 502
var ErrNotFound = errors.New("mojang: username not found")

const (
	profileURL      = "https://api.mojang.com/users/profiles/minecraft/"
	lookupTimeout   = 5 * time.Second
	maxResponseSize = 1 << 16
)

type Profile struct {
	UUID     uuid.UUID
	Username string // canonical case จาก Mojang
}

// Lookup query Mojang profile ของ username. คืน ErrNotFound เมื่อไม่มีตัวตน,
// error อื่น (network/timeout/non-2xx) = upstream ไม่พร้อม (handler map เป็น 502)
func Lookup(ctx context.Context, username string) (Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL+username, nil)
	if err != nil {
		return Profile{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()

	// 204/404 = ไม่มี username นี้ (Mojang เคยตอบทั้งสองแบบ)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return Profile{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Profile{}, errors.New("mojang: unexpected status " + resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return Profile{}, err
	}
	// body ว่าง (บาง edge case ตอบ 200 body ว่างแทน 204)
	if len(body) == 0 {
		return Profile{}, ErrNotFound
	}

	var raw struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Profile{}, err
	}
	if raw.ID == "" {
		return Profile{}, ErrNotFound
	}

	// Mojang ส่ง id เป็น 32 hex ไม่มี dash — uuid.Parse รับได้ทั้งสองแบบ
	id, err := uuid.Parse(raw.ID)
	if err != nil {
		return Profile{}, err
	}
	return Profile{UUID: id, Username: raw.Name}, nil
}
