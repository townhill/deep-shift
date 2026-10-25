package editor

import (
	"fmt"

	"github.com/townhill/deep-shift/internal/sim"
)

// Brush is one palette entry: a family of symbols. Choosing the brush
// again (its key, or a click on it) moves on to the next variant.
type Brush struct {
	Name     string
	Key      string // the key that chooses it, as labelled on the palette
	Variants []rune
}

// Palette lists the brushes in key order: 1 to 9, then 0 and -. A valve
// brush paints a shut valve with a mechanism of its own; a gate brush paints
// a gate linked to the selected valve (D-052).
var Palette = []Brush{
	{"Wall", "1", []rune{sim.SymbolWall}},
	{"Earth", "2", []rune{sim.SymbolEarth}},
	{"Floor", "3", []rune{sim.SymbolEmpty}},
	{"Crystal", "4", []rune{sim.SymbolCrystal}},
	{"Boulder", "5", []rune{sim.SymbolBoulder}},
	{"Player start", "6", []rune{sim.SymbolPlayer}},
	{"Exit", "7", []rune{sim.SymbolExit}},
	{"Water", "8", []rune{sim.SymbolDeepWater, '3', '2', '1'}},
	{"Crawler", "9", []rune{'↑', '→', '↓', '←'}}, // clockwise
	{"Valve", "0", []rune{sim.SymbolValveShut}},
	{"Gate", "-", []rune{sim.SymbolGateClosed}},
}

// gateBrush is the gate brush's index in Palette.
var gateBrush, _, _ = brushFor(sim.SymbolGateClosed)

// oneOnly reports whether a box or flood fill of symbol r places just one,
// where the gesture ended: a second player start or exit would only move the
// first, and a box of valves, each with no gates, is never wanted.
func oneOnly(r rune) bool { return single(r) || r == sim.SymbolValveShut }

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
