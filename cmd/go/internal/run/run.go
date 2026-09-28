// Package run is both the explicit `boru run` subcommand and the
// fallback path the top-level dispatcher takes when no recognised
// subcommand matches: parse the legacy flags (-e, -r, -s, --check,
// -version), and either execute a one-shot script/-e expression or
// drop into the REPL when there is nothing to execute.
package run

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/buildrt"
	"github.com/boru-lang/boru/cmd/go/internal/check"
	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	"github.com/boru-lang/boru/cmd/go/internal/repl"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// Version is the boru CLI version string, populated by the top-level
// package via SetVersion before any Run call. Holding it here (rather
// than reading from cmd/go directly) keeps the import direction one-way:
// cmd/go → internal/run, never the other way.
var Version = "0.1.0-dev"

// SetVersion lets the top-level package inject its Version into the
// run subcommand so -version prints the right value.
func SetVersion(v string) { Version = v }

type cmd struct{}

// New returns the run subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "run" }
func (*cmd) Synopsis() string { return "execute a script or expression (or start the REPL)" }
func (*cmd) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return Execute(args, stdin, stdout, stderr)
}

// splitEvalTail finds the `-e` / `--e` eval flag and splits its trailing
// program arguments off the flag portion. When `-e` is present, `flagArgs`
// runs up to and including its expression token (so the FlagSet parses the
// flags and the expression normally) and `tail` is everything after — the
// program's own argv, which `-e` ends option processing for. A `--`
// separator seen BEFORE any `-e` means the `-e` is a positional, not the
// eval flag, so parsing stays normal. No `-e` (script mode, or REPL) →
// tail is nil and flagArgs is the whole slice.
//
// The scan stops at the first non-flag token — the script path. Every `-e`
// after it is the program's own argument, not this CLI's eval flag, so
// `boru run prog.boru a -e b c` must reach the program whole. Scanning past
// the script path split there instead and silently dropped everything after
// the false match (`c` in that example).
//
// Recognising the script path requires knowing which flags consume a
// following token: in `boru -s 5 -e …` the `5` is -s's value, not a
// positional. fs supplies that — a registered bool flag stands alone,
// anything else in the separated form (no `=`) takes the next token.
func splitEvalTail(fs *flag.FlagSet, args []string) (flagArgs, tail []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return args, nil
		case a == "-e" || a == "--e":
			end := i + 2 // the flag plus its expression token
			if end > len(args) {
				end = len(args) // dangling `-e`; the FlagSet reports it
			}
			return args[:end], args[end:]
		case strings.HasPrefix(a, "-e=") || strings.HasPrefix(a, "--e="):
			return args[:i+1], args[i+1:]
		case len(a) > 1 && a[0] == '-':
			if !strings.Contains(a, "=") && !isBoolFlag(fs, a) {
				i++ // skip this flag's value token
			}
		default:
			return args, nil // the script path: no eval split
		}
	}
	return args, nil
}

