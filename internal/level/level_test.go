package level

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/sim"
)

func baseDoc() map[string]any {
	return map[string]any{
		"format_version":    1,
		"id":                "tiny",
		"title":             "Tiny",
		"width":             5,
		"height":            3,
		"seed":              7,
		"required_crystals": 1,
		"rows":              []string{"#####", "#@*E#", "#####"},
	}
}

func encode(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseValid(t *testing.T) {
	l, err := Parse("tiny.json", encode(t, baseDoc()))
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != "tiny" || l.Width != 5 || l.Height != 3 || l.Seed != 7 || l.RequiredCrystals != 1 || l.Crystals() != 1 {
		t.Fatalf("unexpected level %+v", l)
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.ASCII(), "#####\n#@*E#\n#####\n"; got != want {
		t.Fatalf("ASCII = %q, want %q", got, want)
	}
}

func TestInvalidLevelsFailWithUsefulErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		raw    string // used instead of the mutated base document when set
		want   []string
	}{
		{name: "empty file", raw: " ", want: []string{"file is empty"}},
		{name: "syntax error has line and column", raw: "{\n  \"format_version\": 1,\n  \"id\": ,\n}", want: []string{"t.json:3:9:", "invalid JSON"}},
		{name: "truncated", raw: `{"format_version": 1`, want: []string{"unexpected end of file"}},
		{name: "top-level array", raw: `[1, 2]`, want: []string{"top-level JSON value must be an object"}},
		{name: "top-level null", raw: `null`, want: []string{"must be an object"}},
		{name: "trailing data", raw: `{"format_version": 1} {}`, want: []string{"unexpected data after"}},
		{name: "unknown fields", mutate: func(d map[string]any) { d["entities"] = []any{}; d["Width"] = 3 },
			want: []string{`unknown field(s) "Width", "entities"`, "format_version, id, title"}},
		{name: "boulder in a version 1 file", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*EO", "#####"} },
			want: []string{"rows[1]: symbol(s) 'O' at x=4 need format_version 2 or later (this file is version 1)"}},
		{name: "water and a gate in a version 2 file", mutate: func(d map[string]any) {
			d["format_version"] = 2
			d["rows"] = []string{"#####", "#@*E~", "#O G#"}
		}, want: []string{"rows[1]: symbol(s) '~' at x=4 need format_version 3", "rows[2]: symbol(s) 'G' at x=3 need format_version 3 or later (this file is version 2)"}},
		{name: "mechanisms in a version 2 file", mutate: func(d map[string]any) { d["format_version"] = 2; d["mechanisms"] = []any{} },
			want: []string{"mechanisms: needs format_version 3 or later (this file is version 2)"}},
		{name: "wrong type", mutate: func(d map[string]any) { d["width"] = "wide" },
			want: []string{`field "width" must be an integer, not JSON string`}},
		{name: "negative seed", mutate: func(d map[string]any) { d["seed"] = -1 },
			want: []string{`field "seed" must be an integer`}},
		{name: "missing format_version", mutate: func(d map[string]any) { delete(d, "format_version") },
			want: []string{"format_version: missing"}},
		{name: "crawler in a version 3 file", mutate: func(d map[string]any) {
			d["format_version"] = 3
			d["rows"] = []string{"#####", "#@*E#", "#→ ←#"}
		}, want: []string{"rows[2]: symbol(s) '→' at x=1, '←' at x=3 need format_version 4 or later (this file is version 3)"}},
		{name: "future format_version", mutate: func(d map[string]any) { d["format_version"] = 5 },
			want: []string{"unsupported version 5", "reads format_version 1 to 4"}},
		{name: "format_version zero", mutate: func(d map[string]any) { d["format_version"] = 0 },
			want: []string{"unsupported version 0"}},
		{name: "missing id", mutate: func(d map[string]any) { delete(d, "id") }, want: []string{"id: missing"}},
		{name: "bad id", mutate: func(d map[string]any) { d["id"] = "Cave 1" }, want: []string{`id: "Cave 1" must be`}},
		{name: "blank title", mutate: func(d map[string]any) { d["title"] = "  " }, want: []string{"title: missing or blank"}},
		{name: "width out of range", mutate: func(d map[string]any) { d["width"] = 0 }, want: []string{"width: 0 is out of range 1..256"}},
		{name: "missing height", mutate: func(d map[string]any) { delete(d, "height") }, want: []string{"height: missing"}},
		{name: "row count mismatch", mutate: func(d map[string]any) { d["height"] = 4 }, want: []string{"rows: has 3 rows but height is 4"}},
		{name: "missing rows", mutate: func(d map[string]any) { delete(d, "rows") }, want: []string{"rows: missing"}},
		{name: "row too short", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*E", "#####"} },
			want: []string{"rows[1]: has 4 symbols but width is 5"}},
		{name: "unknown symbol with position", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*EQ", "#####"} },
			want: []string{"rows[1]: unknown tile symbol(s) 'Q' at x=4", "valid symbols are '#' wall"}},
		{name: "tab is not empty", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*E\t", "#####"} },
			want: []string{`'\t' at x=4`}},
		{name: "open-exit dump symbol not allowed", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*EX", "#####"} },
			want: []string{"'X' at x=4"}},
		{name: "no player", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "# *E#", "#####"} },
			want: []string{"rows: no player start ('@') found; exactly one is required"}},
		{name: "two players", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*E#", "#@  #"} },
			want: []string{"found 2 player starts ('@') at (1,1), (1,2)"}},
		{name: "no exit", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@* #", "#####"} },
			want: []string{"no exit ('E') found"}},
		{name: "two exits", mutate: func(d map[string]any) { d["rows"] = []string{"#####", "#@*E#", "#E  #"} },
			want: []string{"found 2 exits ('E') at (3,1), (1,2)"}},
		{name: "quota above crystals", mutate: func(d map[string]any) { d["required_crystals"] = 2 },
			want: []string{"required_crystals: 2 exceeds the 1 crystal(s)", "could never open"}},
		{name: "negative quota", mutate: func(d map[string]any) { d["required_crystals"] = -1 },
			want: []string{"required_crystals: -1 is negative"}},
		{name: "missing quota", mutate: func(d map[string]any) { delete(d, "required_crystals") },
			want: []string{"required_crystals: missing"}},
		{name: "seed beyond 2^53-1", mutate: func(d map[string]any) { d["seed"] = uint64(1) << 53 },
			want: []string{"seed: 9007199254740992 exceeds 9007199254740991"}},
		{name: "blank metadata key", mutate: func(d map[string]any) { d["metadata"] = map[string]string{"": "x"} },
			want: []string{"metadata: keys must not be blank"}},
		{name: "all problems reported together", mutate: func(d map[string]any) {
			d["id"] = "BAD"
			d["required_crystals"] = 5
			d["rows"] = []string{"#####", "#@* #", "#####"}
		}, want: []string{"(3 problems)", "id:", "no exit", "required_crystals: 5 exceeds"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.raw)
			if tt.raw == "" {
				doc := baseDoc()
				tt.mutate(doc)
				data = encode(t, doc)
			}
			l, err := Parse("t.json", data)
			if err == nil {
				t.Fatalf("Parse accepted an invalid cave: %+v", l)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error does not contain %q:\n%v", w, err)
				}
			}
		})
	}
}

