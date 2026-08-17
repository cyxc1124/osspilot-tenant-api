package storage

import (
	"testing"
	"time"
)

func TestRewriteCDN(t *testing.T) {
	in := "https://rgw.example/bucket/key?X-Amz-Signature=1"
	got := rewriteCDN(in, "https://cdn.example")
	if got != "https://cdn.example/bucket/key?X-Amz-Signature=1" {
		t.Fatalf("got %q", got)
	}
	if rewriteCDN(in, "") != in {
		t.Fatal("empty cdn")
	}
}

func TestOverlayAccountWins(t *testing.T) {
	cfg := Overlay(Config{Endpoint: "http://env", Region: "us-east-1"}, map[string]string{
		"s3_endpoint": "http://platform", "s3_region_name": "default",
	})
	ctx := WithAccountS3(t.Context(), "http://account", "cn-east-1")
	cfg = applyAccountS3(ctx, cfg)
	if cfg.Endpoint != "http://account" || cfg.Region != "cn-east-1" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestOverlayUploadTTL(t *testing.T) {
	cfg := Overlay(Config{}, map[string]string{"default_upload_presign_expires": "900"})
	if cfg.UploadTTL != 900*time.Second {
		t.Fatal(cfg.UploadTTL)
	}
	cfg = Overlay(Config{}, map[string]string{"default_upload_presign_expires": "60"})
	if cfg.UploadTTL != 0 {
		t.Fatal("out of range should be ignored")
	}
}
