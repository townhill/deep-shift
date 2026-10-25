package game

// noAudio is the notice for a sound key pressed with no audio device.
const noAudio = "No sound: started with -no-audio"

// Sound is the player's sound settings: M turns all sound off and on, and B
// the music alone. Like the HUD it is pure Go, so the rules and their
// notices are tested without an audio device; the app applies them through
// internal/audio. Settings last for this run only (not saved yet).
type Sound struct {
	available bool // false with -no-audio: there is nothing to play on
	muted     bool
	music     bool
}

// NewSound starts with sound on and the music on or off. available is false
// when the app runs without an audio device.
func NewSound(available, music bool) *Sound {
	return &Sound{available: available, music: music}
}

// ToggleMute turns all sound, music included, off or back on (M) and
// returns the notice to show.
func (s *Sound) ToggleMute() string {
	if !s.available {
		return noAudio
	}
	s.muted = !s.muted
	if s.muted {
		return "Sound off (M: on)"
	}
	return "Sound on"
}

// ToggleMusic turns the music off or back on (B) and returns the notice to
// show. Music turned on while sound is off waits for M.
func (s *Sound) ToggleMusic() string {
	if !s.available {
		return noAudio
	}
	s.music = !s.music
	switch {
	case !s.music:
		return "Music off (B: on)"
	case s.muted:
		return "Music on (sound is off: M)"
	}
	return "Music on"
}

// Muted reports whether sound is off: muted with M, or no audio device.
func (s *Sound) Muted() bool { return !s.available || s.muted }

// MusicPlaying reports whether the music should be heard: sound and music
// are on and no cave is paused. A paused cave is silent, music included, and
// the music resumes where it stopped.
func (s *Sound) MusicPlaying(cavePaused bool) bool {
	return !s.Muted() && s.music && !cavePaused
}
