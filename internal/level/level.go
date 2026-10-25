// Package level reads, validates and fingerprints authored cave files.
//
// Cave files are JSON with these fields (all required except mechanisms and
// metadata):
//
//	{
//	  "format_version": 4,          // 1 to 4; see below
//	  "id": "test_cave",            // ^[a-z0-9][a-z0-9_]*$, at most 64 bytes
//	  "title": "Proving Ground",    // shown in the HUD, at most 80 characters
//	  "width": 40, "height": 24,    // 1..256 each
//	  "seed": 20261017,             // 0..2^53-1, so float64 JSON tools keep it exact
//	  "required_crystals": 8,       // 0..number of '*' tiles
//	  "rows": ["####", "#@E#", ...],// height strings of width symbols each
//	  "mechanisms": [               // version 3; links each valve to its gates
//	    {"valve": {"x": 4, "y": 2}, "gates": [{"x": 9, "y": 5}]}
//	  ],
//	  "metadata": {"author": "..."} // optional free-form strings; ignored by rules
//	}
//
// Row symbols are the shared legend in internal/sim (sim.SymbolLegend).
// Version 2 adds the boulder 'O'. Version 3 adds water ('~' deep, '1'-'3'
// shallow), gates ('G' closed, 'g' open) and valves ('V' shut, 'v' open),
// with mechanisms: every valve and gate is listed in exactly one, and a
// mechanism's gates are open exactly when its valve is. Version 4 adds
// crawlers, written as the arrow of the way each starts facing ('↑' '↓' '←'
// '→'). Older files are still read unchanged and may not contain newer
// symbols.
// Each cave must contain exactly one player start '@' and exactly one exit
// 'E'. Unknown fields are rejected so typos surface instead of being ignored.
package level

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/townhill/deep-shift/internal/sim"
)

// FormatVersion is the newest cave file version this build reads; new caves
// should use it.
const FormatVersion = 4

// MinFormatVersion is the oldest cave file version this build still reads.
const MinFormatVersion = 1

// mechanismFormatVersion is the first version with water, gates, valves and
// the mechanisms field.
const mechanismFormatVersion = 3

// crawlerFormatVersion is the first version with crawlers.
const crawlerFormatVersion = 4

// SymbolVersion returns the first format version whose rows may contain r.
func SymbolVersion(r rune) int {
	if sim.IsCrawlerSymbol(r) {
		return crawlerFormatVersion
	}
	switch r {
	case sim.SymbolBoulder:
		return 2
	case sim.SymbolDeepWater, '1', '2', '3', sim.SymbolGateClosed, sim.SymbolGateOpen, sim.SymbolValveShut, sim.SymbolValveOpen:
		return mechanismFormatVersion
	}
	return 1
}

// MaxSeed is the largest seed accepted: 2^53-1, the largest integer every
// JSON implementation (including float64-based ones) round-trips exactly.
const MaxSeed = 1<<53 - 1

// Limits on a cave's id (in bytes) and title (in characters).
const (
	MaxIDLen    = 64
	MaxTitleLen = 80
)

const maxListedPlaces = 5 // positions listed per problem before eliding

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)

// ValidID reports whether id is a valid cave id: 1 to MaxIDLen lowercase
// letters, digits and '_', starting with a letter or digit. Valid ids are
// also safe file names.
func ValidID(id string) bool { return len(id) <= MaxIDLen && idPattern.MatchString(id) }

// Level is a validated cave file.
type Level struct {
	FormatVersion    int
	ID               string
	Title            string
	Width, Height    int
	Seed             uint64
	RequiredCrystals int
	Rows             []string
	Mechanisms       []sim.Mechanism
	Metadata         map[string]string

	cave sim.Cave
}

// Cave returns a copy of the level's initial layout for sim.New.
func (l *Level) Cave() sim.Cave { return l.cave.Clone() }

// Crystals returns the number of crystals in the layout.
func (l *Level) Crystals() int { return l.cave.CountOccupant(sim.OccupantCrystal) }

// Crawlers returns the number of crawlers in the layout.
func (l *Level) Crawlers() int { return l.cave.CountOccupant(sim.OccupantCrawler) }

