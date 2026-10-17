// Package level reads, validates and fingerprints authored cave files.
//
// Format version 1 is JSON with these fields (all required except metadata):
//
//	{
//	  "format_version": 1,
//	  "id": "test_cave",            // ^[a-z0-9][a-z0-9_]*$, at most 64 bytes
//	  "title": "Proving Ground",    // shown in the HUD, at most 80 characters
//	  "width": 40, "height": 24,    // 1..256 each
//	  "seed": 20261017,             // 0..2^53-1, so float64 JSON tools keep it exact
//	  "required_crystals": 8,       // 0..number of '*' tiles
//	  "rows": ["####", "#@E#", ...],// height strings of width symbols each
//	  "metadata": {"author": "..."} // optional free-form strings; ignored by rules
//	}
//
// Row symbols are the shared legend in internal/sim (sim.SymbolLegend).
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

// FormatVersion is the only cave file version this build reads.
const FormatVersion = 1

// MaxSeed is the largest seed accepted: 2^53-1, the largest integer every
// JSON implementation (including float64-based ones) round-trips exactly.
const MaxSeed = 1<<53 - 1

const (
	maxIDLen        = 64
	maxTitleLen     = 80
	maxListedPlaces = 5 // positions listed per problem before eliding
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)

// Level is a validated cave file.
type Level struct {
	FormatVersion    int
	ID               string
	Title            string
	Width, Height    int
	Seed             uint64
	RequiredCrystals int
	Rows             []string
	Metadata         map[string]string

	cave sim.Cave
}

// Cave returns a copy of the level's initial layout for sim.New.
func (l *Level) Cave() sim.Cave { return l.cave.Clone() }

// Crystals returns the number of crystals in the layout.
func (l *Level) Crystals() int { return l.cave.CountOccupant(sim.OccupantCrystal) }

// Checksum fingerprints the gameplay-relevant content: format version,
// dimensions, seed, quota and rows. Title and metadata are excluded, so
// cosmetic edits do not invalidate replays; any change that could alter a
// simulation does.
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
	return hex.EncodeToString(h.Sum(nil))
}

// Problem is one validation failure, located by JSON field path.
type Problem struct {
	Field string // e.g. "rows[3]" or "required_crystals"
	Msg   string
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
	Metadata         map[string]string `json:"metadata"`
}

// fields lists the format-version-1 keys, in documentation order.
var fields = []string{"format_version", "id", "title", "width", "height", "seed", "required_crystals", "rows", "metadata"}

