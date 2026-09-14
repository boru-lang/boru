package buildrt

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeImage writes raw bytes to a temp file and returns its path.
func writeImage(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(p, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- ReadEmbeddedPayload ---

func TestReadEmbeddedPayloadRoundTrip(t *testing.T) {
	payload, err := EncodePayload(Config{Source: "add 1 2", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	p := writeImage(t, append([]byte("fake host binary"), payload...))

	cfg, ok, err := ReadEmbeddedPayload(p)
	if err != nil {
		t.Fatalf("ReadEmbeddedPayload: %v", err)
	}
	if !ok {
		t.Fatal("expected the payload to be detected")
	}
	if cfg.Source != "add 1 2" || cfg.Seed != 7 {
		t.Errorf("cfg = %+v, want Source=add 1 2 Seed=7", cfg)
	}
}

func TestReadEmbeddedPayloadPlainBinary(t *testing.T) {
	p := writeImage(t, []byte("just a plain binary with no trailer at all"))
	_, ok, err := ReadEmbeddedPayload(p)
	if err != nil {
		t.Fatalf("ReadEmbeddedPayload: %v", err)
	}
	if ok {
		t.Error("plain binary must not report a payload")
	}
}

func TestReadEmbeddedPayloadShortFile(t *testing.T) {
	p := writeImage(t, []byte("tiny"))
	_, ok, err := ReadEmbeddedPayload(p)
	if err != nil {
		t.Fatalf("ReadEmbeddedPayload: %v", err)
	}
	if ok {
		t.Error("short file must not report a payload")
	}
}

func TestReadEmbeddedPayloadMissingFile(t *testing.T) {
	_, ok, err := ReadEmbeddedPayload(filepath.Join(t.TempDir(), "nope"))
	if err == nil || ok {
		t.Fatalf("got ok=%v err=%v, want an open error", ok, err)
	}
}

func TestReadEmbeddedPayloadCorruptLength(t *testing.T) {
	// A valid magic with a length larger than the file must error.
	footer := make([]byte, footerSize)
	copy(footer[:len(magic)], magic)
	binary.BigEndian.PutUint64(footer[len(magic):], 1<<40)
	p := writeImage(t, append([]byte("host"), footer...))

	_, ok, err := ReadEmbeddedPayload(p)
	if err == nil || ok {
		t.Fatalf("got ok=%v err=%v, want a corrupt-length error", ok, err)
	}
}

func TestReadEmbeddedPayloadCorruptJSON(t *testing.T) {
	body := []byte("{not json")
	footer := make([]byte, footerSize)
	copy(footer[:len(magic)], magic)
	binary.BigEndian.PutUint64(footer[len(magic):], uint64(len(body)))
	image := append([]byte("host"), body...)
	image = append(image, footer...)
	p := writeImage(t, image)

	_, ok, err := ReadEmbeddedPayload(p)
	if err == nil || ok {
		t.Fatalf("got ok=%v err=%v, want a JSON decode error", ok, err)
	}
}

// --- AbsDir ---

func TestAbsDir(t *testing.T) {
	got, err := AbsDir("/a/b/c.boru")
	if err != nil {
		t.Fatalf("AbsDir: %v", err)
	}
	if got != "/a/b" {
		t.Errorf("AbsDir(/a/b/c.boru) = %q, want /a/b", got)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err = AbsDir("rel.boru")
	if err != nil {
		t.Fatalf("AbsDir relative: %v", err)
	}
	if got != wd {
		t.Errorf("AbsDir(rel.boru) = %q, want %q", got, wd)
	}
}
