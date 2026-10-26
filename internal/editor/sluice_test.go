package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

func TestDocValvesAndGates(t *testing.T) {
	d, err := docFromDraft(level.Draft{FormatVersion: 2, Rows: []string{"#@...#", "#.....", "#.....", "######"}})
	if err != nil {
		t.Fatal(err)
	}
	v, w := pos(2, 1), pos(4, 1)
	if !d.placeValve(v) || d.at(v) != 'V' || d.mechanismOf(v) != 0 || d.formatVersion != 3 {
		t.Fatalf("valve: %q, mechanisms %+v, version %d", d.at(v), d.mechanisms, d.formatVersion)
	}
	if d.placeValve(v) {
		t.Fatal("placing a valve on a valve with a mechanism changes nothing")
	}
	if !d.placeGate(pos(2, 2), v) || !d.placeGate(pos(3, 2), v) || d.placeGate(pos(3, 2), v) {
		t.Fatal("gates should link once each")
	}
	if !slices.Equal(d.mechanisms[0].Gates, []sim.Pos{pos(2, 2), pos(3, 2)}) {
		t.Fatalf("gates %v", d.mechanisms[0].Gates)
	}
	// A second valve takes a gate over from the first.
	d.placeValve(w)
	if !d.placeGate(pos(3, 2), w) || !slices.Equal(d.mechanisms[0].Gates, []sim.Pos{pos(2, 2)}) ||
		!slices.Equal(d.mechanisms[1].Gates, []sim.Pos{pos(3, 2)}) {
		t.Fatalf("relinking: %+v", d.mechanisms)
	}
	// No valve: a new gate is unlinked; an existing one is left alone.
	if !d.placeGate(pos(5, 2), pos(-1, -1)) || d.owner(pos(5, 2)) >= 0 {
		t.Fatal("a gate with no valve should be placed unlinked")
	}
	if d.placeGate(pos(2, 2), pos(-1, -1)) || d.owner(pos(2, 2)) != 0 {
		t.Fatal("repainting a linked gate with no valve selected should keep its link")
	}
	// Toggling flips the valve and every gate, from either end.
	if !d.toggle(pos(2, 2)) || d.at(v) != 'v' || d.at(pos(2, 2)) != 'g' || d.at(w) != 'V' {
		t.Fatalf("toggle: %q %q %q", d.at(v), d.at(pos(2, 2)), d.at(w))
	}
	if !d.placeGate(pos(1, 2), v) || d.at(pos(1, 2)) != 'g' {
		t.Fatal("a gate joining an open valve starts open")
	}
	if !d.toggle(v) || d.at(pos(1, 2)) != 'G' || d.toggle(pos(1, 3)) {
		t.Fatal("toggling back, and toggling a wall")
	}
	// Painting a gate over its own valve removes the mechanism.
	if !d.placeGate(v, v) || d.mechanismOf(v) >= 0 || d.owner(v) >= 0 {
		t.Fatalf("gate over its valve: %+v", d.mechanisms)
	}
}

func TestDocRegion(t *testing.T) {
	d, _ := docFromDraft(level.Draft{Rows: []string{"..#..", ".##..", "...#.", "####."}})
	got := d.region(pos(0, 0))
	if len(got) != 6 || slices.Contains(got, pos(3, 0)) {
		t.Fatalf("region of the left earth: %v", got)
	}
	if got := d.region(pos(4, 3)); len(got) != 6 {
		t.Fatalf("region of the right earth: %v", got)
	}
	if d.region(pos(9, 9)) != nil {
		t.Fatal("outside the cave")
	}
}

// A sluice built entirely in the editor works in the simulation: turning
// its valve opens its gates.
func TestEditorBuiltSluiceWorks(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 10}) // valve, next to the player start at (1,1)
	click(e, pos(2, 1))
	if v := e.View(); !v.LinkingOK || v.Linking != pos(2, 1) {
		t.Fatalf("painting a valve should select it: %+v", v.Linking)
	}
	if !hasProblem(e, "a valve must operate at least one gate") {
		t.Fatalf("a valve without gates is a problem: %v", e.Problems())
	}
	tap(e, Input{Brush: 11})
	if !strings.Contains(e.View().Hint, "joining valve (2,1)") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	drag(e, false, Input{}, pos(6, 5), pos(7, 5))
	if len(e.Problems()) != 0 || e.Level() == nil {
		t.Fatalf("problems: %v", e.Problems())
	}
	m := e.Level().Mechanisms
	if len(m) != 1 || m[0].Valve != pos(2, 1) || !slices.Equal(m[0].Gates, []sim.Pos{pos(6, 5), pos(7, 5)}) {
		t.Fatalf("mechanisms %+v", m)
	}
	s, err := sim.New(e.Level().Cave(), e.Level().Seed)
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, ev := range s.Step(sim.Input{Interact: true}) {
		if ev.Kind == sim.EventGateOpened {
			opened++
		}
	}
	if opened != 2 || s.Snapshot().Cell(pos(6, 5)).Terrain != sim.TerrainGateOpen {
		t.Fatalf("turning the valve opened %d gates", opened)
	}
}

