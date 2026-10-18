// Package sfx synthesises Deep Shift's original sound cues in code and maps
// simulation events to them.
//
// Every cue is generated from oscillators, deterministic noise and
// envelopes, so there are no audio files to license and the sounds are
// reproducible. The package is pure Go (no audio device): internal/audio
// plays the PCM it produces. Sound is informative but never required
// (SPEC §2): every cue repeats something the screen already shows.
package sfx

import (
	"encoding/binary"
	"math"

	"github.com/townhill/deep-shift/internal/sim"
)

// SampleRate is the rate cues are generated at, in samples per second.
const SampleRate = 48000

// Peak is the largest sample magnitude any cue reaches, as a fraction of
// full scale, leaving headroom when several cues overlap.
const Peak = 0.5

// loudness scales each cue's peak relative to Peak: rare, important events
// are louder than frequent ones such as digging.
var loudness = [CueCount]float64{
	CueCrush: 1, CueComplete: 0.9, CueExitOpen: 0.8, CueCollect: 0.7, CueValve: 0.8,
	CueGate: 0.6, CueBlocked: 0.5, CueLand: 0.6, CuePush: 0.5, CueDig: 0.35,
}

// Cue identifies one sound.
type Cue int

// Cues, in the order ForEvents reports them.
const (
	CueCrush    Cue = iota // a falling object landed on the player
	CueComplete            // the cave is complete
	CueExitOpen            // the quota is met
	CueCollect             // a crystal was collected
	CueValve               // a valve turned
	CueGate                // gates moved
	CueBlocked             // a refused move or action: sealed exit, deep water, jammed gate
	CueLand                // a falling object came to rest
	CuePush                // a boulder was pushed
	CueDig                 // earth was dug
	CueCount
)

func (c Cue) String() string {
	return [...]string{"crush", "complete", "exit-open", "collect", "valve", "gate", "blocked", "land", "push", "dig"}[c]
}

// ForEvents returns the cues for one tick's events: each cue at most once,
// in Cue order, so ten boulders landing together make one sound. Movement,
// falling and rolling make none: they happen too often to be informative.
func ForEvents(events []sim.Event) []Cue {
	var want [CueCount]bool
	for _, e := range events {
		switch e.Kind {
		case sim.EventPlayerCrushed:
			want[CueCrush] = true
		case sim.EventCaveComplete:
			want[CueComplete] = true
		case sim.EventExitOpened:
			want[CueExitOpen] = true
		case sim.EventCollected:
			want[CueCollect] = true
		case sim.EventValveTurned:
			want[CueValve] = true
		case sim.EventGateOpened, sim.EventGateClosed:
			want[CueGate] = true
		case sim.EventExitSealed, sim.EventTooDeep, sim.EventGateJammed:
			want[CueBlocked] = true
		case sim.EventLanded:
			want[CueLand] = true
		case sim.EventBoulderPushed:
			want[CuePush] = true
		case sim.EventDug:
			want[CueDig] = true
		}
	}
	var cues []Cue
	for c, w := range want {
		if w {
			cues = append(cues, Cue(c))
		}
	}
	return cues
}

// PCM returns cue c as 16-bit little-endian stereo samples at SampleRate,
// the format internal/audio plays. The same cue always gives the same bytes.
func PCM(c Cue) []byte {
	return encode(Samples(c))
}

// Samples returns cue c as mono samples whose largest magnitude is Peak
// scaled by the cue's loudness.
func Samples(c Cue) []float64 {
	var buf []float64
	switch c {
	case CueCrush:
		buf = mix(400, sweep(70, 28, 400, 0.9, 6), noise(120, 0.7, 0.15, 30, 11))
	case CueComplete:
		buf = concat(tone(523.25, 90, 0.5, 10), tone(659.25, 90, 0.5, 10), tone(783.99, 90, 0.5, 10),
			vibrato(1046.5, 420, 0.6, 5))
	case CueExitOpen:
		buf = concat(tone(523.25, 70, 0.4, 14), tone(659.25, 70, 0.4, 14), tone(783.99, 70, 0.4, 14), tone(1046.5, 160, 0.45, 8))
	case CueCollect:
		buf = concat(tone(1318.5, 55, 0.45, 20), tone(1975.5, 110, 0.4, 14))
	case CueValve:
		buf = mix(260, sine(1210, 260, 0.35, 16), sine(1747, 220, 0.25, 22), sweep(140, 90, 90, 0.5, 30))
	case CueGate:
		buf = mix(420, square(55, 420, 0.25, 5), noise(420, 0.25, 0.25, 5, 23))
	case CueBlocked:
		buf = concat(squareTone(196, 70, 0.22), squareTone(147, 110, 0.22))
	case CueLand:
		buf = mix(140, sweep(110, 55, 140, 0.6, 28), noise(40, 0.35, 0.2, 60, 37))
	case CuePush:
		buf = noise(200, 0.4, 0.06, 10, 41)
	case CueDig:
		buf = noise(70, 0.3, 0.3, 45, 53)
	}
	return normalise(buf, Peak*loudness[c])
}

