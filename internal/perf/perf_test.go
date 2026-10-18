package perf

import (
	"testing"
	"time"
)

func TestWindowAverageAndP95(t *testing.T) {
	w := NewWindow(100)
	if w.Len() != 0 || w.Avg() != 0 || w.P95() != 0 {
		t.Fatal("empty window is not all zero")
	}
	for i := 1; i <= 100; i++ {
		w.Add(time.Duration(i) * time.Millisecond)
	}
	if w.Len() != 100 || w.Avg() != 50500*time.Microsecond || w.P95() != 95*time.Millisecond {
		t.Fatalf("len %d avg %v p95 %v", w.Len(), w.Avg(), w.P95())
	}
}

func TestWindowKeepsOnlyTheMostRecent(t *testing.T) {
	w := NewWindow(4)
	for _, ms := range []int{100, 100, 100, 100, 1, 2, 3, 4} {
		w.Add(time.Duration(ms) * time.Millisecond)
	}
	if w.Len() != 4 || w.Avg() != 2500*time.Microsecond || w.P95() != 4*time.Millisecond {
		t.Fatalf("len %d avg %v p95 %v", w.Len(), w.Avg(), w.P95())
	}
	w = NewWindow(10)
	w.Add(7 * time.Millisecond)
	if w.P95() != 7*time.Millisecond || w.Avg() != 7*time.Millisecond {
		t.Fatalf("single sample: avg %v p95 %v", w.Avg(), w.P95())
	}
}
