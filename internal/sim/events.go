package sim

import "fmt"

// EventKind classifies something that happened during a tick. Events feed
// audio, particles and HUD adapters; they never mutate world state.
type EventKind uint8

// Event kinds.
const (
	EventMoved         EventKind = iota + 1 // player moved; Pos is the destination
	EventDug                                // earth removed; Pos is the dug cell
	EventCollected                          // crystal collected; Pos is its cell
	EventExitOpened                         // quota met; Pos is the exit cell
	EventExitSealed                         // player bumped a sealed exit; Pos is the exit cell
	EventCaveComplete                       // player entered the open exit; Pos is the exit cell
	EventBoulderFell                        // boulder moved down one cell; From → Pos
	EventBoulderRolled                      // boulder moved sideways off a rounded blocker; From → Pos
	EventBoulderLanded                      // a falling boulder came to rest at Pos
	EventBoulderPushed                      // the player pushed a boulder; From → Pos
	EventPlayerCrushed                      // a falling boulder at From crushed the player at Pos
)

func (k EventKind) String() string {
	switch k {
	case EventMoved:
		return "moved"
	case EventDug:
		return "dug"
	case EventCollected:
		return "collected"
	case EventExitOpened:
		return "exit-opened"
	case EventExitSealed:
		return "exit-sealed"
	case EventCaveComplete:
		return "cave-complete"
	case EventBoulderFell:
		return "boulder-fell"
	case EventBoulderRolled:
		return "boulder-rolled"
	case EventBoulderLanded:
		return "boulder-landed"
	case EventBoulderPushed:
		return "boulder-pushed"
	case EventPlayerCrushed:
		return "player-crushed"
	}
	return fmt.Sprintf("event(%d)", uint8(k))
}

// Event records one occurrence during the tick numbered Tick.
type Event struct {
	Tick uint64
	Kind EventKind
	Pos  Pos
	// From is the starting cell for movement events (player moves and
	// boulder falls, rolls and pushes) and the crushing boulder's cell for
	// player-crushed. It is the zero Pos for other kinds.
	From Pos
}

func (e Event) String() string {
	switch e.Kind {
	case EventMoved, EventBoulderFell, EventBoulderRolled, EventBoulderPushed, EventPlayerCrushed:
		return fmt.Sprintf("t%d %s %s->%s", e.Tick, e.Kind, e.From, e.Pos)
	}
	return fmt.Sprintf("t%d %s %s", e.Tick, e.Kind, e.Pos)
}
