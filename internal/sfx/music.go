package sfx

import (
	"fmt"
	"math"
	"strings"
)

// The music is one original looping tune in the style of an early arcade
// sound chip: a pulse-wave lead, a fast pulse arpeggio, a triangle bass and
// noise drums. It is synthesised like the cues, so there is no audio file to
// license and the loop always gives the same bytes. It is quieter than the
// cues, so it never masks one the player relies on.
const (
	// MusicBPM is the tempo, in quarter notes per minute.
	MusicBPM = 120
	// StepsPerBar is the sequencer's resolution: sixteenth notes in 4/4.
	StepsPerBar = 16
	// MusicBars is the length of the loop.
	MusicBars = 16
	// SamplesPerStep is one sixteenth note, a whole number of samples, so
	// the loop ends exactly on a bar line.
	SamplesPerStep = SampleRate * 60 / MusicBPM / 4
	// MusicLength is the loop's length in samples (32 s).
	MusicLength = MusicBars * StepsPerBar * SamplesPerStep
	// MusicPeak is the loop's largest sample magnitude, half a cue's, so
	// the music under the loudest cue stays below full scale.
	MusicPeak = Peak / 2
)

// bar is one bar of the tune: the chord under it, written as its root in the
// bass octave, and the lead's sixteen steps. A lead step is a note that
// starts there (A4, G#5), "." to hold the note before, or "-" for silence.
type bar struct {
	root  string
	minor bool
	lead  string
}

// tune is in A minor, curious rather than grim (SPEC §2): an eight-bar theme
// over Am F C G, Am F G E, then a syncopated answer over Dm Am F C, Dm Am E E
// whose last bar leads back into the first.
var tune = [MusicBars]bar{
	{"A2", true, "A4 . . . C5 . E5 . A5 . . . G5 . E5 ."},
	{"F2", false, "F5 . . . E5 . D5 . C5 . . . A4 . . ."},
	{"C3", false, "G4 . C5 . E5 . G5 . . . E5 . C5 . . ."},
	{"G2", false, "D5 . . . B4 . D5 . G5 . . . F5 . D5 ."},
	{"A2", true, "E5 . . . A5 . . . C6 . B5 . A5 . E5 ."},
	{"F2", false, "F5 . . . A5 . . . C6 . . . A5 . F5 ."},
	{"G2", false, "G5 . . . F5 . E5 . D5 . . . B4 . . ."},
	{"E2", false, "G#4 . B4 . E5 . G#5 . . . . . . . - -"},
	{"D3", true, "D5 . . F5 . . A5 . . . G5 . F5 . E5 ."},
	{"A2", true, "E5 . . C5 . . A4 . . . C5 . E5 . . ."},
	{"F2", false, "F5 . . A5 . . C6 . . . A5 . G5 . F5 ."},
	{"C3", false, "E5 . . . . . . . - - G5 . E5 . C5 ."},
	{"D3", true, "D5 . F5 . A5 . D6 . . . C6 . A5 . F5 ."},
	{"A2", true, "E5 . . . A5 . . . C6 . B5 . A5 . . ."},
	{"E2", false, "B5 . . . G#5 . . . E5 . . . D5 . B4 ."},
	{"E2", false, "B4 . . . . . . . D5 . C5 . B4 . G#4 ."},
}

// Drum bars: k kick, s snare, h hi-hat, "." nothing. The fill ends every
// eighth bar; its last hit is early enough to die away before the loop ends.
const (
	beat = "k . h . s . h . k . h k s . h ."
	fill = "k . h . s . h . k . s . s s s ."
)

// Voice levels before the loop is normalised to MusicPeak.
const (
	leadLevel  = 0.3
	arpLevel   = 0.1
	bassLevel  = 0.4
	kickLevel  = 0.55
	snareLevel = 0.25
	hatLevel   = 0.08
)

// MusicPCM returns the loop as 16-bit little-endian stereo samples at
// SampleRate, the format internal/audio plays.
func MusicPCM() []byte {
	return encode(MusicSamples())
}

// MusicSamples returns the loop as MusicLength mono samples whose largest
// magnitude is MusicPeak. Every note ends in silence within its own steps,
// so the loop starts and ends silent and repeats without a click.
func MusicSamples() []float64 {
	buf := make([]float64, MusicLength)
	lead, err := leadNotes()
	if err != nil {
		panic(err) // the tune is a constant; tests parse it
	}
	for _, n := range lead {
		pulse(buf, n.step*SamplesPerStep, n.steps*SamplesPerStep, pitch(n.note), 0.25, leadLevel, true)
	}
	for i, b := range tune {
		root := mustNote(b.root)
		third := 4
		if b.minor {
			third = 3
		}
		chord := [4]int{root + 12, root + 12 + third, root + 19, root + 24}
		drums := beat
		if i%8 == 7 {
			drums = fill
		}
		hits := strings.Fields(drums)
		for s := range StepsPerBar {
			at := (i*StepsPerBar + s) * SamplesPerStep
			// A rising arpeggio, one chord tone per step.
			pulse(buf, at, SamplesPerStep, pitch(chord[s%4]), 0.125, arpLevel, false)
			// An octave-jumping bass in eighth notes.
			if s%2 == 0 {
				triangle(buf, at, 2*SamplesPerStep, pitch(root+12*(s/2%2)), bassLevel)
			}
			switch hits[s] {
			case "k":
				add(buf, at, sweep(150, 45, 130, kickLevel, 22))
			case "s":
				add(buf, at, mix(140, noise(140, snareLevel, 0.5, 30, 83), sine(185, 80, snareLevel*0.6, 35)))
			case "h":
				add(buf, at, noise(30, hatLevel, 1, 90, 89))
			}
		}
	}
	return normalise(buf, MusicPeak)
}

