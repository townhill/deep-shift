package sim

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// caveFromRows builds a Cave from rows written in the shared symbol legend.
func caveFromRows(t testing.TB, required int, rows ...string) Cave {
	t.Helper()
	c := Cave{Width: utf8.RuneCountInString(rows[0]), Height: len(rows), RequiredCrystals: required}
	for y, row := range rows {
		if n := utf8.RuneCountInString(row); n != c.Width {
			t.Fatalf("row %d has %d symbols, want %d", y, n, c.Width)
		}
		for x, r := range []rune(row) {
			cell, ok := CellForSymbol(r)
			if !ok {
				t.Fatalf("row %d col %d: unknown symbol %q", y, x, r)
			}
			c.Cells = append(c.Cells, cell)
		}
	}
	return c
}

func newSim(t testing.TB, required int, rows ...string) *Sim {
	t.Helper()
	s, err := New(caveFromRows(t, required, rows...), 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// move waits (stepping with no input) until the movement window is open,
// then steps once with d and returns that tick's events.
func move(t testing.TB, s *Sim, d Dir) []Event {
	t.Helper()
	for i := 0; !s.MoveReady(); i++ {
		if i > MoveIntervalTicks {
			t.Fatalf("movement window never opened (status %s)", s.Status())
		}
		if ev := s.Step(Input{}); len(ev) != 0 {
			t.Fatalf("unexpected events while waiting: %v", ev)
		}
	}
	return s.Step(Input{Move: d})
}

func kinds(events []Event) []EventKind {
	var out []EventKind
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

func wantKinds(t testing.TB, got []Event, want ...EventKind) {
	t.Helper()
	if !slices.Equal(kinds(got), want) {
		t.Fatalf("events = %v, want kinds %v", got, want)
	}
}

func rowsASCII(rows ...string) string { return strings.Join(rows, "\n") + "\n" }

func countTerrain(s *Sim, terr Terrain) int {
	n := 0
	for _, c := range s.st.cells {
		if c.Terrain == terr {
			n++
		}
	}
	return n
}

func countOccupant(s *Sim, o Occupant) int {
	n := 0
	for _, c := range s.st.cells {
		if c.Occupant == o {
			n++
		}
	}
	return n
}
