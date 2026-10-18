// Package assets embeds the original game data that ships inside the binary,
// so the app never depends on the working directory (important when the
// .app is launched from Finder).
package assets

import "embed"

// Levels holds the authored caves under levels/*.json.
//
//go:embed levels/*.json
var Levels embed.FS

// LevelDir is the directory of cave files inside Levels.
const LevelDir = "levels"

// TestCave is the path of the Phase 1 test cave inside Levels.
const TestCave = "levels/test_cave.json"
