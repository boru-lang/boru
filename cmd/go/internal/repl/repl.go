package repl

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/chzyer/readline"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/cmd/go/internal/termback"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"

	udk "github.com/voxgig/udk/go"
)

// PROMPT is the REPL prompt string.
const PROMPT = ">> "

// newReadline and newRegistry are package-level vars for testability.
var newReadline = func(cfg *readline.Config) (readliner, error) {
	return readline.NewEx(cfg)
}

var newRegistry = func() (*native.Registry, error) {
	reg, err := nativeDefaultRegistry()
	if err != nil {
		return nil, err
	}
	// Wire the native-module resolver so `import "boru:<name>"` works at the
	// prompt (and so `describe` can load and document modules) — the same
	// wiring lang.New does for one-shot runs.
	modules.InstallResolver(reg)
	// The real-TTY tuikit backend, so boru:tui words reach the terminal
	// from the prompt too (a duplicate registration is the only failure
	// and means the backend is already there).
	_ = modules.RegisterHostTui(reg, termback.Spec())
	// The terminal probe behind IO.is-tty. The REPL is the one place a real
	// terminal genuinely exists, so shipping a probe that always answered
	// false AT THE PROMPT would be the worst possible gap — and
	// NewFromRegistry installs no capabilities of its own, unlike lang.New.
	native.SetHostStreamProbe(reg, capabilities.OSStreamProbe{})
	return reg, nil
}

// langNewFromRegistry is a test seam (design/TEST-SEAMS.10.md); tests swap
// it to drive the instance-construction error arm — NewFromRegistry only
// fails on a nil registry, which newRegistry never produces.
var langNewFromRegistry = lang.NewFromRegistry

// readliner abstracts the readline interface for testing.
type readliner interface {
	Readline() (string, error)
	Close() error
}

// Start runs the REPL loop, reading from in and writing to out.
// If registryPath is non-empty, a UniversalManager is configured for API operations.
func Start(in io.Reader, out io.Writer, registryPath string) {
	startWithPauseGate(in, out, registryPath, nil)
}

