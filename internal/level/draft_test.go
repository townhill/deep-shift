package level

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/sim"
)

// Every embedded cave is stored in the canonical layout, so an editor that
// opens and saves one unchanged rewrites the same bytes, and a cave's
// checksum (and its golden replays) survive a round trip.
func TestEmbeddedCavesAreCanonical(t *testing.T) {
	names, err := fs.Glob(assets.Levels, assets.LevelDir+"/*.json")
	if err != nil || len(names) == 0 {
		t.Fatalf("no embedded caves: %v", err)
	}
	for _, name := range names {
		data, err := fs.ReadFile(assets.Levels, name)
		if err != nil {
			t.Fatal(err)
		}
		l, err := Parse(name, data)
		if err != nil {
			t.Fatal(err)
		}
		if got := l.Draft().Encode(); !bytes.Equal(got, data) {
			t.Errorf("%s is not in the canonical layout (run cavetool fmt); Encode gives:\n%s", name, got)
		}
		d, err := DecodeDraft(name, data)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, l.Draft()) {
			t.Errorf("%s: DecodeDraft = %+v, want Level.Draft = %+v", name, d, l.Draft())
		}
	}
}

func TestEncodeLayout(t *testing.T) {
	d := Draft{
		FormatVersion: 4, ID: "tiny", Title: `Tiny <&> "quoted"`, Width: 5, Height: 3, Seed: 7, RequiredCrystals: 1,
		Rows:       []string{"#####", "#@*E#", "#V←G#"},
		Mechanisms: []sim.Mechanism{{Valve: sim.Pos{X: 1, Y: 2}, Gates: []sim.Pos{{X: 3, Y: 2}, {X: 3, Y: 1}}}},
		Metadata:   map[string]string{"purpose": "a test", "author": "me"},
	}
	want := `{
  "format_version": 4,
  "id": "tiny",
  "title": "Tiny <&> \"quoted\"",
  "width": 5,
  "height": 3,
  "seed": 7,
  "required_crystals": 1,
  "rows": [
    "#####",
    "#@*E#",
    "#V←G#"
  ],
  "mechanisms": [
    {"valve": {"x": 1, "y": 2}, "gates": [{"x": 3, "y": 2}, {"x": 3, "y": 1}]}
  ],
  "metadata": {
    "author": "me",
    "purpose": "a test"
  }
}
`
	if got := string(d.Encode()); got != want {
		t.Fatalf("Encode:\n%s\nwant:\n%s", got, want)
	}
	d.Mechanisms, d.Metadata, d.Rows = nil, map[string]string{}, nil
	got := string(d.Encode())
	if strings.Contains(got, "mechanisms") || strings.Contains(got, "metadata") || !strings.Contains(got, `"rows": []`+"\n}") {
		t.Fatalf("empty optional fields should be left out and empty rows closed inline:\n%s", got)
	}
}

// randomDraft builds a valid cave of random size and content: a walled box
// with one player, one exit, random interior symbols and, in version 3 and
// later, a valve with gates in the wall.
func randomDraft(rng *rand.Rand) Draft {
	w, h := 4+rng.IntN(20), 4+rng.IntN(12)
	version := 1 + rng.IntN(FormatVersion)
	var interior []rune
	for _, r := range []rune(" .#*O~123↑↓←→") {
		if SymbolVersion(r) <= version {
			interior = append(interior, r)
		}
	}
	grid := make([][]rune, h)
	for y := range grid {
		grid[y] = make([]rune, w)
		for x := range grid[y] {
			switch {
			case x == 0 || y == 0 || x == w-1 || y == h-1:
				grid[y][x] = '#'
			default:
				grid[y][x] = interior[rng.IntN(len(interior))]
			}
		}
	}
	grid[1][1], grid[h-2][w-2] = '@', 'E'
	var ms []sim.Mechanism
	if version >= mechanismFormatVersion && rng.IntN(2) == 0 {
		open := rng.IntN(2) == 0
		valve, gate := 'V', 'G'
		if open {
			valve, gate = 'v', 'g'
		}
		m := sim.Mechanism{Valve: sim.Pos{X: 1, Y: 0}}
		grid[0][1] = valve
		for x := 2; x < w-1; x += 2 {
			grid[0][x] = gate
			m.Gates = append(m.Gates, sim.Pos{X: x, Y: 0})
		}
		ms = append(ms, m)
	}
	crystals := 0
	rows := make([]string, h)
	for y, r := range grid {
		rows[y] = string(r)
		for _, c := range r {
			if c == '*' {
				crystals++
			}
		}
	}
	d := Draft{
		FormatVersion: version, ID: fmt.Sprintf("cave_%d", rng.IntN(1000)), Title: "Random <cave> & co",
		Width: w, Height: h, Seed: rng.Uint64N(MaxSeed + 1), RequiredCrystals: rng.IntN(crystals + 1),
		Rows: rows, Mechanisms: ms,
	}
	if rng.IntN(2) == 0 {
		d.Metadata = map[string]string{"author": "rng", "note": "line one\nline \"two\""}
	}
	return d
}

