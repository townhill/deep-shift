package sim

import "fmt"

// EventKind classifies something that happened during a tick. Events feed
// audio, particles and HUD adapters; they never mutate world state.
type EventKind uint8

// Event kinds.
const (
	EventMoved            EventKind = iota + 1 // player moved; Pos is the destination
	EventDug                                   // earth removed; Pos is the dug cell
	EventCollected                             // crystal collected; Pos is its cell
	EventExitOpened                            // quota met; Pos is the exit cell
	EventExitSealed                            // player bumped a sealed exit; Pos is the exit cell
	EventCaveComplete                          // player entered the open exit; Pos is the exit cell
	EventFell                                  // Object moved down one cell; From → Pos
	EventRolled                                // Object moved sideways off a rounded blocker; From → Pos
	EventLanded                                // falling Object came to rest at Pos
	EventBoulderPushed                         // the player pushed a boulder; From → Pos
	EventPlayerCrushed                         // falling Object at From crushed the player at Pos
	EventTooDeep                               // player bumped deep water at Pos
	EventValveTurned                           // the player turned the valve at Pos
	EventGateOpened                            // the gate at Pos opened
	EventGateClosed                            // the gate at Pos closed
	EventGateJammed                            // closing failed: the gate at Pos is not clear
	EventCrawlerMoved                          // a crawler moved; From → Pos
	EventPlayerCaught                          // the creature (Object) at From caught the player at Pos
	EventCrawlerCrushed                        // falling Object at From crushed the crawler at Pos
	EventCrawlerDrowned                        // water reached the crawler at Pos
	EventGlimmerMoved                          // a glimmer moved; From → Pos
	EventGlimmerShattered                      // falling Object at From shattered the glimmer at Pos
	EventCrystalFormed                         // a shattered glimmer left a crystal at Pos
	EventGlimmerDrowned                        // water reached the glimmer at Pos
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
	case EventFell:
		return "fell"
	case EventRolled:
		return "rolled"
	case EventLanded:
		return "landed"
	case EventBoulderPushed:
		return "boulder-pushed"
	case EventPlayerCrushed:
		return "player-crushed"
	case EventTooDeep:
		return "too-deep"
	case EventValveTurned:
		return "valve-turned"
	case EventGateOpened:
		return "gate-opened"
	case EventGateClosed:
		return "gate-closed"
	case EventGateJammed:
		return "gate-jammed"
	case EventCrawlerMoved:
		return "crawler-moved"
	case EventPlayerCaught:
		return "player-caught"
	case EventCrawlerCrushed:
		return "crawler-crushed"
	case EventCrawlerDrowned:
		return "crawler-drowned"
	case EventGlimmerMoved:
		return "glimmer-moved"
	case EventGlimmerShattered:
		return "glimmer-shattered"
	case EventCrystalFormed:
		return "crystal-formed"
	case EventGlimmerDrowned:
		return "glimmer-drowned"
	}
	return fmt.Sprintf("event(%d)", uint8(k))
}

// Event records one occurrence during the tick numbered Tick.
type Event struct {
	Tick uint64
	Kind EventKind
	Pos  Pos
	// From is the starting cell for movement events (player moves, falls,
	// rolls, pushes and creature moves), the crushing object's cell for
	// crushes and shatters, and the creature's cell for player-caught. It is
	// the zero Pos for other kinds.
	From Pos
	// Object is what fell, rolled, landed, was pushed, crushed or shattered
	// something (a boulder or a crystal), the creature that moved or caught
	// the player, or OccupantNone for other kinds.
	Object Occupant
}

func (e Event) String() string {
	switch e.Kind {
	case EventMoved:
		return fmt.Sprintf("t%d %s %s->%s", e.Tick, e.Kind, e.From, e.Pos)
	case EventFell, EventRolled, EventBoulderPushed, EventPlayerCrushed, EventCrawlerCrushed, EventGlimmerShattered,
		EventPlayerCaught:
		return fmt.Sprintf("t%d %s %s %s->%s", e.Tick, e.Kind, e.Object, e.From, e.Pos)
	case EventCrawlerMoved, EventGlimmerMoved:
		return fmt.Sprintf("t%d %s %s->%s", e.Tick, e.Kind, e.From, e.Pos)
	case EventLanded:
		return fmt.Sprintf("t%d %s %s %s", e.Tick, e.Kind, e.Object, e.Pos)
	}
	return fmt.Sprintf("t%d %s %s", e.Tick, e.Kind, e.Pos)
}
