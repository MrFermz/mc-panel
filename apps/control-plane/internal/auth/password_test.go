package auth

import (
	"strings"
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword() error: %v", err)
		}
		if len(pw) != 20 {
			t.Fatalf("password length = %d, want 20", len(pw))
		}
		for _, c := range pw {
			if !strings.ContainsRune(passwordAlphabet, c) {
				t.Fatalf("password contains char outside a-zA-Z0-9: %q", c)
			}
		}
		if seen[pw] {
			t.Fatalf("duplicate password generated: %s", pw)
		}
		seen[pw] = true
	}
}

func TestHashToken(t *testing.T) {
	// sha256("test") — ค่าอ้างอิงคงที่ กัน regression เรื่อง encoding
	const want = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if got := HashToken("test"); got != want {
		t.Fatalf("HashToken(test) = %s, want %s", got, want)
	}
}
