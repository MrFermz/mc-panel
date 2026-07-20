// Package config โหลด configuration ทั้งหมดจาก environment variables
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr    string
	GRPCAddr    string
	DatabaseURL string
	RedisURL    string
	NATSURL     string
	JWTSecret   string
	// AdminUsername = username ของ admin คนแรกที่ seed ตอน boot (default "admin")
	AdminUsername string
	// NodeToken (optional) ใช้ seed node "local" ตัวแรกใน compose แบบ all-in-one
	NodeToken    string
	CookieSecure bool
	// AllowedOrigins ว่าง = อนุญาตเฉพาะ Origin ที่ host ตรงกับ request (same-host)
	AllowedOrigins []string
	// TrustedProxyCount จำนวน proxy hop ที่เชื่อถือได้หน้า control-plane
	// (production อยู่หลัง Caddy 1 hop). ใช้เลือก entry ที่ถูกต้องจาก
	// X-Forwarded-For — client ปลอม entry ซ้ายมือได้ ต้องนับจากขวา.
	// 0 = ไม่มี proxy น่าเชื่อถือ ใช้ RemoteAddr ตรง ๆ (เช่น dev ไม่มี Caddy)
	TrustedProxyCount int
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		GRPCAddr:        getenv("GRPC_ADDR", ":9090"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisURL:        os.Getenv("REDIS_URL"),
		NATSURL:         os.Getenv("NATS_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		// lowercase เสมอ — DB มี CHECK บังคับ (00018) ตั้ง ADMIN_USERNAME=Admin มาก็ต้อง seed ผ่าน
		AdminUsername: strings.ToLower(strings.TrimSpace(getenv("ADMIN_USERNAME", "admin"))),
		NodeToken:       os.Getenv("NODE_TOKEN"),
		// default 1 = production หลัง Caddy 1 hop; dev set 0 ผ่าน env
		TrustedProxyCount: 1,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}
	if cfg.NATSURL == "" {
		return nil, fmt.Errorf("NATS_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("COOKIE_SECURE must be a boolean, got %q", v)
		}
		cfg.CookieSecure = b
	}

	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}

	if v := os.Getenv("TRUSTED_PROXY_COUNT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_COUNT must be an integer, got %q", v)
		}
		cfg.TrustedProxyCount = n
	}

	return cfg, nil
}

// HTTPPort คืน port จาก listen address (":8080" -> "8080") ใช้กับ -healthcheck
func HTTPPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "8080"
	}
	return port
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
