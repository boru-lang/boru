package buildrt

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/policy"
)

// langOpts returns a default (no-policy, allow-everything) options value for
// the eval tests.
func langOpts() lang.Options { return lang.Options{} }

// --- Eval / runAndPrint contract ---

func TestEvalPrintsResidual(t *testing.T) {
	var out bytes.Buffer
	if err := Eval(&out, "add 1 2", langOpts()); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "3" {
		t.Fatalf("got %q, want 3", got)
	}
}

func TestEvalErrorOnUndefinedWord(t *testing.T) {
	var out bytes.Buffer
	err := Eval(&out, "nope-not-a-word", langOpts())
	if err == nil {
		t.Fatal("expected an error for an undefined word")
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout on error, got %q", out.String())
	}
}

func TestEvalSurfacesCompileFailure(t *testing.T) {
	// A program the bytecode emitter cannot lower errors. There is no
	// second engine to try.
	var out bytes.Buffer
	err := Eval(&out, "(size (for 5 [i]))", langOpts())
	if err == nil {
		t.Fatal("expected an error for a program that does not compile")
	}
}

// --- Main: exit codes, stdout/stderr split, bundled files ---

func TestMainSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "add 1 2"}, nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "3" {
		t.Fatalf("stdout=%q, want 3", got)
	}
}

func TestMainErrorExitsNonZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "nope-not-a-word"}, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for a runtime error")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected a diagnostic on stderr")
	}
}

func TestMainBundledFileImportResolvesInMemory(t *testing.T) {
	// The entry imports a sibling file that exists only in the embedded
	// Files map — never on disk — proving the in-memory FS is wired up.
	entryDir := "/virtual/proj"
	cfg := Config{
		Source:   `import "./lib.boru"` + "\n" + `Lib.x`,
		EntryDir: entryDir,
		Files: map[string][]byte{
			entryDir + "/lib.boru": []byte(`export "Lib" {x:7}`),
		},
	}
	var stdout, stderr bytes.Buffer
	code := Main(cfg, nil, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (stderr=%q)", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "7" {
		t.Fatalf("stdout=%q, want 7", got)
	}
}

func TestMainBadOptionsBlobErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "add 1 2", OptionsBlob: "tape:boguskey:10"}, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected a non-zero exit for an invalid --options blob")
	}
}

// --- Payload encode/decode round-trip ---

func TestPayloadRoundTrip(t *testing.T) {
	cfg := Config{
		Source:   "add 1 2",
		EntryDir: "/x/y",
		Files:    map[string][]byte{"/x/y/lib.boru": []byte("export \"L\" {a:1}")},
		Registry: "reg",
		Seed:     99,
	}
	payload, err := EncodePayload(cfg)
	if err != nil {
		t.Fatalf("EncodePayload: %v", err)
	}
	// Simulate appending to a host binary image.
	image := append([]byte("PRETEND-BINARY-CONTENT"), payload...)

	got, ok, err := DecodePayload(image)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for an image with a payload")
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func TestDecodePayloadNoMagic(t *testing.T) {
	_, ok, err := DecodePayload([]byte("just a plain binary with no trailer"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for an image without a payload")
	}
}

func TestDecodePayloadShortImage(t *testing.T) {
	_, ok, _ := DecodePayload([]byte("tiny"))
	if ok {
		t.Fatal("expected ok=false for an image smaller than the footer")
	}
}

func TestDecodePayloadCorruptLength(t *testing.T) {
	// Magic present but the declared length overruns the image.
	footer := make([]byte, footerSize)
	copy(footer[:magicSize], magic)
	// length far larger than the body
	for i := magicSize; i < footerSize; i++ {
		footer[i] = 0xff
	}
	image := append([]byte("body"), footer...)
	_, ok, err := DecodePayload(image)
	if err == nil {
		t.Fatal("expected an error for a corrupt (overrunning) length")
	}
	if ok {
		t.Fatal("expected ok=false on corruption")
	}
}

// EvalColor renders a structured runtime error through the ANSI
// renderer when color is on, and stays byte-plain when off.
func TestEvalColorErrorRendering(t *testing.T) {
	var out bytes.Buffer
	err := EvalColor(&out, "99 uppr", langOpts(), true)
	if err == nil || !strings.Contains(err.Error(), "\x1b[") {
		t.Fatalf("color=true must render ANSI, got %v", err)
	}
	if !strings.Contains(err.Error(), "error: ") {
		t.Errorf("the error: prefix must survive the colored path: %v", err)
	}
	err = EvalColor(&out, "99 uppr", langOpts(), false)
	if err == nil || strings.Contains(err.Error(), "\x1b[") {
		t.Fatalf("color=false must stay plain, got %v", err)
	}
}

// A built binary must see its own command line. Both call sites already
// passed os.Args[1:] into Main and it was dropped on the floor, so a
// `boru build` tool on $PATH could not be given an argument — which is not a
// tool. This is the whole point of the packaging story.
func TestMainSurfacesArgsAsIOArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := Config{Source: "import \"boru:io\"\nIO.args\n"}
	if code := Main(cfg, []string{"alpha", "--beta"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"alpha", "--beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("IO.args lost %q; got %q", want, out)
		}
	}
}

// With no args installed IO.args is the empty list, never none or an error,
// so a program can iterate unconditionally.
func TestMainNoArgsIsEmptyList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := Config{Source: "import \"boru:io\"\nsize (IO.args)\n"}
	if code := Main(cfg, nil, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0") {
		t.Errorf("want an empty args list, got %q", stdout.String())
	}
}

