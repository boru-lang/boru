package buildrt

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func TestSeam5EvalInitError(t *testing.T) {
	boom := errors.New("init boom")
	orig := langNew
	langNew = func(...lang.Options) (*lang.Boru, error) { return nil, boom }
	t.Cleanup(func() { langNew = orig })

	err := Eval(io.Discard, "1 2 +", lang.Options{}, CompileOff)
	if err == nil || !strings.Contains(err.Error(), "init error: init boom") {
		t.Fatalf("Eval = %v, want init error wrapping %v", err, boom)
	}
}

func TestSeam5MainInitError(t *testing.T) {
	boom := errors.New("init boom")
	orig := langNew
	langNew = func(...lang.Options) (*lang.Boru, error) { return nil, boom }
	t.Cleanup(func() { langNew = orig })

	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "1"}, nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Main = %d, want 1", code)
	}
	if got := stderr.String(); got != "init error: init boom\n" {
		t.Fatalf("stderr = %q, want %q", got, "init error: init boom\n")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestSeam5EncodePayloadMarshalError(t *testing.T) {
	boom := errors.New("marshal boom")
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, boom }
	t.Cleanup(func() { jsonMarshal = orig })

	_, err := EncodePayload(Config{Source: "1"})
	if !errors.Is(err, boom) {
		t.Fatalf("EncodePayload = %v, want wrapped %v", err, boom)
	}
	if !strings.Contains(err.Error(), "encode payload:") {
		t.Fatalf("EncodePayload error = %q, want encode payload prefix", err)
	}
}

// seam5PayloadFile writes a valid built-executable image (junk prefix +
// encoded payload) to a temp file and returns its path.
func seam5PayloadFile(t *testing.T) string {
	t.Helper()
	payload, err := EncodePayload(Config{Source: "1 2 +"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "built-exe")
	image := append([]byte("FAKEBINARYIMAGE"), payload...)
	if err := os.WriteFile(path, image, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSeam5ReadEmbeddedPayloadStatError(t *testing.T) {
	boom := errors.New("stat boom")
	orig := fileStat
	fileStat = func(*os.File) (os.FileInfo, error) { return nil, boom }
	t.Cleanup(func() { fileStat = orig })

	_, _, err := ReadEmbeddedPayload(seam5PayloadFile(t))
	if !errors.Is(err, boom) {
		t.Fatalf("ReadEmbeddedPayload = %v, want wrapped %v", err, boom)
	}
	if !strings.Contains(err.Error(), "read self:") {
		t.Fatalf("error = %q, want read self prefix", err)
	}
}

func TestSeam5ReadEmbeddedPayloadFooterReadError(t *testing.T) {
	boom := errors.New("read boom")
	orig := fileReadAt
	fileReadAt = func(*os.File, []byte, int64) (int, error) { return 0, boom }
	t.Cleanup(func() { fileReadAt = orig })

	_, _, err := ReadEmbeddedPayload(seam5PayloadFile(t))
	if !errors.Is(err, boom) {
		t.Fatalf("ReadEmbeddedPayload = %v, want wrapped %v", err, boom)
	}
	if !strings.Contains(err.Error(), "read self:") {
		t.Fatalf("error = %q, want read self prefix", err)
	}
}

func TestSeam5ReadEmbeddedPayloadBodyReadError(t *testing.T) {
	boom := errors.New("read boom")
	calls := 0
	orig := fileReadAt
	fileReadAt = func(f *os.File, b []byte, off int64) (int, error) {
		calls++
		if calls == 1 {
			// Footer read: pass through so the magic check succeeds.
			return f.ReadAt(b, off)
		}
		return 0, boom
	}
	t.Cleanup(func() { fileReadAt = orig })

	_, _, err := ReadEmbeddedPayload(seam5PayloadFile(t))
	if !errors.Is(err, boom) {
		t.Fatalf("ReadEmbeddedPayload = %v, want wrapped %v", err, boom)
	}
	if !strings.Contains(err.Error(), "embedded payload:") {
		t.Fatalf("error = %q, want embedded payload prefix", err)
	}
	if calls != 2 {
		t.Fatalf("fileReadAt called %d times, want 2", calls)
	}
}

// Resolution errors are injected so every OS exercises the same failure.
func TestSeam5AbsDirError(t *testing.T) {
	boom := errors.New("absolute path unavailable")
	resolve := func(string) (string, error) { return "", boom }
	if _, err := absDirWithAbs("rel/prog.boru", resolve); !errors.Is(err, boom) {
		t.Fatalf("AbsDir must propagate resolution failure: %v", err)
	}
}
