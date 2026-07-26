// avatar.go — รูปประจำตัวผู้เล่นของ Minecraft: ดึง skin จาก Mojang แล้ว crop เป็น "หน้า"
// (face 8x8 + hat overlay) ขยายแบบ nearest-neighbor
//
// control-plane เป็นคนดึงเอง ไม่ให้ browser ยิง third-party host (leak IP ของ user +
// เพิ่ม host ที่ต้องเชื่อใจ) ตาม posture ของ repo. ชั้น cache เป็นของกลางอยู่ที่
// internal/avatarcache — ที่นี่มีแค่ "วิธีได้รูปมา" ของเกมนี้
package minecraft

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/game-manager/control-plane/internal/games"
)

const (
	sessionURL   = "https://sessionserver.mojang.com/session/minecraft/profile/"
	fetchTimeout = 6 * time.Second
	maxSkinSize  = 1 << 20 // skin PNG ปกติ 64x64 ไม่กี่ KB — เผื่อไว้พอ กัน body ยักษ์

	faceScale = 16 // 8x8 → 128x128 (nearest-neighbor, คม ๆ แบบ pixel art)
)

var avatarClient = &http.Client{Timeout: fetchTimeout}

// FetchAvatar = games/avatarcache.Fetcher ของ Minecraft — ดึง skin แล้วคืน PNG หน้า 128x128
// คืน games.ErrNoAvatar เมื่อรู้แน่ว่า uuid นี้ไม่มี skin (offline-mode uuid / ไม่มี texture)
func FetchAvatar(ctx context.Context, id uuid.UUID) ([]byte, error) {
	idHex := strings.ReplaceAll(id.String(), "-", "")
	skinURL, err := skinURL(ctx, idHex)
	if err != nil {
		return nil, err
	}
	skin, err := fetchSkin(ctx, skinURL)
	if err != nil {
		return nil, err
	}
	return cropFace(skin)
}

// skinURL: query session profile → decode textures property → SKIN url
func skinURL(ctx context.Context, idHex string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL+idHex, nil)
	if err != nil {
		return "", err
	}
	resp, err := avatarClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 204/404 = ไม่มีโปรไฟล์ (offline uuid) → ไม่มี skin
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return "", games.ErrNoAvatar
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("minecraft: session status " + resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSkinSize))
	if err != nil {
		return "", err
	}

	var profile struct {
		Properties []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", err
	}

	var textureB64 string
	for _, p := range profile.Properties {
		if p.Name == "textures" {
			textureB64 = p.Value
			break
		}
	}
	if textureB64 == "" {
		return "", games.ErrNoAvatar
	}

	raw, err := base64.StdEncoding.DecodeString(textureB64)
	if err != nil {
		return "", err
	}
	var tex struct {
		Textures struct {
			Skin struct {
				URL string `json:"url"`
			} `json:"SKIN"`
		} `json:"textures"`
	}
	if err := json.Unmarshal(raw, &tex); err != nil {
		return "", err
	}
	// กัน SSRF: profile property มาจาก Mojang แต่ URL ในนั้นต้องชี้ textures ของ Mojang เท่านั้น
	// ไม่งั้นกลายเป็น open proxy ยิง host อะไรก็ได้. Mojang ส่ง url เป็น http:// (ไม่ใช่ https)
	// จึงรับทั้งสอง scheme แล้ว upgrade เป็น https ตอนดึงจริง
	url := tex.Textures.Skin.URL
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	if !strings.HasPrefix(url, "textures.minecraft.net/") {
		return "", games.ErrNoAvatar
	}
	return "https://" + url, nil
}

func fetchSkin(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := avatarClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("minecraft: skin status " + resp.Status)
	}
	img, err := png.Decode(io.LimitReader(resp.Body, maxSkinSize))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// cropFace ตัดหน้า 8x8 (base ที่ (8,8) + hat overlay ที่ (40,8)) แล้วขยายแบบ nearest-neighbor
// รองรับ skin 64x64 และ legacy 64x32 (face อยู่ตำแหน่งเดียวกัน). hat ทับเฉพาะ pixel ที่ไม่โปร่งใส
func cropFace(skin image.Image) ([]byte, error) {
	b := skin.Bounds()
	if b.Dx() < 64 || b.Dy() < 32 {
		return nil, games.ErrNoAvatar
	}
	ox, oy := b.Min.X, b.Min.Y

	const faceSize = 8
	out := image.NewNRGBA(image.Rect(0, 0, faceSize*faceScale, faceSize*faceScale))

	for fy := 0; fy < faceSize; fy++ {
		for fx := 0; fx < faceSize; fx++ {
			base := color.NRGBAModel.Convert(skin.At(ox+8+fx, oy+8+fy)).(color.NRGBA)
			hat := color.NRGBAModel.Convert(skin.At(ox+40+fx, oy+8+fy)).(color.NRGBA)
			px := base
			if hat.A > 0 {
				px = overlay(base, hat)
			}
			for dy := 0; dy < faceScale; dy++ {
				for dx := 0; dx < faceScale; dx++ {
					out.SetNRGBA(fx*faceScale+dx, fy*faceScale+dy, px)
				}
			}
		}
	}

	return encodePNG(out)
}

// overlay: alpha-composite hat ทับ base (src-over) — hat.A=255 = ทับเต็ม
func overlay(base, hat color.NRGBA) color.NRGBA {
	if hat.A == 255 {
		return hat
	}
	a := float64(hat.A) / 255
	blend := func(s, d uint8) uint8 {
		return uint8(float64(s)*a + float64(d)*(1-a) + 0.5)
	}
	return color.NRGBA{
		R: blend(hat.R, base.R),
		G: blend(hat.G, base.G),
		B: blend(hat.B, base.B),
		A: 255,
	}
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytesBuffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// bytesBuffer: io.Writer เล็ก ๆ เลี่ยง import bytes เพิ่มโดยไม่จำเป็น
type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
