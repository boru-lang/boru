//go:build unix

package capabilities

import (
	"os"

	"golang.org/x/sys/unix"
)

// mmap_unix.go — memory-mapped files via mmap(2). A writable region uses
// MAP_SHARED so msync/close flush changes back to the file; a read-only
// region uses MAP_PRIVATE. The whole file is mapped and the caller
// windows it (see locks.go), so no page-alignment dance is needed.

// mmap syscall seams (runner-independent branch coverage).
var (
	mmapFn   = unix.Mmap
	munmapFn = unix.Munmap
	msyncFn  = unix.Msync
)

func osMmapFile(f *os.File, length int, writable bool) ([]byte, error) {
	prot := unix.PROT_READ
	flags := unix.MAP_PRIVATE
	if writable {
		prot |= unix.PROT_WRITE
		flags = unix.MAP_SHARED
	}
	return mmapFn(int(f.Fd()), 0, length, prot, flags)
}

func osMunmap(b []byte) error { return munmapFn(b) }

// osMsync flushes the mapping; msync(MS_SYNC) reaches the file itself, so
// the fd is not needed here (it is on Windows).
func osMsync(_ *os.File, b []byte) error { return msyncFn(b, unix.MS_SYNC) }
