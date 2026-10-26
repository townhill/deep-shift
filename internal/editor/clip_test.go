package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// selectBox Alt-drags from a to b.
func selectBox(e *Editor, a, b sim.Pos) { drag(e, false, Input{Alt: true}, a, b) }

// rowsOf returns the symbols of box b as strings.
func rowsOf(e *Editor, b Box) []string {
	var rows []string
	for y := b.Y0; y <= b.Y1; y++ {
		rows = append(rows, string(e.doc.grid[y][b.X0:b.X1+1]))
	}
	return rows
}

func TestCopyAndPaste(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 4})
	click(e, pos(3, 3))
	tap(e, Input{Brush: 5})
	click(e, pos(4, 4))
	selectBox(e, pos(2, 2), pos(5, 4))
	if v := e.View(); !v.SelectionOK || v.Selection != (Box{2, 2, 5, 4}) || !strings.Contains(v.Hint, "Selected 4 x 3") {
		t.Fatalf("selection %+v %v, hint %q", v.Selection, v.SelectionOK, v.Hint)
	}
	want := rowsOf(e, Box{2, 2, 5, 4})
	tap(e, Input{Copy: true})
	if !strings.Contains(e.View().Hint, "Copied 4 x 3") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	undos := len(e.undo)
	tap(e, Input{Paste: true})
	e.Update(over(e, pos(10, 8), Input{}))
	if v := e.View(); v.Paste == nil || v.PasteW != 4 || len(v.Paste) != 12 || v.GhostOK {
		t.Fatalf("paste preview: %d cells, %d wide, ghost %v", len(v.Paste), v.PasteW, v.GhostOK)
	}
	click(e, pos(10, 8))
	click(e, pos(20, 2)) // still pasting: a second copy
	if got := rowsOf(e, Box{10, 8, 13, 10}); !slices.Equal(got, want) {
		t.Fatalf("pasted %q, want %q", got, want)
	}
	if got := rowsOf(e, Box{20, 2, 23, 4}); !slices.Equal(got, want) || len(e.undo) != undos+2 {
		t.Fatalf("second paste %q, %d undo steps", got, len(e.undo)-undos)
	}
	if !strings.HasPrefix(e.View().Hint, "Pasting 4 x 3: click to place it") {
		t.Fatalf("hint while pasting %q", e.View().Hint)
	}
	tap(e, Input{Back: true}) // stops pasting
	if e.pasting || e.View().Paste != nil || strings.Contains(e.View().Hint, "Pasting") {
		t.Fatalf("Esc should stop pasting, and the hint say so: %q", e.View().Hint)
	}
	tap(e, Input{Undo: true})
	if rowsOf(e, Box{20, 2, 23, 4})[1] != "...." {
		t.Fatal("each paste is one undo step")
	}
	// Pasting at the bottom-right corner places the copy's top-left 2x2
	// (".." over ".*") and leaves out what does not fit.
	tap(e, Input{Paste: true})
	click(e, pos(28, 14))
	if got := rowsOf(e, Box{28, 14, 29, 15}); !slices.Equal(got, []string{"..", ".*"}) || e.doc.width() != 30 || e.doc.height() != 16 {
		t.Fatalf("clipped paste: %q, cave %dx%d", got, e.doc.width(), e.doc.height())
	}
	drag(e, true, Input{}, pos(5, 5)) // a right-click stops pasting and erases nothing
	if e.pasting || e.doc.at(pos(5, 5)) != '.' {
		t.Fatal("a right-click should only stop pasting")
	}
}

func TestOptionClickStillPicks(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	drag(e, false, Input{Alt: true}, pos(5, 5)) // earth
	if e.symbol() != '.' || e.selected {
		t.Fatalf("Option-click picked %q, selected %v", e.symbol(), e.selected)
	}
}

func TestCutDeleteAndDeselect(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	selectBox(e, pos(3, 3), pos(6, 6))
	tap(e, Input{Cut: true})
	if got := rowsOf(e, Box{3, 3, 6, 6}); got[0] != "    " || len(e.undo) != 1 || !strings.Contains(e.View().Hint, "Cut 4 x 4") {
		t.Fatalf("cut: %q, %d undo steps", got, len(e.undo))
	}
	tap(e, Input{Paste: true})
	click(e, pos(3, 3))
	tap(e, Input{Back: true})
	if e.Dirty() {
		t.Fatal("pasting the cut back should restore the cave")
	}
	tap(e, Input{Backspace: true})
	if rowsOf(e, Box{3, 3, 6, 6})[2] != "    " {
		t.Fatal("Delete should clear the selection to floor")
	}
	if got := tap(e, Input{Back: true}); got != RequestNone || e.selected {
		t.Fatalf("Esc with a selection deselects: request %v, selected %v", got, e.selected)
	}
	tap(e, Input{Undo: true})
	if got := tap(e, Input{Back: true}); got != RequestLeave {
		t.Fatalf("then Esc leaves a clean cave: %v", got)
	}
}

func TestCopyNeedsASelectionAndPasteACopy(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Copy: true})
	if !strings.Contains(e.View().Hint, "Select cells to copy first") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	tap(e, Input{Paste: true})
	if e.pasting || !strings.Contains(e.View().Hint, "Nothing copied yet") {
		t.Fatalf("hint %q", e.View().Hint)
	}
}

