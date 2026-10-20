package build

import (
	"os"
	"runtime"
)

// runnable reports whether st is a file the host will execute. Windows has no
// execute bit — os.Stat reports 0o666 for every writable file, and the OS runs
// a file by its extension (each output here carries exeSuffix) — so there a
// regular file is enough; elsewhere some execute bit must be set.
func runnable(st os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return st.Mode().IsRegular()
	}
	return st.Mode()&0o111 != 0
}
