package replay

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// fixtureDir holds the golden replays, shared with cavetool and CI.
var fixtureDir = filepath.Join("..", "..", "testdata", "replays")

func loadFixture(t *testing.T, name string) *Replay {
	t.Helper()
	path := filepath.Join(fixtureDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(path, data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// freshTestCave parses the embedded cave from bytes each time, so repeated
// runs share no state at all.
func freshTestCave(t *testing.T) *level.Level {
	t.Helper()
	l, err := level.Load(assets.Levels, assets.TestCave)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestGoldenFixtures(t *testing.T) {
	names, err := filepath.Glob(filepath.Join(fixtureDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 2 {
		t.Fatalf("found %d fixtures in %s, want at least 2", len(names), fixtureDir)
	}
	levels, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range names {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r := loadFixture(t, filepath.Base(path))
			if strings.TrimSpace(r.Description) == "" {
				t.Error("fixture has no human explanation in description")
			}
			l, ok := level.ByID(levels, r.CaveID)
			if !ok {
				t.Fatalf("no embedded cave %q", r.CaveID)
			}
			res, err := Run(r, l)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Check(res); err != nil {
				t.Fatalf("golden mismatch (do not regenerate blindly; see docs/PHYSICS.md):\n%v\nfinal state:\n%s", err, res.Sim.ASCII())
			}
		})
	}
}

// SPEC §11 Phase 1: the same cave, seed and commands give identical hashes
// across 100 separate runs. The trace hash makes this hold at every tick.
func TestReplayIsDeterministicAcross100Runs(t *testing.T) {
	r := loadFixture(t, "test_cave_complete.json")
	var first *Result
	for run := range 100 {
		res, err := Run(r, freshTestCave(t))
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = res
			continue
		}
		if res.FinalHash != first.FinalHash || res.TraceHash != first.TraceHash {
			t.Fatalf("run %d diverged: final %s trace %s, run 0 had final %s trace %s",
				run, res.FinalHash, res.TraceHash, first.FinalHash, first.TraceHash)
		}
	}
	if err := r.Check(first); err != nil {
		t.Fatalf("100 runs agreed but not with the golden expectation: %v", err)
	}
}

func TestExitFixtureProvesQuotaGate(t *testing.T) {
	r := loadFixture(t, "test_cave_sealed_exit.json")
	res, err := Run(r, freshTestCave(t))
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sim
	if res.Status != sim.StatusPlaying || s.ExitOpen() || res.Collected != s.Required()-1 {
		t.Fatalf("status %s, exit open %v, collected %d of %d", res.Status, s.ExitOpen(), res.Collected, s.Required())
	}
	if got, want := s.Player(), (sim.Pos{X: 4, Y: 1}); got != want {
		t.Fatalf("player at %s, want %s beside the exit", got, want)
	}
}

func TestDifferentSeedChangesHashes(t *testing.T) {
	r := loadFixture(t, "test_cave_complete.json")
	r.Seed++
	res, err := Run(r, freshTestCave(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalHash.String() == r.Expect.FinalHash {
		t.Fatal("seed is not part of the hashed state")
	}
}

func TestRunRejectsMismatchedContext(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Replay)
		want   string
	}{
		{"other cave", func(r *Replay) { r.CaveID = "other" }, `replay is for cave "other"`},
		{"cave edited", func(r *Replay) { r.CaveChecksum = strings.Repeat("0", 64) }, "has changed since the replay was recorded"},
		{"future rules", func(r *Replay) { r.RuleVersion = sim.RuleVersion + 1 },
			fmt.Sprintf("rule version %d; this build implements rule version %d", sim.RuleVersion+1, sim.RuleVersion)},
		{"older rules", func(r *Replay) { r.RuleVersion = 1 },
			fmt.Sprintf("rule version 1; this build implements rule version %d", sim.RuleVersion)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := loadFixture(t, "test_cave_complete.json")
			tt.mutate(r)
			_, err := Run(r, freshTestCave(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidReplays(t *testing.T) {
	valid := func() *Replay {
		return &Replay{
			FormatVersion: 1, CaveID: "test_cave", CaveChecksum: strings.Repeat("ab", 32),
			Seed: 1, RuleVersion: 1, Ticks: 10,
			Commands: []Command{{0, "right"}, {4, "down"}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*Replay)
		raw    string
		want   string
	}{
		{name: "unknown field", raw: `{"format_version": 1, "inputs": []}`, want: `unknown field "inputs"`},
		{name: "trailing data", raw: `{} {}`, want: "unexpected data after"},
		{name: "version", mutate: func(r *Replay) { r.FormatVersion = 9 }, want: "format_version 9 unsupported"},
		{name: "cave id", mutate: func(r *Replay) { r.CaveID = "" }, want: "cave_id is missing"},
		{name: "checksum", mutate: func(r *Replay) { r.CaveChecksum = "xyz" }, want: "not a 64-digit hex"},
		{name: "rule version", mutate: func(r *Replay) { r.RuleVersion = 0 }, want: "rule_version is missing"},
		{name: "zero ticks", mutate: func(r *Replay) { r.Ticks = 0 }, want: "ticks 0 out of range"},
		{name: "unknown action", mutate: func(r *Replay) { r.Commands[1].Action = "jump" }, want: `commands[1]: unknown action "jump"`},
		{name: "duplicate tick", mutate: func(r *Replay) { r.Commands[1].Tick = 0 }, want: "commands[1]: tick 0 does not follow tick 0"},
		{name: "out of order", mutate: func(r *Replay) { r.Commands = []Command{{5, "up"}, {2, "up"}} }, want: "does not follow tick 5"},
		{name: "beyond end", mutate: func(r *Replay) { r.Commands[1].Tick = 10 }, want: "tick 10 is not before ticks 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.raw)
			if tt.raw == "" {
				r := valid()
				tt.mutate(r)
				var err error
				if data, err = Encode(r); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Parse("r.json", data)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestEncodeRoundTripsAndMatchesFixtureLayout(t *testing.T) {
	path := filepath.Join(fixtureDir, "test_cave_complete.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(path, data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Encode(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("fixture is not in canonical Encode layout; re-encode it so diffs stay reviewable")
	}
	back, err := Parse("roundtrip", out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, r) {
		t.Fatal("Encode/Parse round trip changed the replay")
	}
}
