//go:build linux || darwin

package native

import (
	"os"
	"testing"

	"github.com/creack/pty"
)

func openTestTerminal(t *testing.T) *os.File {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open initialized PTY: %v", err)
	}
	t.Cleanup(func() {
		_ = slave.Close()
		_ = master.Close()
	})
	return slave
}
