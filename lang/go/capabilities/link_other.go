//go:build !darwin

package capabilities

import "os"

// Linux link(2) and Windows CreateHardLink preserve the source symlink.
func linkNoFollow(target, linkPath string) error {
	return os.Link(target, linkPath)
}
