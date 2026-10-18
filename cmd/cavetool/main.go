// Command cavetool validates caves and runs replays without any display:
// the headless companion to the game for cloud sessions, CI and debugging.
//
//	cavetool validate [FILE...]             validate cave files (default: the embedded caves)
//	cavetool show FILE                      print a cave's summary and ASCII grid
//	cavetool replay [-cave FILE] [-print-expect] [-trace] REPLAY...
//	                                        run replays, print the final state and
//	                                        check each replay's expect block
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/townhill/deep-shift/assets"
	"github.com/townhill/deep-shift/internal/level"
	"github.com/townhill/deep-shift/internal/replay"
	"github.com/townhill/deep-shift/internal/sim"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage:
  cavetool validate [FILE...]      validate cave files (default: the embedded caves)
  cavetool show FILE               print a cave's summary and ASCII grid
  cavetool replay [-cave FILE] [-print-expect] [-trace] REPLAY...
                                   run replays and check their expect blocks;
                                   -trace prints events and the grid on every
                                   tick where something happened
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "validate":
		err = validate(args[1:], stdout)
	case "show":
		err = show(args[1:], stdout)
	case "replay":
		err = runReplays(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "cavetool: unknown command %q\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "cavetool %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

func loadFile(name string) (*level.Level, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return level.Parse(name, data)
}

func describe(w io.Writer, source string, l *level.Level) {
	fmt.Fprintf(w, "ok %s: id=%s title=%q %dx%d quota=%d/%d seed=%d",
		source, l.ID, l.Title, l.Width, l.Height, l.RequiredCrystals, l.Crystals(), l.Seed)
	water := 0
	for _, c := range l.Cave().Cells {
		water += int(c.Water)
	}
	if water > 0 || len(l.Mechanisms) > 0 {
		fmt.Fprintf(w, " water=%d mechanisms=%d", water, len(l.Mechanisms))
	}
	fmt.Fprintf(w, " checksum=%s\n", l.Checksum())
}

func validate(files []string, stdout io.Writer) error {
	if len(files) == 0 {
		levels, err := level.LoadDir(assets.Levels, assets.LevelDir)
		for _, l := range levels {
			describe(stdout, "embedded", l)
		}
		return err
	}
	var errs []error
	for _, name := range files {
		l, err := loadFile(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		describe(stdout, name, l)
	}
	return errors.Join(errs...)
}

func show(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("want exactly one cave file")
	}
	l, err := loadFile(args[0])
	if err != nil {
		return err
	}
	s, err := sim.New(l.Cave(), l.Seed)
	if err != nil {
		return err
	}
	describe(stdout, args[0], l)
	fmt.Fprintf(stdout, "state_hash=%s\n%s", s.StateHash(), s.ASCII())
	return nil
}

func runReplays(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	caveFile := fs.String("cave", "", "cave file to replay against (default: look up cave_id among the embedded caves)")
	printExpect := fs.Bool("print-expect", false, "print the expect block the run produced, for reviewing a new fixture")
	trace := fs.Bool("trace", false, "print events and the grid on every tick where something happened")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("no replay files given")
	}

	var override *level.Level
	if *caveFile != "" {
		l, err := loadFile(*caveFile)
		if err != nil {
			return err
		}
		override = l
	}
	embedded, err := level.LoadDir(assets.Levels, assets.LevelDir)
	if err != nil {
		return err
	}

	var errs []error
	for _, name := range fs.Args() {
		if err := runReplay(name, override, embedded, *printExpect, *trace, stdout); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func runReplay(name string, override *level.Level, embedded []*level.Level, printExpect, trace bool, stdout io.Writer) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	r, err := replay.Parse(name, data)
	if err != nil {
		return err
	}
	l := override
	if l == nil {
		var ok bool
		if l, ok = level.ByID(embedded, r.CaveID); !ok {
			return fmt.Errorf("no embedded cave with id %q (use -cave)", r.CaveID)
		}
	}
	var observe func(uint64, []sim.Event, *sim.Sim)
	if trace {
		observe = func(tick uint64, events []sim.Event, s *sim.Sim) {
			if len(events) == 0 {
				return
			}
			fmt.Fprintf(stdout, "tick %d:", tick)
			for _, e := range events {
				fmt.Fprintf(stdout, " %s %s", e.Kind, describeEvent(e))
			}
			fmt.Fprintf(stdout, "\n%s", s.ASCII())
		}
	}
	res, err := replay.RunObserved(r, l, observe)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: %s\n%s", name, res.Summary(), res.Sim.ASCII())
	if printExpect {
		out, err := replay.Encode(&replay.Replay{
			FormatVersion: r.FormatVersion, Description: r.Description, CaveID: r.CaveID,
			CaveChecksum: r.CaveChecksum, Seed: r.Seed, RuleVersion: r.RuleVersion,
			Ticks: r.Ticks, Commands: r.Commands, Expect: replay.ExpectFrom(res),
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "--- replay with the expect block this run produced (review before committing) ---\n%s", out)
		return nil
	}
	if r.Expect == nil {
		return errors.New("no expect block to check (run with -print-expect to author one)")
	}
	if err := r.Check(res); err != nil {
		return fmt.Errorf("does not match its expect block:\n%w", err)
	}
	fmt.Fprintf(stdout, "%s: matches expect block\n", name)
	return nil
}

func describeEvent(e sim.Event) string {
	switch e.Kind {
	case sim.EventMoved:
		return fmt.Sprintf("%s->%s", e.From, e.Pos)
	case sim.EventFell, sim.EventRolled, sim.EventBoulderPushed, sim.EventPlayerCrushed:
		return fmt.Sprintf("%s %s->%s", e.Object, e.From, e.Pos)
	case sim.EventLanded:
		return fmt.Sprintf("%s %s", e.Object, e.Pos)
	}
	return e.Pos.String()
}
