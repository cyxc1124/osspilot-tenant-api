package objects

import (
	"context"
	"sort"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

const (
	defaultMaxKeys = 100
	maxListKeys    = 1000
	maxOpKeys      = 1000
	scanCap        = 2000
)

func hiddenKey(key string) bool {
	return strings.HasPrefix(key, TrashPrefix) || strings.HasPrefix(key, VersionPrefix)
}

func isDirectoryMarker(key string, size int64) bool {
	return strings.HasSuffix(key, "/") && size == 0
}

func normalizeListPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, "/") {
		return trimmed
	}
	return trimmed + "/"
}

func isDirectChildPrefix(key, parentPrefix string) bool {
	if !strings.HasSuffix(key, "/") {
		return false
	}
	parent := normalizeListPrefix(parentPrefix)
	if parent != "" && !strings.HasPrefix(key, parent) {
		return false
	}
	rel := strings.Trim(key[len(parent):], "/")
	return rel != "" && !strings.Contains(rel, "/")
}

func collectPrefixes(page storage.ListPage, prefix string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(page.Prefixes))
	for _, p := range page.Prefixes {
		if hiddenKey(p) {
			continue
		}
		out = append(out, p)
		seen[p] = struct{}{}
	}
	for _, obj := range page.Objects {
		if hiddenKey(obj.Key) || !isDirectoryMarker(obj.Key, obj.Size) {
			continue
		}
		if _, ok := seen[obj.Key]; ok {
			continue
		}
		if !isDirectChildPrefix(obj.Key, prefix) {
			continue
		}
		out = append(out, obj.Key)
		seen[obj.Key] = struct{}{}
	}
	sort.Strings(out)
	return out
}

func ListFromS3(ctx context.Context, s3 *storage.Client, bucket, prefix, token string, maxKeys int) ([]summary, []string, *string, bool, error) {
	page, err := s3.ListPrefix(ctx, bucket, prefix, token, int32(maxKeys))
	if err != nil {
		return nil, nil, nil, false, err
	}
	prefixes := collectPrefixes(page, prefix)
	items := make([]summary, 0, len(page.Objects))
	for _, obj := range page.Objects {
		if obj.Key == "" || hiddenKey(obj.Key) || isDirectoryMarker(obj.Key, obj.Size) {
			continue
		}
		items = append(items, summary{
			Key: obj.Key, Size: obj.Size, ETag: obj.ETag, LastModified: obj.LastModified,
		})
	}
	var next *string
	if page.Truncated && page.Token != "" {
		next = &page.Token
	}
	return items, prefixes, next, page.Truncated, nil
}

func ListTrashFromS3(ctx context.Context, s3 *storage.Client, bucket, prefix, token string, maxKeys int) ([]trashItem, *string, bool, []string, error) {
	page, err := s3.ListPrefixFlat(ctx, bucket, TrashPrefix+prefix, token, int32(maxKeys))
	if err != nil {
		return nil, nil, false, nil, err
	}
	items := make([]trashItem, 0, len(page.Objects))
	s3Keys := make([]string, 0, len(page.Objects))
	for _, obj := range page.Objects {
		orig, ok := FromTrashKey(obj.Key)
		if !ok {
			continue
		}
		items = append(items, trashItem{
			Key: orig, Size: obj.Size, LastModified: obj.LastModified,
		})
		s3Keys = append(s3Keys, obj.Key)
	}
	var next *string
	if page.Truncated && page.Token != "" {
		next = &page.Token
	}
	return items, next, page.Truncated, s3Keys, nil
}

type record struct {
	Key          string
	Size         int64
	ContentType  *string
	ETag         *string
	StorageClass *string
	UploadedBy   *int64
	LastModified *string
	CreatedAt    *string
	UpdatedAt    *string
	Username     *string
}

func foldList(prefix string, maxKeys int, recs []record, moreInDB bool) (items []record, prefixes []string, token *string, truncated bool) {
	if maxKeys < 1 {
		maxKeys = 1
	}
	items = []record{}
	prefixes = []string{}
	lastPrefix := ""
	tokenKey := ""
	n := 0
	for _, rec := range recs {
		if strings.HasPrefix(rec.Key, TrashPrefix) || strings.HasPrefix(rec.Key, VersionPrefix) {
			tokenKey = rec.Key
			continue
		}
		if prefix != "" && !strings.HasPrefix(rec.Key, prefix) {
			tokenKey = rec.Key
			continue
		}
		rest := rec.Key[len(prefix):]
		if rest == "" {
			tokenKey = rec.Key
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			p := prefix + rest[:i+1]
			if p == lastPrefix {
				tokenKey = rec.Key
				continue
			}
			if n >= maxKeys {
				truncated = true
				break
			}
			prefixes = append(prefixes, p)
			lastPrefix = p
			n++
			tokenKey = rec.Key
			continue
		}
		lastPrefix = ""
		if n >= maxKeys {
			truncated = true
			break
		}
		items = append(items, rec)
		n++
		tokenKey = rec.Key
	}
	if !truncated && moreInDB {
		truncated = true
	}
	if truncated && tokenKey != "" {
		token = &tokenKey
	}
	return
}

func likePrefix(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix) + "%"
}