func TestVersion2ReadsBoulders(t *testing.T) {
	doc := baseDoc()
	doc["format_version"] = 2
	doc["rows"] = []string{"#####", "#@*E#", "#O  #", "#####"}
	doc["height"] = 4
	l, err := Parse("v2.json", encode(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if s.Boulders() != 1 || s.Cell(sim.Pos{X: 1, Y: 2}).Falling {
		t.Fatalf("boulder not loaded at rest:\n%s", s.ASCII())
	}
}

// v3Doc is a version 3 cave: a valve beside the player, a closed gate
// holding back deep and shallow water.
func v3Doc() map[string]any {
	return map[string]any{
		"format_version":    3,
		"id":                "sluice",
		"title":             "Sluice",
		"width":             6,
		"height":            3,
		"seed":              7,
		"required_crystals": 1,
		"rows":              []string{"######", "#@V*E#", "#~3G #"},
		"mechanisms":        []any{map[string]any{"valve": map[string]int{"x": 2, "y": 1}, "gates": []any{map[string]int{"x": 3, "y": 2}}}},
	}
}

func TestVersion3ReadsWaterGatesAndValves(t *testing.T) {
	l, err := Parse("v3.json", encode(t, v3Doc()))
	if err != nil {
		t.Fatal(err)
	}
	want := []sim.Mechanism{{Valve: sim.Pos{X: 2, Y: 1}, Gates: []sim.Pos{{X: 3, Y: 2}}}}
	if len(l.Mechanisms) != 1 || l.Mechanisms[0].Valve != want[0].Valve || len(l.Mechanisms[0].Gates) != 1 || l.Mechanisms[0].Gates[0] != want[0].Gates[0] {
		t.Fatalf("mechanisms = %+v, want %+v", l.Mechanisms, want)
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if s.Water() != sim.WaterCapacity+3 || s.Cell(sim.Pos{X: 3, Y: 2}).Terrain != sim.TerrainGateClosed {
		t.Fatalf("water %d, gate %s:\n%s", s.Water(), s.Cell(sim.Pos{X: 3, Y: 2}).Terrain, s.ASCII())
	}
	if v, ok := s.AdjacentValve(); !ok || v != (sim.Pos{X: 2, Y: 1}) {
		t.Fatalf("AdjacentValve = %s, %v", v, ok)
	}
}

func TestVersion4ReadsCrawlers(t *testing.T) {
	doc := baseDoc()
	doc["format_version"] = 4
	doc["rows"] = []string{"#####", "#@*E#", "#↑→↓#", "#←  #", "#####"}
	doc["height"] = 5
	l, err := Parse("v4.json", encode(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		p sim.Pos
		d sim.Dir
	}{{sim.Pos{X: 1, Y: 2}, sim.DirUp}, {sim.Pos{X: 2, Y: 2}, sim.DirRight}, {sim.Pos{X: 3, Y: 2}, sim.DirDown}, {sim.Pos{X: 1, Y: 3}, sim.DirLeft}} {
		if got := s.Cell(c.p); got.Occupant != sim.OccupantCrawler || got.Facing != c.d {
			t.Fatalf("cell %s = %+v, want a crawler facing %s", c.p, got, c.d)
		}
	}
	if s.Crawlers() != 4 || s.ASCII() != "#####\n#@*E#\n#↑→↓#\n#←  #\n#####\n" {
		t.Fatalf("%d crawlers:\n%s", s.Crawlers(), s.ASCII())
	}
}

func TestInvalidMechanismsFailWithUsefulErrors(t *testing.T) {
	mech := func(m map[string]any) func(map[string]any) {
		return func(d map[string]any) { d["mechanisms"] = []any{m} }
	}
	at := func(x, y int) map[string]int { return map[string]int{"x": x, "y": y} }
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   []string
	}{
		{"not a list", func(d map[string]any) { d["mechanisms"] = map[string]any{} }, []string{"mechanisms: must be a list"}},
		{"unknown key", mech(map[string]any{"valve": at(2, 1), "gate": []any{at(3, 2)}}), []string{`unknown field "gate"`}},
		{"coordinate not an integer", mech(map[string]any{"valve": map[string]any{"x": "2", "y": 1}, "gates": []any{at(3, 2)}}), []string{"mechanisms: must be a list", "string"}},
		{"valve missing", mech(map[string]any{"gates": []any{at(3, 2)}}), []string{"mechanisms[0].valve: missing"}},
		{"y missing", mech(map[string]any{"valve": map[string]int{"x": 2}, "gates": []any{at(3, 2)}}), []string{`mechanisms[0].valve: needs both "x" and "y"`}},
		{"no gates", mech(map[string]any{"valve": at(2, 1)}), []string{"mechanisms[0].gates: missing or empty", "not listed: 'G' at (3,2)"}},
		{"outside the cave", mech(map[string]any{"valve": at(2, 1), "gates": []any{at(3, 2), at(9, 0)}}), []string{"mechanisms[0].gates[1]: (9,0) is outside the 6x3 cave"}},
		{"not a valve", mech(map[string]any{"valve": at(1, 1), "gates": []any{at(3, 2)}}), []string{`mechanisms[0].valve: (1,1) is '@' in rows, not a valve ('V' or 'v')`, "not listed: 'V' at (2,1)"}},
		{"not a gate", mech(map[string]any{"valve": at(2, 1), "gates": []any{at(4, 2)}}), []string{`mechanisms[0].gates[0]: (4,2) is ' ' in rows, not a gate`}},
		{"listed twice", mech(map[string]any{"valve": at(2, 1), "gates": []any{at(3, 2), at(3, 2)}}), []string{"mechanisms[0].gates[1]: gate (3,2) is already listed at mechanisms[0].gates[0]"}},
		{"gate open, valve shut", func(d map[string]any) { d["rows"] = []string{"######", "#@V*E#", "#~3g #"} },
			[]string{`mechanisms[0].gates[0]: gate (3,2) is 'g' but valve (2,1) is 'V'`}},
		{"unlisted valve", func(d map[string]any) { delete(d, "mechanisms") }, []string{"every valve and gate must be listed", "'V' at (2,1), 'G' at (3,2)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := v3Doc()
			tt.mutate(doc)
			l, err := Parse("t.json", encode(t, doc))
			if err == nil {
				t.Fatalf("Parse accepted an invalid cave: %+v", l)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error does not contain %q:\n%v", w, err)
				}
			}
		})
	}
}

func TestValidationErrorIsInspectable(t *testing.T) {
	doc := baseDoc()
	doc["width"] = 999
	_, err := Parse("t.json", encode(t, doc))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %T is not a *ValidationError", err)
	}
	if ve.Source != "t.json" || len(ve.Problems) == 0 || ve.Problems[0].Field != "width" {
		t.Fatalf("unexpected problems %+v", ve.Problems)
	}
}

func TestChecksumCoversGameplayNotCosmetics(t *testing.T) {
	sum := func(mutate func(map[string]any)) string {
		doc := baseDoc()
		if mutate != nil {
			mutate(doc)
		}
		l, err := Parse("t.json", encode(t, doc))
		if err != nil {
			t.Fatal(err)
		}
		return l.Checksum()
	}
	base := sum(nil)
	if base != sum(nil) {
		t.Fatal("checksum is not stable")
	}
	same := map[string]func(map[string]any){
		"title":    func(d map[string]any) { d["title"] = "Renamed" },
		"metadata": func(d map[string]any) { d["metadata"] = map[string]string{"author": "someone"} },
		"id":       func(d map[string]any) { d["id"] = "renamed" },
	}
	for name, m := range same {
		if sum(m) != base {
			t.Errorf("changing %s changed the checksum", name)
		}
	}
	different := map[string]func(map[string]any){
		"seed":  func(d map[string]any) { d["seed"] = 8 },
		"quota": func(d map[string]any) { d["required_crystals"] = 0 },
		"rows":  func(d map[string]any) { d["rows"] = []string{"#####", "#@*E.", "#####"} },
	}
	for name, m := range different {
		if sum(m) == base {
			t.Errorf("changing %s did not change the checksum", name)
		}
	}
}

// Mechanism links are gameplay: rewiring a valve must change the checksum.
func TestChecksumCoversMechanisms(t *testing.T) {
	sum := func(doc map[string]any) string {
		l, err := Parse("t.json", encode(t, doc))
		if err != nil {
			t.Fatal(err)
		}
		return l.Checksum()
	}
	a := v3Doc()
	a["rows"] = []string{"######", "#@V*E#", "#~GG #"}
	a["mechanisms"] = []any{map[string]any{"valve": map[string]int{"x": 2, "y": 1}, "gates": []any{map[string]int{"x": 2, "y": 2}, map[string]int{"x": 3, "y": 2}}}}
	b := v3Doc()
	b["rows"] = a["rows"]
	b["mechanisms"] = []any{map[string]any{"valve": map[string]int{"x": 2, "y": 1}, "gates": []any{map[string]int{"x": 3, "y": 2}, map[string]int{"x": 2, "y": 2}}}}
	if sum(a) == sum(b) {
		t.Fatal("reordering a mechanism's gates did not change the checksum")
	}
}

func TestEmbeddedLevelsValidate(t *testing.T) {
	levels, err := LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) == 0 {
		t.Fatal("no embedded levels found")
	}
	for _, l := range levels {
		if _, err := sim.New(l.Cave(), l.Seed); err != nil {
			t.Errorf("%s: %v", l.ID, err)
		}
	}
}

