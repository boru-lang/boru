package capabilities

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Windows byte-range locks are mandatory. Coordinate on a reserved byte
// beyond filesystem size limits so ordinary file I/O stays possible,
// preserving the advisory contract. Handle close/process exit releases it.
func osLockFile(f *os.File, shared, block bool) error {
	var flags uint32
	if !shared {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if !block {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, advisoryLockRange())
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLockBusy
	}
	return err
}

func osUnlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, advisoryLockRange())
}

func advisoryLockRange() *windows.Overlapped {
	return &windows.Overlapped{OffsetHigh: 0x7fffffff, Offset: 0xfffffffe}
}
