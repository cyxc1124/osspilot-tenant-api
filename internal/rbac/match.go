package rbac

const (
	ActionRead         = "read"
	ActionWrite        = "write"
	ActionDelete       = "delete"
	ActionEdit         = "edit"
	ActionAdmin        = "admin"
	ActionBucketCreate = "bucket_create"
	ActionBucketDelete = "bucket_delete"
	ActionShare        = "share"
	ActionRestore      = "restore"
	ActionAudit        = "audit"
)

var validActions = map[string]struct{}{
	ActionRead: {}, ActionWrite: {}, ActionDelete: {}, ActionEdit: {}, ActionAdmin: {},
	ActionBucketCreate: {}, ActionBucketDelete: {}, ActionShare: {}, ActionRestore: {}, ActionAudit: {},
}

type Rule struct {
	BucketName *string
	Prefix     *string
	Actions    []string
	UserID     *int64
	RoleName   *string
	GroupID    *int64
}

func ValidAction(name string) bool {
	_, ok := validActions[name]
	return ok
}

func ValidateActions(actions []string) string {
	if len(actions) == 0 {
		return "actions must not be empty"
	}
	for _, a := range actions {
		if !ValidAction(a) {
			return "Unknown actions: " + a
		}
	}
	return ""
}

func accountLevel(userID int64, role string, groupIDs []int64, action string, rules []Rule) bool {
	return Allowed(userID, role, groupIDs, "", "", action, rules)
}

// CreatorAllows: 有账号级 bucket_create 的人对自建桶全权。
func CreatorAllows(userID int64, role string, groupIDs []int64, createdBy *int64, rules []Rule) bool {
	if createdBy == nil || *createdBy != userID {
		return false
	}
	return accountLevel(userID, role, groupIDs, ActionBucketCreate, rules)
}

// CreatorScopedList: read+bucket_create 的人列表只看自己建的桶。
func CreatorScopedList(userID int64, role string, groupIDs []int64, rules []Rule) bool {
	if accountLevel(userID, role, groupIDs, ActionAdmin, rules) {
		return false
	}
	if accountLevel(userID, role, groupIDs, ActionRead, rules) && !accountLevel(userID, role, groupIDs, ActionBucketCreate, rules) {
		return false
	}
	return accountLevel(userID, role, groupIDs, ActionBucketCreate, rules)
}

func bucketOnlyRules(rules []Rule, bucket string) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.BucketName != nil && *r.BucketName == bucket {
			out = append(out, r)
		}
	}
	return out
}

func Allowed(userID int64, role string, groupIDs []int64, bucket, prefix, action string, rules []Rule) bool {
	groups := map[int64]struct{}{}
	for _, id := range groupIDs {
		groups[id] = struct{}{}
	}
	best := -1
	matched := map[string]struct{}{}
	for _, rule := range rules {
		if !ruleApplies(rule, userID, role, groups) {
			continue
		}
		if !bucketMatches(rule.BucketName, bucket) {
			continue
		}
		n, ok := prefixMatchLen(rule.Prefix, prefix)
		if !ok {
			continue
		}
		if n > best {
			best = n
			matched = setOf(rule.Actions)
		} else if n == best {
			for _, a := range rule.Actions {
				matched[a] = struct{}{}
			}
		}
	}
	if best < 0 {
		return false
	}
	if _, ok := matched[ActionAdmin]; ok {
		return true
	}
	_, ok := matched[action]
	return ok
}

func ruleApplies(rule Rule, userID int64, role string, groups map[int64]struct{}) bool {
	if rule.UserID != nil {
		return *rule.UserID == userID
	}
	if rule.RoleName != nil {
		return *rule.RoleName == role
	}
	if rule.GroupID != nil {
		_, ok := groups[*rule.GroupID]
		return ok
	}
	return false
}

func bucketMatches(ruleBucket *string, bucket string) bool {
	if ruleBucket == nil || *ruleBucket == "" {
		return true
	}
	if bucket == "" {
		return false
	}
	return *ruleBucket == bucket
}

func prefixMatchLen(rulePrefix *string, objectPrefix string) (int, bool) {
	rule := ""
	if rulePrefix != nil {
		rule = *rulePrefix
	}
	if rule == "" || rule == "*" {
		return 0, true
	}
	if len(objectPrefix) >= len(rule) && objectPrefix[:len(rule)] == rule {
		return len(rule), true
	}
	return 0, false
}

func setOf(actions []string) map[string]struct{} {
	out := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		out[a] = struct{}{}
	}
	return out
}
