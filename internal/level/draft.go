package level

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/townhill/deep-shift/internal/sim"
)

// Draft is a cave file's content before validation: what an editor holds
// while a cave is being changed, and what Encode writes. Unlike a Level it
// may be invalid (two player starts, a quota above the crystal count);
// Parse(Encode()) reports every problem.
type Draft struct {
	FormatVersion    int
	ID               string
	Title            string
	Width, Height    int
	Seed             uint64
	RequiredCrystals int
	Rows             []string
	Mechanisms       []sim.Mechanism
	Metadata         map[string]string
}

// Draft returns a copy of the level's content, which Encode writes back
// exactly as the canonical file of this level.
func (l *Level) Draft() Draft {
	return Draft{
		FormatVersion:    l.FormatVersion,
		ID:               l.ID,
		Title:            l.Title,
		Width:            l.Width,
		Height:           l.Height,
		Seed:             l.Seed,
		RequiredCrystals: l.RequiredCrystals,
		Rows:             slices.Clone(l.Rows),
		Mechanisms:       cloneMechanisms(l.Mechanisms),
		Metadata:         maps.Clone(l.Metadata),
	}
}

// Clone returns a deep copy of d.
func (d Draft) Clone() Draft {
	d.Rows = slices.Clone(d.Rows)
	d.Mechanisms = cloneMechanisms(d.Mechanisms)
	d.Metadata = maps.Clone(d.Metadata)
	return d
}

func cloneMechanisms(ms []sim.Mechanism) []sim.Mechanism {
	if ms == nil {
		return nil
	}
	out := make([]sim.Mechanism, len(ms))
	for i, m := range ms {
		out[i] = sim.Mechanism{Valve: m.Valve, Gates: slices.Clone(m.Gates)}
	}
	return out
}

// Encode writes d in the canonical cave file layout: fields in documentation
// order, one row per line so the grid reads as a picture, one mechanism per
// line, metadata keys sorted, and no mechanisms or metadata field when there
// are none. Every embedded cave is stored in this layout (cavetool fmt
// checks it), so an editor that loads and saves a cave unchanged rewrites
// the same bytes.
func (d Draft) Encode() []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"format_version\": %d,\n", d.FormatVersion)
	fmt.Fprintf(&b, "  \"id\": %s,\n", jsonString(d.ID))
	fmt.Fprintf(&b, "  \"title\": %s,\n", jsonString(d.Title))
	fmt.Fprintf(&b, "  \"width\": %d,\n", d.Width)
	fmt.Fprintf(&b, "  \"height\": %d,\n", d.Height)
	fmt.Fprintf(&b, "  \"seed\": %d,\n", d.Seed)
	fmt.Fprintf(&b, "  \"required_crystals\": %d,\n", d.RequiredCrystals)
	b.WriteString("  \"rows\": [")
	for i, r := range d.Rows {
		b.WriteString(separator(i))
		b.WriteString(jsonString(r))
	}
	b.WriteString(closing(len(d.Rows), "]"))
	if len(d.Mechanisms) > 0 {
		b.WriteString(",\n  \"mechanisms\": [")
		for i, m := range d.Mechanisms {
			b.WriteString(separator(i))
			fmt.Fprintf(&b, "{\"valve\": %s, \"gates\": [", jsonPos(m.Valve))
			for j, g := range m.Gates {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(jsonPos(g))
			}
			b.WriteString("]}")
		}
		b.WriteString(closing(len(d.Mechanisms), "]"))
	}
	if len(d.Metadata) > 0 {
		b.WriteString(",\n  \"metadata\": {")
		for i, k := range slices.Sorted(maps.Keys(d.Metadata)) {
			b.WriteString(separator(i))
			fmt.Fprintf(&b, "%s: %s", jsonString(k), jsonString(d.Metadata[k]))
		}
		b.WriteString(closing(len(d.Metadata), "}"))
	}
	b.WriteString("\n}\n")
	return b.Bytes()
}

// separator starts the i-th element of a list written one element per line.
func separator(i int) string {
	if i == 0 {
		return "\n    "
	}
	return ",\n    "
}

// closing ends a list of n elements written one element per line.
func closing(n int, bracket string) string {
	if n == 0 {
		return bracket
	}
	return "\n  " + bracket
}

// jsonString quotes s as a JSON string, leaving '<', '>' and '&' readable.
func jsonString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // encoding a string cannot fail
	return strings.TrimSuffix(b.String(), "\n")
}

func jsonPos(p sim.Pos) string { return fmt.Sprintf("{\"x\": %d, \"y\": %d}", p.X, p.Y) }

// DecodeDraft reads a cave file without validating its content, so an editor
// can open a cave that still has problems (a work in progress saved with two
// player starts, say) and fix them. It fails only when the file is not a
// JSON object of the cave file's fields with the right types. Missing fields
// are left at their zero values.
func DecodeDraft(source string, data []byte) (Draft, error) {
	f, err := decode(source, data)
	if err != nil {
		return Draft{}, err
	}
	deref := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	d := Draft{
		FormatVersion:    deref(f.FormatVersion),
		Width:            deref(f.Width),
		Height:           deref(f.Height),
		RequiredCrystals: deref(f.RequiredCrystals),
		Rows:             f.Rows,
		Metadata:         f.Metadata,
	}
	if f.ID != nil {
		d.ID = *f.ID
	}
	if f.Title != nil {
		d.Title = *f.Title
	}
	if f.Seed != nil {
		d.Seed = *f.Seed
	}
	if f.Mechanisms != nil {
		var files []mechanismFile
		dec := json.NewDecoder(bytes.NewReader(f.Mechanisms))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&files); err != nil {
			return Draft{}, fmt.Errorf("%s: mechanisms must be a list of {\"valve\": {\"x\", \"y\"}, \"gates\": [{\"x\", \"y\"}, ...]}: %s",
				source, strings.TrimPrefix(err.Error(), "json: "))
		}
		for i, mf := range files {
			field := fmt.Sprintf("mechanisms[%d]", i)
			valve, err := mechanismPos(source, field+".valve", mf.Valve)
			if err != nil {
				return Draft{}, err
			}
			m := sim.Mechanism{Valve: valve}
			for j := range mf.Gates {
				g, err := mechanismPos(source, fmt.Sprintf("%s.gates[%d]", field, j), &mf.Gates[j])
				if err != nil {
					return Draft{}, err
				}
				m.Gates = append(m.Gates, g)
			}
			d.Mechanisms = append(d.Mechanisms, m)
		}
	}
	return d, nil
}

func mechanismPos(source, field string, pf *posFile) (sim.Pos, error) {
	if pf == nil || pf.X == nil || pf.Y == nil {
		return sim.Pos{}, fmt.Errorf("%s: %s needs both \"x\" and \"y\"", source, field)
	}
	return sim.Pos{X: *pf.X, Y: *pf.Y}, nil
}
