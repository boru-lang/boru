package repl

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// The Phase 2 entry-point pins (plan p2 cases): a REPL line runs
// COMPILED-BY-DEFAULT — no unattributed interpreter entry fires for a
// compilable line — and per-line def/import state persists across lines
// exactly as the interpreter loop did.

// armedRegistry wraps the production newRegistry with an interp-entry
// observer (the C4 seam, eng interp_entry.go) recording unattributed
// entries — check-mode entries are attributed and expected (the compile
// pipeline's front-end).
func armedRegistry(t *testing.T, entries *[]string) {
	t.Helper()
	prev := newRegistry
	t.Cleanup(func() { newRegistry = prev })
	newRegistry = func() (*native.Registry, error) {
		reg, err := prev()
		if err != nil {
			return nil, err
		}
		reg.ArmInterpEntryHook(func(e core.InterpEntry) {
			if e.Attribution == "" {
				*entries = append(*entries, e.Seam)
			}
		})
		return reg, nil
	}
}

func TestStartLineRunsCompiled(t *testing.T) {
	var entries []string
	armedRegistry(t, &entries)
	in := strings.NewReader("1 add 2\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "3") {
		t.Fatalf("expected output to contain '3', got %q", out.String())
	}
	if len(entries) != 0 {
		t.Errorf("unattributed interpreter entries on a compiled REPL line: %v", entries)
	}
}

func TestStartStatePersistsAcrossCompiledLines(t *testing.T) {
	in := strings.NewReader("def x 41\nx add 1\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "42") {
		t.Fatalf("def state must persist across compiled lines; got %q", out.String())
	}
}

// A REFUSED line falls back to the interpreter (attributed as the sound
// fallback path is not yet — that unattributed Engine.Run is the Phase
// 10/11 burn-down, pinned red in lang/go's frontier cases) and still
// produces the interpreter's exact result — the REPL never errors on a
// refusal.
func TestStartRefusedLineFallsBackWithResult(t *testing.T) {
	in := strings.NewReader("for 3 [1 2]\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "1 2 1 2 1 2") {
		t.Fatalf("refused line must fall back to the interpreter's result; got %q", out.String())
	}
}

// A line that does not compile PRINTS the failure. The REPL used to re-run
// it on the interpreter, silently, on the argument that an interactive line's
// performance debt was not worth a per-line message. It was never about
// performance: the line had hit a compiler defect and nothing said so — and a
// REPL is where the user is most likely to be the one who can report it.
// The fixture is the mid-expression fn-value apply (`for 3 [1 2]` above
// compiles natively, so it pins the compiled path rather than this one).
func TestStartLineThatDoesNotCompilePrintsTheFailure(t *testing.T) {
	in := strings.NewReader(`def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"X"} s)` + "\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "bytecode compilation FAILED") {
		t.Fatalf("a line that does not compile must say so; got %q", out.String())
	}
}

// The NewFromRegistry construction-error arm (seam-driven; the production
// registry is never nil).
func TestStartInstanceInitErrorReported(t *testing.T) {
	prev := langNewFromRegistry
	t.Cleanup(func() { langNewFromRegistry = prev })
	langNewFromRegistry = func(*native.Registry) (*lang.Boru, error) {
		return nil, errors.New("boom")
	}
	in := strings.NewReader("1 add 2\n")
	out := &bytes.Buffer{}
	Start(in, out, "")
	if !strings.Contains(out.String(), "init error") {
		t.Fatalf("expected the init-error arm, got %q", out.String())
	}
}
