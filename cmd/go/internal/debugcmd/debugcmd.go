// Package debugcmd implements the `boru debug` subcommand: the interactive
// debugger (design/BORU-DEBUGGER.0.md) and the front door to cross-process
// debugging (design/DEBUG-MODULE.0.md §7.2/§7.3).
//
//	boru debug [--script F] [--no-check] [--color M] <file.boru> [args...]
//	    Launch file.boru under the interactive debugger: pause between
//	    source lines, stop on Debug.break, inspect the stack / scope /
//	    backtrace, and evaluate expressions at a pause. --script reads
//	    debugger commands from a file (batch/CI mode). Runs on the
//	    interpreter (the trace does not fire on the compiled VM path).
//
//	boru debug serve [--bind 127.0.0.1:7777] [--token T] [file.boru]
//	    Load file.boru (if given) into a runtime, then serve its registry's
//	    debug introspection over HTTP and block until interrupted. Writes a
//	    discovery file ($TMPDIR/boru-debug.json) so `attach` can find it.
//
//	boru debug attach [--url U] [--token T] <words|defs|heap|eval|events> [arg]
//	    Connect to a running `boru debug serve` (via the discovery file or an
//	    explicit --url) and interrogate it.
//
// Serve/attach transport is plain HTTP with an optional Bearer token (a
// static token or a vault capability id), the same posture as the api
// service — the host-level realization of the attach/serverless surfaces
// while the boru Service model (SERVICES.0.md) and a language-level socket
// primitive are still RFC-only.
package debugcmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/check"
	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/debugger"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/debugserve"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// langNew is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive runServe's init-error arm — lang.New only fails on registry
// construction errors that nothing here can provoke.
var langNew = lang.New

// jsonMarshal is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive writeDiscovery's marshal-error arm, which is unreachable with
// the plain discovery shape.
var jsonMarshal = json.Marshal

type cmdImpl struct{}

// New returns the debug subcommand.
func New() command.Command { return &cmdImpl{} }

func (*cmdImpl) Name() string { return "debug" }
func (*cmdImpl) Synopsis() string {
	return "interactive debugger for a program; or serve/attach cross-process introspection"
}

func (*cmdImpl) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: boru debug [flags] <file.boru> | serve | attach ...")
		return 1
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "attach":
		return runAttach(args[1:], stdout, stderr)
	default:
		// Anything else is an interactive launch: boru debug [flags] <file.boru>
		// (design/BORU-DEBUGGER.0.md §4).
		return runLaunch(args, stdin, stdout, stderr)
	}
}

