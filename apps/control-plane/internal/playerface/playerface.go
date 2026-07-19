// Package playerface ดึง skin ของผู้เล่นจาก Mojang (control-plane มี egress ผ่าน edge network)
// แล้ว crop เป็น "หน้า" (face + hat overlay) เสิร์ฟให้ web — ไม่ให้ browser ยิง third-party host เอง
// (leak IP ของ user + เพิ่ม host ที่ต้องเชื่อใจ) ตาม posture ของ repo
//
// cache เก็บใน Postgres (ตาราง player_faces) ไม่ใช่ in-memory: skin/ชื่อที่เปลี่ยนจะ refresh
// เองเมื่อ entry เกิน TTL, กัน rate-limit ของ Mojang, รอดข้าม restart และ — จุดสำคัญ —
// **เสิร์ฟรูปเก่าที่เก็บไว้ได้ตอน Mojang ติดต่อไม่ได้** (graceful degradation)
package playerface

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

	"github.com/mc-panel/control-plane/internal/store"
)

// ErrNoSkin: uuid นี้ไม่มี skin ให้ (offline-mode uuid ที่ Mojang ไม่รู้จัก, โปรไฟล์ไม่มี texture)
// handler map เป็น 404 → web fallback ไปตัวอักษรย่อ
var ErrNoSkin = errors.New("playerface: no skin for uuid")

const (
	sessionURL   = "https://sessionserver.mojang.com/session/minecraft/profile/"
	fetchTimeout = 6 * time.Second
	maxSkinSize  = 1 << 20 // skin PNG ปกติ 64x64 ไม่กี่ KB — เผื่อไว้พอ กัน body ยักษ์

	// TTL: hit เก็บนาน (skin ไม่ค่อยเปลี่ยน + ลด load Mojang), miss เก็บสั้นให้ retry ได้เร็ว
	hitTTL  = 6 * time.Hour
	missTTL = 15 * time.Minute

	faceScale = 16 // 8x8 → 128x128 (nearest-neighbor, คม ๆ แบบ pixel art)
)

// Cache ดึง+crop face ต่อ uuid โดยใช้ Postgres เป็น cache store (ผ่าน store.Store)
type Cache struct {
	client *http.Client
	st     *store.Store
}

func NewCache(st *store.Store) *Cache {
	return &Cache{
		client: &http.Client{Timeout: fetchTimeout},
		st:     st,
	}
}

// Face คืน PNG ของหน้าผู้เล่น (128x128). อ่านจาก cache ก่อน ถ้าสด (ยังไม่เกิน TTL) คืนเลย;
// ถ้าเก่า/ยังไม่มี ลองดึงใหม่จาก Mojang. Mojang ล่มแต่มี cache เก่า → คืน cache เก่า
// (นี่คือเหตุผลหลักที่ย้ายมาเก็บ storage). คืน ErrNoSkin เมื่อรู้ว่าไม่มี skin
func (c *Cache) Face(ctx context.Context, id uuid.UUID) ([]byte, error) {
	cached, err := c.st.GetPlayerFace(ctx, id)
	hasCached := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		// DB error จริง (ไม่ใช่ "ไม่มีแถว") — ยังลองดึงจาก Mojang ต่อ ถือว่าไม่มี cache
		hasCached = false
	}

	if hasCached && time.Now().Before(freshUntil(cached)) {
		return serveCached(cached)
	}

	facePNG, ferr := c.fetch(ctx, strings.ReplaceAll(id.String(), "-", ""))
	switch {
	case ferr == nil:
		_ = c.st.SavePlayerFace(ctx, id, facePNG) // best-effort — เสิร์ฟได้แม้เขียน cache พลาด
		return facePNG, nil
	case errors.Is(ferr, ErrNoSkin):
		_ = c.st.SavePlayerFace(ctx, id, nil) // negative cache
		return nil, ErrNoSkin
	default:
		// Mojang/network ล่ม — ตกกลับไปใช้ cache เก่าถ้ามี (แม้จะ stale) ไม่งั้นค่อยคืน error
		if hasCached {
			return serveCached(cached)
		}
		return nil, ferr
	}
}

// freshUntil: hit (มีรูป) สดนาน, miss (negative) สดสั้น ให้ retry Mojang ไวขึ้น
func freshUntil(f *store.PlayerFace) time.Time {
	if f.PNG == nil {
		return f.FetchedAt.Add(missTTL)
	}
	return f.FetchedAt.Add(hitTTL)
}

func serveCached(f *store.PlayerFace) ([]byte, error) {
	if f.PNG == nil {
		return nil, ErrNoSkin
	}
	return f.PNG, nil
}

func (c *Cache) fetch(ctx context.Context, idHex string) ([]byte, error) {
	skinURL, err := c.skinURL(ctx, idHex)
	if err != nil {
		return nil, err
	}
	skin, err := c.fetchSkin(ctx, skinURL)
	if err != nil {
		return nil, err
	}
	return cropFace(skin)
}

// skinURL: query session profile → decode textures property → SKIN url
func (c *Cache) skinURL(ctx context.Context, idHex string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sessionURL+idHex, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 204/404 = ไม่มีโปรไฟล์ (offline uuid) → ไม่มี skin
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return "", ErrNoSkin
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("playerface: session status " + resp.Status)
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
		return "", ErrNoSkin
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
		return "", ErrNoSkin
	}
	return "https://" + url, nil
}

func (c *Cache) fetchSkin(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("playerface: skin status " + resp.Status)
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
		return nil, ErrNoSkin
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
