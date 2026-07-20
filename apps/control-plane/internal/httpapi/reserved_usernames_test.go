package httpapi

import "testing"

// username ที่เก็บลง DB ต้องเป็น lowercase เสมอ (CHECK constraint ใน migration 00018)
// — canonicalUsername ต้อง lower + trim และผลลัพธ์ต้องผ่าน usernameRe ที่ไม่รับตัวพิมพ์ใหญ่แล้ว
func TestCanonicalUsername(t *testing.T) {
	cases := map[string]string{
		"Alice":     "alice",
		"ALICE":     "alice",
		"  Bob  ":   "bob",
		"my.User-1": "my.user-1",
		"already":   "already",
	}
	for in, want := range cases {
		got := canonicalUsername(in)
		if got != want {
			t.Errorf("canonicalUsername(%q) = %q, want %q", in, got, want)
		}
		if !usernameRe.MatchString(got) {
			t.Errorf("canonicalUsername(%q) = %q, which fails usernameRe", in, got)
		}
	}

	// regex ต้องไม่รับตัวพิมพ์ใหญ่แล้ว — ทางเข้าทุกเส้นต้อง canonical ก่อนถึงจะผ่าน
	if usernameRe.MatchString("Alice") {
		t.Error("usernameRe accepted uppercase 'Alice', want reject")
	}
}

func TestNormalizeUsername(t *testing.T) {
	cases := map[string]string{
		"admin":     "admin",
		"ADMIN":     "admin",
		"A.D.M.I.N": "admin",
		"a-d-m-i-n": "admin",
		"admin_":    "admin",
		"mc-panel":  "mcpanel",
		"___":       "",
		"alice":     "alice",
	}
	for in, want := range cases {
		if got := normalizeUsername(in); got != want {
			t.Errorf("normalizeUsername(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsReservedUsername(t *testing.T) {
	reserved := []string{
		"admin", "Admin", "ADMIN", "a.d.m.i.n", "a-d-m-i-n", "admin_",
		"root", "system", "mcpanel", "mc-panel", "MC_PANEL",
		"support", "owner", "operator", "moderator",
		"___", // เหลือแต่ separator — normalize แล้วว่าง
	}
	for _, u := range reserved {
		if !isReservedUsername(u) {
			t.Errorf("isReservedUsername(%q) = false, want true", u)
		}
	}

	// เทียบแบบตรงตัวเป๊ะบนรูป normalized — ชื่อปกติที่ "มีคำสงวนอยู่ข้างใน" ต้องผ่าน
	allowed := []string{
		"nodeman", "administrators", "adminx", "rooted", "systemic",
		"alice", "bob.smith", "my-admin", "supporter",
	}
	for _, u := range allowed {
		if isReservedUsername(u) {
			t.Errorf("isReservedUsername(%q) = true, want false", u)
		}
	}
}