// isBoolFlag reports whether the `-x` / `--x` token names a flag the flag
// package parses without a following value. An unrecognised name is treated
// as value-taking; fs.Parse rejects it either way, so the split's guess
// cannot change the outcome.
func isBoolFlag(fs *flag.FlagSet, tok string) bool {
	f := fs.Lookup(strings.TrimLeft(tok, "-"))
	if f == nil {
		return false
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// Execute is the legacy CLI body. It owns the flag set for the
// no-subcommand invocation form (boru [-e expr] [script.boru]). When
// no source is provided it starts the REPL, preserving the original
// CLI's default behaviour.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("boru", flag.ContinueOnError)
	fs.SetOutput(stderr)

	evalExpr := fs.String("e", "", "evaluate expression")
	registry := fs.String("r", "", "registry path")
	seed := fs.Int64("s", 0, "random seed for ID generation (default: current time)")
	showVersion := fs.Bool("version", false, "print version and exit")
	checkFirst := fs.Bool("check", false, "verbose pre-flight: print ALL check diagnostics (the pre-flight itself runs by default; this flag adds the advisory tiers to stderr)")
	noCheck := fs.Bool("no-check", false, "skip the static pre-flight check before execution (also enabled by BORU_NO_CHECK)")
	optionsStr := fs.String("options", "", "engine options as jsonic (e.g. tape:initial:65536,tape:grows:9)")
	colorMode := fs.String("color", "auto", "diagnostic color: auto (terminal-only, honors NO_COLOR), always, never")
	compileReport := fs.Bool("compile-report", false, "after the run, print each runtime-constructed callback's stamp outcome (compiled to the VM, or the reason its body did not compile) to stderr")
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: boru [options] [script.boru]\n       boru do <words...>\n       boru check [file|dir ...]\n       boru test [--coverage] [file|dir ...]\n       boru help [subcommand]\n       boru describe [word|module]\n       boru fmt [file.boru ...]\n       boru build <prog.boru> [-o name]\n       boru prep [dir]\n       boru pack [dir]\n       boru clean [dir]\n       boru lsp [-p <port>]\n       boru exec [-bind host:port] [-p <port>] [-r <registry>]\n       boru registry -r <folder> -p <port>\n       boru serve <svc> [flags] [+ <svc> [flags]]...\n       boru ctl [--api url] [--token tok] <op> [name]\n       boru tui [--api url] [--token tok]\n       boru install <name>-x.y.z [-r <url>]\n       boru register [-r <url>]\n       boru login [-r <url>]\n       boru publish [-r <url>] [dir]\n\nOptions:\n")
		fs.PrintDefaults()
	}

	// `-e` ends option processing (the node -e / python -c convention): a
	// program's own arguments — including a dash-prefixed first one, e.g.
	// `boru -e '…' --fast` — must reach it via IO.args, not be swallowed by
	// this FlagSet. Go's flag package otherwise keeps parsing flags after
	// -e's expression (there is no non-flag positional to stop it), so the
	// dash arg errors out. Split the eval tail off before parsing; a
	// trailing script file already stops parsing naturally, so only -e
	// needs this.
	flagArgs, evalTail := splitEvalTail(fs, args)
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "boru %s\n", Version)
		return 0
	}

	// Expand a leading ~ the shell left verbatim (e.g. -r=~/reg or a
	// quoted "~/script.boru") in the registry path and the script path.
	reg := pathutil.Expand(*registry)

	var source string
	var hasSource bool
	var scriptArgs []string
	// baseDir anchors the script's relative imports at the script's own
	// directory, as `check` and `build` do; `-e` has no file and keeps the
	// process cwd (NUR083).
	var baseDir string

	if *evalExpr != "" {
		source = *evalExpr
		hasSource = true
		// -e mode: everything after the expression is a script argument
		// (IO.args), dash-prefixed or not. A single leading `--` — the
		// historical separator callers used to force the dash arg through —
		// is stripped so `boru -e … -- --fast` and `boru -e … --fast` agree.
		scriptArgs = evalTail
		if len(scriptArgs) > 0 && scriptArgs[0] == "--" {
			scriptArgs = scriptArgs[1:]
		}
	} else if fs.NArg() > 0 {
		filename := pathutil.Expand(fs.Arg(0))
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		source = string(data)
		hasSource = true
		// Positionals after the script path reach the program as IO.args.
		scriptArgs = fs.Args()[1:]
		if abs, aerr := filepath.Abs(filename); aerr == nil {
			baseDir = filepath.Dir(abs)
		}
	}

	if hasSource {
		// CHECK-BY-DEFAULT (completion plan Phase 5.2): every run
		// pre-flights unless --no-check / BORU_NO_CHECK. The default gate is
		// QUIET — diagnostics print only when an Error-severity finding
		// aborts the run, so a clean program's stderr stays empty and the
		// advisory tiers don't become per-run noise. An explicit --check
		// keeps the verbose behavior (all diagnostics, every severity).
		// Sequenced after the guard-narrowing legalization per the plan's
		// FP-honesty rule; `boru check --soft` remains the advisory surface.
		color := lang.ResolveColor(nil, stderr, *colorMode)
		// The policy resolves BEFORE the pre-flight: the check executes an
		// imported module's body, and it must run under the profile the run
		// does (NUR079).
		pol, err := pf.Resolve()
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if !*noCheck && os.Getenv("BORU_NO_CHECK") == "" {
			if err := check.PreflightPolicyAt(stderr, source, reg, *seed, *checkFirst, color, baseDir, pol); err != nil {
				fmt.Fprintf(stderr, "%s\n", err)
				return 1
			}
		}
		o := lang.Options{Registry: reg, Seed: *seed, Policy: pol, ScriptArgs: scriptArgs, BaseDir: baseDir,
			// The CLI is a host that hands the program the real environment
			// and the real answer about its own streams.
			Env: capabilities.OSEnvOps{}, Streams: capabilities.OSStreamProbe{}}
		if *optionsStr != "" {
			m, perr := lang.ParseOptions(*optionsStr)
			if perr != nil {
				fmt.Fprintf(stderr, "error: %s\n", perr)
				return 1
			}
			if aerr := lang.ApplyOptions(&o, m); aerr != nil {
				fmt.Fprintf(stderr, "error: %s\n", aerr)
				return 1
			}
		}
		var report io.Writer
		if *compileReport {
			report = stderr
		}
		if err := buildrt.EvalReport(stdout, report, stderr, source, o, color); err != nil {
			// `IO.exit N` is a request, not a failure: exit with the code
			// the program asked for and print nothing, including for a
			// non-zero code (design/CLI-PROGRAMS.0.md §4).
			if code, isExit := lang.ExitCode(err); isExit {
				return code
			}
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
		return 0
	}

	// No source provided: drop into the REPL.
	fmt.Fprintf(stdout, "boru %s\n", Version)
	repl.Start(stdin, stdout, reg)
	return 0
}

