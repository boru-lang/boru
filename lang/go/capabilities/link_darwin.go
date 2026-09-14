package capabilities

import (
	"os"

	"golang.org/x/sys/unix"
)

// Darwin's link(2), used by os.Link, follows the source symlink. linkat
// with flags=0 explicitly links the directory entry, as on Linux/Windows.
func linkNoFollow(target, linkPath string) error {
	if err := unix.Linkat(unix.AT_FDCWD, target, unix.AT_FDCWD, linkPath, 0); err != nil {
		return &os.LinkError{Op: "link", Old: target, New: linkPath, Err: err}
	}
	return nil
}
