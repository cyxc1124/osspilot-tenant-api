package objects

import "strings"

func ValidUserKey(key string) bool {
	if key == "" || key == ".trash" || key == ".versions" {
		return false
	}
	return !strings.HasPrefix(key, trashPrefix) && !strings.HasPrefix(key, ".versions/")
}
