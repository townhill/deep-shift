// Package assets embeds the original game data that ships inside the binary,
// so the app never depends on the working directory (important when the
// .app is launched from Finder).
package assets

import (
	"embed"
	"slices"
)

// Levels holds the authored caves under levels/*.json.
//
//go:embed levels/*.json
var Levels embed.FS

// LevelDir is the directory of cave files inside Levels.
const LevelDir = "levels"

// TestCave is the path of the Phase 1 test cave inside Levels.
const TestCave = "levels/test_cave.json"

// Campaign lists the main caves in play order: SPEC §8's starter caves.
var Campaign = []string{"cave_01", "cave_02", "cave_03", "cave_04"}

// Extras lists embedded caves outside the campaign. The Phase 1 test cave
// stays playable here because its golden replays pin the rule-1 behaviour.
var Extras = []string{"test_cave"}

// PlayOrder lists every embedded cave id in the order the app plays them:
// the campaign, then the extras.
var PlayOrder = append(slices.Clone(Campaign), Extras...)
