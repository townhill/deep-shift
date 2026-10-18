package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTool(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestValidateEmbeddedCaves(t *testing.T) {
	code, out, errOut := runTool("validate")
	if code != 0 || !strings.Contains(out, "ok embedded: id=test_cave") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestValidateReportsBadFiles(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"format_version": 1, "id": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runTool("validate", "../../assets/levels/test_cave.json", bad)
	if code != 1 || !strings.Contains(errOut, "bad.json: invalid cave file") || !strings.Contains(errOut, "title: missing") {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
}

func TestShowPrintsGrid(t *testing.T) {
	code, out, errOut := runTool("show", "../../assets/levels/test_cave.json")
	if code != 0 || !strings.Contains(out, "#@   E#") || !strings.Contains(out, "state_hash=") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestReplayFixturesMatch(t *testing.T) {
	fixtures, err := filepath.Glob("../../testdata/replays/*.json")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	code, out, errOut := runTool(append([]string{"replay"}, fixtures...)...)
	if code != 0 || strings.Count(out, "matches expect block") != len(fixtures) {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	if code, _, errOut := runTool("explode"); code != 2 || !strings.Contains(errOut, "usage:") {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
}

func TestReplayTracePrintsEvents(t *testing.T) {
	code, out, errOut := runTool("replay", "-trace", "../../testdata/replays/test_cave_sealed_exit.json")
	if code != 0 || !strings.Contains(out, "tick 12: exit-sealed (5,1)") || !strings.Contains(out, "tick 0: moved (1,1)->(2,1)") {
		t.Fatalf("exit %d\nstdout head: %.400s\nstderr: %s", code, out, errOut)
	}
}