// Checksum fingerprints the gameplay-relevant content: format version,
// dimensions, seed, quota, rows and mechanisms. Title and metadata are
// excluded, so cosmetic edits do not invalidate replays; any change that
// could alter a simulation does. A cave without mechanisms hashes exactly as
// it did before mechanisms existed.
func (l *Level) Checksum() string {
	h := sha256.New()
	h.Write([]byte("deep-shift/level-checksum/v1\x00"))
	var b []byte
	b = binary.LittleEndian.AppendUint32(b, uint32(l.FormatVersion))
	b = binary.LittleEndian.AppendUint32(b, uint32(l.Width))
	b = binary.LittleEndian.AppendUint32(b, uint32(l.Height))
	b = binary.LittleEndian.AppendUint64(b, l.Seed)
	b = binary.LittleEndian.AppendUint32(b, uint32(l.RequiredCrystals))
	h.Write(b)
	for _, r := range l.Rows {
		h.Write([]byte(r))
		h.Write([]byte{'\n'})
	}
	if len(l.Mechanisms) > 0 {
		b = binary.LittleEndian.AppendUint32(b[:0], uint32(len(l.Mechanisms)))
		for _, m := range l.Mechanisms {
			b = binary.LittleEndian.AppendUint32(b, uint32(m.Valve.X))
			b = binary.LittleEndian.AppendUint32(b, uint32(m.Valve.Y))
			b = binary.LittleEndian.AppendUint32(b, uint32(len(m.Gates)))
			for _, g := range m.Gates {
				b = binary.LittleEndian.AppendUint32(b, uint32(g.X))
				b = binary.LittleEndian.AppendUint32(b, uint32(g.Y))
			}
		}
		h.Write([]byte("mechanisms\x00"))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Problem is one validation failure, located by JSON field path and, where
// it concerns particular cells, by their positions.
type Problem struct {
	Field string // e.g. "rows[3]" or "required_crystals"
	Msg   string
	At    []sim.Pos // the cells at fault, if any, for editors to point at
}

// ValidationError lists every problem found in a cave file.
type ValidationError struct {
	Source   string
	Problems []Problem
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: invalid cave file (%d problem", e.Source, len(e.Problems))
	if len(e.Problems) != 1 {
		b.WriteByte('s')
	}
	b.WriteByte(')')
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  - %s: %s", p.Field, p.Msg)
	}
	return b.String()
}

// file is the on-disk shape. Pointers distinguish "missing" from zero.
type file struct {
	FormatVersion    *int              `json:"format_version"`
	ID               *string           `json:"id"`
	Title            *string           `json:"title"`
	Width            *int              `json:"width"`
	Height           *int              `json:"height"`
	Seed             *uint64           `json:"seed"`
	RequiredCrystals *int              `json:"required_crystals"`
	Rows             []string          `json:"rows"`
	Mechanisms       json.RawMessage   `json:"mechanisms"`
	Metadata         map[string]string `json:"metadata"`
}

// posFile and mechanismFile are the on-disk shape of mechanisms.
type posFile struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

type mechanismFile struct {
	Valve *posFile  `json:"valve"`
	Gates []posFile `json:"gates"`
}

// fields lists the keys of the newest format version, in documentation
// order.
var fields = []string{"format_version", "id", "title", "width", "height", "seed", "required_crystals", "rows", "mechanisms", "metadata"}

// Parse decodes and validates a cave file. source names the file in error
// messages. On validation failure the error is a *ValidationError.
func Parse(source string, data []byte) (*Level, error) {
	f, err := decode(source, data)
	if err != nil {
		return nil, err
	}
	return validate(source, f)
}

// decode checks a cave file's JSON shape: a single object of known fields
// with the right types. It does not validate the content.
func decode(source string, data []byte) (*file, error) {
	// First pass: syntax, a single top-level object, and no unknown keys
	// (checked by name rather than by matching encoding/json error text).
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return nil, decodeError(source, data, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected data after the top-level JSON object", source)
	}
	if raw == nil {
		return nil, fmt.Errorf("%s: top-level JSON value must be an object", source)
	}
	var unknown []string
	for k := range raw {
		if !slices.Contains(fields, k) {
			unknown = append(unknown, fmt.Sprintf("%q", k))
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return nil, fmt.Errorf("%s: unknown field(s) %s; cave file fields are %s",
			source, strings.Join(unknown, ", "), strings.Join(fields, ", "))
	}

	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, decodeError(source, data, err)
	}
	return &f, nil
}

// Load reads and parses one cave file from fsys.
func Load(fsys fs.FS, name string) (*Level, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return Parse(name, data)
}

// LoadDir loads every *.json cave in dir, sorted by file name, and rejects
// duplicate ids.
func LoadDir(fsys fs.FS, dir string) ([]*Level, error) {
	names, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	slices.Sort(names)
	var levels []*Level
	var errs []error
	seen := map[string]string{}
	for _, name := range names {
		l, err := Load(fsys, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if prev, dup := seen[l.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: id %q is already used by %s", name, l.ID, prev))
			continue
		}
		seen[l.ID] = name
		levels = append(levels, l)
	}
	return levels, errors.Join(errs...)
}

// ByID returns the level with the given id from levels.
func ByID(levels []*Level, id string) (*Level, bool) {
	for _, l := range levels {
		if l.ID == id {
			return l, true
		}
	}
	return nil, false
}

func decodeError(source string, data []byte, err error) error {
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		line, col := lineCol(data, syn.Offset)
		return fmt.Errorf("%s:%d:%d: invalid JSON: %v", source, line, col, syn)
	case errors.As(err, &typ) && typ.Field == "":
		return fmt.Errorf("%s: top-level JSON value must be an object, not %s", source, typ.Value)
	case errors.As(err, &typ):
		line, col := lineCol(data, typ.Offset)
		return fmt.Errorf("%s:%d:%d: field %q must be %s, not JSON %s", source, line, col, typ.Field, describeType(typ.Type.String()), typ.Value)
	case errors.Is(err, io.EOF):
		return fmt.Errorf("%s: file is empty", source)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("%s: invalid JSON: unexpected end of file", source)
	}
	return fmt.Errorf("%s: %w", source, err)
}

func describeType(goType string) string {
	switch goType {
	case "int", "uint64":
		return "an integer"
	case "string":
		return "a string"
	case "[]string":
		return "an array of strings"
	case "map[string]string":
		return "an object of string values"
	}
	return goType
}

// lineCol converts an encoding/json error offset into the 1-based line and
// column of the offending byte. encoding/json reports the offset just after
// that byte, hence the -1.
func lineCol(data []byte, offset int64) (line, col int) {
	offset = min(max(offset-1, 0), int64(len(data)))
	before := data[:offset]
	line = bytes.Count(before, []byte{'\n'}) + 1
	col = int(offset) - bytes.LastIndexByte(before, '\n')
	return line, col
}

func validate(source string, f *file) (*Level, error) {
	v := &ValidationError{Source: source}
	r := &reporter{v}

	// The version decides how everything else is read, so stop early.
	switch {
	case f.FormatVersion == nil:
		r.add("format_version", "missing; this build reads format_version %d to %d", MinFormatVersion, FormatVersion)
		return nil, v
	case *f.FormatVersion < MinFormatVersion || *f.FormatVersion > FormatVersion:
		r.add("format_version", "unsupported version %d; this build reads format_version %d to %d", *f.FormatVersion, MinFormatVersion, FormatVersion)
		return nil, v
	}
	l := &Level{FormatVersion: *f.FormatVersion, Rows: f.Rows, Metadata: f.Metadata}

	switch {
	case f.ID == nil:
		r.add("id", "missing")
	case !ValidID(*f.ID):
		r.add("id", "%q must be 1-%d characters of lowercase letters, digits and '_', starting with a letter or digit", *f.ID, MaxIDLen)
	default:
		l.ID = *f.ID
	}
	switch {
	case f.Title == nil || strings.TrimSpace(*f.Title) == "":
		r.add("title", "missing or blank")
	case utf8.RuneCountInString(*f.Title) > MaxTitleLen:
		r.add("title", "longer than %d characters", MaxTitleLen)
	default:
		l.Title = *f.Title
	}
	dimension := func(field string, p *int) int {
		switch {
		case p == nil:
			r.add(field, "missing")
		case *p < 1 || *p > sim.MaxDimension:
			r.add(field, "%d is out of range 1..%d", *p, sim.MaxDimension)
		default:
			return *p
		}
		return 0
	}
	l.Width = dimension("width", f.Width)
	l.Height = dimension("height", f.Height)
	switch {
	case f.Seed == nil:
		r.add("seed", "missing")
	case *f.Seed > MaxSeed:
		r.add("seed", "%d exceeds %d (2^53-1), the largest integer all JSON tools keep exact", *f.Seed, uint64(MaxSeed))
	default:
		l.Seed = *f.Seed
	}
	for k := range f.Metadata {
		if strings.TrimSpace(k) == "" {
			r.add("metadata", "keys must not be blank")
			break
		}
	}

	crystals := validateRows(f.Rows, l, r)
	validateMechanisms(f.Mechanisms, l, r)

	switch {
	case f.RequiredCrystals == nil:
		r.add("required_crystals", "missing")
	case *f.RequiredCrystals < 0:
		r.add("required_crystals", "%d is negative", *f.RequiredCrystals)
	case crystals >= 0 && *f.RequiredCrystals > crystals:
		r.add("required_crystals", "%d exceeds the %d crystal(s) ('%c') in rows, so the exit could never open", *f.RequiredCrystals, crystals, sim.SymbolCrystal)
	default:
		l.RequiredCrystals = *f.RequiredCrystals
	}

	if len(v.Problems) > 0 {
		return nil, v
	}
	l.cave.RequiredCrystals = l.RequiredCrystals
	l.cave.Mechanisms = l.Mechanisms
	// Defence in depth: the simulation's own invariants must agree.
	if err := l.cave.Validate(); err != nil {
		r.add("rows", "rejected by the simulation: %v", err)
		return nil, v
	}
	return l, nil
}

// validateRows checks the grid and fills l.cave. It returns the crystal
// count, or -1 if the grid was unusable for counting.
func validateRows(rows []string, l *Level, r *reporter) int {
	if rows == nil {
		r.add("rows", "missing")
		return -1
	}
	if l.Height > 0 && len(rows) != l.Height {
		r.add("rows", "has %d rows but height is %d", len(rows), l.Height)
	}
	if l.Width == 0 || l.Height == 0 || len(rows) != l.Height {
		return -1
	}

	cells := make([]sim.Cell, 0, l.Width*l.Height)
	var players, exits []sim.Pos
	crystals, gridOK := 0, true
	for y, row := range rows {
		field := fmt.Sprintf("rows[%d]", y)
		if n := utf8.RuneCountInString(row); n != l.Width {
			r.add(field, "has %d symbols but width is %d", n, l.Width)
			gridOK = false
			continue
		}
		var unknown, tooNew []string
		var unknownAt, tooNewAt []sim.Pos
		need := 0
		x := 0
		for _, sym := range row {
			p := sim.Pos{X: x, Y: y}
			cell, ok := sim.CellForSymbol(sym)
			if !ok {
				unknown = append(unknown, fmt.Sprintf("%q at x=%d", sym, x))
				unknownAt = append(unknownAt, p)
				gridOK = false
			}
			if v := SymbolVersion(sym); ok && v > l.FormatVersion {
				tooNew = append(tooNew, fmt.Sprintf("%q at x=%d", sym, x))
				tooNewAt = append(tooNewAt, p)
				need = max(need, v)
				gridOK = false
			}
			switch sym {
			case sim.SymbolPlayer:
				players = append(players, p)
			case sim.SymbolExit:
				exits = append(exits, p)
			case sim.SymbolCrystal:
				crystals++
			}
			cells = append(cells, cell)
			x++
		}
		if len(unknown) > 0 {
			r.addAt(field, unknownAt, "unknown tile symbol(s) %s; valid symbols are %s", elide(unknown), sim.SymbolLegend)
		}
		if len(tooNew) > 0 {
			r.addAt(field, tooNewAt, "symbol(s) %s need format_version %d or later (this file is version %d)",
				elide(tooNew), need, l.FormatVersion)
		}
	}
	countCheck := func(name string, sym rune, found []sim.Pos) {
		switch len(found) {
		case 0:
			r.add("rows", "no %s ('%c') found; exactly one is required", name, sym)
		case 1:
		default:
			places := make([]string, len(found))
			for i, p := range found {
				places[i] = p.String()
			}
			r.addAt("rows", found, "found %d %ss ('%c') at %s; exactly one is required", len(found), name, sym, elide(places))
		}
	}
	countCheck("player start", sim.SymbolPlayer, players)
	countCheck("exit", sim.SymbolExit, exits)
	if gridOK {
		l.cave = sim.Cave{Width: l.Width, Height: l.Height, Cells: cells}
	}
	return crystals
}

// validateMechanisms decodes the mechanisms field and checks it against the
// grid in l.cave (when the grid is usable), filling l.Mechanisms.
func validateMechanisms(raw json.RawMessage, l *Level, r *reporter) {
	var files []mechanismFile
	if raw != nil {
		if l.FormatVersion < mechanismFormatVersion {
			r.add("mechanisms", "needs format_version %d or later (this file is version %d)", mechanismFormatVersion, l.FormatVersion)
			return
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&files); err != nil {
			r.add("mechanisms", "must be a list of {\"valve\": {\"x\", \"y\"}, \"gates\": [{\"x\", \"y\"}, ...]}: %s",
				strings.TrimPrefix(err.Error(), "json: "))
			return
		}
	}
	grid := l.cave.Cells != nil
	cellAt := func(p sim.Pos) sim.Cell { return l.cave.Cells[p.Y*l.Width+p.X] }
	cell := func(p sim.Pos) sim.Terrain { return cellAt(p).Terrain }
	symbol := func(p sim.Pos) rune { return sim.Symbol(cellAt(p), false) }
	used := map[sim.Pos]string{} // valve or gate position → the field that claimed it
	pos := func(field string, pf *posFile, want func(sim.Terrain) bool, what string, symbols string) (sim.Pos, bool) {
		switch {
		case pf == nil:
			r.add(field, "missing")
		case pf.X == nil || pf.Y == nil:
			r.add(field, "needs both \"x\" and \"y\"")
		case *pf.X < 0 || *pf.Y < 0 || l.Width > 0 && *pf.X >= l.Width || l.Height > 0 && *pf.Y >= l.Height:
			r.add(field, "(%d,%d) is outside the %dx%d cave", *pf.X, *pf.Y, l.Width, l.Height)
		default:
			p := sim.Pos{X: *pf.X, Y: *pf.Y}
			if prev, dup := used[p]; dup {
				r.addAt(field, []sim.Pos{p}, "%s %s is already listed at %s", what, p, prev)
				return p, false
			}
			used[p] = field
			if grid && !want(cell(p)) {
				r.addAt(field, []sim.Pos{p}, "%s is %q in rows, not a %s (%s)", p, symbol(p), what, symbols)
				return p, false
			}
			return p, true
		}
		return sim.Pos{}, false
	}
	for i, mf := range files {
		field := fmt.Sprintf("mechanisms[%d]", i)
		valve, valveOK := pos(field+".valve", mf.Valve, sim.Terrain.Valve, "valve", "'V' or 'v'")
		if len(mf.Gates) == 0 {
			var at []sim.Pos
			if valveOK {
				at = []sim.Pos{valve}
			}
			r.addAt(field+".gates", at, "missing or empty; a valve must operate at least one gate")
		}
		m := sim.Mechanism{Valve: valve}
		for j := range mf.Gates {
			gf := fmt.Sprintf("%s.gates[%d]", field, j)
			g, ok := pos(gf, &mf.Gates[j], sim.Terrain.Gate, "gate", "'G' or 'g'")
			if ok && valveOK && (cell(g) == sim.TerrainGateOpen) != (cell(valve) == sim.TerrainValveOpen) {
				r.addAt(gf, []sim.Pos{g, valve}, "gate %s is %q but valve %s is %q; a gate is open ('g') exactly when its valve is ('v')",
					g, symbol(g), valve, symbol(valve))
			}
			m.Gates = append(m.Gates, g)
		}
		l.Mechanisms = append(l.Mechanisms, m)
	}
	if !grid {
		return
	}
	var orphans []string
	var orphansAt []sim.Pos
	for i, c := range l.cave.Cells {
		p := sim.Pos{X: i % l.Width, Y: i / l.Width}
		if _, listed := used[p]; !listed && (c.Terrain.Valve() || c.Terrain.Gate()) {
			orphans = append(orphans, fmt.Sprintf("%q at %s", sim.Symbol(c, false), p))
			orphansAt = append(orphansAt, p)
		}
	}
	if len(orphans) > 0 {
		r.addAt("mechanisms", orphansAt, "every valve and gate must be listed in a mechanism; not listed: %s", elide(orphans))
	}
}

// reporter collects problems into a ValidationError.
type reporter struct{ v *ValidationError }

func (r *reporter) add(field, format string, args ...any) { r.addAt(field, nil, format, args...) }

// addAt records a problem with the cells it concerns.
func (r *reporter) addAt(field string, at []sim.Pos, format string, args ...any) {
	r.v.Problems = append(r.v.Problems, Problem{field, fmt.Sprintf(format, args...), at})
}

func elide(items []string) string {
	if len(items) <= maxListedPlaces {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:maxListedPlaces], ", "), len(items)-maxListedPlaces)
}
