package versions

import (
	"fmt"
	"net/url"
	"path"
)

const (
	prefix        = ".versions"
	sourceRestore = "restore"
	defaultLimit  = 50
	maxLimit      = 200
)

func storageKey(bucket, objectKey string, n int) string {
	return fmt.Sprintf("%s/%s/%s/v%d", prefix, bucket, url.PathEscape(objectKey), n)
}

func downloadName(objectKey string, n int) string {
	base := path.Base(objectKey)
	if base == "." || base == "/" {
		base = objectKey
	}
	return fmt.Sprintf("%s.v%d", base, n)
}
