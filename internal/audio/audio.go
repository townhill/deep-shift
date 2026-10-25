// Package audio plays sfx cues and the looping music through Ebitengine's
// audio package. Like internal/render it only presents: it never affects the
// simulation.
//
// Ebitengine creates the audio device on the first update. On macOS it uses
// AudioToolbox and on Linux ALSA or PulseAudio, all without cgo, so this
// package adds no build dependencies. Its output has not been heard on a
// Mac yet: compile-only, pending the Mac playtest.
package audio

import (
	"bytes"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/townhill/deep-shift/internal/sfx"
)

// Player plays sound cues and the music. A nil *Player is valid and silent,
// for running without an audio device.
type Player struct {
	players [sfx.CueCount]*audio.Player
	music   *audio.Player
	muted   bool
	musicOn bool
}

// New creates the process's audio context, one player per cue and the
// music, which loops forever and starts paused. Call it at most once.
func New() *Player {
	ctx := audio.NewContext(sfx.SampleRate)
	p := &Player{}
	for c := sfx.Cue(0); c < sfx.CueCount; c++ {
		p.players[c] = ctx.NewPlayerFromBytes(sfx.PCM(c))
	}
	pcm := sfx.MusicPCM()
	music, err := ctx.NewPlayer(audio.NewInfiniteLoop(bytes.NewReader(pcm), int64(len(pcm))))
	if err != nil {
		panic(err) // never for an in-memory stream, as NewPlayerFromBytes assumes
	}
	p.music = music
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

// SetMuted turns the cues off (stopping any playing) or back on. The music
// has its own switch, SetMusic.
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

// SetMusic plays or pauses the music. Paused music resumes where it stopped.
func (p *Player) SetMusic(on bool) {
	if p == nil || on == p.musicOn {
		return
	}
	p.musicOn = on
	if on {
		p.music.Play()
	} else {
		p.music.Pause()
	}
}
