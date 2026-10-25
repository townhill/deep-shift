package sfx

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func TestMusicLoopIsWholeBarsAndSilentAtTheSeam(t *testing.T) {
	if SamplesPerStep*MusicBPM*4 != SampleRate*60 {
		t.Fatalf("a step is not a whole number of samples at %d BPM", MusicBPM)
	}
	s := MusicSamples()
	if len(s) != MusicLength || MusicLength != 32*SampleRate {
		t.Fatalf("loop is %d samples (MusicLength %d), want 16 bars of 4/4 at 120 BPM: %d", len(s), MusicLength, 32*SampleRate)
	}
	// The loop restarts from its first sample straight after its last, so
	// both ends must be silent for the repeat not to click.
	if math.Abs(s[0]) > 1e-3 || math.Abs(s[len(s)-1]) > 1e-3 {
		t.Fatalf("loop starts at %.4f and ends at %.4f, want silence", s[0], s[len(s)-1])
	}
}

func TestMusicPeaksBelowTheCuesWithHeadroom(t *testing.T) {
	peak := 0.0
	for _, v := range MusicSamples() {
		peak = max(peak, math.Abs(v))
	}
	if math.Abs(peak-MusicPeak) > 1e-9 {
		t.Fatalf("music peaks at %.4f, want %.4f", peak, MusicPeak)
	}
	// The loudest cue peaks at Peak; under the music it must not clip.
	if MusicPeak >= Peak || MusicPeak+Peak > 1 {
		t.Fatalf("music peak %.2f with cue peak %.2f: want quieter than the cues and no clipping together", MusicPeak, Peak)
	}
}

func TestMusicIsDeterministicStereo(t *testing.T) {
	pcm := MusicPCM()
	if !bytes.Equal(pcm, MusicPCM()) {
		t.Fatal("music differs between generations")
	}
	if len(pcm) != 4*MusicLength {
		t.Fatalf("%d bytes, want 4 per sample (16-bit stereo)", len(pcm))
	}
}

func TestMusicNeverDropsOut(t *testing.T) {
	s := MusicSamples()
	for b := range MusicBars {
		bar := s[b*StepsPerBar*SamplesPerStep : (b+1)*StepsPerBar*SamplesPerStep]
		sum := 0.0
		for _, v := range bar {
			sum += v * v
		}
		if rms := math.Sqrt(sum / float64(len(bar))); rms < MusicPeak/20 {
			t.Errorf("bar %d is nearly silent: RMS %.4f", b+1, rms)
		}
	}
}

func TestTuneIsWellFormed(t *testing.T) {
	notes, err := leadNotes()
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := mustNote("E4"), mustNote("D6")
	for _, n := range notes {
		if n.note < lo || n.note > hi {
			t.Errorf("lead note %d at step %d is outside E4..D6", n.note, n.step)
		}
		if n.step+n.steps > MusicBars*StepsPerBar {
			t.Errorf("lead note at step %d runs past the loop", n.step)
		}
	}
	for _, b := range tune {
		if _, err := noteNumber(b.root); err != nil {
			t.Error(err)
		}
	}
	for _, d := range []string{beat, fill} {
		hits := strings.Fields(d)
		if len(hits) != StepsPerBar {
			t.Errorf("drum bar %q has %d steps, want %d", d, len(hits), StepsPerBar)
		}
		for _, h := range hits {
			if h != "k" && h != "s" && h != "h" && h != "." {
				t.Errorf("drum bar %q: unknown hit %q", d, h)
			}
		}
	}
}

func TestNoteNumbers(t *testing.T) {
	for name, want := range map[string]int{"C4": 60, "A4": 69, "G#5": 80, "E2": 40, "C#0": 13} {
		if got, err := noteNumber(name); err != nil || got != want {
			t.Errorf("noteNumber(%q) = %d, %v; want %d", name, got, err, want)
		}
	}
	for _, bad := range []string{"", "A", "H4", "A#", "Ab4", "A9", "A44"} {
		if _, err := noteNumber(bad); err == nil {
			t.Errorf("noteNumber(%q) accepted", bad)
		}
	}
	if math.Abs(pitch(69)-440) > 1e-9 || math.Abs(pitch(81)-880) > 1e-9 {
		t.Errorf("A4 = %.3f Hz, A5 = %.3f Hz", pitch(69), pitch(81))
	}
}

// The app generates the loop once at startup, before the window opens.
func BenchmarkMusicPCM(b *testing.B) {
	for b.Loop() {
		MusicPCM()
	}
}