// runLaunch is the interactive-debugger entry: read + preflight the file
// exactly as `boru run` does, wire a registry, and run the program's
// tokens under a debugger.Session (design/BORU-DEBUGGER.0.md §4-§5).
func runLaunch(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	script := fs.String("script", "", "read debugger commands from this file instead of stdin (batch/CI mode)")
	noCheck := fs.Bool("no-check", false, "skip the static pre-flight check before debugging (also enabled by BORU_NO_CHECK)")
	colorMode := fs.String("color", "auto", "diagnostic color: auto (terminal-only, honors NO_COLOR), always, never")
	postMortem := fs.Bool("post-mortem", false, "on an uncaught error, open an inspection prompt over the fault state before exiting")
	breakOnError := fs.Bool("break-on-error", false, "pause at every raise BEFORE it unwinds — including errors a do-handler will catch")
	dapMode := fs.Bool("dap", false, "serve the Debug Adapter Protocol over stdio (for editors); the program still comes from the command line")
	var breaks []string
	fs.Func("break", "set a breakpoint before the run starts: a source line ('12', 'file:12') or a word name ('add'); repeatable", func(v string) error {
		breaks = append(breaks, v)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return 1
	}
	file := fs.Arg(0)
	if file == "" {
		fmt.Fprintln(stderr, "usage: boru debug [--script F] [--no-check] [--color M] <file.boru> [args...]")
		return 1
	}
	path := pathutil.Expand(file)
	// Relative imports resolve against the script's own directory, as
	// `check` and `build` anchor them — the pre-flight and the session's
	// registries alike (NUR083: `run` and `debug` used the process cwd).
	baseDir := ""
	if abs, aerr := filepath.Abs(path); aerr == nil {
		baseDir = filepath.Dir(abs)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "boru debug: read %s: %s\n", file, err)
		return 1
	}
	source := string(data)

	// Check-by-default, mirroring `boru run` (run.Execute): quiet gate,
	// --no-check / BORU_NO_CHECK to skip.
	// nil registry: the same call `boru run` makes at this point (run.go), and
	// for the same reason — the session's registry is not built until
	// newRegistry below, so there is no installed EnvOps to read NO_COLOR
	// from yet, and ResolveColor falls back to the process environment.
	color := lang.ResolveColor(nil, stderr, *colorMode)
	if !*noCheck && os.Getenv("BORU_NO_CHECK") == "" {
		if cerr := check.PreflightColorAt(stderr, source, "", 0, false, color, baseDir); cerr != nil {
			fmt.Fprintf(stderr, "%s\n", cerr)
			return 1
		}
	}

	// newRegistry builds a launch-wired registry: once for the session's
	// own run, and again for each `replay` (a fresh re-run needs a clean
	// slate with the same wiring — output, file identity, program args).
	newRegistry := func() (*native.Registry, error) {
		a, err := langNew()
		if err != nil {
			return nil, err
		}
		reg := a.NativeRegistry()
		reg.Output = stdout
		reg.BaseFile = path
		reg.BaseDir = baseDir
		if *script != "" {
			// Batch mode: commands come from the --script file, so the launch
			// stdin belongs entirely to the PROGRAM (IO.stdin). Interactive
			// mode leaves reg.Input alone — the prompt's reader and a
			// stdin-reading program cannot safely share one descriptor (the
			// prompt's Scanner reads ahead), a documented Phase-1 limit.
			reg.Input = stdin
		}
		if extra := fs.Args()[1:]; len(extra) > 0 {
			// Positionals after the script path reach the program as IO.args,
			// like `boru run`.
			native.SetHostScriptArgs(reg, extra)
		}
		return reg, nil
	}
	reg, err := newRegistry()
	if err != nil {
		fmt.Fprintf(stderr, "boru debug: init: %s\n", err)
		return 1
	}

	tokens, perr := parser.Parse(source)
	if perr != nil {
		fmt.Fprintf(stderr, "boru debug: parse %s: %s\n", file, perr)
		return 1
	}

	if *dapMode {
		// Editor mode: stdout carries DAP frames only — no banner, no
		// prompt, no residual prints. The adapter owns the session.
		if *script != "" {
			fmt.Fprintln(stderr, "boru debug: --dap and --script are mutually exclusive")
			return 1
		}
		return debugger.RunDAP(reg, debugger.Config{
			File: path, Source: source, BreakOnError: *breakOnError,
		}, tokens, source, stdin, stdout)
	}

	cmdIn := stdin
	if *script != "" {
		f, oerr := os.Open(pathutil.Expand(*script))
		if oerr != nil {
			fmt.Fprintf(stderr, "boru debug: script: %s\n", oerr)
			return 1
		}
		defer func() { _ = f.Close() }()
		cmdIn = f
	}

	sess := debugger.New(reg, debugger.Config{
		In:  cmdIn,
		Out: stdout,
		// The expanded path, matching reg.BaseFile, so pause locations
		// and error attribution name one canonical file string.
		File:         path,
		Source:       source,
		Echo:         *script != "",
		BreakOnError: *breakOnError,
		PostMortem:   *postMortem,
		NewRegistry:  newRegistry,
	})
	fmt.Fprintf(stdout, "boru debug: %s — type 'help' for commands\n", file)
	for _, b := range breaks {
		fmt.Fprintln(stdout, sess.SetBreak(b))
	}
	res, rerr := sess.RunProgram(tokens, source)
	if rerr != nil {
		if *postMortem {
			sess.PostMortem(rerr)
		}
		var ae *lang.BoruError
		if color && errors.As(rerr, &ae) {
			fmt.Fprintf(stderr, "error: %s\n", ae.Render(lang.RenderOpts{Color: true}))
		} else {
			fmt.Fprintf(stderr, "error: %s\n", rerr)
		}
		return 1
	}
	if len(res) > 0 {
		// Residuals render print-style (FormatForPrint), matching `boru run`
		// and the session's own stack/defs renderers.
		parts := make([]string, len(res))
		for i, v := range res {
			parts[i] = native.FormatForPrint(v)
		}
		fmt.Fprintln(stdout, strings.Join(parts, " "))
	}
	fmt.Fprintln(stdout, "(program exited)")
	return 0
}

