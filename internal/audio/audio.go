// Package audio plays sfx cues through Ebitengine's audio package. Like
// internal/render it only presents: it never affects the simulation.
//
// Ebitengine creates the audio device on the first update. On macOS it uses
// AudioToolbox and on Linux ALSA or PulseAudio, all without cgo, so this
// package adds no build dependencies. Its output has not been heard on a
// Mac yet: compile-only, pending the Mac playtest.
package audio

import (
	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/townhill/deep-shift/internal/sfx"
)

// Player plays sound cues. A nil *Player is valid and silent, for running
// without an audio device.
type Player struct {
	players [sfx.CueCount]*audio.Player
	muted   bool
}

// New creates the process's audio context and one player per cue. Call it at
// most once.
func New() *Player {
	ctx := audio.NewContext(sfx.SampleRate)
	p := &Player{}
	for c := sfx.Cue(0); c < sfx.CueCount; c++ {
		p.players[c] = ctx.NewPlayerFromBytes(sfx.PCM(c))
	}
	return p
}

// Play starts cue c from the beginning unless muted. A cue that is still
// sounding restarts, so a burst of the same event never piles up.
func (p *Player) Play(c sfx.Cue) {
	if p == nil || p.muted {
		return
	}
	pl := p.players[c]
	if err := pl.Rewind(); err != nil {
		return
	}
	pl.Play()
}

// SetMuted turns sound off (stopping anything playing) or back on.
func (p *Player) SetMuted(muted bool) {
	if p == nil {
		return
	}
	p.muted = muted
	if muted {
		for _, pl := range p.players {
			pl.Pause()
		}
	}
}

// Muted reports whether sound is off. A nil Player is always muted.
func (p *Player) Muted() bool { return p == nil || p.muted }
