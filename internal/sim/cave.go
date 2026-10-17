package sim

import (
	"errors"
	"fmt"
)

// Cave is the initial layout handed to New. It is a plain value with no
// file-format concerns; internal/level converts authored files into Caves.
type Cave struct {
	Width, Height int
	// Cells holds Width*Height cells in row-major order: index = y*Width + x.
	Cells []Cell
	// RequiredCrystals is the quota that opens the exit. Zero means the exit
	// is open from the first tick.
	RequiredCrystals int
}

// Index returns the row-major index of p. It does not check bounds.
func (c *Cave) Index(p Pos) int { return p.Y*c.Width + p.X }

// InBounds reports whether p lies inside the cave.
func (c *Cave) InBounds(p Pos) bool {
	return p.X >= 0 && p.Y >= 0 && p.X < c.Width && p.Y < c.Height
}

// Clone returns a deep copy of c.
func (c Cave) Clone() Cave {
	c.Cells = append([]Cell(nil), c.Cells...)
	return c
}

// CountOccupant returns how many cells hold occupant o.
func (c *Cave) CountOccupant(o Occupant) int {
	n := 0
	for _, cell := range c.Cells {
		if cell.Occupant == o {
			n++
		}
	}
	return n
}

// Validate checks the structural invariants the simulation relies on. It is
// deliberately independent of any file format so that hand-built caves (in
// tests or future editors) get the same protection as authored levels.
// All problems found are reported together.
func (c *Cave) Validate() error {
	if c.Width < 1 || c.Height < 1 || c.Width > MaxDimension || c.Height > MaxDimension {
		return fmt.Errorf("cave dimensions %dx%d out of range: each side must be 1..%d", c.Width, c.Height, MaxDimension)
	}
	if len(c.Cells) != c.Width*c.Height {
		return fmt.Errorf("cave has %d cells, want width*height = %d", len(c.Cells), c.Width*c.Height)
	}

	var errs []error
	var players []Pos
	crystals := 0
	for i, cell := range c.Cells {
		p := Pos{i % c.Width, i / c.Width}
		if cell.Terrain >= terrainCount {
			errs = append(errs, fmt.Errorf("cell %s: unknown terrain %d", p, cell.Terrain))
			continue
		}
		if cell.Occupant >= occupantCount {
			errs = append(errs, fmt.Errorf("cell %s: unknown occupant %d", p, cell.Occupant))
			continue
		}
		if cell.Occupant == OccupantNone {
			continue
		}
		switch cell.Terrain {
		case TerrainWall:
			errs = append(errs, fmt.Errorf("cell %s: %s cannot occupy a wall", p, cell.Occupant))
		case TerrainExit:
			errs = append(errs, fmt.Errorf("cell %s: %s cannot start on the exit", p, cell.Occupant))
		}
		switch cell.Occupant {
		case OccupantPlayer:
			players = append(players, p)
		case OccupantCrystal:
			crystals++
		}
	}
	switch len(players) {
	case 0:
		errs = append(errs, errors.New("cave has no player start"))
	case 1:
	default:
		errs = append(errs, fmt.Errorf("cave has %d player starts at %v; exactly one is required", len(players), players))
	}
	if c.RequiredCrystals < 0 {
		errs = append(errs, fmt.Errorf("required crystals %d is negative", c.RequiredCrystals))
	} else if c.RequiredCrystals > crystals {
		errs = append(errs, fmt.Errorf("required crystals %d exceeds the %d crystals in the cave", c.RequiredCrystals, crystals))
	}
	return errors.Join(errs...)
}
