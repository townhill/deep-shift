package sim

import "fmt"

// EventKind classifies something that happened during a tick. Events feed
// audio, particles and HUD adapters; they never mutate world state.
type EventKind uint8

// Event kinds.
const (
	EventMoved        EventKind = iota + 1 // player moved; Pos is the destination
	EventDug                               // earth removed; Pos is the dug cell
	EventCollected                         // crystal collected; Pos is its cell
	EventExitOpened                        // quota met; Pos is the exit cell
	EventExitSealed                        // player bumped a sealed exit; Pos is the exit cell
	EventCaveComplete                      // player entered the open exit; Pos is the exit cell
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
	}
	return fmt.Sprintf("event(%d)", uint8(k))
}

// Event records one occurrence during the tick numbered Tick.
type Event struct {
	Tick uint64
	Kind EventKind
	Pos  Pos
}

func (e Event) String() string { return fmt.Sprintf("t%d %s %s", e.Tick, e.Kind, e.Pos) }
