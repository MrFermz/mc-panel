package provision

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func (p *Provisioner) fetchJSON(ctx context.Context, url string, v any) error {
	// metadata เป็นไฟล์เล็ก — timeout สั้นกว่า download ปกติมาก
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func newHasher(algo string) hash.Hash {
	switch algo {
	case "sha1":
		return sha1.New()
	case "sha256":
		return sha256.New()
	default:
		return nil
	}
}

// downloadFile โหลด url ลง dest พร้อม verify checksum (เมื่อ upstream ให้มา)
// idempotent: ไฟล์ที่มีอยู่แล้วและ checksum ตรง (หรือไม่มี checksum ให้เทียบ) จะข้าม
func (p *Provisioner) downloadFile(ctx context.Context, url, dest, algo, wantSum string) error {
	if fileVerified(dest, algo, wantSum) {
		log.Printf("download skipped (already present): %s", dest)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}

	// เขียนลง .part ก่อนแล้วค่อย rename — กันไฟล์ครึ่ง ๆ กลาง ๆ ถูกนับว่าเสร็จตอน redeliver
	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	h := newHasher(algo)
	var w io.Writer = f
	if h != nil && wantSum != "" {
		w = io.MultiWriter(f, h)
	}
	_, copyErr := io.Copy(w, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("download %s: %w", url, copyErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if h != nil && wantSum != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != wantSum {
			os.Remove(tmp)
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", url, got, wantSum)
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	log.Printf("downloaded: %s -> %s", url, dest)
	return nil
}

func fileVerified(dest, algo, wantSum string) bool {
	fi, err := os.Stat(dest)
	if err != nil || fi.Size() == 0 {
		return false
	}
	h := newHasher(algo)
	if h == nil || wantSum == "" {
		// upstream ไม่มี checksum ให้เทียบ — มีไฟล์ไม่ว่างถือว่าโหลดสำเร็จแล้ว
		return true
	}
	f, err := os.Open(dest)
	if err != nil {
		return false
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == wantSum
}