// A multi-file program's bundled sources must OVERLAY the host filesystem,
// not replace it. Replacing sent every write into RAM, where it was
// discarded at exit with a 0 status — a silent data-loss bug that only
// appeared once a program grew past one file.
func TestMainBundledFilesDoNotHideTheHostFilesystem(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "written.txt")

	// A bundled file makes cfg.Files non-empty, taking the overlay branch.
	cfg := Config{
		Source:   "import \"boru:io\"\nIO.write (make Pathon \"" + target + "\") \"payload\"\n",
		EntryDir: dir,
		Files:    map[string][]byte{filepath.Join(dir, "bundled.boru"): []byte("export \"B\" {x:1}")},
	}
	var stdout, stderr bytes.Buffer
	if code := Main(cfg, nil, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("the write never reached the real filesystem: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("file contents = %q, want payload", got)
	}
}

// A policy baked in at build time is compiled and enforced.
func TestMainEnforcesBakedProfile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "denied.txt")
	prof, err := policy.LoadAuto("read-only")
	if err != nil {
		t.Skipf("read-only profile unavailable: %v", err)
	}
	cfg := Config{
		Source:  "import \"boru:io\"\nIO.write (make Pathon \"" + target + "\") \"x\"\n",
		Profile: permsflags.ProfileFromPolicy(prof),
	}
	var stdout, stderr bytes.Buffer
	code := Main(cfg, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Error("a read-only baked policy allowed a write")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("the denied write still produced a file")
	}
}

// A corrupt baked profile is reported, not ignored.
func TestMainRejectsUnresolvableProfile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := Config{
		Source:  "1",
		Profile: &policy.Profile{Name: "broken", Extends: "no-such-profile"},
	}
	if code := Main(cfg, nil, nil, &stdout, &stderr); code == 0 {
		t.Error("an unresolvable baked profile was accepted")
	}
	if !strings.Contains(stderr.String(), "baked policy") {
		t.Errorf("stderr does not name the baked policy: %q", stderr.String())
	}
}

// --- IO.exit in a built binary ---

// A built tool chooses its own exit status. This is what makes a boru
// program usable in a shell pipeline at all: `if mytool …; then` reads the
// status, and before C1 every built binary could only ever say 0 or 1.
func TestMainExitCodeIsProcessStatus(t *testing.T) {
	for _, want := range []int{0, 1, 3, 125} {
		var stdout, stderr bytes.Buffer
		src := `import "boru:io"  IO.exit ` + strconv.Itoa(want)
		if code := Main(Config{Source: src}, nil, nil, &stdout, &stderr); code != want {
			t.Errorf("IO.exit %d: exit status %d (stderr: %s)", want, code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("IO.exit %d printed to stderr: %q", want, stderr.String())
		}
	}
}

// The residual stack is not flushed on exit, and an out-of-range code is a
// compile failure (status 1) rather than a status.
func TestMainExitSuppressesResidualAndRejectsRange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Main(Config{Source: `import "boru:io"  42  IO.exit 0`}, nil, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit status %d, want 0", code)
	}
	if strings.Contains(stdout.String(), "42") {
		t.Errorf("residual stack printed on exit: %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Main(Config{Source: `import "boru:io"  IO.exit 200`}, nil, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("out-of-range exit status %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "0..125") {
		t.Errorf("stderr does not explain the range: %q", stderr.String())
	}
}

// A built binary whose program does not compile FAILS, loudly, with the
// compiler defect named — it used to run the program on the interpreter and
// say nothing, deliberately, because a shipped tool warning about its own
// engine on every invocation is noise in someone else's pipeline. The noise
// argument only held while the run still produced an answer. It does not: the
// binary cannot run the program at all, so the only honest thing it can do is
// say why. `boru build` declines to ship such a binary in the first place
// (TestBuildFailsOnUncompilableProgram); this is the backstop for a payload
// built before that gate, or one whose program stopped compiling under a
// newer runtime.
func TestMainFailsLoudlyWhenTheProgramDoesNotCompile(t *testing.T) {
	const doesNotCompile = `def m {n: 3} def xs (for (m get "n") [1]) xs`
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: doesNotCompile}, nil, nil, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit=0, want a failure (stdout=%q stderr=%q)", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "bytecode compilation FAILED") {
		t.Errorf("stderr=%q, want the compile failure named", stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "" {
		t.Errorf("stdout=%q, want nothing — the program never ran", got)
	}
}
