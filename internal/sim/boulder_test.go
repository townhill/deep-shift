package sim

import (
	"slices"
	"testing"
)

// idle steps n ticks with no input and returns all events.
func idle(s *Sim, n int) []Event {
	var all []Event
	for range n {
		all = append(all, s.Step(Input{})...)
	}
	return all
}

func boulderAt(s *Sim, p Pos) bool { return s.Cell(p).Occupant == OccupantBoulder }

// SPEC §11 Phase 2: an unsupported rock falls one cell on the defined update
// tick and never skips a cell.
func TestBoulderFallsOneCellPerGravityStep(t *testing.T) {
	s := newSim(t, 0,
		"O@",
		"  ",
		"  ",
		"  ",
		"##",
	)
	wantY := []int{1, 1, 2, 2, 3, 3, 3, 3} // after ticks 0..7
	for tick, y := range wantY {
		ev := s.Step(Input{})
		if !boulderAt(s, Pos{0, y}) || s.Boulders() != 1 {
			t.Fatalf("after tick %d: boulder not at (0,%d):\n%s", tick, y, s.ASCII())
		}
		for _, e := range ev {
			if e.Kind == EventFell && (e.Pos.X != e.From.X || e.Pos.Y != e.From.Y+1) {
				t.Fatalf("tick %d: fall %v skipped or slid", tick, e)
			}
			if e.Kind == EventFell && tick%GravityIntervalTicks != 0 {
				t.Fatalf("tick %d: fell outside a gravity tick", tick)
			}
		}
		if tick == 6 {
			wantKinds(t, ev, EventLanded)
		}
	}
	if s.Cell(Pos{0, 3}).Falling {
		t.Fatal("boulder still falling after landing")
	}
}

func TestSupportedBoulderStaysAtRest(t *testing.T) {
	tests := []struct {
		name string
		rows []string
	}{
		{"on earth", []string{"O@", ".."}},
		{"on wall", []string{"O@", "##"}},
		{"on the cave floor", []string{"O@"}},
		{"on the exit", []string{"O@", "E "}},
		{"on the player", []string{"O ", "@ "}},
		{"on a boulder, flat both sides", []string{"#O#@", "#O# ", "####"}},
		{"on a boulder, diagonals blocked", []string{" O @", ".O. ", "####"}},
		{"on a crystal, diagonals blocked", []string{" O @", ".*. ", "####"}},
		{"on a flat blocker with open sides", []string{"  O  @", "  .   ", "######"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim(t, 0, tt.rows...)
			before := s.ASCII()
			if ev := idle(s, 12); len(ev) != 0 {
				t.Fatalf("supported boulder produced events %v", ev)
			}
			if s.ASCII() != before || s.Status() != StatusPlaying {
				t.Fatalf("cave changed:\n%s\nwant:\n%s", s.ASCII(), before)
			}
		})
	}
}

func TestRollingCriteriaAndTieBreak(t *testing.T) {
	tests := []struct {
		name string
		rows []string
		want Pos // where the top boulder is after the first gravity step
	}{
		{"rolls left first when both sides are open", []string{"  O  @", "  O   ", "######"}, Pos{1, 0}},
		{"rolls right when the left side is blocked", []string{" #O  @", "  O   ", "######"}, Pos{3, 0}},
		{"rolls right when the left diagonal is blocked", []string{"  O  @", " .O   ", "######"}, Pos{3, 0}},
		{"rolls off a crystal", []string{"  O  @", "  *   ", "######"}, Pos{1, 0}},
		{"no roll when both diagonals are blocked", []string{"  O  @", " .O.  ", "######"}, Pos{2, 0}},
		{"no roll off the player", []string{"  O  ", "  @  ", "#####"}, Pos{2, 0}},
		{"no roll at the cave edge", []string{"O#  @", "O    ", "#####"}, Pos{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim(t, 0, tt.rows...)
			ev := s.Step(Input{})
			if !boulderAt(s, tt.want) {
				t.Fatalf("top boulder not at %s after one gravity step:\n%s", tt.want, s.ASCII())
			}
			if rolled := tt.want.X != indexOfO(tt.rows[0]); rolled {
				wantKinds(t, ev, EventRolled)
				if !s.Cell(tt.want).Falling {
					t.Fatal("a rolled boulder must be marked falling")
				}
			} else if len(ev) != 0 {
				t.Fatalf("boulder that should rest produced %v", ev)
			}
		})
	}
}

