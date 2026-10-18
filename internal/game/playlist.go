package game

import (
	"fmt"

	"github.com/townhill/deep-shift/internal/level"
)

// Playlist plays a sequence of caves, one session at a time. N (Controls.Next)
// moves to the next cave from the pause, completion or death screen,
// wrapping after the last.
type Playlist struct {
	levels       []*level.Level
	index        int
	viewW, viewH int
	session      *Session
}

// NewPlaylist starts at levels[start].
func NewPlaylist(levels []*level.Level, start, viewW, viewH int) (*Playlist, error) {
	if len(levels) == 0 || start < 0 || start >= len(levels) {
		return nil, fmt.Errorf("playlist: start %d out of range for %d caves", start, len(levels))
	}
	s, err := NewSession(levels[start], viewW, viewH)
	if err != nil {
		return nil, err
	}
	return &Playlist{levels: levels, index: start, viewW: viewW, viewH: viewH, session: s}, nil
}

// Update forwards c to the current session, or switches cave on Next.
func (p *Playlist) Update(c Controls) {
	if c.Next && (p.session.Paused() || p.session.Ended()) {
		next := (p.index + 1) % len(p.levels)
		s, err := NewSession(p.levels[next], p.viewW, p.viewH)
		if err != nil {
			p.session.Notify(fmt.Sprintf("Cannot open %s: %v", p.levels[next].ID, err))
			return
		}
		p.index, p.session = next, s
		return
	}
	p.session.Update(c)
}

// Session returns the current cave's session.
func (p *Playlist) Session() *Session { return p.session }

// Index returns the position of the current cave in the playlist.
func (p *Playlist) Index() int { return p.index }

// OrderLevels returns levels arranged by ids, and fails if an id is missing
// or a level is not listed, so a new cave cannot be silently left out.
func OrderLevels(levels []*level.Level, ids []string) ([]*level.Level, error) {
	if len(levels) != len(ids) {
		return nil, fmt.Errorf("play order lists %d caves but %d are embedded", len(ids), len(levels))
	}
	ordered := make([]*level.Level, 0, len(ids))
	for _, id := range ids {
		l, ok := level.ByID(levels, id)
		if !ok {
			return nil, fmt.Errorf("play order names %q, which is not an embedded cave", id)
		}
		ordered = append(ordered, l)
	}
	return ordered, nil
}

// IndexOf returns the position of the cave with the given id.
func IndexOf(levels []*level.Level, id string) (int, bool) {
	for i, l := range levels {
		if l.ID == id {
			return i, true
		}
	}
	return 0, false
}
