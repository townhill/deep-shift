package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/townhill/deep-shift/internal/level"
)

// ErrExists reports that a save would replace a file the editor did not
// open or save itself.
var ErrExists = errors.New("a file with that name already exists")

// Storage keeps cave files for the editor. The app uses DirStorage; tests
// keep files in memory.
type Storage interface {
	// Dir is the folder new caves are saved in; empty when there is none.
	Dir() string
	// Write saves data at path. Unless replace is set it fails with
	// ErrExists when a file is already there.
	Write(path string, data []byte, replace bool) error
	// List returns the cave files in Dir, sorted by path.
	List() ([]File, error)
}

// File is a cave file found in a Storage.
type File struct {
	Path  string
	Draft level.Draft
	Err   error // why the file cannot be opened; Draft is unset
}

// DirStorage keeps caves as JSON files in a folder.
type DirStorage struct {
	Path string // the folder; created on the first save
}

// Dir returns the folder.
func (s DirStorage) Dir() string { return s.Path }

// Write saves data at path atomically: it writes a temporary file beside it
// and renames it into place, so a failed save never leaves half a cave.
func (s DirStorage) Write(path string, data []byte, replace bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !replace {
		if _, err := os.Stat(path); err == nil {
			return ErrExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // after a successful rename there is nothing to remove
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil { // CreateTemp makes it private
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// List reads every *.json file in the folder. A folder that does not exist
// yet holds no caves.
func (s DirStorage) List() ([]File, error) {
	if s.Path == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(s.Path, name)
		f := File{Path: path}
		data, err := os.ReadFile(path)
		if err == nil {
			f.Draft, err = level.DecodeDraft(name, data)
		}
		if err == nil {
			_, err = docFromDraft(f.Draft)
		}
		if err != nil {
			f.Err = fmt.Errorf("%s: %w", name, err)
			f.Draft = level.Draft{}
		}
		files = append(files, f)
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}
