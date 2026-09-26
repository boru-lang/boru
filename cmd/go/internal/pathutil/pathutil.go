// Package pathutil holds small filesystem-path helpers shared across the
// CLI commands. Its reason to exist is one recurring hazard: a leading ~
// in a user-supplied path is only expanded by the shell when it begins an
// unquoted word, so a ~ written after "--flag=", inside quotes, or coming
// from an environment variable reaches the program verbatim and, used as
// a path, names a literal "~" directory in the working folder instead of
// the user's home. Every CLI flag or env var that takes a filesystem path
// runs its value through ExpandTilde/Expand so that hazard is handled in
// one place.
package pathutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExpandTilde replaces a leading ~ (alone, or immediately before a path
// separator) in path with homeDir. A bare "~" and "~/sub" expand;
// "~user" is left untouched (we do not resolve other users' homes), as is
// any path with no leading tilde or when homeDir is empty (there is then
// no home to expand to). The expanded path is cleaned via filepath.Join.
func ExpandTilde(path, homeDir string) string {
	if path == "" || path[0] != '~' || homeDir == "" {
		return path
	}
	if path == "~" {
		return homeDir
	}
	// path != "~" with path[0]=='~' guarantees len(path) >= 2, so path[1]
	// is safe. os.IsPathSeparator covers both '/' and, on Windows, '\\'.
	if os.IsPathSeparator(path[1]) {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// Expand is ExpandTilde using the current user's home directory as
// reported by the OS (which honors $HOME on Unix, %USERPROFILE% on
// Windows). When the home folder cannot be determined the path is
// returned unchanged. Use this at CLI call sites that do not already have
// a resolved home directory in hand.
func Expand(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := userHomeDir()
	if err != nil {
		return path
	}
	return ExpandTilde(path, home)
}

// WalkSources walks the tree under root with walk (filepath.WalkDir, or a
// command's test seam for it), skipping every `.boru/` directory — the
// build and install cache, never source — and hands add each regular file
// whose name has suffix. It is the ONE walk the tree-walking subcommands
// share (fmt, check, test), so the skip rule cannot drift between them
// (NUR082: `boru test` used to descend into `.boru/`).
func WalkSources(root string, walk func(string, fs.WalkDirFunc) error, suffix string, add func(string)) error {
	return walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".boru" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			add(path)
		}
		return nil
	})
}
