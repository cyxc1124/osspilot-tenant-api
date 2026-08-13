package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	JWTSecret      string
	TokenTTL       time.Duration
	DefaultJWTUsed bool
}

func Load() Config {
	secret := getenv("JWT_SECRET", "change-me-in-development")
	minutes := getenvInt("JWT_ACCESS_TOKEN_EXPIRE_MINUTES", 60)
	return Config{
		HTTPAddr:       getenv("HTTP_ADDR", ":8000"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      secret,
		TokenTTL:       time.Duration(minutes) * time.Minute,
		DefaultJWTUsed: os.Getenv("JWT_SECRET") == "",
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