// Parse decodes and validates a cave file. source names the file in error
// messages. On validation failure the error is a *ValidationError.
func Parse(source string, data []byte) (*Level, error) {
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
		return nil, fmt.Errorf("%s: unknown field(s) %s; format version %d fields are %s",
			source, strings.Join(unknown, ", "), FormatVersion, strings.Join(fields, ", "))
	}

	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, decodeError(source, data, err)
	}
	return validate(source, &f)
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
	add := func(field, format string, args ...any) {
		v.Problems = append(v.Problems, Problem{field, fmt.Sprintf(format, args...)})
	}

	// The version decides how everything else is read, so stop early.
	switch {
	case f.FormatVersion == nil:
		add("format_version", "missing; this build reads format_version %d", FormatVersion)
		return nil, v
	case *f.FormatVersion != FormatVersion:
		add("format_version", "unsupported version %d; this build reads format_version %d", *f.FormatVersion, FormatVersion)
		return nil, v
	}
	l := &Level{FormatVersion: *f.FormatVersion, Rows: f.Rows, Metadata: f.Metadata}

	switch {
	case f.ID == nil:
		add("id", "missing")
	case len(*f.ID) > maxIDLen || !idPattern.MatchString(*f.ID):
		add("id", "%q must be 1-%d characters of lowercase letters, digits and '_', starting with a letter or digit", *f.ID, maxIDLen)
	default:
		l.ID = *f.ID
	}
	switch {
	case f.Title == nil || strings.TrimSpace(*f.Title) == "":
		add("title", "missing or blank")
	case utf8.RuneCountInString(*f.Title) > maxTitleLen:
		add("title", "longer than %d characters", maxTitleLen)
	default:
		l.Title = *f.Title
	}
	dimension := func(field string, p *int) int {
		switch {
		case p == nil:
			add(field, "missing")
		case *p < 1 || *p > sim.MaxDimension:
			add(field, "%d is out of range 1..%d", *p, sim.MaxDimension)
		default:
			return *p
		}
		return 0
	}
	l.Width = dimension("width", f.Width)
	l.Height = dimension("height", f.Height)
	switch {
	case f.Seed == nil:
		add("seed", "missing")
	case *f.Seed > MaxSeed:
		add("seed", "%d exceeds %d (2^53-1), the largest integer all JSON tools keep exact", *f.Seed, uint64(MaxSeed))
	default:
		l.Seed = *f.Seed
	}
	for k := range f.Metadata {
		if strings.TrimSpace(k) == "" {
			add("metadata", "keys must not be blank")
			break
		}
	}

	crystals := validateRows(f.Rows, l, add)

	switch {
	case f.RequiredCrystals == nil:
		add("required_crystals", "missing")
	case *f.RequiredCrystals < 0:
		add("required_crystals", "%d is negative", *f.RequiredCrystals)
	case crystals >= 0 && *f.RequiredCrystals > crystals:
		add("required_crystals", "%d exceeds the %d crystal(s) ('%c') in rows, so the exit could never open", *f.RequiredCrystals, crystals, sim.SymbolCrystal)
	default:
		l.RequiredCrystals = *f.RequiredCrystals
	}

	if len(v.Problems) > 0 {
		return nil, v
	}
	l.cave.RequiredCrystals = l.RequiredCrystals
	// Defence in depth: the simulation's own invariants must agree.
	if err := l.cave.Validate(); err != nil {
		add("rows", "rejected by the simulation: %v", err)
		return nil, v
	}
	return l, nil
}

// validateRows checks the grid and fills l.cave. It returns the crystal
// count, or -1 if the grid was unusable for counting.
func validateRows(rows []string, l *Level, add func(field, format string, args ...any)) int {
	if rows == nil {
		add("rows", "missing")
		return -1
	}
	if l.Height > 0 && len(rows) != l.Height {
		add("rows", "has %d rows but height is %d", len(rows), l.Height)
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
			add(field, "has %d symbols but width is %d", n, l.Width)
			gridOK = false
			continue
		}
		var unknown []string
		x := 0
		for _, r := range row {
			cell, ok := sim.CellForSymbol(r)
			if !ok {
				unknown = append(unknown, fmt.Sprintf("%q at x=%d", r, x))
				gridOK = false
			}
			switch r {
			case sim.SymbolPlayer:
				players = append(players, sim.Pos{X: x, Y: y})
			case sim.SymbolExit:
				exits = append(exits, sim.Pos{X: x, Y: y})
			case sim.SymbolCrystal:
				crystals++
			}
			cells = append(cells, cell)
			x++
		}
		if len(unknown) > 0 {
			add(field, "unknown tile symbol(s) %s; valid symbols are %s", elide(unknown), sim.SymbolLegend)
		}
	}
	countCheck := func(name string, sym rune, found []sim.Pos) {
		switch len(found) {
		case 0:
			add("rows", "no %s ('%c') found; exactly one is required", name, sym)
		case 1:
		default:
			places := make([]string, len(found))
			for i, p := range found {
				places[i] = p.String()
			}
			add("rows", "found %d %ss ('%c') at %s; exactly one is required", len(found), name, sym, elide(places))
		}
	}
	countCheck("player start", sim.SymbolPlayer, players)
	countCheck("exit", sim.SymbolExit, exits)
	if gridOK {
		l.cave = sim.Cave{Width: l.Width, Height: l.Height, Cells: cells}
	}
	return crystals
}

func elide(items []string) string {
	if len(items) <= maxListedPlaces {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:maxListedPlaces], ", "), len(items)-maxListedPlaces)
}
