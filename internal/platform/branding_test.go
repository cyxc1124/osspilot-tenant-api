package platform

import "testing"

func TestBrandingFrom(t *testing.T) {
	d := brandingFrom(nil)
	if d.LogoText != "O" || d.Title != "OssPilot 对象存储" || d.Subtitle != "租户控制台" {
		t.Fatalf("%+v", d)
	}
	got := brandingFrom(map[string]string{
		"tenant_login_logo_text": "X",
		"tenant_login_title":     "自定义标题",
		"tenant_login_subtitle":  "  ",
	})
	if got.LogoText != "X" || got.Title != "自定义标题" || got.Subtitle != "租户控制台" {
		t.Fatalf("%+v", got)
	}
}
