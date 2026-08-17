package httpx

type catalogMsg struct {
	ZH string
	EN string
}

// ponytail: port of legacy ERROR_CATALOG + Go-only aliases; add a row when a new user-facing detail lands.
var catalog = map[string]catalogMsg{
	"auth.not_authenticated":       {"未登录或登录已失效", "Not authenticated"},
	"auth.invalid_credentials":     {"用户名或密码错误", "Invalid username or password"},
	"auth.account_disabled":        {"账号已禁用", "Account is disabled"},
	"auth.no_tenant_portal":        {"无权访问租户控制台", "No tenant portal access"},
	"auth.no_ops_portal":           {"无权访问运营控制台", "No access to ops portal"},
	"auth.user_not_found":          {"用户不存在", "User not found"},
	"auth.wrong_password":          {"当前密码不正确", "Current password is incorrect"},
	"auth.password_unchanged":      {"新密码不能与当前密码相同", "New password must differ from current password"},
	"auth.password_required":       {"需要先修改密码", "password change required"},
	"rbac.platform_admin_required": {"需要平台管理员权限", "Platform admin role required"},
	"rbac.permission_denied":       {"权限不足", "Permission denied"},
	"rbac.tenant_access_denied":    {"无权访问该租户", "Tenant access denied"},
	"rbac.resource_not_found":      {"资源不存在", "Resource not found"},
	"bucket.not_found":             {"存储桶不存在", "Bucket not found"},
	"bucket.not_empty":             {"存储桶非空，请先删除桶内所有对象后再删除存储桶", "Bucket is not empty. Delete all objects before deleting the bucket."},
	"object.not_found":             {"对象不存在", "Object not found"},
	"object.invalid_key":           {"无效的对象路径", "Invalid object key"},
	"object.directory_exists":      {"目录已存在", "Directory already exists"},
	"object.dest_exists":           {"目标已存在", "Destination already exists"},
	"upload.task_not_found":        {"上传任务不存在", "Upload task not found"},
	"quota.file_too_large":         {"文件大小超过允许的上传限制", "File exceeds maximum upload size"},
	"quota.tenant_exceeded":        {"已超出租户存储配额", "Tenant quota exceeded"},
	"quota.bucket_exceeded":        {"已超出存储桶容量配额", "Bucket quota exceeded"},
	"region.not_found":             {"存储区域不存在", "Region not found"},
	"region.not_active":            {"存储区域未启用", "Storage region is not active"},
	"user.username_exists":         {"用户名已存在", "Username already exists"},
	"user.cannot_disable_self":     {"不能禁用自己的账号", "Cannot disable your own account"},
	"user.cannot_delete_self":      {"不能删除自己的账号", "Cannot delete your own account"},
	"share.not_found":              {"分享链接不存在", "Share link not found"},
	"share.expired":                {"分享链接已过期", "Share link has expired"},
	"share.limit_reached":          {"分享链接访问次数已达上限", "Share link access limit reached"},
	"share.bad_password":           {"分享密码错误或缺失", "Invalid or missing password"},
	"preview.unsupported":          {"不支持预览此文件类型", "Preview not supported for this file type"},
	"preview.text_too_large":       {"文件过大，无法文本预览", "File too large for text preview"},
	"common.internal_error":        {"服务器内部错误", "Internal server error"},
	"common.no_fields_to_update":   {"没有可更新的字段", "No fields to update"},
	"ceph.not_configured":          {"未配置 Ceph 管理 API，请在运营端系统设置中填写", "Ceph management API is not configured (set in ops system settings)"},
	"alert.rule_not_found":         {"告警规则不存在", "Alert rule not found"},
	"alert.event_not_found":        {"告警事件不存在", "Alert event not found"},
	"alert.already_resolved":       {"告警已处理", "Alert already resolved"},
	"alert.channel_not_found":      {"通知渠道不存在", "Notification channel not found"},
}

var byDetail = map[string]string{
	"Not authenticated":              "auth.not_authenticated",
	"missing token":                  "auth.not_authenticated",
	"invalid token":                  "auth.not_authenticated",
	"Invalid username or password":   "auth.invalid_credentials",
	"Account is disabled":            "auth.account_disabled",
	"No tenant portal access":        "auth.no_tenant_portal",
	"No access to ops portal":        "auth.no_ops_portal",
	"No ops portal access":           "auth.no_ops_portal",
	"User not found":                 "auth.user_not_found",
	"Account not found":              "auth.user_not_found",
	"Current password is incorrect":  "auth.wrong_password",
	"New password must differ from current password": "auth.password_unchanged",
	"new_password must differ from old_password":     "auth.password_unchanged",
	"password change required":       "auth.password_required",
	"Platform admin role required":   "rbac.platform_admin_required",
	"Permission denied":              "rbac.permission_denied",
	"Tenant access denied":           "rbac.tenant_access_denied",
	"Resource not found":             "rbac.resource_not_found",
	"Bucket not found":               "bucket.not_found",
	"Bucket is not empty":            "bucket.not_empty",
	"Bucket is not empty. Delete all objects before deleting the bucket.": "bucket.not_empty",
	"Object not found":               "object.not_found",
	"Invalid object key":             "object.invalid_key",
	"Directory already exists":       "object.directory_exists",
	"Destination already exists":     "object.dest_exists",
	"Upload task not found":          "upload.task_not_found",
	"File exceeds maximum upload size": "quota.file_too_large",
	"Tenant quota exceeded":          "quota.tenant_exceeded",
	"Bucket quota exceeded":          "quota.bucket_exceeded",
	"Region not found":               "region.not_found",
	"Storage region not found":       "region.not_found",
	"Storage region is not active":   "region.not_active",
	"Username already exists":        "user.username_exists",
	"Cannot disable your own account": "user.cannot_disable_self",
	"Cannot delete your own account": "user.cannot_delete_self",
	"Share link not found":           "share.not_found",
	"Share link has expired":         "share.expired",
	"Share link access limit reached": "share.limit_reached",
	"Invalid or missing password":    "share.bad_password",
	"Preview not supported for this file type": "preview.unsupported",
	"File too large for text preview": "preview.text_too_large",
	"Internal server error":          "common.internal_error",
	"No fields to update":            "common.no_fields_to_update",
	"Ceph management API is not configured (set in ops system settings)": "ceph.not_configured",
	"Alert rule not found":           "alert.rule_not_found",
	"Alert event not found":          "alert.event_not_found",
	"Alert already resolved":         "alert.already_resolved",
	"Notification channel not found": "alert.channel_not_found",
}

func Localize(detail, locale string) (string, string) {
	code := byDetail[detail]
	if code == "" {
		if _, ok := catalog[detail]; ok {
			code = detail
		}
	}
	if code == "" {
		return detail, ""
	}
	msg, ok := catalog[code]
	if !ok {
		return detail, code
	}
	if locale == "en-US" {
		return msg.EN, code
	}
	return msg.ZH, code
}
