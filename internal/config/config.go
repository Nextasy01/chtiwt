package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL   string
	HTTPAddr      string
	RTMPAddr      string
	StateDir      string
	FFmpegPath    string
	PublicRTMPURL string
	SecureCookies bool
	SessionTTL    time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		RTMPAddr:      envOr("RTMP_ADDR", ":1935"),
		StateDir:      envOr("STATE_DIR", "./state"),
		FFmpegPath:    envOr("FFMPEG_PATH", "ffmpeg"),
		PublicRTMPURL: envOr("PUBLIC_RTMP_URL", "rtmp://localhost:1935/live"),
		SecureCookies: envBool("SECURE_COOKIES", false),
		SessionTTL:    envDuration("SESSION_TTL", 30*24*time.Hour),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
