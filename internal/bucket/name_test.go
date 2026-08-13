package bucket

import "testing"

func TestValidateName(t *testing.T) {
	ok := []string{"abc", "my-bucket-1", "a.b.c"}
	for _, n := range ok {
		if err := validateName(n); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	bad := []string{"ab", "A-bucket", "a..b", "a.-b", "a-.b", "-abc", "abc-"}
	for _, n := range bad {
		if err := validateName(n); err == nil {
			t.Fatalf("%s: want error", n)
		}
	}
}
