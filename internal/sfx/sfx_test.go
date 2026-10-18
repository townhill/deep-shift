package sfx

import (
	"bytes"
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/townhill/deep-shift/internal/sim"
)

func TestCuesAreDeterministicShortAndWithinHeadroom(t *testing.T) {
	for c := Cue(0); c < CueCount; c++ {
		s := Samples(c)
		if d := float64(len(s)) / SampleRate * 1000; d < 40 || d > 800 {
			t.Errorf("%s lasts %.0f ms, want 40..800", c, d)
		}
		peak := 0.0
		for _, v := range s {
			peak = max(peak, math.Abs(v))
		}
		if want := Peak * loudness[c]; math.Abs(peak-want) > 1e-9 || peak > Peak {
			t.Errorf("%s peaks at %.4f, want %.4f (never above %.2f)", c, peak, want, Peak)
		}
		// Envelopes start and end at silence, so cues never click.
		if math.Abs(s[0]) > 1e-3 || math.Abs(s[len(s)-1]) > 1e-3 {
			t.Errorf("%s starts at %.4f and ends at %.4f, want silence", c, s[0], s[len(s)-1])
		}
		if !bytes.Equal(PCM(c), PCM(c)) {
			t.Errorf("%s differs between generations", c)
		}
	}
}

func TestCuesAreDistinct(t *testing.T) {
	for a := Cue(0); a < CueCount; a++ {
		for b := a + 1; b < CueCount; b++ {
			if slices.Equal(Samples(a), Samples(b)) {
				t.Errorf("%s and %s sound identical", a, b)
			}
		}
	}
}

func TestPCMIsLittleEndianStereoInt16(t *testing.T) {
	s, pcm := Samples(CueCollect), PCM(CueCollect)
	if len(pcm) != 4*len(s) {
		t.Fatalf("%d bytes for %d samples, want 4 per sample", len(pcm), len(s))
	}
	for i, v := range s {
		l := int16(binary.LittleEndian.Uint16(pcm[4*i:]))
		r := int16(binary.LittleEndian.Uint16(pcm[4*i+2:]))
		if l != r || l != int16(math.Round(v*math.MaxInt16)) {
			t.Fatalf("sample %d: left %d right %d, want %d", i, l, r, int16(math.Round(v*math.MaxInt16)))
		}
	}
}

func TestForEventsMapsDedupesAndOrders(t *testing.T) {
	ev := func(k sim.EventKind) sim.Event { return sim.Event{Kind: k} }
	tests := []struct {
		name   string
		events []sim.Event
		want   []Cue
	}{
		{"quiet movement", []sim.Event{ev(sim.EventMoved), ev(sim.EventFell), ev(sim.EventRolled)}, nil},
		{"dig", []sim.Event{ev(sim.EventMoved), ev(sim.EventDug)}, []Cue{CueDig}},
		{"many landings, one sound", []sim.Event{ev(sim.EventLanded), ev(sim.EventLanded), ev(sim.EventLanded)}, []Cue{CueLand}},
		{"blocked three ways", []sim.Event{ev(sim.EventExitSealed), ev(sim.EventTooDeep), ev(sim.EventGateJammed)}, []Cue{CueBlocked}},
		{"valve and gates", []sim.Event{ev(sim.EventValveTurned), ev(sim.EventGateOpened), ev(sim.EventGateOpened)}, []Cue{CueValve, CueGate}},
		{"gates close too", []sim.Event{ev(sim.EventGateClosed)}, []Cue{CueGate}},
		{"most important first", []sim.Event{ev(sim.EventDug), ev(sim.EventCollected), ev(sim.EventExitOpened), ev(sim.EventLanded), ev(sim.EventPlayerCrushed)},
			[]Cue{CueCrush, CueExitOpen, CueCollect, CueLand, CueDig}},
		{"push and complete", []sim.Event{ev(sim.EventBoulderPushed), ev(sim.EventCaveComplete)}, []Cue{CueComplete, CuePush}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForEvents(tt.events); !slices.Equal(got, tt.want) {
				t.Fatalf("ForEvents = %v, want %v", got, tt.want)
			}
		})
	}
}
