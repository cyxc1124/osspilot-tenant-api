package stats

import "testing"

func TestUsagePercent(t *testing.T) {
	if usagePercent(10, nil) != nil {
		t.Fatal("nil quota")
	}
	q := int64(0)
	if usagePercent(10, &q) != nil {
		t.Fatal("zero quota")
	}
	q = 200
	got := usagePercent(50, &q)
	if got == nil || *got != 0.25 {
		t.Fatalf("got %#v", got)
	}
}

func TestParsePeriod(t *testing.T) {
	p, ok := parsePeriod("")
	if !ok || p != "24h" {
		t.Fatalf("default %s %v", p, ok)
	}
	if _, ok := parsePeriod("1h"); ok {
		t.Fatal("1h should fail")
	}
	if p, ok := parsePeriod("7d"); !ok || p != "7d" {
		t.Fatalf("7d %s %v", p, ok)
	}
}
