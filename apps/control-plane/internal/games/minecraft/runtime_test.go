package minecraft

import "testing"

func TestRuntimeImage(t *testing.T) {
	tests := []struct {
		variant     string
		gameVersion string
		want        string
	}{
		{"velocity", "3.4.0", "game-manager/runtime-java:25"},
		{"velocity", "", "game-manager/runtime-java:25"},

		// calendar versioning (Mojang เปลี่ยนเป็น YY.N ตั้งแต่ 2025) -> Java ใหม่สุด
		{"vanilla", "26.2", "game-manager/runtime-java:25"},
		{"paper", "26.1.2", "game-manager/runtime-java:25"},
		{"vanilla", "25.0", "game-manager/runtime-java:25"},

		{"vanilla", "1.21.4", "game-manager/runtime-java:21"},
		{"paper", "1.21", "game-manager/runtime-java:21"},
		{"fabric", "1.20.5", "game-manager/runtime-java:21"},
		{"vanilla", "1.20.6", "game-manager/runtime-java:21"},
		{"vanilla", "1.22", "game-manager/runtime-java:21"},

		{"paper", "1.20.4", "game-manager/runtime-java:17"},
		{"vanilla", "1.20", "game-manager/runtime-java:17"},
		{"vanilla", "1.20.0", "game-manager/runtime-java:17"},
		{"forge", "1.17", "game-manager/runtime-java:17"},
		{"paper", "1.17.1", "game-manager/runtime-java:17"},
		{"fabric", "1.19.2", "game-manager/runtime-java:17"},
		{"vanilla", "1.18.2", "game-manager/runtime-java:17"},

		{"vanilla", "1.16.5", "game-manager/runtime-java:8"},
		{"forge", "1.12.2", "game-manager/runtime-java:8"},
		{"vanilla", "1.8.9", "game-manager/runtime-java:8"},
		{"vanilla", "1.0", "game-manager/runtime-java:8"},

		// snapshot / รูปแบบแปลก ๆ / parse ไม่ได้ -> fallback ใหม่สุด :25
		// (Java backward-compatible: jar เก่ารันบน JVM ใหม่ได้ ปลอดภัยเป็น default)
		{"vanilla", "24w14a", "game-manager/runtime-java:25"},
		{"vanilla", "1.20.5-rc1", "game-manager/runtime-java:25"},
		{"vanilla", "", "game-manager/runtime-java:25"},
		{"vanilla", "2.0", "game-manager/runtime-java:25"},
		{"vanilla", "1", "game-manager/runtime-java:25"},
		{"vanilla", "1.20.4.1", "game-manager/runtime-java:25"},
		{"vanilla", "1.-3", "game-manager/runtime-java:25"},
		{"vanilla", " 1.16.5", "game-manager/runtime-java:8"},
	}
	for _, tt := range tests {
		if got := RuntimeImage(tt.variant, tt.gameVersion); got != tt.want {
			t.Errorf("RuntimeImage(%q, %q) = %q, want %q", tt.variant, tt.gameVersion, got, tt.want)
		}
	}
}
