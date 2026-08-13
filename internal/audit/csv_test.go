package audit

import "testing"

func TestCSVHeaders(t *testing.T) {
	want := []string{"用户 ID", "用户名", "租户", "bucket", "object key", "操作类型", "源 IP", "User-Agent", "操作结果", "错误信息", "时间"}
	if len(csvHeaders) != len(want) {
		t.Fatalf("len %d", len(csvHeaders))
	}
	for i := range want {
		if csvHeaders[i] != want[i] {
			t.Fatalf("col %d got %q", i, csvHeaders[i])
		}
	}
}
