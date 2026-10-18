package sim

import "strings"

// Tile symbols. One legend serves authored level rows (internal/level) and
// ASCII state dumps, so a dump of a fresh cave reads back as the same layout.
const (
	SymbolEmpty      = ' '
	SymbolEarth      = '.'
	SymbolWall       = '#'
	SymbolCrystal    = '*'
	SymbolPlayer     = '@'
	SymbolBoulder    = 'O' // boulder; level files need format_version 2 or later
	SymbolExit       = 'E' // exit, sealed (and the only exit symbol in files)
	SymbolExitOpen   = 'X' // exit, open; dumps only, not accepted in files
	SymbolDeepWater  = '~' // empty floor full of water (WaterCapacity units)
	SymbolGateClosed = 'G'
	SymbolGateOpen   = 'g'
	SymbolValveShut  = 'V'
	SymbolValveOpen  = 'v'
)

// Shallow water on empty floor is written as its amount, '1' to '3'
// (WaterCapacity-1). Water, gates and valves need format_version 3 or later.

// SymbolLegend describes the symbols accepted in level rows, for error
// messages and documentation.
const SymbolLegend = `'#' wall, '.' earth, ' ' empty, '*' crystal, '@' player start, 'E' exit, 'O' boulder, ` +
	`'~' deep water, '1'-'3' shallow water, 'G'/'g' closed/open gate, 'V'/'v' shut/open valve`

// CellForSymbol maps a level-file symbol to its initial cell. Crystals,
// boulders, water and the player start on empty terrain; boulders start at
// rest.
func CellForSymbol(r rune) (Cell, bool) {
	if r >= '1' && r < '0'+WaterCapacity {
		return Cell{Terrain: TerrainEmpty, Water: uint8(r - '0')}, true
	}
	switch r {
	case SymbolEmpty:
		return Cell{Terrain: TerrainEmpty}, true
	case SymbolEarth:
		return Cell{Terrain: TerrainEarth}, true
	case SymbolWall:
		return Cell{Terrain: TerrainWall}, true
	case SymbolCrystal:
		return Cell{Terrain: TerrainEmpty, Occupant: OccupantCrystal}, true
	case SymbolPlayer:
		return Cell{Terrain: TerrainEmpty, Occupant: OccupantPlayer}, true
	case SymbolExit:
		return Cell{Terrain: TerrainExit}, true
	case SymbolBoulder:
		return Cell{Terrain: TerrainEmpty, Occupant: OccupantBoulder}, true
	case SymbolDeepWater:
		return Cell{Terrain: TerrainEmpty, Water: WaterCapacity}, true
	case SymbolGateClosed:
		return Cell{Terrain: TerrainGateClosed}, true
	case SymbolGateOpen:
		return Cell{Terrain: TerrainGateOpen}, true
	case SymbolValveShut:
		return Cell{Terrain: TerrainValveShut}, true
	case SymbolValveOpen:
		return Cell{Terrain: TerrainValveOpen}, true
	}
	return Cell{}, false
}

// Symbol returns the dump symbol for c. Occupants take precedence over
// water, and water over empty terrain. Water in an open gate or in the
// player's cell is not shown.
func Symbol(c Cell, exitOpen bool) rune {
	switch c.Occupant {
	case OccupantPlayer:
		return SymbolPlayer
	case OccupantCrystal:
		return SymbolCrystal
	case OccupantBoulder:
		return SymbolBoulder
	}
	switch c.Terrain {
	case TerrainEmpty:
		switch {
		case c.Deep():
			return SymbolDeepWater
		case c.Water > 0:
			return rune('0' + c.Water)
		}
		return SymbolEmpty
	case TerrainEarth:
		return SymbolEarth
	case TerrainWall:
		return SymbolWall
	case TerrainExit:
		if exitOpen {
			return SymbolExitOpen
		}
		return SymbolExit
	case TerrainGateClosed:
		return SymbolGateClosed
	case TerrainGateOpen:
		return SymbolGateOpen
	case TerrainValveShut:
		return SymbolValveShut
	case TerrainValveOpen:
		return SymbolValveOpen
	}
	return '?'
}

// ASCII renders the current grid, one line per row, for debugging and
// golden tests.
func (s *Sim) ASCII() string {
	return renderASCII(s.initial.Width, s.st.cells, s.st.exitOpen)
}

// ASCII renders the snapshot grid, one line per row.
func (s Snapshot) ASCII() string {
	return renderASCII(s.Width, s.Cells, s.ExitOpen)
}

func renderASCII(width int, cells []Cell, exitOpen bool) string {
	var b strings.Builder
	b.Grow(len(cells) + len(cells)/max(width, 1))
	for i, c := range cells {
		b.WriteRune(Symbol(c, exitOpen))
		if (i+1)%width == 0 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
