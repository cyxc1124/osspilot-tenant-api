package bucket

import "testing"

func TestValidatePolicy(t *testing.T) {
	ok := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect":   "Allow",
				"Action":   "s3:GetObject",
				"Resource": "arn:aws:s3:::demo/*",
			},
		},
	}
	if err := validatePolicy(ok); err != nil {
		t.Fatal(err)
	}
	if err := validatePolicy(map[string]any{"Version": "2099-01-01", "Statement": ok["Statement"]}); err == nil {
		t.Fatal("want version error")
	}
	if err := validatePolicy(map[string]any{"Version": "2012-10-17", "Statement": []any{}}); err == nil {
		t.Fatal("want empty statement error")
	}
	if err := validatePolicy(map[string]any{
		"Version":   "2012-10-17",
		"Statement": []any{map[string]any{"Effect": "Allow", "Principal": "*"}},
	}); err == nil {
		t.Fatal("want action error")
	}
}
