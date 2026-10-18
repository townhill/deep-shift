// Package replay records and re-runs tick-stamped player input.
//
// A replay pins the full determinism tuple (SPEC §5): the cave (by id and
// gameplay checksum), the seed, the rule version and the ordered commands.
// Re-running it must reproduce the recorded hashes exactly; golden fixtures
// live in testdata/replays/ at the repository root.
//
// Format version 1:
//
//	{
//	  "format_version": 1,
//	  "description": "Why this fixture exists and what it demonstrates.",
//	  "cave_id": "test_cave",
//	  "cave_checksum": "<level.Checksum()>",
//	  "seed": 20261017,
//	  "rule_version": 1,
//	  "ticks": 600,                          // ticks to simulate
//	  "commands": [
//	    {"tick": 0, "action": "right"},      // strictly increasing ticks < ticks
//	    ...
//	  ],
//	  "expect": {                            // optional golden results
//	    "final_tick": 590, "status": "complete", "collected": 8,
//	    "final_hash": "<hex>", "trace_hash": "<hex>"
//	  }
//	}
//
// A command is the Move input for exactly one tick; ticks without a command
// have no input. Commands given while the movement window is closed are
// ignored by the rules, deterministically.
package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/sim"
)

// FormatVersion is the only replay file version this build reads.
const FormatVersion = 1

// MaxTicks bounds a replay's length (about 9.7 days of play at 20 TPS).
const MaxTicks = 1 << 24

// Replay is a parsed replay file.
type Replay struct {
	FormatVersion int       `json:"format_version"`
	Description   string    `json:"description"`
	CaveID        string    `json:"cave_id"`
	CaveChecksum  string    `json:"cave_checksum"`
	Seed          uint64    `json:"seed"`
	RuleVersion   uint32    `json:"rule_version"`
	Ticks         uint64    `json:"ticks"`
	Commands      []Command `json:"commands"`
	Expect        *Expect   `json:"expect,omitempty"`
}

// Command is the move input for one tick.
type Command struct {
	Tick   uint64 `json:"tick"`
	Action string `json:"action"` // "up", "down", "left" or "right"
}

// Expect holds golden results. Fixture changes need review and a reason;
// never regenerate them wholesale after a rules refactor.
type Expect struct {
	FinalTick uint64 `json:"final_tick"`
	Status    string `json:"status"`
	Collected int    `json:"collected"`
	FinalHash string `json:"final_hash"`
	TraceHash string `json:"trace_hash"`
}

// Result is the outcome of running a replay.
type Result struct {
	FinalTick uint64
	Status    sim.Status
	Collected int
	FinalHash sim.Hash
	// TraceHash is SHA-256 over the state hash at tick 0 followed by the
	// state hash after every simulated tick, so two runs with equal trace
	// hashes agreed at every tick, not just at the end.
	TraceHash sim.Hash
	Sim       *sim.Sim // final state, for inspection
}

// Parse decodes and validates a replay file.
func Parse(source string, data []byte) (*Replay, error) {
	var r Replay
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s: unexpected data after the top-level JSON object", source)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return &r, nil
}

// Validate checks the replay's internal consistency (not its cave).
func (r *Replay) Validate() error {
	var errs []error
	if r.FormatVersion != FormatVersion {
		errs = append(errs, fmt.Errorf("format_version %d unsupported; this build reads %d", r.FormatVersion, FormatVersion))
	}
	if r.CaveID == "" {
		errs = append(errs, errors.New("cave_id is missing"))
	}
	if b, err := hex.DecodeString(r.CaveChecksum); err != nil || len(b) != sha256.Size {
		errs = append(errs, fmt.Errorf("cave_checksum %q is not a 64-digit hex SHA-256", r.CaveChecksum))
	}
	if r.Seed > level.MaxSeed {
		errs = append(errs, fmt.Errorf("seed %d exceeds %d", r.Seed, uint64(level.MaxSeed)))
	}
	if r.RuleVersion == 0 {
		errs = append(errs, errors.New("rule_version is missing"))
	}
	if r.Ticks == 0 || r.Ticks > MaxTicks {
		errs = append(errs, fmt.Errorf("ticks %d out of range 1..%d", r.Ticks, MaxTicks))
	}
	for i, c := range r.Commands {
		if _, ok := sim.ParseDir(c.Action); !ok {
			errs = append(errs, fmt.Errorf("commands[%d]: unknown action %q (want up, down, left or right)", i, c.Action))
		}
		if c.Tick >= r.Ticks {
			errs = append(errs, fmt.Errorf("commands[%d]: tick %d is not before ticks %d", i, c.Tick, r.Ticks))
		}
		if i > 0 && c.Tick <= r.Commands[i-1].Tick {
			errs = append(errs, fmt.Errorf("commands[%d]: tick %d does not follow tick %d; ticks must strictly increase", i, c.Tick, r.Commands[i-1].Tick))
		}
	}
	return errors.Join(errs...)
}

