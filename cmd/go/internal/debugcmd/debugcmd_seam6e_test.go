package debugcmd

// debugcmd_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for
// runServe's init-error and listen-error arms and writeDiscovery's
// marshal-error arm.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func TestServeInitError(t *testing.T) {
	orig := langNew
	langNew = func(...lang.Options) (*lang.Boru, error) { return nil, errors.New("init boom") }
	t.Cleanup(func() { langNew = orig })
	var stdout, stderr bytes.Buffer
	code := runServe(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "init boom") {
		t.Errorf("stderr = %q, want init boom", stderr.String())
	}
}

func TestServeListenError(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()) // keep the discovery file out of the real tmp
	var stdout, stderr bytes.Buffer
	// A non-loopback bind without --allow-public is declined before listening.
	code := runServe([]string{"--bind", "203.0.113.1:0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "debug serve:") {
		t.Errorf("stderr = %q, want serve error", stderr.String())
	}
	if !strings.Contains(stdout.String(), "serving introspection") {
		t.Errorf("stdout = %q, want serving line", stdout.String())
	}
}

func TestWriteDiscoveryMarshalError(t *testing.T) {
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshal = orig })
	path := filepath.Join(t.TempDir(), "boru-debug.json")
	writeDiscovery(path, "127.0.0.1:1", "")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discovery file written despite marshal error (stat err=%v)", err)
	}
}

// writeDiscovery publishes by rename so a reader never sees a half-written
// file. Both failure arms of that publish must leave NO discovery file at all
// — a stale or empty one is exactly what the rename is there to prevent — and
// must leave no temp litter in the directory either.

// The temp file cannot be created: the containing directory does not exist.
func TestWriteDiscoveryTempCreateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "boru-debug.json")
	writeDiscovery(path, "127.0.0.1:1", "")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discovery file written despite an uncreatable temp (stat err=%v)", err)
	}
}

// The rename fails: the destination is a DIRECTORY, so renaming a plain file
// over it is declined. The temp must be cleaned up rather than left behind.
func TestWriteDiscoveryPublishError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boru-debug.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDiscovery(path, "127.0.0.1:1", "")
	// The directory is still a directory — nothing overwrote it.
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		t.Fatalf("stat %s = (%v, err=%v), want the directory intact", path, st, err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		names := make([]string, len(ents))
		for i, e := range ents {
			names[i] = e.Name()
		}
		t.Errorf("temp file left behind after a failed publish: %v", names)
	}
}

// TestDiscoveryPublishNeverTears is the regression test for the atomic
// publish. A reader that stats the path and then reads it — precisely what
// `attach` and TestServeShutdownOnSignal do — must never observe a prefix of
// the JSON. Before writeDiscovery published by rename, this probe tore on
// roughly 77% of reads (100,944 of 130,332 at 30k republishes), which is what
// made TestServeShutdownOnSignal fail intermittently under load with
// "discovery file malformed: unexpected end of JSON input". A tenth of that
// iteration count is still far past the threshold where a torn read appears,
// so this stays fast while pinning the property.
func TestDiscoveryPublishNeverTears(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boru-debug.json")

	stop := make(chan struct{})
	var reads, torn int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := os.Stat(path); err != nil {
				continue
			}
			atomic.AddInt64(&reads, 1)
			if _, _, err := readDiscovery(path); err != nil {
				atomic.AddInt64(&torn, 1)
			}
		}
	}()
	for i := 0; i < 3000; i++ {
		writeDiscovery(path, "127.0.0.1:7777", "tok")
	}
	close(stop)
	wg.Wait()

	if n := atomic.LoadInt64(&torn); n != 0 {
		t.Errorf("%d torn reads of %d — the discovery file is not being published atomically",
			n, atomic.LoadInt64(&reads))
	}
	// Guard against the assertion passing vacuously: if the reader never got
	// a single read in, the test proved nothing. Skip rather than fail, so a
	// starved scheduler cannot turn this into a flake.
	if atomic.LoadInt64(&reads) == 0 {
		t.Skip("reader goroutine never observed the file; no tearing window was sampled")
	}
}
