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
	// Mechanisms links each valve to the gates it operates. Every valve and
	// gate cell belongs to exactly one mechanism.
	Mechanisms []Mechanism
}

// Mechanism is a valve and the sluice gates it opens and closes together.
// The gates always share the valve's state: all open or all closed.
type Mechanism struct {
	Valve Pos
	Gates []Pos // in the order their events are reported
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
	ms := make([]Mechanism, len(c.Mechanisms))
	for i, m := range c.Mechanisms {
		ms[i] = Mechanism{Valve: m.Valve, Gates: append([]Pos(nil), m.Gates...)}
	}
	c.Mechanisms = ms
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
	crystals, glimmers := 0, 0
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
		if cell.Falling && !cell.Occupant.Falls() {
			errs = append(errs, fmt.Errorf("cell %s: only boulders and crystals can be falling", p))
		}
		switch {
		case cell.Occupant.Creature() && (cell.Facing < DirUp || cell.Facing > DirRight):
			errs = append(errs, fmt.Errorf("cell %s: a %s must face up, down, left or right", p, cell.Occupant))
		case !cell.Occupant.Creature() && cell.Facing != DirNone:
			errs = append(errs, fmt.Errorf("cell %s: only crawlers and glimmers have a facing", p))
		case cell.Occupant.Creature() && cell.Water > 0:
			errs = append(errs, fmt.Errorf("cell %s: a %s cannot start in water", p, cell.Occupant))
		}
		switch {
		case cell.Water > WaterCapacity:
			errs = append(errs, fmt.Errorf("cell %s: water %d exceeds the capacity %d", p, cell.Water, WaterCapacity))
		case cell.Water > 0 && !cell.HoldsWater():
			errs = append(errs, fmt.Errorf("cell %s: water cannot share a cell with %s on %s terrain", p, cell.Occupant, cell.Terrain))
		}
		if cell.Occupant == OccupantNone {
			continue
		}
		if (cell.Occupant == OccupantBoulder || cell.Occupant.Creature()) && cell.Terrain == TerrainEarth {
			errs = append(errs, fmt.Errorf("cell %s: a %s cannot be embedded in earth", p, cell.Occupant))
		}
		switch cell.Terrain {
		case TerrainWall, TerrainGateClosed, TerrainValveShut, TerrainValveOpen:
			errs = append(errs, fmt.Errorf("cell %s: %s cannot occupy a %s", p, cell.Occupant, cell.Terrain))
		case TerrainExit:
			errs = append(errs, fmt.Errorf("cell %s: %s cannot start on the exit", p, cell.Occupant))
		}
		switch cell.Occupant {
		case OccupantPlayer:
			players = append(players, p)
		case OccupantCrystal:
			crystals++
		case OccupantGlimmer:
			glimmers++
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
	} else if most := crystals + ShatterCrystals*glimmers; c.RequiredCrystals > most {
		if glimmers == 0 {
			errs = append(errs, fmt.Errorf("required crystals %d exceeds the %d crystals in the cave", c.RequiredCrystals, crystals))
		} else {
			errs = append(errs, fmt.Errorf("required crystals %d exceeds the %d the cave can yield (%d placed, up to %d from each of %d glimmer(s))",
				c.RequiredCrystals, most, crystals, ShatterCrystals, glimmers))
		}
	}
	errs = append(errs, c.validateMechanisms()...)
	return errors.Join(errs...)
}

// validateMechanisms checks that every valve and gate belongs to exactly one
// mechanism, each mechanism has at least one gate, and its gates share its
// valve's state.
func (c *Cave) validateMechanisms() []error {
	var errs []error
	owner := make([]int, len(c.Cells)) // 1 + mechanism index, 0 for none
	claim := func(i int, p Pos, want func(Terrain) bool, what string) bool {
		if !c.InBounds(p) {
			errs = append(errs, fmt.Errorf("mechanism %d: %s %s is outside the cave", i, what, p))
			return false
		}
		k := c.Index(p)
		switch {
		case !want(c.Cells[k].Terrain):
			errs = append(errs, fmt.Errorf("mechanism %d: %s %s is %s, not a %s", i, what, p, c.Cells[k].Terrain, what))
		case owner[k] != 0:
			errs = append(errs, fmt.Errorf("mechanism %d: %s %s already belongs to mechanism %d", i, what, p, owner[k]-1))
		default:
			owner[k] = i + 1
			return true
		}
		return false
	}
	for i, m := range c.Mechanisms {
		if !claim(i, m.Valve, Terrain.Valve, "valve") {
			continue
		}
		if len(m.Gates) == 0 {
			errs = append(errs, fmt.Errorf("mechanism %d: valve %s operates no gates", i, m.Valve))
		}
		open := c.Cells[c.Index(m.Valve)].Terrain == TerrainValveOpen
		for _, g := range m.Gates {
			if claim(i, g, Terrain.Gate, "gate") && (c.Cells[c.Index(g)].Terrain == TerrainGateOpen) != open {
				errs = append(errs, fmt.Errorf("mechanism %d: gate %s must be %s like its valve %s", i, g, gateState(open), m.Valve))
			}
		}
	}
	for k, cell := range c.Cells {
		if (cell.Terrain.Valve() || cell.Terrain.Gate()) && owner[k] == 0 {
			errs = append(errs, fmt.Errorf("cell %s: %s belongs to no mechanism", Pos{k % c.Width, k / c.Width}, cell.Terrain))
		}
	}
	return errs
}

func gateState(open bool) string {
	if open {
		return "open"
	}
	return "closed"
}