// discoveryPath is where `serve` advertises its URL+token for `attach`.
func discoveryPath() string {
	return filepath.Join(os.TempDir(), "boru-debug.json")
}

type discovery struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bind := fs.String("bind", "127.0.0.1:7777", "loopback address to serve on")
	token := fs.String("token", "", "require this Bearer token (a static token or vault capability id)")
	allowPublic := fs.Bool("allow-public", false, "permit a non-loopback bind (exposes introspection to the network)")
	step := fs.Bool("step", false, "run the program under the REMOTE stepping debugger: it pauses at the first line awaiting `boru debug attach` actions")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	a, err := langNew()
	if err != nil {
		fmt.Fprintf(stderr, "debug serve: init: %s\n", err)
		return 1
	}
	reg := a.NativeRegistry()

	var rem *debugger.Remote
	if *step {
		// Stepping mode (BORU-DEBUGGER.0.md §8.3 Phase 4): the program
		// runs CONCURRENTLY with the server, under a session whose
		// pauses park awaiting remote actions. It starts paused at the
		// first line, so an attach client finds it waiting.
		file := fs.Arg(0)
		if file == "" {
			fmt.Fprintln(stderr, "usage: boru debug serve --step [--bind A] [--token T] <file.boru>")
			return 1
		}
		path := pathutil.Expand(file)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			fmt.Fprintf(stderr, "debug serve: read %s: %s\n", file, rerr)
			return 1
		}
		source := string(data)
		tokens, perr := parser.Parse(source)
		if perr != nil {
			fmt.Fprintf(stderr, "debug serve: parse %s: %s\n", file, perr)
			return 1
		}
		reg.Output = stdout
		reg.BaseFile = path
		sess := debugger.New(reg, debugger.Config{
			In: strings.NewReader(""), Out: io.Discard,
			File: path, Source: source,
		})
		rem = debugger.NewRemote(sess)
		go func() {
			_, runErr := sess.RunProgram(tokens, source)
			rem.Finish(runErr)
		}()
	} else if file := fs.Arg(0); file != "" {
		// Load the program (if given) so its defs populate the registry
		// that `attach` will introspect.
		src, rerr := os.ReadFile(file)
		if rerr != nil {
			fmt.Fprintf(stderr, "debug serve: read %s: %s\n", file, rerr)
			return 1
		}
		if _, runErr := a.Run(string(src)); runErr != nil {
			fmt.Fprintf(stderr, "debug serve: run %s: %s\n", file, runErr)
			return 1
		}
	}

	srv := debugserve.NewServer(reg, *token)
	handler := srv.Handler()
	if rem != nil {
		// The stepping routes mount in FRONT: /debug/eval is shadowed by
		// the session-aware eval (a raw registry eval would fire the
		// armed debug hook from the HTTP goroutine and block behind the
		// parked engine's session lock).
		mux := http.NewServeMux()
		rem.Register(mux, srv.Auth)
		mux.Handle("/", handler)
		handler = mux
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Advertise for `attach`. Best-effort; removed on shutdown.
	dp := discoveryPath()
	writeDiscovery(dp, *bind, *token)
	defer func() { _ = os.Remove(dp) }()

	fmt.Fprintf(stdout, "boru debug: serving introspection on http://%s (token: %v)\n", *bind, *token != "")
	if err := srv.ListenAndServeHandler(ctx, *bind, *allowPublic, handler); err != nil {
		fmt.Fprintf(stderr, "debug serve: %s\n", err)
		return 1
	}
	if rem != nil {
		// Release a parked engine with quit (detach) and DRAIN the run
		// before returning — side effects complete, exactly like the TTY
		// and DAP detach paths.
		rem.Shutdown()
		if e := rem.ExitError(); e != "" {
			fmt.Fprintf(stderr, "debug serve: program: %s\n", e)
			return 1
		}
	}
	return 0
}

