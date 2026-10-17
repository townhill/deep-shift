package sim

import "strings"

// Tile symbols. One legend serves authored level rows (internal/level) and
// ASCII state dumps, so a dump of a fresh cave reads back as the same layout.
const (
	SymbolEmpty    = ' '
	SymbolEarth    = '.'
	SymbolWall     = '#'
	SymbolCrystal  = '*'
	SymbolPlayer   = '@'
	SymbolExit     = 'E' // exit, sealed (and the only exit symbol in files)
	SymbolExitOpen = 'X' // exit, open; dumps only, not accepted in files
)

// SymbolLegend describes the symbols accepted in level rows, for error
// messages and documentation.
const SymbolLegend = `'#' wall, '.' earth, ' ' empty, '*' crystal, '@' player start, 'E' exit`

// CellForSymbol maps a level-file symbol to its initial cell. Crystals and the
// player start on empty terrain.
func CellForSymbol(r rune) (Cell, bool) {
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
	}
	return Cell{}, false
}

// Symbol returns the dump symbol for c. Occupants take precedence over
// terrain.
func Symbol(c Cell, exitOpen bool) rune {
	switch c.Occupant {
	case OccupantPlayer:
		return SymbolPlayer
	case OccupantCrystal:
		return SymbolCrystal
	}
	switch c.Terrain {
	case TerrainEmpty:
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