func TestPastedSluicesGetTheirOwnLinks(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "") // valve (12,6), gates (13,11), (14,11)
	tap(e, over(e, pos(12, 6), Input{Toggle: true}))    // open, so the copy is open too
	selectBox(e, pos(12, 6), pos(14, 11))
	tap(e, Input{Copy: true})
	drag(e, false, Input{Alt: true}, pos(12, 6)) // the original valve is selected: the copy must not join it
	tap(e, Input{Paste: true})
	click(e, pos(20, 6))
	m := e.doc.mechanisms
	if len(m) != 2 || m[1].Valve != pos(20, 6) || !slices.Equal(m[1].Gates, []sim.Pos{pos(21, 11), pos(22, 11)}) {
		t.Fatalf("mechanisms %+v", m)
	}
	if e.doc.at(pos(20, 6)) != 'v' || e.doc.at(pos(21, 11)) != 'g' || !slices.Equal(m[0].Gates, []sim.Pos{pos(13, 11), pos(14, 11)}) {
		t.Fatal("the copy should be open, and the original keep its gates")
	}
	if err := checkInvariants(e); err != nil {
		t.Fatal(err)
	}
}

func TestAValveCopiedWithoutItsGatesHasNone(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	selectBox(e, pos(11, 6), pos(12, 7)) // the valve, but not its gates
	tap(e, Input{Copy: true})
	tap(e, Input{Paste: true})
	click(e, pos(20, 2))
	m := e.doc.mechanisms
	if len(m) != 2 || m[1].Valve != pos(21, 2) || len(m[1].Gates) != 0 || !hasProblem(e, "a valve must operate at least one gate") {
		t.Fatalf("mechanisms %+v, problems %v", m, e.Problems())
	}
}

func TestPastedGatesWithoutTheirValveJoinTheSelectedOne(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	selectBox(e, pos(13, 11), pos(14, 11)) // the gates, not the valve
	tap(e, Input{Copy: true})
	tap(e, Input{Paste: true})
	click(e, pos(20, 13))
	if e.doc.owner(pos(20, 13)) >= 0 || !hasProblem(e, "not listed") {
		t.Fatal("with no valve selected, pasted gates are unlinked")
	}
	tap(e, Input{Back: true})
	tap(e, Input{Undo: true})
	drag(e, false, Input{Alt: true}, pos(12, 6)) // select the valve
	tap(e, Input{Paste: true})
	click(e, pos(20, 13))
	if got := e.doc.mechanisms[0].Gates; !slices.Equal(got, []sim.Pos{pos(13, 11), pos(14, 11), pos(20, 13), pos(21, 13)}) || len(e.Problems()) != 0 {
		t.Fatalf("gates %v, problems %v", got, e.Problems())
	}
}

func TestPastingAPlayerStartMovesIt(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	selectBox(e, pos(1, 1), pos(2, 2))
	tap(e, Input{Copy: true})
	tap(e, Input{Paste: true})
	click(e, pos(10, 10))
	if occurrences(e, '@') != 1 || e.doc.at(pos(10, 10)) != '@' || e.doc.at(pos(1, 1)) != ' ' {
		t.Fatal("a pasted player start moves the cave's own")
	}
}

func TestSelectAllAndShrinkingKeepTheSelectionInside(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{SelectAll: true})
	if v := e.View(); v.Selection != (Box{0, 0, 29, 15}) {
		t.Fatalf("Cmd+A selected %+v", v.Selection)
	}
	tap(e, Input{Shrink: sim.DirRight})
	tap(e, Input{Shrink: sim.DirDown})
	if v := e.View(); !v.SelectionOK || v.Selection != (Box{0, 0, 28, 14}) {
		t.Fatalf("after shrinking, selection %+v %v", v.Selection, v.SelectionOK)
	}
	tap(e, Input{Copy: true}) // must not read outside the cave
	if c, ok := e.Clip(); !ok || c.W != 29 || c.H != 15 {
		t.Fatalf("clip %dx%d", c.W, c.H)
	}
	selectBox(e, pos(27, 13), pos(28, 14))
	for range 3 {
		tap(e, Input{Shrink: sim.DirRight})
	}
	if e.selected {
		t.Fatal("a selection left wholly outside the cave is dropped")
	}
}

func TestClipCarriesToAnotherCave(t *testing.T) {
	a, _ := open(t, embedded(t, "cave_04").Draft(), "")
	selectBox(a, pos(12, 6), pos(14, 11))
	tap(a, Input{Copy: true})
	clip, ok := a.Clip()
	if !ok {
		t.Fatal("no clip")
	}
	b, _ := open(t, NewCave(), "")
	b.SetClip(clip)
	tap(b, Input{Paste: true})
	click(b, pos(5, 2))
	if len(b.doc.mechanisms) != 1 || b.doc.mechanisms[0].Valve != pos(5, 2) || b.doc.formatVersion != level.FormatVersion {
		t.Fatalf("mechanisms %+v", b.doc.mechanisms)
	}
	if err := checkInvariants(b); err != nil {
		t.Fatal(err)
	}
}
