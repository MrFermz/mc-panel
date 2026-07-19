package httpapi

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSanitizeDisplayName(t *testing.T) {
	cases := map[string]string{
		"  Steve  ":       "Steve",
		"Steve\nAlex":     "SteveAlex", // ชื่อหลายบรรทัดทำ layout เพี้ยน — control char ถูกทิ้ง
		"\x00\x07admin":   "admin",
		"":                "",
		"   ":             "",
		"สตีฟ นักขุด":     "สตีฟ นักขุด",
		"Steve\tthe\tMan": "StevetheMan",
	}
	for in, want := range cases {
		if got := sanitizeDisplayName(in); got != want {
			t.Errorf("sanitizeDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvatarURL(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")

	if got := avatarURL(id, nil); got != nil {
		t.Errorf("avatarURL with no avatar = %q, want nil", *got)
	}

	at := time.Unix(1700000000, 0)
	got := avatarURL(id, &at)
	if got == nil {
		t.Fatal("avatarURL returned nil for a user with an avatar")
	}
	// ?v= ต้องมาจาก avatar_updated_at — ไม่งั้นเปลี่ยนรูปแล้ว browser ยังโชว์รูปเก่าจาก cache
	want := "/api/users/11111111-2222-3333-4444-555555555555/avatar?v=1700000000"
	if *got != want {
		t.Errorf("avatarURL = %q, want %q", *got, want)
	}
}
