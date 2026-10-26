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

func TestValidateReportsGlimmers(t *testing.T) {
	cave := filepath.Join(t.TempDir(), "glimmers.json")
	doc := `{"format_version": 5, "id": "glimmers", "title": "Glimmers", "width": 6, "height": 3, "seed": 1,
		"required_crystals": 10, "rows": ["######", "#@*E▶#", "######"]}`
	if err := os.WriteFile(cave, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runTool("validate", cave)
	if code != 0 || !strings.Contains(out, "quota=10/1") || !strings.Contains(out, " glimmers=1 ") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	code, out, errOut = runTool("validate")
	if code != 0 || !strings.Contains(out, "id=glimmer_hollow") || !strings.Contains(out, "water=9 mechanisms=2 glimmers=3") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
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

func TestReplayTracePrintsValveAndWater(t *testing.T) {
	code, out, errOut := runTool("replay", "-trace", "../../testdata/replays/cave_04_complete.json")
	if code != 0 || !strings.Contains(out, "tick 64: too-deep (13,6)") ||
		!strings.Contains(out, "tick 205: valve-turned (12,6) gate-opened (13,11) gate-opened (14,11)") {
		t.Fatalf("exit %d\nstdout head: %.400s\nstderr: %s", code, out, errOut)
	}
	code, out, errOut = runTool("validate")
	if code != 0 || !strings.Contains(out, "id=cave_04") || !strings.Contains(out, "water=44 mechanisms=1") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestReplayTracePrintsCrawlers(t *testing.T) {
	code, out, errOut := runTool("replay", "-trace", "../../testdata/replays/crawler_warren_caught.json")
	if code != 0 || !strings.Contains(out, "tick 126: crawler-moved (10,3)") ||
		!strings.Contains(out, "tick 132: player-caught (9,3)") || !strings.Contains(out, "status=dead") {
		t.Fatalf("exit %d\nstdout tail: %s\nstderr: %s", code, out[max(0, len(out)-600):], errOut)
	}
	code, out, errOut = runTool("validate")
	if code != 0 || !strings.Contains(out, "id=crawler_warren") || !strings.Contains(out, "water=9 mechanisms=1 crawlers=3") {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}

func TestFmtChecksAndRewrites(t *testing.T) {
	dir := t.TempDir()
	messy := filepath.Join(dir, "messy.json")
	// Valid content in a non-canonical layout: fields out of order, the grid
	// on one line. fmt must keep the content and change only the layout.
	src := `{"rows": ["#####", "#@*E#", "#####"], "id": "messy", "title": "Messy", "format_version": 1,
	"width": 5, "height": 3, "seed": 7, "required_crystals": 1}`
	if err := os.WriteFile(messy, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runTool("fmt", "-check", messy)
	if code != 1 || !strings.Contains(out, "not canonical: "+messy) {
		t.Fatalf("fmt -check: exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	if data, _ := os.ReadFile(messy); string(data) != src {
		t.Fatal("fmt -check changed the file")
	}
	code, out, errOut = runTool("fmt", messy)
	if code != 0 || !strings.Contains(out, "formatted "+messy) {
		t.Fatalf("fmt: exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
	data, _ := os.ReadFile(messy)
	if !strings.HasPrefix(string(data), "{\n  \"format_version\": 1,\n  \"id\": \"messy\",") || !strings.Contains(string(data), "\n    \"#@*E#\",\n") {
		t.Fatalf("not rewritten canonically:\n%s", data)
	}
	if code, out, _ := runTool("fmt", "-check", messy); code != 0 || out != "" {
		t.Fatalf("second check: exit %d, stdout %q", code, out)
	}
	if code, _, errOut := runTool("validate", messy); code != 0 {
		t.Fatalf("formatted cave no longer validates: %s", errOut)
	}
}

func TestFmtEmbeddedCavesAreCanonical(t *testing.T) {
	caves, err := filepath.Glob("../../assets/levels/*.json")
	if err != nil || len(caves) == 0 {
		t.Fatalf("no caves: %v", err)
	}
	if code, out, errOut := runTool(append([]string{"fmt", "-check"}, caves...)...); code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, out, errOut)
	}
}
