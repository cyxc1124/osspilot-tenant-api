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
