package minecraft

import "testing"

// imageSource ต้องรับเฉพาะ runtime image ของเกมนี้ — ref ของเกมอื่นที่หลุดมาถึง
// (เช่นตอน instance ไม่มี meta.json แล้ว registry ตกไปใช้เกม default) ต้องคืน zero value
// ให้ EnsureRuntimeImage บอกตรง ๆ ว่าเตรียมให้ไม่ได้ ไม่ใช่ไปประกอบชื่อ Adoptium มั่ว ๆ
// (เคสจริงที่เคยเจอ: `game-manager/runtime-steam:1` → pull `eclipse-temurin:1-jre`)
func TestImageSourceOnlyClaimsOwnImages(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		pullFrom string
	}{
		{"own image", "game-manager/runtime-java:21", "docker.io/library/eclipse-temurin:21-jre"},
		{"custom namespace", "acme/runtime-java:8", "docker.io/library/eclipse-temurin:8-jre"},
		{"another game", "game-manager/runtime-steam:1", ""},
		{"non-numeric tag", "game-manager/runtime-java:latest", ""},
		{"no tag", "game-manager/runtime-java", ""},
		{"empty tag", "game-manager/runtime-java:", ""},
		{"suffix lookalike", "game-manager/notruntime-java:21", ""},
	}

	for _, tt := range tests {
		if got := imageSource(tt.ref).PullFrom; got != tt.pullFrom {
			t.Errorf("%s: imageSource(%q).PullFrom = %q, want %q", tt.name, tt.ref, got, tt.pullFrom)
		}
	}
}

// image ของเกมนี้ไม่ผูกสถาปัตยกรรม — JVM ของ Adoptium มีทุก arch ปล่อยให้ node เลือกเอง
func TestImageSourceHasNoPlatformPin(t *testing.T) {
	if got := imageSource("game-manager/runtime-java:21").Platform; got != "" {
		t.Errorf("Platform = %q, want empty (node เลือกตาม arch ของตัวเอง)", got)
	}
}
