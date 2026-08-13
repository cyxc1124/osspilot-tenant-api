package bucket

import (
	"fmt"
)

var allowedPolicyVersions = map[string]bool{"2012-10-17": true, "2008-10-17": true}

func validatePolicy(policy map[string]any) error {
	if policy == nil {
		return fmt.Errorf("Policy must be a JSON object")
	}
	version, _ := policy["Version"].(string)
	if !allowedPolicyVersions[version] {
		return fmt.Errorf("Version must be one of: 2012-10-17, 2008-10-17")
	}
	raw, ok := policy["Statement"]
	if !ok {
		return fmt.Errorf("Statement must be a non-empty array")
	}
	stmts, ok := asList(raw)
	if !ok || len(stmts) == 0 {
		return fmt.Errorf("Statement must be a non-empty array")
	}
	for i, item := range stmts {
		stmt, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("Statement[%d] must be a JSON object", i)
		}
		effect, _ := stmt["Effect"].(string)
		if effect != "Allow" && effect != "Deny" {
			return fmt.Errorf("Statement[%d].Effect must be Allow or Deny", i)
		}
		if _, hasAction := stmt["Action"]; !hasAction {
			if _, hasNot := stmt["NotAction"]; !hasNot {
				return fmt.Errorf("Statement[%d] must include Action or NotAction", i)
			}
		}
		if _, hasRes := stmt["Resource"]; !hasRes {
			if _, hasNot := stmt["NotResource"]; !hasNot {
				return fmt.Errorf("Statement[%d] must include Resource or NotResource", i)
			}
		}
		for _, field := range []string{"Action", "NotAction", "Resource", "NotResource"} {
			v, exists := stmt[field]
			if !exists {
				continue
			}
			if !isStringOrStringList(v) {
				return fmt.Errorf("Statement[%d].%s must be a string or array", i, field)
			}
		}
	}
	return nil
}

func asList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	default:
		return nil, false
	}
}

func isStringOrStringList(v any) bool {
	switch t := v.(type) {
	case string:
		return true
	case []any:
		for _, item := range t {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}