func indexOfO(row string) int { return slices.Index([]rune(row), SymbolBoulder) }

// A rolled boulder falls into the diagonal on the next gravity step: the
// diagonal move takes two steps and never teleports.
func TestRolledBoulderFallsNextStep(t *testing.T) {
	s := newSim(t, 0,
		"  O  @",
		"  O   ",
		"  #   ",
		"######",
	)
	wantKinds(t, s.Step(Input{}), EventRolled) // tick 0: (2,0) -> (1,0)
	wantKinds(t, s.Step(Input{}))              // tick 1: no gravity
	ev := s.Step(Input{})                      // tick 2: falls to (1,1)
	wantKinds(t, ev, EventFell)
	if ev[0].From != (Pos{1, 0}) || ev[0].Pos != (Pos{1, 1}) {
		t.Fatalf("fall = %v", ev[0])
	}
	idle(s, 1)
	wantKinds(t, s.Step(Input{}), EventFell) // tick 4: (1,1) -> (1,2)
	idle(s, 1)
	wantKinds(t, s.Step(Input{}), EventLanded) // tick 6: on the wall
}

// SPEC §11 Phase 2: each rock updates at most once per physics tick. A
// boulder rolling right lands in a cell the left-to-right scan has not
// reached yet; it must not fall again in the same step.
func TestBoulderUpdatesAtMostOncePerStep(t *testing.T) {
	s := newSim(t, 0,
		"#O  @",
		"#O   ",
		"##   ",
		"#####",
	)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventRolled)
	if !boulderAt(s, Pos{2, 0}) || boulderAt(s, Pos{2, 1}) {
		t.Fatalf("rolled boulder moved twice in one step:\n%s", s.ASCII())
	}
}

func TestStackFallsTogetherOneCellEach(t *testing.T) {
	s := newSim(t, 0,
		"O@",
		"O ",
		"O ",
		"  ",
		"  ",
		"##",
	)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventFell, EventFell, EventFell)
	for i, e := range ev {
		if e.Pos.Y != e.From.Y+1 {
			t.Fatalf("event %d moved more than one cell: %v", i, e)
		}
	}
	if got := s.ASCII(); got != rowsASCII(" @", "O ", "O ", "O ", "  ", "##") {
		t.Fatalf("stack after one step:\n%s", got)
	}
}

func TestPushRules(t *testing.T) {
	tests := []struct {
		name   string
		rows   []string
		dir    Dir
		pushed bool
	}{
		{"right into empty floor", []string{"@O ", "###"}, DirRight, true},
		{"left into empty floor", []string{" O@", "###"}, DirLeft, true},
		{"no chain push", []string{"@OO ", "####"}, DirRight, false},
		{"blocked by earth", []string{"@O.", "###"}, DirRight, false},
		{"blocked by wall", []string{"@O#", "###"}, DirRight, false},
		{"blocked by crystal", []string{"@O*", "###"}, DirRight, false},
		{"blocked by the exit", []string{"@OE", "###"}, DirRight, false},
		{"blocked by the cave edge", []string{"@O", "##"}, DirRight, false},
		{"not downwards", []string{"@", "O", "#"}, DirDown, false},
		{"not upwards", []string{" ", "O", "@"}, DirUp, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim(t, 0, tt.rows...)
			start := s.Player()
			ev := s.Step(Input{Move: tt.dir})
			if !tt.pushed {
				if s.Player() != start || len(ev) != 0 || !s.MoveReady() {
					t.Fatalf("blocked push changed something: player %s events %v ready %v\n%s", s.Player(), ev, s.MoveReady(), s.ASCII())
				}
				return
			}
			boulder := start.Add(tt.dir)
			wantKinds(t, ev, EventMoved, EventBoulderPushed)
			if s.Player() != boulder || !boulderAt(s, boulder.Add(tt.dir)) {
				t.Fatalf("after push:\n%s", s.ASCII())
			}
			if ev[1].From != boulder || ev[1].Pos != boulder.Add(tt.dir) {
				t.Fatalf("push event = %v", ev[1])
			}
			if s.MoveReady() {
				t.Fatal("a push must use the movement window like a move")
			}
		})
	}
}

