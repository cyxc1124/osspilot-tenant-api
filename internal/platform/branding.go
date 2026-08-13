package platform

const (
	defaultLogo     = "O"
	defaultTitle    = "OssPilot 对象存储"
	defaultSubtitle = "租户控制台"
)

type Branding struct {
	LogoText string `json:"logo_text"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

func brandingFrom(rows map[string]string) Branding {
	if rows == nil {
		rows = map[string]string{}
	}
	return Branding{
		LogoText: pick(rows, "tenant_login_logo_text", defaultLogo),
		Title:    pick(rows, "tenant_login_title", defaultTitle),
		Subtitle: pick(rows, "tenant_login_subtitle", defaultSubtitle),
	}
}
