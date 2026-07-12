package jobs

import "testing"

func TestDockerImage(t *testing.T) {
	tests := []struct {
		serverType string
		mcVersion  string
		want       string
	}{
		{"velocity", "3.4.0", "mcpanel/mc-runtime:25"},
		{"velocity", "", "mcpanel/mc-runtime:25"},

		// calendar versioning (Mojang เปลี่ยนเป็น YY.N ตั้งแต่ 2025) -> Java ใหม่สุด
		{"vanilla", "26.2", "mcpanel/mc-runtime:25"},
		{"paper", "26.1.2", "mcpanel/mc-runtime:25"},
		{"vanilla", "25.0", "mcpanel/mc-runtime:25"},

		{"vanilla", "1.21.4", "mcpanel/mc-runtime:21"},
		{"paper", "1.21", "mcpanel/mc-runtime:21"},
		{"fabric", "1.20.5", "mcpanel/mc-runtime:21"},
		{"vanilla", "1.20.6", "mcpanel/mc-runtime:21"},
		{"vanilla", "1.22", "mcpanel/mc-runtime:21"},

		{"paper", "1.20.4", "mcpanel/mc-runtime:17"},
		{"vanilla", "1.20", "mcpanel/mc-runtime:17"},
		{"vanilla", "1.20.0", "mcpanel/mc-runtime:17"},
		{"forge", "1.17", "mcpanel/mc-runtime:17"},
		{"paper", "1.17.1", "mcpanel/mc-runtime:17"},
		{"fabric", "1.19.2", "mcpanel/mc-runtime:17"},
		{"vanilla", "1.18.2", "mcpanel/mc-runtime:17"},

		{"vanilla", "1.16.5", "mcpanel/mc-runtime:8"},
		{"forge", "1.12.2", "mcpanel/mc-runtime:8"},
		{"vanilla", "1.8.9", "mcpanel/mc-runtime:8"},
		{"vanilla", "1.0", "mcpanel/mc-runtime:8"},

		// snapshot / รูปแบบแปลก ๆ / parse ไม่ได้ -> fallback ใหม่สุด :25
		// (Java backward-compatible: jar เก่ารันบน JVM ใหม่ได้ ปลอดภัยเป็น default)
		{"vanilla", "24w14a", "mcpanel/mc-runtime:25"},
		{"vanilla", "1.20.5-rc1", "mcpanel/mc-runtime:25"},
		{"vanilla", "", "mcpanel/mc-runtime:25"},
		{"vanilla", "2.0", "mcpanel/mc-runtime:25"},
		{"vanilla", "1", "mcpanel/mc-runtime:25"},
		{"vanilla", "1.20.4.1", "mcpanel/mc-runtime:25"},
		{"vanilla", "1.-3", "mcpanel/mc-runtime:25"},
		{"vanilla", " 1.16.5", "mcpanel/mc-runtime:8"},
	}
	for _, tt := range tests {
		if got := DockerImage(tt.serverType, tt.mcVersion); got != tt.want {
			t.Errorf("DockerImage(%q, %q) = %q, want %q", tt.serverType, tt.mcVersion, got, tt.want)
		}
	}
}
