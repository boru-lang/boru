package capabilities

import (
	"os"
	"path/filepath"
)

// isAbsPath reports whether path names a location without a working
// directory: filepath.IsAbs, or ROOTED with no volume — `\tmp\x`, `/tmp/x` —
// which Windows reads as the root of the current drive (filepath.IsAbs is
// false for it, so code that treated it as relative put it under the working
// directory instead). The in-memory model has no current drive, so there a
// rooted path is simply absolute.
func isAbsPath(path string) bool {
	return filepath.IsAbs(path) || driveRooted(path)
}

// driveRooted reports whether path is rooted with no volume.
func driveRooted(path string) bool {
	return filepath.VolumeName(path) == "" && path != "" && os.IsPathSeparator(path[0])
}

// joinWorkingDir resolves a path that is not filepath.IsAbs against the
// working directory wd the way Windows does: a drive-rooted path names the
// root of wd's drive (filepath.Abs's reading), anything else sits under wd.
func joinWorkingDir(wd, path string) string {
	if driveRooted(path) {
		return filepath.Join(filepath.VolumeName(wd)+`\`, path)
	}
	return filepath.Join(wd, path)
}
