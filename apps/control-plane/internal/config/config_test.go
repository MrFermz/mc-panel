package config

import "testing"

func TestHTTPPort(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":8080", "8080"},
		{"0.0.0.0:9000", "9000"},
		{"", "8080"},
		{"garbage", "8080"},
	}
	for _, tt := range tests {
		if got := HTTPPort(tt.addr); got != tt.want {
			t.Errorf("HTTPPort(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_URL", "redis://x")
	t.Setenv("NATS_URL", "nats://x")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000, https://panel.example.com/ ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.GRPCAddr != ":9090" {
		t.Errorf("default addrs = %q %q", cfg.HTTPAddr, cfg.GRPCAddr)
	}
	if cfg.AdminEmail != "admin@mcpanel.local" {
		t.Errorf("default admin email = %q", cfg.AdminEmail)
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure = false, want true")
	}
	if len(cfg.AllowedOrigins) != 2 ||
		cfg.AllowedOrigins[0] != "http://localhost:3000" ||
		cfg.AllowedOrigins[1] != "https://panel.example.com" {
		t.Errorf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}

	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Error("Load() without JWT_SECRET should fail")
	}
}
