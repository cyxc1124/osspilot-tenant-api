package objects

import "testing"

func TestShouldQueue(t *testing.T) {
	if shouldQueue(1) {
		t.Fatal("one key stays sync")
	}
	if !shouldQueue(2) {
		t.Fatal("two keys queue")
	}
	if !shouldQueue(1000) {
		t.Fatal("large batch queues")
	}
}