// note is one lead note: its MIDI number, the step it starts on and how many
// steps it lasts.
type note struct {
	note, step, steps int
}

// leadNotes reads the lead's steps, all bars in order, into notes.
func leadNotes() ([]note, error) {
	var notes []note
	held := false
	for i, b := range tune {
		steps := strings.Fields(b.lead)
		if len(steps) != StepsPerBar {
			return nil, fmt.Errorf("bar %d: %d lead steps, want %d", i+1, len(steps), StepsPerBar)
		}
		for s, tok := range steps {
			switch tok {
			case ".":
				if !held {
					return nil, fmt.Errorf("bar %d step %d: nothing to hold", i+1, s+1)
				}
				notes[len(notes)-1].steps++
			case "-":
				held = false
			default:
				n, err := noteNumber(tok)
				if err != nil {
					return nil, fmt.Errorf("bar %d step %d: %w", i+1, s+1, err)
				}
				notes = append(notes, note{note: n, step: i*StepsPerBar + s, steps: 1})
				held = true
			}
		}
	}
	return notes, nil
}

// noteNumber returns the MIDI number of a note name such as A4 or G#5,
// where C4 is 60 and A4 is 440 Hz.
func noteNumber(name string) (int, error) {
	semitone := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	if len(name) < 2 {
		return 0, fmt.Errorf("note %q: want a letter and an octave, like A4", name)
	}
	n, ok := semitone[name[0]]
	rest := name[1:]
	if strings.HasPrefix(rest, "#") {
		n, rest = n+1, rest[1:]
	}
	if !ok || len(rest) != 1 || rest[0] < '0' || rest[0] > '8' {
		return 0, fmt.Errorf("note %q: want a letter, an optional #, and an octave 0-8", name)
	}
	return 12*(int(rest[0]-'0')+1) + n, nil
}

func mustNote(name string) int {
	n, err := noteNumber(name)
	if err != nil {
		panic(err)
	}
	return n
}

// pitch is a MIDI note's frequency in equal temperament.
func pitch(n int) float64 {
	return 440 * math.Pow(2, float64(n-69)/12)
}

// gate is a held note's envelope at sample i of n: a 3 ms attack, a decay
// towards sustain, and a 10 ms release that reaches silence on the note's
// last sample. fall is how far the decay has gone, from 1 down towards 0;
// callers multiply it by perSample(decay) each sample, which is much faster
// than an exponential per sample.
func gate(i, n int, fall, sustain float64) float64 {
	a := min(1, float64(i)/float64(ms(3)))
	r := min(1, float64(n-1-i)/float64(ms(10)))
	return a * r * (sustain + (1-sustain)*fall)
}

// perSample is the factor an exponential decay at rate per second falls by
// in one sample.
func perSample(decay float64) float64 { return math.Exp(-decay / SampleRate) }

// advance moves a phase in cycles on by step, wrapping into [0, 1).
func advance(phase, step float64) float64 {
	if phase += step; phase >= 1 {
		phase--
	}
	return phase
}

// pulse adds a pulse wave of n samples at buf[at:], high for duty of each
// cycle. PolyBLEP rounds its edges so high notes do not alias, and the
// offset is removed so a note does not thump. A lead note gains a gentle
// vibrato after 150 ms.
func pulse(buf []float64, at, n int, freq, duty, amp float64, vibrato bool) {
	dt := freq / SampleRate
	phase, fall, k := 0.0, 1.0, perSample(3)
	for i := range n {
		v, fallen := -1.0, phase-duty
		if fallen < 0 {
			v, fallen = 1, fallen+1
		}
		v += blep(phase, dt) - blep(fallen, dt)
		buf[at+i] += amp * gate(i, n, fall, 0.6) * (v - (2*duty - 1))
		fall *= k
		step := dt
		if t := float64(i) / SampleRate; vibrato && t > 0.15 {
			depth := 0.004 * min(1, (t-0.15)/0.1)
			step *= 1 + depth*math.Sin(2*math.Pi*5.5*t)
		}
		phase = advance(phase, step)
	}
}

// blep is the PolyBLEP correction for a unit step at phase 0, for a wave
// advancing dt cycles per sample.
func blep(t, dt float64) float64 {
	switch {
	case t < dt:
		t /= dt
		return t + t - t*t - 1
	case t > 1-dt:
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

// triangle adds a plucked triangle-wave bass note of n samples at buf[at:].
func triangle(buf []float64, at, n int, freq, amp float64) {
	phase, fall, k := 0.25, 1.0, perSample(10) // start at a zero crossing
	for i := range n {
		buf[at+i] += amp * gate(i, n, fall, 0.3) * (4*math.Abs(phase-0.5) - 1)
		fall *= k
		phase = advance(phase, freq/SampleRate)
	}
}

// add mixes part into buf from at, cut off at buf's end.
func add(buf []float64, at int, part []float64) {
	for i := range min(len(part), len(buf)-at) {
		buf[at+i] += part[i]
	}
}