// startWithPauseGate is the shared loop body used by both Start (no
// gate) and Service.Start (with a pause gate). When paused is non-nil
// and set to true, entered lines are discarded with a "paused" notice
// instead of being evaluated. The readline loop keeps spinning so the
// prompt stays responsive.
func startWithPauseGate(in io.Reader, out io.Writer, registryPath string, paused *atomic.Bool) {
	rl, err := newReadline(&readline.Config{
		Prompt:          PROMPT,
		HistoryFile:     historyFile(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdin:           toReadCloser(in),
		Stdout:          out,
	})
	if err != nil {
		fmt.Fprintf(out, "readline error: %s\n", err)
		return
	}
	defer rl.Close()

	// Shared registry so set/get state persists across REPL lines.
	registry, regErr := newRegistry()
	if regErr != nil {
		fmt.Fprintf(out, "init error: %s\n", regErr)
		return
	}
	registry.SetParseFunc(parser.Parse)

	um := udk.NewUniversalManager(map[string]any{
		"registry": registryPath,
	})
	registry.Manager = um

	registry.Output = out

	// One *Boru per session over the persistent registry: each line runs
	// COMPILED-BY-DEFAULT (RunAutoValues — the same CompileTry semantics as
	// `boru run`), and a refused line re-runs on the interpreter —
	// containment for a compile failure, never a fallback the design
	// leans on. Check-pass def/import effects persist across lines on the
	// compiled path by SnapshotForCompile's keep-on-compile contract;
	// fallback lines interpret against the same registry, so state
	// persistence is unchanged either way (plan Phase 2).
	boruInst, boruErr := langNewFromRegistry(registry)
	if boruErr != nil {
		fmt.Fprintf(out, "init error: %s\n", boruErr)
		return
	}

	meta := NewMetaRegistry()
	var lastStack []native.Value
	// Diagnostics render with the ANSI palette when out is a real
	// terminal (NO_COLOR honored); the plain rendering is byte-identical
	// to the historical output.
	color := native.ResolveColor(boruInst.NativeRegistry(), out, "auto")
	renderErr := func(err error) string {
		return renderREPLError(err, color)
	}

	for {
		line, err := rl.Readline()
		if err != nil { // EOF or interrupt
			return
		}

		if line == "" {
			continue
		}

		// `exit` and `quit` are extra REPL words that shut down the loop,
		// matching the EOF behaviour without needing Ctrl-D. Checked BEFORE
		// the pause gate so a paused Service can still be exited from stdin
		// (the gate would otherwise swallow the line as ignored input).
		if isExitWord(line) {
			return
		}

		if paused != nil && paused.Load() {
			fmt.Fprintln(out, "  (paused — input ignored)")
			continue
		}

		// Check for meta commands (/help, /stack, etc.).
		handled, metaErr := meta.ParseAndRun(line, &MetaContext{
			Out:      out,
			Registry: registry,
			Stack:    lastStack,
		})
		if handled {
			if metaErr != nil {
				fmt.Fprintf(out, "  error: %s\n", metaErr)
			}
			continue
		}

		// The parse probe keeps the historical "parse error:" prefix for
		// malformed lines (RunAutoValues would fold a parse failure into the
		// generic error arm); a well-formed line then runs compiled-by-default.
		if _, perr := parser.Parse(line); perr != nil {
			fmt.Fprintf(out, "  parse error: %s\n", renderErr(perr))
			continue
		}

		// One outcome, the same as every other surface: the line compiles
		// and runs, or it does not compile and the REPL prints the failure.
		// This used to re-run the line on the interpreter, silently — the
		// argument being that an interactive line's performance debt was not
		// worth a per-line warning. It was never about performance: the line
		// had hit a compiler defect and nothing said so, and a REPL is where
		// a user is most likely to be the one who can report it.
		result, _, _, err := boruInst.RunAutoValues(line)
		if err != nil {
			// `IO.exit N` from a REPL line ENDS THE SESSION, the same way
			// the bare `exit` word does — the interactive analogue of a
			// program terminating. The code is reported rather than
			// returned: a REPL's own exit status is the session's, not any
			// one line's, and swallowing it silently would make an
			// interactive `IO.exit 3` indistinguishable from `IO.exit 0`.
			if code, isExit := lang.ExitCode(err); isExit {
				fmt.Fprintf(out, "  exit %d\n", code)
				return
			}
			fmt.Fprintf(out, "  error: %s\n", renderErr(err))
			continue
		}

		lastStack = result

		if len(result) > 0 {
			parts := make([]string, len(result))
			for i, v := range result {
				parts[i] = v.String()
			}
			fmt.Fprintln(out, strings.Join(parts, " "))
		}
	}
}

// isExitWord reports whether a REPL line is one of the shutdown words
// (`exit` or `quit`), ignoring surrounding whitespace.
func isExitWord(line string) bool {
	switch strings.TrimSpace(line) {
	case "exit", "quit":
		return true
	default:
		return false
	}
}

// userHomeDir is a package-level var for testability.
var userHomeDir = os.UserHomeDir

func historyFile() string {
	home, err := userHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".boru_history")
}

// toReadCloser wraps an io.Reader in an io.ReadCloser if needed.
func toReadCloser(r io.Reader) io.ReadCloser {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc
	}
	return io.NopCloser(r)
}

// renderREPLError formats an error for the REPL: a structured BoruError
// re-renders through the diagnostic renderer with the ANSI palette when
// color is on; everything else (and the color-off path) keeps the plain
// Error() text.
func renderREPLError(err error, color bool) string {
	var ae *native.BoruError
	if color && errors.As(err, &ae) {
		return ae.Render(native.RenderOpts{Color: true})
	}
	return err.Error()
}