func TestFallingBoulderCannotBePushed(t *testing.T) {
	c := caveFromRows(t, 0, "@O ", "#.#")
	c.Cells[1].Falling = true // as if it had just landed this step
	s, err := New(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	ev := s.Step(Input{Move: DirRight}) // push refused; gravity then lands it
	wantKinds(t, ev, EventLanded)
	if s.Player() != (Pos{0, 0}) {
		t.Fatal("pushed a falling boulder")
	}
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventBoulderPushed)
}

// A pushed boulder moves only one cell in its tick: even on a gravity tick it
// waits for the next gravity step before falling.
func TestPushedBoulderFallsOnTheNextGravityStep(t *testing.T) {
	s := newSim(t, 0, "@O ", "## ", "###")
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventBoulderPushed) // tick 0
	wantKinds(t, s.Step(Input{}))                                               // tick 1
	wantKinds(t, s.Step(Input{}), EventFell)                                    // tick 2
	if !boulderAt(s, Pos{2, 1}) {
		t.Fatalf("pushed boulder did not fall:\n%s", s.ASCII())
	}
}

// SPEC §11 Phase 2: falling rock collision kills the player and ends the cave.
func TestCrushing(t *testing.T) {
	tests := []struct {
		name     string
		rows     []string
		moves    []Dir // one per movement window
		crushed  bool
		deadTick uint64
	}{
		{"falling boulder lands on the player", []string{"O ", "  ", "@ ", "##"}, nil, true, 2},
		{"walking under a falling boulder", []string{" O", "  ", "@ ", "##"}, []Dir{DirRight}, true, 2},
		{"running straight down from under a boulder", []string{"O", "@", " ", " ", "#"}, []Dir{DirDown}, true, 2},
		{"stepping aside from under a boulder is safe", []string{"O  ", "@  ", "###"}, []Dir{DirRight}, false, 0},
		{"a resting boulder never crushes", []string{"O ", "@ ", "##"}, nil, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSim(t, 0, tt.rows...)
			var crush []Event
			for tick := range 40 {
				var in Input
				if tick%MoveIntervalTicks == 0 && tick/MoveIntervalTicks < len(tt.moves) {
					in.Move = tt.moves[tick/MoveIntervalTicks]
				}
				for _, e := range s.Step(in) {
					if e.Kind == EventPlayerCrushed {
						crush = append(crush, e)
					}
				}
			}
			if !tt.crushed {
				if len(crush) != 0 || s.Status() != StatusPlaying {
					t.Fatalf("player crushed: %v\n%s", crush, s.ASCII())
				}
				return
			}
			if len(crush) != 1 || s.Status() != StatusDead {
				t.Fatalf("crush events %v, status %s\n%s", crush, s.Status(), s.ASCII())
			}
			if crush[0].Tick != tt.deadTick || crush[0].Pos != s.Player() || crush[0].From != s.Player().Add(DirUp) {
				t.Fatalf("crush event %v, player at %s", crush[0], s.Player())
			}
			if s.Tick() != tt.deadTick+1 || s.MoveReady() {
				t.Fatalf("dead cave kept running: tick %d", s.Tick())
			}
		})
	}
}

func TestDeathFreezesAndResetRestores(t *testing.T) {
	rows := []string{"O ", "  ", "@ ", "##"}
	s := newSim(t, 0, rows...)
	fresh := newSim(t, 0, rows...)
	idle(s, 3)
	if s.Status() != StatusDead {
		t.Fatalf("status %s, want dead", s.Status())
	}
	hash, tick := s.StateHash(), s.Tick()
	if ev := idle(s, 10); ev != nil || s.StateHash() != hash || s.Tick() != tick {
		t.Fatal("dead cave changed")
	}
	if s.Step(Input{Move: DirRight}) != nil || s.Player() != (Pos{0, 2}) {
		t.Fatal("dead player moved")
	}
	s.Reset()
	if s.StateHash() != fresh.StateHash() || s.Status() != StatusPlaying {
		t.Fatal("Reset after death did not restore the initial state")
	}
	idle(s, 3)
	if s.StateHash() != hash {
		t.Fatal("death after Reset is not reproducible")
	}
}

