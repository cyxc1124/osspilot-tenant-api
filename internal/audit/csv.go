package audit

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"
)

var csvHeaders = []string{
	"用户 ID", "用户名", "租户", "bucket", "object key", "操作类型", "源 IP", "User-Agent", "操作结果", "错误信息", "时间",
}

func writeCSV(w io.Writer, items []Entry) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeaders); err != nil {
		return err
	}
	for _, e := range items {
		if err := cw.Write([]string{
			intp(e.UserID), strp(e.Username), strp(e.TenantName), strp(e.BucketName), strp(e.ObjectKey),
			e.Action, strp(e.SourceIP), strp(e.UserAgent), e.Status, strp(e.ErrorMessage),
			e.CreatedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func strp(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intp(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}
