//go:build !windows

package capabilities

import "path/filepath"

// isAbsPath reports whether path names a location without a working
// directory. Off Windows every rooted path is absolute, so this is
// filepath.IsAbs; rooted_windows.go adds Windows' drive-rooted paths.
func isAbsPath(path string) bool { return filepath.IsAbs(path) }

// joinWorkingDir resolves a relative path under the working directory wd.
func joinWorkingDir(wd, path string) string { return filepath.Join(wd, path) }