func TestAltClickSelectsAValve(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "") // valve (12,6), gates (13,11) and (14,11)
	drag(e, false, Input{Alt: true}, pos(14, 11))       // a gate selects its valve
	if v := e.View(); !v.LinkingOK || v.Linking != pos(12, 6) || e.brush != gateBrush {
		t.Fatalf("Alt-click on a gate: linking %v %v, brush %d", v.Linking, v.LinkingOK, e.brush)
	}
	if !strings.Contains(e.View().Hint, "Valve (12,6) selected") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	click(e, pos(20, 13))
	if got := e.doc.mechanisms[0].Gates; !slices.Equal(got, []sim.Pos{pos(13, 11), pos(14, 11), pos(20, 13)}) || len(e.Problems()) != 0 {
		t.Fatalf("gates %v, problems %v", got, e.Problems())
	}
	// Removing the selected valve clears the selection.
	tap(e, Input{Brush: 1})
	click(e, pos(12, 6))
	if e.View().LinkingOK {
		t.Fatal("the selection should go with the valve")
	}
	tap(e, Input{Brush: 11})
	click(e, pos(21, 13))
	if !strings.Contains(e.View().Hint, "A gate needs a valve") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	// A gate that belongs to no valve says so on Alt-click.
	drag(e, false, Input{Alt: true}, pos(21, 13))
	if !strings.Contains(e.View().Hint, "belongs to no valve") {
		t.Fatalf("hint %q", e.View().Hint)
	}
}

func TestOTogglesAMechanism(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	tap(e, over(e, pos(13, 11), Input{Toggle: true}))
	if e.doc.at(pos(12, 6)) != 'v' || e.doc.at(pos(13, 11)) != 'g' || e.doc.at(pos(14, 11)) != 'g' || len(e.Problems()) != 0 {
		t.Fatalf("toggled: %q %q %q, problems %v", e.doc.at(pos(12, 6)), e.doc.at(pos(13, 11)), e.doc.at(pos(14, 11)), e.Problems())
	}
	if !strings.Contains(e.View().Hint, "now starts open") {
		t.Fatalf("hint %q", e.View().Hint)
	}
	tap(e, Input{Undo: true})
	if e.Dirty() {
		t.Fatal("undo should shut it again")
	}
	tap(e, over(e, pos(2, 2), Input{Toggle: true}))
	if !strings.Contains(e.View().Hint, "point at one") || e.Dirty() {
		t.Fatalf("O on floor: hint %q", e.View().Hint)
	}
}

func TestValveBrushRaisesTheFormatVersionAndBoxesPlaceOne(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_02").Draft(), "")
	if e.doc.formatVersion != 2 {
		t.Fatalf("cave_02 is version %d", e.doc.formatVersion)
	}
	tap(e, Input{Brush: 10})
	drag(e, false, Input{Shift: true}, pos(3, 3), pos(6, 5))
	if occurrences(e, 'V') != 1 || e.doc.at(pos(6, 5)) != 'V' || e.doc.formatVersion != 3 {
		t.Fatalf("box of valves: %d valves, version %d", occurrences(e, 'V'), e.doc.formatVersion)
	}
	if !strings.Contains(e.View().Hint, "Format version raised to 3") {
		t.Fatalf("hint %q", e.View().Hint)
	}
}

