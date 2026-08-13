package auth

const (
	RoleTenantAdmin  = "tenant_admin"
	RoleNormalUser   = "normal_user"
	RoleReadonlyUser = "readonly_user"
	RoleUploadUser   = "upload_user"
	RoleAuditUser    = "audit_user"
)

func ValidRole(name string) bool {
	switch name {
	case RoleTenantAdmin, RoleNormalUser, RoleReadonlyUser, RoleUploadUser, RoleAuditUser:
		return true
	}
	return false
}

func AccountID(u *User) int64 {
	if u == nil {
		return 0
	}
	if u.AccountID != 0 {
		return u.AccountID
	}
	return u.ID
}

func IsAdmin(u *User) bool {
	return u != nil && u.Role == RoleTenantAdmin
}