// Eval runs source through lang.New(...).Run and writes the carrier
// stack to w. Exposed for the do subcommand, which builds source
// from positional args. Equivalent to EvalWithPolicy with a nil
// policy.
func Eval(w io.Writer, source string, registry string, seed int64) error {
	return EvalWithPolicy(w, source, registry, seed, nil)
}

// OptionsFor assembles the lang.Options the legacy positional CLI
// arguments map to. Exposed for sibling subcommands (do) so they
// don't import lang directly.
func OptionsFor(registry string, seed int64, pol lang.Policy) lang.Options {
	// The terminal probe rides along, so `boru do` and `boru test` answer
	// IO.is-tty honestly rather than always false. Redirected streams answer
	// false anyway — OSStreamProbe inspects the endpoint the runtime holds.
	return lang.Options{Registry: registry, Seed: seed, Policy: pol,
		Streams: capabilities.OSStreamProbe{}}
}

// EvalWithPolicy is Eval with an explicit Policy. Pass nil for pol
// to preserve the historical default (no checks).
func EvalWithPolicy(w io.Writer, source string, registry string, seed int64, pol lang.Policy) error {
	return EvalOptions(w, source, lang.Options{Registry: registry, Seed: seed, Policy: pol})
}

// EvalOptions runs source under the full Options set (registry, seed,
// policy, tape bounds). The CLI builds Options from its flags —
// including --options — and calls this. There is one engine: the program
// compiles and its bytecode runs, or it does not compile and that is an
// error.
func EvalOptions(w io.Writer, source string, o lang.Options) error {
	return buildrt.Eval(w, source, o)
}

// EvalOptionsColor is EvalOptions with the caller-resolved color decision
// for structured error rendering (the --color flag).
func EvalOptionsColor(w io.Writer, source string, o lang.Options, color bool) error {
	return buildrt.EvalColor(w, source, o, color)
}
