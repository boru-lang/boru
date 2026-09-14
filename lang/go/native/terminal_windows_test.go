package native

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func openTestTerminal(t *testing.T) *os.File {
	t.Helper()
	// CI redirects stdout; open the console explicitly. A headless test
	// process may need its own console before CONOUT$ is available.
	f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		kernel := windows.NewLazySystemDLL("kernel32.dll")
		if ok, _, allocErr := kernel.NewProc("AllocConsole").Call(); ok == 0 {
			t.Fatalf("allocate test console: %v (open: %v)", allocErr, err)
		}
		t.Cleanup(func() { _, _, _ = kernel.NewProc("FreeConsole").Call() })
		f, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("open test console: %v", err)
		}
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
