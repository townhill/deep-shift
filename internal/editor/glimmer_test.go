package editor

import (
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// glimmerKey is the glimmer brush's number in Input.Brush: '=' is the
// twelfth key in cmd/deepshift's brushKeys, after 1 to 9, 0 and -.
const glimmerKey = 12

// glimmersClockwise lists the glimmer symbols as the brush turns them.
var glimmersClockwise = []rune{'▲', '▶', '▼', '◀'}

func TestGlimmerBrushOnEqualsTurnsClockwise(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: glimmerKey})
	if b := Palette[e.brush]; b.Key != "=" || b.Name != "Glimmer" || e.symbol() != '▲' {
		t.Fatalf("brush %d is %s on %q painting %q; want the glimmer facing up on =", glimmerKey, b.Name, b.Key, e.symbol())
	}
	// Pressing = again turns it clockwise, all the way round and back to up.
	for _, want := range []rune{'▶', '▼', '◀', '▲'} {
		tap(e, Input{Brush: glimmerKey})
		if e.symbol() != want {
			t.Fatalf("pressing = again gave %q, want %q", e.symbol(), want)
		}
	}
	// So does clicking its slot.
	x, y := testLayout.Slot(e.brush)
	tap(e, Input{MouseX: x + 4, MouseY: y + 4, Paint: true})
	if e.symbol() != '▶' {
		t.Fatalf("clicking the slot gave %q, want the glimmer turned right", e.symbol())
	}
	// Every variant is a glimmer in the simulation, and they go round
	// clockwise like the crawler's.
	facings := []sim.Dir{sim.DirUp, sim.DirRight, sim.DirDown, sim.DirLeft}
	for i, r := range Palette[e.brush].Variants {
		c, ok := sim.CellForSymbol(r)
		if !ok || c.Occupant != sim.OccupantGlimmer || c.Facing != facings[i] {
			t.Errorf("variant %d %q: %+v, want a glimmer facing %v", i, r, c, facings[i])
		}
	}
	if got := e.View().Hint; !strings.Contains(got, "Brush =: glimmer facing right") {
		t.Fatalf("hint %q", got)
	}
}

func TestGlimmerBrushRaisesTheFormatVersionAndTheCaveLoads(t *testing.T) {
	d := NewCave()
	d.FormatVersion = 4 // the last version with no glimmers
	e, st := open(t, d, "")
	tap(e, Input{Brush: glimmerKey})
	p := pos(5, 5)
	click(e, p)
	if e.doc.at(p) != '▲' || e.doc.formatVersion != 5 {
		t.Fatalf("painted %q, version %d; want a glimmer and version 5", e.doc.at(p), e.doc.formatVersion)
	}
	if !strings.Contains(e.View().Hint, "Format version raised to 5") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	if e.Level() == nil {
		t.Fatalf("a painted glimmer should leave the cave valid: %v", e.Problems())
	}
	tap(e, Input{Save: true})
	data, ok := st.files["/caves/new_cave.json"]
	if !ok {
		t.Fatalf("not saved: %q", e.View().Hint)
	}
	l, err := level.Parse("new_cave.json", data)
	if err != nil {
		t.Fatalf("the saved cave does not load: %v\n%s", err, data)
	}
	if l.FormatVersion != 5 || l.Glimmers() != 1 || []rune(l.Rows[p.Y])[p.X] != '▲' {
		t.Fatalf("loaded version %d with %d glimmers, row %q", l.FormatVersion, l.Glimmers(), l.Rows[p.Y])
	}
	// A cave that already holds glimmers keeps version 5.
	tap(e, Input{Undo: true})
	if e.doc.formatVersion != 4 {
		t.Fatalf("undo should restore version 4, got %d", e.doc.formatVersion)
	}
}

func TestAltClickPicksEachGlimmerFacing(t *testing.T) {
	d := NewCave()
	row := []rune(d.Rows[2])
	for i, r := range glimmersClockwise {
		row[2+i] = r
	}
	d.Rows[2] = string(row)
	e, _ := open(t, d, "")
	for i, r := range glimmersClockwise {
		tap(e, Input{Brush: 1}) // the wall, so the pick is seen to change the brush
		drag(e, false, Input{Alt: true}, pos(2+i, 2))
		if b := Palette[e.brush]; b.Key != "=" || e.symbol() != r {
			t.Fatalf("Alt-click on %q picked %s %q", r, b.Name, e.symbol())
		}
	}
	if e.Dirty() {
		t.Fatal("picking changed the cave")
	}
}

func TestBoxAndFillOfGlimmersCoverTheArea(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: glimmerKey})
	tap(e, Input{Brush: glimmerKey}) // facing right
	drag(e, false, Input{Shift: true}, pos(3, 3), pos(6, 5))
	if n := occurrences(e, '▶'); n != 12 {
		t.Fatalf("a 4x3 box placed %d glimmers, want 12", n)
	}
	// A Cmd-click fills the rest of the earth: 28x14 inside the wall, less
	// the player start, the exit and the box.
	drag(e, false, Input{Cmd: true}, pos(10, 10))
	if n := occurrences(e, '▶'); n != 28*14-2 {
		t.Fatalf("after the flood fill %d glimmers, want %d", n, 28*14-2)
	}
	// Only the player start, the exit and valves place just one.
	for _, b := range Palette {
		for _, r := range b.Variants {
			if want := r == sim.SymbolPlayer || r == sim.SymbolExit || r == sim.SymbolValveShut; oneOnly(r) != want {
				t.Errorf("oneOnly(%q) = %v, want %v", r, !want, want)
			}
		}
	}
}