func TestCmdClickFloodFills(t *testing.T) {
	e, _ := open(t, NewCave(), "")
	tap(e, Input{Brush: 3}) // floor
	drag(e, false, Input{Cmd: true}, pos(10, 8))
	if n := occurrences(e, '.'); n != 0 {
		t.Fatalf("%d earth cells left after filling the earth", n)
	}
	if occurrences(e, ' ') != 28*14-2 || occurrences(e, '@') != 1 || occurrences(e, 'E') != 1 || len(e.undo) != 1 {
		t.Fatalf("fill: %d floor cells, %d undo steps", occurrences(e, ' '), len(e.undo))
	}
	// Cmd with the right button fills with floor; filling floor with floor
	// changes nothing.
	drag(e, true, Input{Cmd: true}, pos(10, 8))
	if len(e.undo) != 1 {
		t.Fatal("a fill that changes nothing is not an undo step")
	}
	tap(e, Input{Undo: true})
	if e.Dirty() {
		t.Fatal("one undo should take the whole fill back")
	}
	tap(e, Input{Brush: 6})
	drag(e, false, Input{Cmd: true}, pos(10, 8))
	if occurrences(e, '@') != 1 || e.doc.at(pos(10, 8)) != '@' {
		t.Fatal("filling with the player start just moves it")
	}
}

func TestReachable(t *testing.T) {
	grid := func(rows ...string) ([]sim.Cell, []bool, int) {
		var cells []sim.Cell
		var unknown []bool
		for _, row := range rows {
			for _, r := range row {
				c, ok := sim.CellForSymbol(r)
				cells = append(cells, c)
				unknown = append(unknown, !ok)
			}
		}
		return cells, unknown, len([]rune(rows[0]))
	}
	tests := []struct {
		name     string
		rows     []string
		reached  string // the reachable cells, as 'x', row by row
		crystals int
		exit     bool
	}{
		{"walls, earth and crystals", []string{"#@.*#", "#.#.#", "#*#E#"}, ".xxx.|.x.x.|.x.x.", 2, true},
		{"deep water blocks, shallow does not", []string{"@~.", "1#*", "*.."}, "x.x|x.x|xxx", 2, false},
		{"deep water walls a region off", []string{"@~*", "1#~", "*~*"}, "x..|x..|x..", 1, false},
		{"boulders, closed gates and valves block", []string{"@O*", "G##", "*V."}, "x..|...|...", 0, false},
		{"open gates pass", []string{"@g*"}, "xxx", 1, false},
		{"crawlers and glimmers are passed: they move on", []string{"@↑▶*"}, "xxxx", 1, false},
		{"the exit is not passed", []string{"@E*"}, "xx.", 0, true},
		{"unknown symbols block", []string{"@?*"}, "x..", 0, false},
	}
	for _, tc := range tests {
		cells, unknown, w := grid(tc.rows...)
		r := reachable(cells, unknown, w)
		var b strings.Builder
		for i, ok := range r.cells {
			if i > 0 && i%w == 0 {
				b.WriteByte('|')
			}
			if ok {
				b.WriteByte('x')
			} else {
				b.WriteByte('.')
			}
		}
		if b.String() != tc.reached || r.crystals != tc.crystals || r.exit != tc.exit {
			t.Errorf("%s: reached %s, %d crystals, exit %v; want %s, %d, %v", tc.name, b.String(), r.crystals, r.exit, tc.reached, tc.crystals, tc.exit)
		}
	}
	if r := reachable([]sim.Cell{{}}, nil, 1); r.cells != nil {
		t.Fatal("no player start: no search")
	}
}

func TestReachOverlay(t *testing.T) {
	e, _ := open(t, embedded(t, "cave_04").Draft(), "")
	if e.View().Reach != nil {
		t.Fatal("the overlay starts off")
	}
	tap(e, Input{Reach: true})
	v := e.View()
	if v.Reach == nil || !v.Reach[1*30+1] || v.Reach[13*30+3] {
		t.Fatal("the start is reachable; the basin below the flooded shaft is not")
	}
	if !strings.Contains(strings.Join(v.Info, "|"), "Reach (walking and digging, nothing moving): 3 of 9 crystals (quota 7), not the exit") {
		t.Fatalf("info %q", v.Info)
	}
	tap(e, Input{Reach: true})
	if e.View().Reach != nil {
		t.Fatal("R turns it off")
	}
}

// cmd/deepshift maps the keys 1-9, 0 and - to the brushes in this order.
func TestPaletteKeys(t *testing.T) {
	var keys []string
	for _, b := range Palette {
		keys = append(keys, b.Key)
	}
	if got := strings.Join(keys, ""); got != "1234567890-=" {
		t.Fatalf("palette keys %q, want 1234567890-= to match cmd/deepshift's brushKeys", got)
	}
	if Palette[gateBrush].Variants[0] != sim.SymbolGateClosed {
		t.Fatal("gateBrush is not the gate brush")
	}
}
