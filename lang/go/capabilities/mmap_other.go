//go:build !unix && !windows

package capabilities

import (
	"errors"
	"os"
)

// mmap_other.go — the non-unix fallback (js/wasm, plan9): no mmap(2), so
// mapping refuses cleanly. Not compiled on unix, so it carries no
// cover-gate obligation there.
var errMmapUnsupported = errors.New("memory-mapped files are not supported on this platform")

func osMmapFile(*os.File, int, bool) ([]byte, error) { return nil, errMmapUnsupported }
func osMunmap([]byte) error                          { return nil }
func osMsync([]byte) error                           { return nil }
