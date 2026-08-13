package platform

import "testing"

func TestProjectableKeys(t *testing.T) {
	for _, k := range []string{"trash_cleanup_enabled", "version_cleanup_enabled", "multipart_cleanup_enabled", "tenant_login_title"} {
		if _, ok := projectable[k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if _, ok := projectable["lifecycle_cleanup_enabled"]; ok {
		t.Fatal("lifecycle stays on ops")
	}
}
