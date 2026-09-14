package buildrt

// golden_exec_test.go — every self-embedding executable this project has
// ever produced must still recognise itself. The magics differ in length
// across renames, so this also pins the offset arithmetic.

import (
	"os"
	"path/filepath"
	"testing"
)

func wireGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "wire", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEveryShippedExecutableStillDetected(t *testing.T) {
	for _, f := range []string{"exec.vltexec.golden", "exec.boruexec.golden", "exec.aqlexec.golden"} {
		img := wireGolden(t, f)

		cfg, ok, err := DecodePayload(img)
		if err != nil || !ok {
			t.Errorf("%s: DecodePayload ok=%v err=%v", f, ok, err)
			continue
		}
		if cfg.Source != "1 print" {
			t.Errorf("%s: source = %q, want \"1 print\"", f, cfg.Source)
		}

		// And via the tail-read path used on every CLI invocation.
		p := filepath.Join(t.TempDir(), "exe")
		if err := os.WriteFile(p, img, 0o755); err != nil {
			t.Fatal(err)
		}
		cfg, ok, err = ReadEmbeddedPayload(p)
		if err != nil || !ok {
			t.Errorf("%s: ReadEmbeddedPayload ok=%v err=%v", f, ok, err)
			continue
		}
		if cfg.Source != "1 print" {
			t.Errorf("%s via file: source = %q", f, cfg.Source)
		}
	}
}

// A file long enough to hold a length field but shorter than the widest
// readable trailer must be handled by the tail-read clamp, not an
// out-of-range read. Real hosts are megabytes; only a stub hits this.
func TestReadEmbeddedPayloadShorterThanWidestTrailer(t *testing.T) {
	for _, n := range []int{8, 10, 16} {
		p := filepath.Join(t.TempDir(), "small")
		if err := os.WriteFile(p, make([]byte, n), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := ReadEmbeddedPayload(p); ok || err != nil {
			t.Errorf("%d-byte image: ok=%v err=%v, want false/nil", n, ok, err)
		}
	}
}
