//go:build !linux && !darwin && !windows

package native

import (
	"os"
	"testing"
)

func openTestTerminal(t *testing.T) *os.File {
	t.Helper()
	t.Skip("terminal fixture is available on Linux, macOS, and Windows")
	return nil
}
