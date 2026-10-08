package fmt

import (
	"bytes"
	"strings"
	"testing"
)

// NUR084: `-h` is a help request on every subcommand — fmt prints its usage
// to stdout and exits 0, rather than reading `-h` as a file to format.
func TestFmtHelpExitsZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		var out, errOut bytes.Buffer
		if code := Run([]string{flag}, &out, &errOut); code != 0 {
			t.Fatalf("fmt %s: exit %d, want 0; stderr: %s", flag, code, errOut.String())
		}
		if !strings.HasPrefix(out.String(), "usage: boru fmt") {
			t.Fatalf("fmt %s: stdout = %q, want the usage", flag, out.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("fmt %s: stderr = %q, want empty", flag, errOut.String())
		}
	}
}