func writeDiscovery(path, bind, token string) {
	d := discovery{URL: "http://" + bind, Token: token, PID: os.Getpid()}
	data, err := jsonMarshal(d)
	if err != nil {
		return
	}
	// Publish ATOMICALLY, the way api.writeDiscoveryFile already does for
	// $TMPDIR/boru-api.json — this function was the odd one out. Two reasons,
	// both real:
	//
	//  1. TEARING. A reader (`attach` below, and any supervisor waiting for
	//     serve to come up) polls for this path and reads it the moment it
	//     exists. os.WriteFile creates+truncates first and writes after, so
	//     the reader can stat a file that is still zero-length and fail with
	//     "unexpected end of JSON input". Reproducible under CPU load.
	//  2. SYMLINK TOCTOU. This file carries the bearer TOKEN and lives in a
	//     shared temp dir. os.WriteFile follows a pre-planted symlink at the
	//     target, writing the token wherever the link points. os.CreateTemp
	//     uses O_CREATE|O_EXCL with an unpredictable name, and rename
	//     replaces the link rather than following it.
	//
	// CreateTemp opens at 0600, so the mode the old WriteFile asked for is
	// preserved. Rename within one directory is atomic: a reader sees either
	// no file or the complete one, never a prefix.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".boru-debug-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	// Still best-effort, as before: every failure leaves NO discovery file
	// rather than a partial one, and serve carries on unadvertised.
	if werr != nil || cerr != nil || os.Rename(name, path) != nil {
		_ = os.Remove(name)
	}
}

func readDiscovery(path string) (url, token string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var d discovery
	if err := json.Unmarshal(data, &d); err != nil {
		return "", "", err
	}
	return d.URL, d.Token, nil
}

func runAttach(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("debug attach", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("url", "", "debug server URL (default: read from discovery file)")
	token := fs.String("token", "", "Bearer token (default: read from discovery file)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *url == "" {
		u, t, err := readDiscovery(discoveryPath())
		if err != nil {
			fmt.Fprintf(stderr, "debug attach: no --url and no discovery file: %s\n", err)
			return 1
		}
		*url = u
		if *token == "" {
			*token = t
		}
	}

	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "usage: boru debug attach [--url U] [--token T] "+
			"<words|defs|heap|eval SRC|events ID|pause|step|next|out|continue|quit|break SPEC|delete SPEC>")
		return 1
	}
	c := debugserve.NewClient(*url, *token)
	return attachVerb(c, rest, stdout, stderr)
}

