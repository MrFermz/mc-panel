package console

import (
	"fmt"
	"sync"
	"testing"
)

func TestRingKeepsLastNLines(t *testing.T) {
	r := NewRing(5)

	if got := r.Snapshot(); len(got) != 0 {
		t.Fatalf("empty ring snapshot = %v, want empty", got)
	}

	r.Append([]string{"a", "b", "c"})
	got := r.Snapshot()
	want := []string{"a", "b", "c"}
	assertLines(t, got, want)

	r.Append([]string{"d", "e", "f", "g"})
	assertLines(t, r.Snapshot(), []string{"c", "d", "e", "f", "g"})

	// batch ใหญ่กว่า buffer — ต้องเหลือเฉพาะท้ายสุด
	r.Append([]string{"1", "2", "3", "4", "5", "6", "7"})
	assertLines(t, r.Snapshot(), []string{"3", "4", "5", "6", "7"})
}

func TestRingConcurrentAccess(t *testing.T) {
	r := NewRing(RingSize)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				r.Append([]string{fmt.Sprintf("w%d-%d", n, j)})
				r.Snapshot()
			}
		}(i)
	}
	wg.Wait()
	if got := len(r.Snapshot()); got != RingSize {
		t.Fatalf("snapshot length = %d, want %d", got, RingSize)
	}
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("snapshot = %v, want %v", got, want)
		}
	}
}
