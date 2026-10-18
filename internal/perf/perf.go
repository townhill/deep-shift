// Package perf keeps rolling timing statistics for the performance overlay
// (SPEC §12: record average and p95 timings on the reference Mac). It only
// observes: nothing in the simulation reads it.
package perf

import (
	"slices"
	"time"
)

// Window holds the most recent durations, up to its capacity.
type Window struct {
	samples []time.Duration
	next    int
	full    bool
}

// NewWindow returns a window that keeps the last n durations.
func NewWindow(n int) *Window { return &Window{samples: make([]time.Duration, n)} }

// Add records d, replacing the oldest sample once the window is full.
func (w *Window) Add(d time.Duration) {
	w.samples[w.next] = d
	w.next++
	if w.next == len(w.samples) {
		w.next, w.full = 0, true
	}
}

// Len returns how many samples the window holds.
func (w *Window) Len() int {
	if w.full {
		return len(w.samples)
	}
	return w.next
}

// Avg returns the mean of the samples, or 0 if there are none.
func (w *Window) Avg() time.Duration {
	n := w.Len()
	if n == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range w.samples[:n] {
		sum += d
	}
	return sum / time.Duration(n)
}

// P95 returns the 95th-percentile sample (nearest rank), or 0 if there are
// none.
func (w *Window) P95() time.Duration {
	n := w.Len()
	if n == 0 {
		return 0
	}
	s := slices.Clone(w.samples[:n])
	slices.Sort(s)
	return s[(95*n+99)/100-1]
}