func TestTestCave(t *testing.T) {
	l, err := Load(assets.Levels, assets.TestCave)
	if err != nil {
		t.Fatal(err)
	}
	if l.ID != "test_cave" || l.Width != 40 || l.Height != 24 || l.RequiredCrystals != 8 || l.Crystals() != 11 {
		t.Fatalf("test cave changed shape: id=%s %dx%d quota %d of %d", l.ID, l.Width, l.Height, l.RequiredCrystals, l.Crystals())
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if s.Player() != (sim.Pos{X: 1, Y: 1}) || s.ExitOpen() {
		t.Fatalf("player at %s, exit open %v", s.Player(), s.ExitOpen())
	}
}

func TestLoadDirRejectsDuplicateIDs(t *testing.T) {
	doc := baseDoc()
	fsys := fstest.MapFS{
		"levels/a.json": {Data: encode(t, doc)},
		"levels/b.json": {Data: encode(t, doc)},
		"levels/c.json": {Data: []byte("{")},
	}
	levels, err := LoadDir(fsys, "levels")
	if err == nil {
		t.Fatal("LoadDir accepted duplicate ids")
	}
	for _, w := range []string{`levels/b.json: id "tiny" is already used by levels/a.json`, "levels/c.json"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error does not contain %q:\n%v", w, err)
		}
	}
	if len(levels) != 1 {
		t.Fatalf("got %d valid levels, want 1", len(levels))
	}
}
