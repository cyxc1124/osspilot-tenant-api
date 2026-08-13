package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	JWTSecret         string
	TokenTTL          time.Duration
	DefaultJWTUsed    bool
	S3Endpoint        string
	RGWAccessKey      string
	RGWSecretKey      string
	S3Region          string
	DownloadCDNURL    string
	PreviewCDNURL     string
	ObjectHTTPDomain  string
	ObjectHTTPSDomain string
	ProjectionSecret  string
}

func Load() Config {
	secret := getenv("JWT_SECRET", "change-me-in-development")
	minutes := getenvInt("JWT_ACCESS_TOKEN_EXPIRE_MINUTES", 60)
	return Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8000"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		JWTSecret:         secret,
		TokenTTL:          time.Duration(minutes) * time.Minute,
		DefaultJWTUsed:    os.Getenv("JWT_SECRET") == "",
		S3Endpoint:        os.Getenv("S3_ENDPOINT"),
		RGWAccessKey:      os.Getenv("RGW_ACCESS_KEY"),
		RGWSecretKey:      os.Getenv("RGW_SECRET_KEY"),
		S3Region:          getenv("S3_REGION", "us-east-1"),
		DownloadCDNURL:    os.Getenv("DOWNLOAD_CDN_URL"),
		PreviewCDNURL:     os.Getenv("PREVIEW_CDN_URL"),
		ObjectHTTPDomain:  os.Getenv("OBJECT_HTTP_DOMAIN"),
		ObjectHTTPSDomain: os.Getenv("OBJECT_HTTPS_DOMAIN"),
		ProjectionSecret:  os.Getenv("PROJECTION_SECRET"),
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
