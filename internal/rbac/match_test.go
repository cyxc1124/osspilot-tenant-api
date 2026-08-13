package rbac

import "testing"

func strp(s string) *string { return &s }
func i64p(n int64) *int64   { return &n }

func TestAllowedLongestPrefix(t *testing.T) {
	uid := int64(2)
	rules := []Rule{
		{UserID: &uid, Prefix: strp(""), Actions: []string{ActionRead}},
		{UserID: &uid, BucketName: strp("docs"), Prefix: strp("pub/"), Actions: []string{ActionRead, ActionWrite}},
		{UserID: &uid, BucketName: strp("docs"), Prefix: strp("pub/secret/"), Actions: []string{ActionRead}},
	}
	if !Allowed(uid, "normal_user", nil, "docs", "pub/a.txt", ActionWrite, rules) {
		t.Fatal("write on pub/ should use pub/ rule")
	}
	if Allowed(uid, "normal_user", nil, "docs", "pub/secret/a.txt", ActionWrite, rules) {
		t.Fatal("longer prefix should drop write")
	}
	if !Allowed(uid, "normal_user", nil, "docs", "pub/secret/a.txt", ActionRead, rules) {
		t.Fatal("read still granted by secret prefix")
	}
}

func TestAllowedAdminImpliesAll(t *testing.T) {
	uid := int64(3)
	rules := []Rule{{UserID: &uid, Actions: []string{ActionAdmin}}}
	if !Allowed(uid, "normal_user", nil, "any", "x", ActionDelete, rules) {
		t.Fatal("admin should grant delete")
	}
}

func TestAllowedSubjects(t *testing.T) {
	uid := int64(4)
	gid := int64(9)
	role := "upload_user"
	rules := []Rule{
		{RoleName: &role, Actions: []string{ActionWrite}},
		{GroupID: &gid, BucketName: strp("b"), Actions: []string{ActionDelete}},
	}
	if !Allowed(uid, role, nil, "", "", ActionWrite, rules) {
		t.Fatal("role rule should match account-level write")
	}
	if Allowed(uid, role, nil, "b", "k", ActionDelete, rules) {
		t.Fatal("group rule requires membership")
	}
	if !Allowed(uid, "normal_user", []int64{gid}, "b", "k", ActionDelete, rules) {
		t.Fatal("group member should get delete")
	}
}

func TestValidateActions(t *testing.T) {
	if ValidateActions(nil) == "" {
		t.Fatal("empty actions")
	}
	if ValidateActions([]string{"read", "nope"}) == "" {
		t.Fatal("unknown action")
	}
	if ValidateActions([]string{ActionRead, ActionShare}) != "" {
		t.Fatal("valid actions")
	}
}