// attachVerb dispatches one attach query. Split out so it is unit-testable
// against an in-process httptest server.
func attachVerb(c *debugserve.Client, rest []string, stdout, stderr io.Writer) int {
	switch rest[0] {
	case "words":
		words, err := c.Words()
		if err != nil {
			return fail(stderr, err)
		}
		for _, w := range words {
			fmt.Fprintln(stdout, w)
		}
	case "defs":
		defs, err := c.Defs()
		if err != nil {
			return fail(stderr, err)
		}
		names := make([]string, 0, len(defs))
		for n := range defs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(stdout, "%s = %s\n", n, defs[n])
		}
	case "heap":
		h, err := c.Heap()
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "alloc=%d total-alloc=%d heap-objects=%d num-gc=%d\n",
			h.Alloc, h.TotalAlloc, h.HeapObjects, h.NumGC)
	case "eval":
		src := strings.Join(rest[1:], " ")
		result, evalErr, err := c.Eval(src)
		if err != nil {
			return fail(stderr, err)
		}
		if evalErr != "" {
			fmt.Fprintf(stderr, "eval error: %s\n", evalErr)
			return 1
		}
		fmt.Fprintln(stdout, result)
	case "events":
		if len(rest) < 2 {
			fmt.Fprintln(stderr, "usage: boru debug attach events <invocation-id>")
			return 1
		}
		evs, err := c.Events(rest[1])
		if err != nil {
			return fail(stderr, err)
		}
		for _, e := range evs {
			fmt.Fprintf(stdout, "[%d] %s = %s\n", e.Seq, e.Label, e.Value)
		}
	case "pause":
		st, err := c.StepPause()
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, renderStepState(st))
	case "step", "next", "out", "continue":
		st, err := c.StepAction(rest[0])
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, renderStepState(waitForStop(c, st.Seq)))
	case "quit":
		if _, err := c.StepAction("quit"); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, "(detached — the program continues to completion)")
	case "break", "delete":
		if len(rest) < 2 {
			fmt.Fprintf(stderr, "usage: boru debug attach %s <line|word>\n", rest[0])
			return 1
		}
		result, err := c.StepBreak(rest[1], rest[0] == "delete")
		if err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintln(stdout, result)
	default:
		fmt.Fprintf(stderr, "boru debug attach: unknown query %q\n", rest[0])
		return 1
	}
	return 0
}

// stepPollDeadline / stepPollInterval bound waitForStop's polling; the
// deadline is a test seam (design/TEST-SEAMS.10.md).
var (
	stepPollDeadline = 5 * time.Second
	stepPollInterval = 20 * time.Millisecond
)

// waitForStop polls the remote pause state after an action until the
// program stops again (a NEW pause — seq beyond the delivered action's
// — or exit), or the deadline passes (the state then reads "running":
// a long computation, checkable again with `attach pause`).
func waitForStop(c *debugserve.Client, afterSeq int) debugserve.StepState {
	deadline := time.Now().Add(stepPollDeadline)
	for {
		st, err := c.StepPause()
		if err == nil && (st.State == "exited" || (st.State == "paused" && st.Seq > afterSeq)) {
			return st
		}
		if time.Now().After(deadline) {
			if err != nil {
				return debugserve.StepState{State: "running"}
			}
			return st
		}
		time.Sleep(stepPollInterval)
	}
}

// renderStepState renders one StepState as the attach CLI's answer —
// the remote twin of the TTY pause banner.
func renderStepState(st debugserve.StepState) string {
	switch st.State {
	case "paused":
		where := ""
		if st.InBody {
			where = " (in body)"
		}
		loc := fmt.Sprintf("at %s:%s%s  %s", st.File, rowLabel(st.Row), where, st.Line)
		head := loc
		if st.Detail != "" {
			head = st.Detail + " — " + loc
		}
		return "paused " + head + "\n  stack: " + st.Stack
	case "exited":
		if st.Exit != "" {
			return "exited: " + st.Exit
		}
		return "exited"
	}
	return st.State
}

// rowLabel renders a 1-based source row, "?" when unknown.
func rowLabel(row int) string {
	if row == 0 {
		return "?"
	}
	return fmt.Sprintf("%d", row)
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "debug attach: %s\n", err)
	return 1
}
