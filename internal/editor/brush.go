package editor

import (
	"fmt"

	"github.com/townhill/deep-shift/internal/sim"
)

// Brush is one palette entry: a family of symbols. Choosing the brush
// again (its number key, or a click on it) moves on to the next variant.
type Brush struct {
	Name     string
	Variants []rune
}

// Palette lists the brushes in key order (1 to 9). Valves and gates are not
// on it yet: they need linking into mechanisms, which the editor cannot do
// yet. Caves loaded with them keep them.
var Palette = []Brush{
	{"Wall", []rune{sim.SymbolWall}},
	{"Earth", []rune{sim.SymbolEarth}},
	{"Floor", []rune{sim.SymbolEmpty}},
	{"Crystal", []rune{sim.SymbolCrystal}},
	{"Boulder", []rune{sim.SymbolBoulder}},
	{"Player start", []rune{sim.SymbolPlayer}},
	{"Exit", []rune{sim.SymbolExit}},
	{"Water", []rune{sim.SymbolDeepWater, '3', '2', '1'}},
	{"Crawler", []rune{'↑', '→', '↓', '←'}}, // clockwise
}

// eraser is what the right mouse button paints: empty floor.
const eraser = sim.SymbolEmpty

// brushFor finds the brush and variant that paint symbol r.
func brushFor(r rune) (brush, variant int, ok bool) {
	for i, b := range Palette {
		for j, v := range b.Variants {
			if v == r {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// symbolName describes a symbol for the status lines.
func symbolName(r rune) string {
	switch r {
	case sim.SymbolEmpty:
		return "floor"
	case sim.SymbolEarth:
		return "earth"
	case sim.SymbolWall:
		return "wall"
	case sim.SymbolCrystal:
		return "crystal"
	case sim.SymbolPlayer:
		return "player start"
	case sim.SymbolBoulder:
		return "boulder"
	case sim.SymbolExit:
		return "exit"
	case sim.SymbolDeepWater:
		return "deep water"
	case '1', '2', '3':
		return fmt.Sprintf("shallow water (%c of %d)", r, sim.WaterCapacity)
	case sim.SymbolGateClosed:
		return "closed gate"
	case sim.SymbolGateOpen:
		return "open gate"
	case sim.SymbolValveShut:
		return "shut valve"
	case sim.SymbolValveOpen:
		return "open valve"
	}
	if c, ok := sim.CellForSymbol(r); ok && c.Occupant == sim.OccupantCrawler {
		return "crawler facing " + c.Facing.String()
	}
	return fmt.Sprintf("unknown symbol %q", r)
}