// Run simulates r against l, which must be the cave the replay was
// recorded on.
func Run(r *Replay, l *level.Level) (*Result, error) { return RunObserved(r, l, nil) }

// RunObserved is Run with an optional observer called after every simulated
// tick with that tick's number, its events and the simulation. The observer
// must not mutate the simulation.
func RunObserved(r *Replay, l *level.Level, observe func(tick uint64, events []sim.Event, s *sim.Sim)) (*Result, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if r.CaveID != l.ID {
		return nil, fmt.Errorf("replay is for cave %q, not %q", r.CaveID, l.ID)
	}
	if sum := l.Checksum(); r.CaveChecksum != sum {
		return nil, fmt.Errorf("cave %q has changed since the replay was recorded (checksum %s, replay expects %s)", l.ID, sum, r.CaveChecksum)
	}
	if r.RuleVersion != sim.RuleVersion {
		return nil, fmt.Errorf("replay was recorded with rule version %d; this build implements rule version %d", r.RuleVersion, sim.RuleVersion)
	}
	s, err := sim.New(l.Cave(), r.Seed)
	if err != nil {
		return nil, err
	}

	trace := sha256.New()
	h := s.StateHash()
	trace.Write(h[:])
	next := 0
	for tick := uint64(0); tick < r.Ticks; tick++ {
		var in sim.Input
		if next < len(r.Commands) && r.Commands[next].Tick == tick {
			in.Move, _ = sim.ParseDir(r.Commands[next].Action)
			next++
		}
		events := s.Step(in)
		h = s.StateHash()
		trace.Write(h[:])
		if observe != nil {
			observe(tick, events, s)
		}
	}

	res := &Result{
		FinalTick: s.Tick(),
		Status:    s.Status(),
		Collected: s.Collected(),
		FinalHash: h,
		Sim:       s,
	}
	copy(res.TraceHash[:], trace.Sum(nil))
	return res, nil
}

// Check compares res with the replay's expectations.
func (r *Replay) Check(res *Result) error {
	if r.Expect == nil {
		return errors.New("replay has no expect block")
	}
	e := r.Expect
	var errs []error
	mismatch := func(field string, got, want any) {
		errs = append(errs, fmt.Errorf("%s = %v, want %v", field, got, want))
	}
	if res.FinalTick != e.FinalTick {
		mismatch("final_tick", res.FinalTick, e.FinalTick)
	}
	if res.Status.String() != e.Status {
		mismatch("status", res.Status, e.Status)
	}
	if res.Collected != e.Collected {
		mismatch("collected", res.Collected, e.Collected)
	}
	if got := res.FinalHash.String(); got != e.FinalHash {
		mismatch("final_hash", got, e.FinalHash)
	}
	if got := res.TraceHash.String(); got != e.TraceHash {
		mismatch("trace_hash", got, e.TraceHash)
	}
	return errors.Join(errs...)
}

// ExpectFrom returns the expectation block that res satisfies, for authoring
// a new fixture. Review the result before committing it.
func ExpectFrom(res *Result) *Expect {
	return &Expect{
		FinalTick: res.FinalTick,
		Status:    res.Status.String(),
		Collected: res.Collected,
		FinalHash: res.FinalHash.String(),
		TraceHash: res.TraceHash.String(),
	}
}

// Encode writes r in the canonical fixture layout: indented JSON with one
// command per line, so diffs of fixtures stay reviewable.
func Encode(r *Replay) ([]byte, error) {
	var b bytes.Buffer
	field := func(name string, v any, last bool) error {
		val, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "  %q: %s", name, val)
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
		return nil
	}
	b.WriteString("{\n")
	for _, f := range []struct {
		name string
		v    any
	}{
		{"format_version", r.FormatVersion},
		{"description", r.Description},
		{"cave_id", r.CaveID},
		{"cave_checksum", r.CaveChecksum},
		{"seed", r.Seed},
		{"rule_version", r.RuleVersion},
		{"ticks", r.Ticks},
	} {
		if err := field(f.name, f.v, false); err != nil {
			return nil, err
		}
	}
	b.WriteString("  \"commands\": [")
	for i, c := range r.Commands {
		val, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n    ")
		b.Write(val)
	}
	if len(r.Commands) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteByte(']')
	if r.Expect != nil {
		val, err := json.MarshalIndent(r.Expect, "  ", "  ")
		if err != nil {
			return nil, err
		}
		b.WriteString(",\n  \"expect\": ")
		b.Write(val)
	}
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}

// Summary is a one-line human description of a result.
func (res *Result) Summary() string {
	return strings.Join([]string{
		fmt.Sprintf("tick=%d", res.FinalTick),
		fmt.Sprintf("status=%s", res.Status),
		fmt.Sprintf("collected=%d", res.Collected),
		fmt.Sprintf("final_hash=%s", res.FinalHash),
		fmt.Sprintf("trace_hash=%s", res.TraceHash),
	}, " ")
}