func ms(n float64) int { return int(n * SampleRate / 1000) }

// env is a short linear attack then exponential decay at rate per second.
func env(i, n int, decay float64) float64 {
	t := float64(i) / SampleRate
	a := min(1, float64(i)/float64(ms(4)))     // 4 ms attack: no click at the start
	r := min(1, float64(n-1-i)/float64(ms(6))) // 6 ms release: no click at the end
	return a * r * math.Exp(-decay*t)
}

func sine(freq, durMs, amp, decay float64) []float64 {
	n := ms(durMs)
	out := make([]float64, n)
	for i := range out {
		out[i] = amp * env(i, n, decay) * math.Sin(2*math.Pi*freq*float64(i)/SampleRate)
	}
	return out
}

// tone is a sine with its first two harmonics, which reads as a bell.
func tone(freq, durMs, amp, decay float64) []float64 {
	return mix(durMs, sine(freq, durMs, amp, decay), sine(2*freq, durMs, amp/3, decay*1.5), sine(3*freq, durMs, amp/6, decay*2))
}

func vibrato(freq, durMs, amp, decay float64) []float64 {
	n := ms(durMs)
	out := make([]float64, n)
	phase := 0.0
	for i := range out {
		t := float64(i) / SampleRate
		phase += 2 * math.Pi * freq * (1 + 0.006*math.Sin(2*math.Pi*6*t)) / SampleRate
		out[i] = amp * env(i, n, decay) * (math.Sin(phase) + math.Sin(2*phase)/3)
	}
	return out
}

// sweep is a sine whose pitch glides from f0 to f1: thuds and booms.
func sweep(f0, f1, durMs, amp, decay float64) []float64 {
	n := ms(durMs)
	out := make([]float64, n)
	phase := 0.0
	for i := range out {
		f := f0 + (f1-f0)*float64(i)/float64(n)
		phase += 2 * math.Pi * f / SampleRate
		out[i] = amp * env(i, n, decay) * math.Sin(phase)
	}
	return out
}

func square(freq, durMs, amp, decay float64) []float64 {
	n := ms(durMs)
	out := make([]float64, n)
	for i := range out {
		v := 1.0
		if math.Mod(freq*float64(i)/SampleRate, 1) >= 0.5 {
			v = -1
		}
		out[i] = amp * env(i, n, decay) * v
	}
	return out
}

// squareTone is a softened square: a sine plus its third harmonic.
func squareTone(freq, durMs, amp float64) []float64 {
	return mix(durMs, sine(freq, durMs, amp, 8), sine(3*freq, durMs, amp/3, 8))
}

// noise is low-passed white noise from a fixed seed. smooth in (0,1]: lower
// is darker.
func noise(durMs, amp, smooth, decay float64, seed uint64) []float64 {
	n := ms(durMs)
	out := make([]float64, n)
	rng := sim.NewSplitMix64(seed)
	lp := 0.0
	for i := range out {
		white := float64(rng.Next()>>11)/float64(1<<53)*2 - 1
		lp += smooth * (white - lp)
		out[i] = amp * env(i, n, decay) * lp / math.Sqrt(smooth)
	}
	return out
}

func mix(durMs float64, parts ...[]float64) []float64 {
	out := make([]float64, ms(durMs))
	for _, p := range parts {
		for i := range min(len(p), len(out)) {
			out[i] += p[i]
		}
	}
	return out
}

func concat(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// normalise scales buf so its largest magnitude is exactly peak.
func normalise(buf []float64, peak float64) []float64 {
	m := 0.0
	for _, v := range buf {
		m = max(m, math.Abs(v))
	}
	if m == 0 {
		return buf
	}
	for i := range buf {
		buf[i] *= peak / m
	}
	return buf
}

func encode(samples []float64) []byte {
	out := make([]byte, 0, 4*len(samples))
	for _, v := range samples {
		s := uint16(int16(math.Round(v * math.MaxInt16)))
		out = binary.LittleEndian.AppendUint16(out, s) // left
		out = binary.LittleEndian.AppendUint16(out, s) // right
	}
	return out
}
