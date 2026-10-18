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
var Campaign = []string{"cave_01", "cave_02", "cave_03", "cave_04", "cave_05"}

// Extras lists embedded caves outside the campaign: the crawler example
// cave (SPEC §7: every new system ships with one), and the Phase 1 test
// cave, which stays playable because its golden replays pin the rule-1
// behaviour.
var Extras = []string{"crawler_warren", "test_cave"}

// PlayOrder lists every embedded cave id in the order the app plays them:
// the campaign, then the extras.
var PlayOrder = append(slices.Clone(Campaign), Extras...)