func TestEnteringTheExitEndsTheTickBeforeGravity(t *testing.T) {
	s := newSim(t, 0,
		" O",
		"@E",
	)
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventCaveComplete)
	if s.Status() != StatusComplete || !boulderAt(s, Pos{1, 0}) {
		t.Fatalf("status %s\n%s", s.Status(), s.ASCII())
	}
}

func TestBoulderCaveValidation(t *testing.T) {
	c := caveFromRows(t, 0, "@ ", "  ")
	c.Cells[1].Falling = true // falling flag on an empty cell
	if _, err := New(c, 0); err == nil {
		t.Fatal("accepted a falling flag without a boulder")
	}
	c = caveFromRows(t, 0, "@ ", "  ")
	c.Cells[1] = Cell{Terrain: TerrainEarth, Occupant: OccupantBoulder}
	if _, err := New(c, 0); err == nil {
		t.Fatal("accepted a boulder embedded in earth")
	}
}

// Crystals obey the same gravity as boulders (owner decision, D-019).
func TestCrystalFallsLikeABoulder(t *testing.T) {
	s := newSim(t, 0,
		"*@",
		"  ",
		"  ",
		"##",
	)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventFell)
	if ev[0].Object != OccupantCrystal || ev[0].Pos != (Pos{0, 1}) {
		t.Fatalf("fall = %v", ev[0])
	}
	idle(s, 1)
	wantKinds(t, s.Step(Input{}), EventFell)
	idle(s, 1)
	ev = s.Step(Input{})
	wantKinds(t, ev, EventLanded)
	if s.Cell(Pos{0, 2}).Occupant != OccupantCrystal || s.Cell(Pos{0, 2}).Falling {
		t.Fatalf("crystal did not land at (0,2):\n%s", s.ASCII())
	}
}

func TestCrystalRollsOffRoundedObjects(t *testing.T) {
	s := newSim(t, 0,
		"  *  @",
		"  O   ",
		"  #   ",
		"######",
	)
	ev := s.Step(Input{})
	wantKinds(t, ev, EventRolled)
	if ev[0].Object != OccupantCrystal || ev[0].Pos != (Pos{1, 0}) {
		t.Fatalf("roll = %v", ev[0])
	}
}

func TestFallingCrystalCrushesThePlayer(t *testing.T) {
	s := newSim(t, 0, "* ", "  ", "@ ", "##")
	var crush []Event
	for _, e := range idle(s, 4) {
		if e.Kind == EventPlayerCrushed {
			crush = append(crush, e)
		}
	}
	if len(crush) != 1 || crush[0].Object != OccupantCrystal || s.Status() != StatusDead {
		t.Fatalf("crush events %v, status %s", crush, s.Status())
	}
}

func TestWalkingIntoAFallingCrystalCollectsIt(t *testing.T) {
	s := newSim(t, 1,
		" *",
		"@ ",
		"  ",
		"  ",
		"##",
	)
	wantKinds(t, s.Step(Input{}), EventFell) // tick 0: crystal falls beside the player
	if !s.Cell(Pos{1, 1}).Falling {
		t.Fatal("crystal should be falling")
	}
	wantKinds(t, s.Step(Input{Move: DirRight}), EventMoved, EventCollected)
	if s.Collected() != 1 || s.Status() != StatusPlaying || s.Cell(Pos{1, 1}).Falling {
		t.Fatalf("collected %d, status %s, player cell %+v", s.Collected(), s.Status(), s.Cell(Pos{1, 1}))
	}
}

// Only objects on open floor fall. A crystal embedded in earth (possible in
// hand-built caves, not in level files) stays until dug out.
func TestObjectEmbeddedInEarthDoesNotFall(t *testing.T) {
	c := caveFromRows(t, 0, "@.", "  ")
	c.Cells[1].Occupant = OccupantCrystal
	s, err := New(c, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ev := idle(s, 6); len(ev) != 0 || s.Cell(Pos{1, 0}).Occupant != OccupantCrystal {
		t.Fatalf("embedded crystal moved: %v\n%s", ev, s.ASCII())
	}
}
