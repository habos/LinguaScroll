// Package config loads process configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config is everything the API and worker read from the environment.
// .env.example lists every key.
type Config struct {
	Env         string // dev | test | prod
	HTTPAddr    string
	DatabaseURL string
	// FakeVendors swaps the YouTubeClient and LanguageDetector for fakes,
	// so a fresh clone runs with no API keys.
	FakeVendors   bool
	YouTubeAPIKey string
	AdminUser     string
	AdminPassword string
	// RateLimitPerMinute is the per-token request budget.
	RateLimitPerMinute int
}

// Load reads Config from the environment, applying dev defaults.
func Load() (Config, error) {
	c := Config{
		Env:                get("APP_ENV", "dev"),
		HTTPAddr:           get("HTTP_ADDR", ":8080"),
		DatabaseURL:        get("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/linguascroll?sslmode=disable"),
		YouTubeAPIKey:      os.Getenv("YOUTUBE_API_KEY"),
		AdminUser:          os.Getenv("ADMIN_USER"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		RateLimitPerMinute: 120,
	}
	var err error
	if c.FakeVendors, err = getBool("FAKE_VENDORS", c.YouTubeAPIKey == ""); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("RATE_LIMIT_PER_MINUTE"); v != "" {
		if c.RateLimitPerMinute, err = strconv.Atoi(v); err != nil || c.RateLimitPerMinute <= 0 {
			return Config{}, fmt.Errorf("RATE_LIMIT_PER_MINUTE: must be a positive integer, got %q", v)
		}
	}
	if !c.FakeVendors && c.YouTubeAPIKey == "" {
		return Config{}, errors.New("YOUTUBE_API_KEY is required when FAKE_VENDORS=0")
	}
	if c.Env == "prod" && (c.AdminUser == "" || c.AdminPassword == "") {
		return Config{}, errors.New("ADMIN_USER and ADMIN_PASSWORD are required in prod")
	}
	return c, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}