func TestEncodeRoundTripsRandomCaves(t *testing.T) {
	rng := rand.New(rand.NewPCG(2026, 1025))
	for i := range 500 {
		d := randomDraft(rng)
		data := d.Encode()
		l, err := Parse("random.json", data)
		if err != nil {
			t.Fatalf("case %d: %v\n%s", i, err, data)
		}
		if got := l.Draft(); !reflect.DeepEqual(got, d) {
			t.Fatalf("case %d: Parse(Encode(d)).Draft() = %+v, want %+v", i, got, d)
		}
		dd, err := DecodeDraft("random.json", data)
		if err != nil || !reflect.DeepEqual(dd, d) {
			t.Fatalf("case %d: DecodeDraft = %+v, %v; want %+v", i, dd, err, d)
		}
	}
}

func TestDecodeDraftOpensInvalidCaves(t *testing.T) {
	d := Draft{
		FormatVersion: 2, ID: "Bad ID", Title: "", Width: 4, Height: 2, RequiredCrystals: 5,
		Rows:       []string{"@@?E", "#~G#"},
		Mechanisms: []sim.Mechanism{{Valve: sim.Pos{X: 9, Y: 9}}},
	}
	data := d.Encode()
	got, err := DecodeDraft("wip.json", data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, d) {
		t.Fatalf("DecodeDraft = %+v, want %+v", got, d)
	}
	if _, err := Parse("wip.json", data); err == nil {
		t.Fatal("Parse accepted an invalid cave")
	}

	for _, tc := range []struct{ name, data, want string }{
		{"syntax", `{"id": }`, "invalid JSON"},
		{"unknown field", `{"rowz": []}`, `unknown field(s) "rowz"`},
		{"wrong type", `{"width": "wide"}`, `field "width" must be an integer`},
		{"mechanism shape", `{"mechanisms": [{"valve": {"x": 1}}]}`, `mechanisms[0].valve needs both "x" and "y"`},
		{"mechanism field", `{"mechanisms": [{"valve": {"x": 1, "y": 1}, "doors": []}]}`, `unknown field "doors"`},
	} {
		if _, err := DecodeDraft("t.json", []byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}

// Problems about particular cells carry their positions, so an editor can
// outline them.
func TestProblemsLocateCells(t *testing.T) {
	p := func(x, y int) sim.Pos { return sim.Pos{X: x, Y: y} }
	tests := []struct {
		name  string
		draft Draft
		field string
		at    []sim.Pos
	}{
		{"unknown symbol", Draft{Rows: []string{"#@E?#", "#####"}}, "rows[0]", []sim.Pos{p(3, 0)}},
		{"symbol too new", Draft{FormatVersion: 2, Rows: []string{"#@E~#", "##~##"}}, "rows[1]", []sim.Pos{p(2, 1)}},
		{"two players", Draft{Rows: []string{"#@E@#", "#####"}}, "rows", []sim.Pos{p(1, 0), p(3, 0)}},
		{"orphan gate", Draft{Rows: []string{"#@EG#", "#####"}}, "mechanisms", []sim.Pos{p(3, 0)}},
		{"gate state", Draft{Rows: []string{"#@EV#", "#g###"}, Mechanisms: []sim.Mechanism{{Valve: p(3, 0), Gates: []sim.Pos{p(1, 1)}}}},
			"mechanisms[0].gates[0]", []sim.Pos{p(1, 1), p(3, 0)}},
		{"not a gate", Draft{Rows: []string{"#@EV#", "#####"}, Mechanisms: []sim.Mechanism{{Valve: p(3, 0), Gates: []sim.Pos{p(0, 1)}}}},
			"mechanisms[0].gates[0]", []sim.Pos{p(0, 1)}},
		{"no gates", Draft{Rows: []string{"#@EV#", "#####"}, Mechanisms: []sim.Mechanism{{Valve: p(3, 0)}}},
			"mechanisms[0].gates", []sim.Pos{p(3, 0)}},
	}
	for _, tc := range tests {
		d := tc.draft
		if d.FormatVersion == 0 {
			d.FormatVersion = FormatVersion
		}
		d.ID, d.Title, d.Width, d.Height = "t", "T", 5, 2
		_, err := Parse("t.json", d.Encode())
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s: err = %v, want a ValidationError", tc.name, err)
		}
		i := slices.IndexFunc(ve.Problems, func(pr Problem) bool { return pr.Field == tc.field })
		if i < 0 || !reflect.DeepEqual(ve.Problems[i].At, tc.at) {
			t.Errorf("%s: problems %+v, want field %s at %v", tc.name, ve.Problems, tc.field, tc.at)
		}
	}
}
