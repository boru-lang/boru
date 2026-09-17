// Package check implements
// `boru check [flags] <file.boru|dir ...>` and `boru check [flags] -e EXPR`
// — run the static type-checker over boru source and report diagnostics.
//
// EVERY target named on the command line is checked. A file target is
// checked as named; a directory target contributes every `.boru` file
// beneath it (skipping `.boru/`). Diagnostics accumulate across targets —
// a file that fails does not stop the files after it — and the command
// exits non-zero when ANY target reported an Error-severity diagnostic,
// so `boru check src/*.boru` in CI is green only when every one of those
// files is clean.
//
// Without --soft, the presence of any Error-severity diagnostic
// causes the command to exit non-zero (the default mode used by
// CI). --soft downgrades every diagnostic to advisory.
package check

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boru-lang/boru/cmd/go/internal/command"
	"github.com/boru-lang/boru/cmd/go/internal/flagutil"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	lang "github.com/boru-lang/boru/lang/go"
)

// langNew is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive the init-error arms of Emit/Run/Preflight — lang.New only fails
// on registry construction errors that no Opts value can provoke.
var langNew = lang.New

// jsonMarshalIndent is a test seam (design/TEST-SEAMS.10.md); tests swap
// it to drive Run's marshal-error arm, which is unreachable with the
// plain CheckResult shape.
var jsonMarshalIndent = json.MarshalIndent

// osReadFile and walkDir are test seams (design/TEST-SEAMS.10.md): under
// the suite's root uid a path os.Stat has just resolved cannot then fail
// to read, and filepath.WalkDir never surfaces a per-entry error, so
// tests swap these to drive the two I/O failure arms of target
// resolution.
var (
	osReadFile = os.ReadFile
	walkDir    = filepath.WalkDir
)

type cmd struct{}

// New returns the check subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "check" }
func (*cmd) Synopsis() string { return "static type-check scripts or an expression" }
func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	return RunCLI(args, stdout, stderr)
}

// Opts are the check knobs RunCLI resolves once and every checked
// target shares.
type Opts struct {
	Registry string
	Seed     int64
	JSON     bool
	Soft     bool
	Strict   bool
	// Pedantic promotes the advisory tiers: with it set, warning- and
	// info-severity diagnostics gate the exit code the way errors
	// already do. It composes UNDER Soft — `--soft` still means "never
	// gate", so `--soft --pedantic` exits 0 — which keeps each flag's
	// documented meaning intact (design/ROC-ADOPTION-PLAN.0.md, A3).
	Pedantic bool
	Color    bool
}

// Target is one unit of work: Source is the boru text and Path is the
// file it was read from. Path is empty for an -e expression (and for a
// library caller that only has the text), which is also what selects
// the import anchor — see RunTargets.
type Target struct {
	Path   string
	Source string
}

// RunCLI is the entry point for the check subcommand, parsing flags
// from args.
func RunCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	expr := fs.String("e", "", "type-check this inline expression instead of file targets")
	jsonOut := fs.Bool("json", false, "emit the CheckResult as JSON (an array keyed by file for several targets)")
	soft := fs.Bool("soft", false, "report diagnostics but still exit 0")
	strict := fs.Bool("strict", false, "additionally report every dispatch over a dynamic operand")
	pedantic := fs.Bool("pedantic", false, "gate on the advisory tiers too, not only on errors (composes under --soft)")
	emit := fs.Bool("emit", false, "print the bytecode disassembly instead of the check report")
	colorMode := fs.String("color", "auto", "colorize diagnostics: auto|always|never")
	registry := fs.String("r", "", "registry path")
	var seed int64
	fs.Int64Var(&seed, "s", 0, "random seed")

	// -h is answered before parsing: flag.ContinueOnError would print its
	// own flag dump and return ErrHelp, which this command then reports as
	// exit 1 — the very defect NUR084 records. printUsage is the contract
	// `boru help check` points the reader at, and asking for help is not
	// an error, so it exits 0 (main's behaviour, preserved through the
	// FlagSet rewrite).
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printUsage(stdout)
			return 0
		}
	}

	args, ok := tolerateLegacyTail(args, stderr)
	if !ok {
		return 1
	}
	// Interleaved parsing (flagutil.ParseInterleaved) so `check a.boru
	// b.boru --json` reads two files and a flag, not three files.
	targets, perr := flagutil.ParseInterleaved(fs, args)
	if perr != nil {
		// The FlagSet has already written the usage listing (-h) or the
		// rejected-flag message to stderr; adding our own would double it.
		return 1
	}

	// -e "" is a legal (empty) program, so presence — not emptiness —
	// decides whether an expression was asked for.
	sawExpr := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "e" {
			sawExpr = true
		}
	})
	switch {
	case sawExpr && len(targets) > 0:
		fmt.Fprintf(stderr, "error: boru check takes either -e EXPR or file targets, not both\n")
		return 1
	case !sawExpr && len(targets) == 0:
		fmt.Fprintf(stderr, "error: boru check requires a script file or -e expression\n")
		return 1
	}

	work := []Target{{Source: *expr}}
	if !sawExpr {
		files, err := resolveTargets(targets)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if len(files) == 0 {
			// Checking nothing is the failure mode this command exists to
			// prevent, so an empty expansion is an error, not a quiet 0.
			fmt.Fprintf(stderr, "error: no .boru files under %s\n", strings.Join(targets, " "))
			return 1
		}
		work = files
	}

	if *emit {
		return runEmit(stdout, stderr, work, *registry, seed)
	}
	opts := Opts{
		Registry: *registry,
		Seed:     seed,
		JSON:     *jsonOut,
		Soft:     *soft,
		Strict:   *strict,
		Pedantic: *pedantic,
		Color:    lang.ResolveColor(nil, stderr, *colorMode),
	}
	if err := RunTargets(stdout, stderr, work, opts); err != nil {
		fmt.Fprintf(stderr, "%s\n", err)
		return 1
	}
	return 0
}

// tolerateLegacyTail preserves the two tolerances the hand-rolled
// pre-FlagSet argument loop had, which a bare FlagSet would answer with
// a different message:
//
//   - a trailing bare `-e` reported "boru check -e requires an
//     expression"; flag would say "flag needs an argument: -e";
//   - a trailing bare `--color` was ignored (the mode stayed "auto") and
//     the run then failed for the missing target; flag would reject it.
//
// It returns false when the command should exit 1 with the message it
// has already written.
func tolerateLegacyTail(args []string, stderr io.Writer) ([]string, bool) {
	if len(args) == 0 {
		return args, true
	}
	switch args[len(args)-1] {
	case "-e", "--e":
		fmt.Fprintf(stderr, "error: boru check -e requires an expression\n")
		return nil, false
	case "-color", "--color":
		return args[:len(args)-1], true
	}
	return args, true
}

// resolveTargets expands the command-line targets into the files to
// check and reads each one.
//
// A file target is taken as named. A directory target contributes every
// `.boru` file beneath it, in sorted order, with `.boru/` never walked:
// that directory holds fetched and generated package artifacts, so
// checking it reports findings about code the user did not write and
// cannot fix (`boru fmt` skips it for the same reason; `boru test`'s
// discover does not — NUR082). A path named twice is checked once.
//
// Resolution completes BEFORE any file is checked, so a mistyped target
// fails the invocation with nothing checked rather than surfacing
// halfway through a report that already looks like a clean run.
func resolveTargets(args []string) ([]Target, error) {
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	for _, a := range args {
		// Expand a leading ~ the shell left verbatim (e.g. a quoted path).
		t := pathutil.Expand(a)
		info, err := os.Stat(t)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(t)
			continue
		}
		var found []string
		if err := walkDir(t, func(path string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() {
				if d.Name() == ".boru" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".boru") {
				found = append(found, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		sort.Strings(found)
		for _, f := range found {
			add(f)
		}
	}
	out := make([]Target, 0, len(paths))
	for _, p := range paths {
		data, err := osReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, Target{Path: p, Source: string(data)})
	}
	return out, nil
}

// runEmit prints the bytecode disassembly for every target. With more
// than one target each block is introduced by a `; file:` comment line,
// which the disassembly's own comment syntax makes harmless to a reader
// or a downstream tool.
func runEmit(stdout, stderr io.Writer, targets []Target, registry string, seed int64) int {
	for _, t := range targets {
		if len(targets) > 1 {
			fmt.Fprintf(stdout, "; file: %s\n", t.Path)
		}
		if err := EmitAt(stdout, stderr, t.Source, registry, seed, anchorOf(t.Path)); err != nil {
			fmt.Fprintf(stderr, "%s\n", err)
			return 1
		}
	}
	return 0
}

// anchorOf is the directory a target's relative file imports resolve
// against: the file's own directory, or "" (the process cwd) for source
// that did not come from a file.
//
// The result must be ABSOLUTE. It is assigned to the registry's BaseDir,
// and `resolveBareModule` walks parents from there looking for a `.boru/`
// package directory; that walk terminates when `filepath.Dir(dir) == dir`,
// which a relative anchor reaches immediately. A relative anchor therefore
// truncates the search and the checker reports fabricated `undefined_word`
// errors against perfectly good code — the worst failure class for a gate
// people run in CI. `build.go` anchors absolutely for the same reason.
//
// Falling back to the relative directory when Abs fails keeps the previous
// behaviour rather than dropping the anchor entirely: Abs only fails when
// the cwd is unavailable, in which case a relative anchor is no worse than
// the empty one.
func anchorOf(path string) string {
	if path == "" {
		return ""
	}
	dir := filepath.Dir(path)
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// printUsage writes the subcommand's flag set. `boru help check` points
// the reader at `boru check -h` for exactly this, so a flag that is not
// listed here is not discoverable through the documented path — the
// reason `--pedantic` arrived with it (design/ROC-ADOPTION-PLAN.0.md, A3).
// `run`, `do`, `test` and `build` get this free from flag.FlagSet; check
// parses argv by hand because `-e EXPR` stops the scan, so it spells the
// same contract out. Keep in step with CLI.md's `boru check` flag list.
func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: boru check [options] <script.boru>
       boru check [options] -e EXPR

Type-check without running. Exits non-zero when any Error-severity
diagnostic is reported.

Options:
  -e EXPR        type-check an inline expression
  --json         emit the full CheckResult as JSON on stdout
  --soft         never gate: exit 0 even when diagnostics are reported
  --strict       also report every dispatch over a dynamic operand (info)
  --pedantic     gate on the advisory tiers too, not only on errors;
                 composes under --soft, so --soft --pedantic exits 0
  --emit         print the bytecode disassembly instead of checking
  --color MODE   auto (default), always, or never
  -r PATH        registry path
  -s SEED        random seed
  -h, --help     print this message
`)
}

// Emit runs the bytecode recording pass over source and prints the
// Program disassembly to stdout, or the precise refusal reason when
// the emitter cannot lower the program (debug/tooling surface —
// design/legacy/boru-bytecode-plan.0.ignore, Stage 1 gate and the DX section).
func Emit(stdout, stderr io.Writer, source string) error {
	return EmitAt(stdout, stderr, source, "", 0, "")
}

// EmitAt is Emit with the registry, seed, and relative-import anchor
// resolved by the caller — the same anchoring PreflightColorAt does, so
// `boru check --emit src/prog.boru` records the program its own
// directory describes. An empty baseDir keeps the process cwd.
func EmitAt(stdout, stderr io.Writer, source, registry string, seed int64, baseDir string) error {
	a, err := langNew(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}
	if baseDir != "" {
		a.NativeRegistry().BaseDir = baseDir
	}
	prog, reason, res, err := a.CompileCheck(source)
	for i := range res.Diagnostics {
		d := &res.Diagnostics[i]
		fmt.Fprintf(stderr, "check: %s [%s] %s: %s\n", atPos(d.Row, d.Col), d.Severity, d.Code, d.Detail)
	}
	if err != nil {
		return fmt.Errorf("error: %s", err)
	}
	if prog == nil {
		fmt.Fprintf(stdout, "uncompilable: %s\n", reason)
		writeSiteReport(stdout, res.SiteCounts, nil)
		return nil
	}
	fmt.Fprint(stdout, prog.Disassemble())
	islands := make([]string, len(prog.Fallbacks))
	for i, fb := range prog.Fallbacks {
		islands[i] = fb.Desc
	}
	writeSiteReport(stdout, res.SiteCounts, islands)
	return nil
}

// writeSiteReport prints the compile report (design/legacy/boru-bytecode-plan.0.ignore
// DX section): the per-class dispatch-site tally that answers "why didn't
// this compile to a single path?", plus the interpreter islands a
// compiled program falls back into for each fallback span.
func writeSiteReport(w io.Writer, counts map[string]int, islands []string) {
	if len(counts) > 0 {
		fmt.Fprintf(w, "; sites: mono=%d poly=%d dynamic=%d meta=%d\n",
			counts["mono"], counts["poly"], counts["dynamic"], counts["meta"])
	}
	if len(islands) > 0 {
		fmt.Fprintf(w, "; islands: %s\n", strings.Join(islands, ", "))
	}
}

func atPos(row, col int) string {
	if row == 0 {
		return "-"
	}
	return fmt.Sprintf("%d:%d", row, col)
}

// Run executes the static type-checker over source and writes the
// carrier stack and diagnostics to the provided writers. It returns
// an error for a parse/execution failure; diagnostics on their own
// do not fail the run (they're printed to stderr).
//
// When jsonOut is true, the entire CheckResult is emitted to stdout
// as a single JSON object suitable for editor / tooling integration.
//
// When soft is false (the default), any Error-severity diagnostic
// causes a non-nil error to be returned so the caller propagates a
// non-zero exit code. Passing soft=true downgrades every diagnostic
// to advisory: Run returns nil as long as the underlying analysis
// completes.
func Run(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft, strict bool) error {
	return RunColor(stdout, stderr, source, registry, seed, jsonOut, soft, strict, false)
}

// RunColor is Run with the color decision resolved by the caller
// (lang.ResolveColor); the rich per-diagnostic blocks render through
// the shared diagnostic renderer either way. Source that did not come
// from a named file resolves its relative imports against the process
// cwd — RunTargets with a Target whose Path is set anchors instead.
func RunColor(stdout, stderr io.Writer, source, registry string, seed int64, jsonOut, soft, strict, color bool) error {
	return RunTargets(stdout, stderr, []Target{{Source: source}}, Opts{
		Registry: registry, Seed: seed, JSON: jsonOut, Soft: soft, Strict: strict, Color: color,
	})
}

// fileResult is one element of the JSON array a multi-target check
// emits. The single-target form still emits the bare CheckResult object
// tools already parse.
type fileResult struct {
	File   string           `json:"file"`
	Result lang.CheckResult `json:"result"`
	Error  string           `json:"error,omitempty"`
}

// RunTargets checks every target and writes the report.
//
// Each target is analysed on its OWN engine instance with its relative
// file imports anchored to its own directory — the same anchoring
// PreflightColorAt does for `boru build` (NUR044). A multi-file check
// has no single cwd that could be right for every target, so the
// question each file is asked is the one its own neighbours answer.
//
// Diagnostics accumulate: analysis continues past a failing target, and
// the returned error reports how many targets failed. With one target
// the output and the error text are exactly the single-file report the
// command has always produced.
func RunTargets(stdout, stderr io.Writer, targets []Target, o Opts) error {
	if len(targets) == 0 {
		// Nothing to check is not a failure for a library caller; the CLI
		// refuses an empty target expansion before it gets here.
		return nil
	}
	multi := len(targets) > 1
	var total lang.CheckSummary
	var reports []fileResult
	failed := 0
	var firstFail error

	for _, t := range targets {
		a, err := langNew(lang.Options{Registry: o.Registry, Seed: o.Seed})
		if err != nil {
			return fmt.Errorf("init error: %s", err)
		}
		if dir := anchorOf(t.Path); dir != "" {
			a.NativeRegistry().BaseDir = dir
		}
		if o.Strict {
			a.SetStrictCheck(true)
		}
		res, cerr := a.Check(t.Source)
		total.Errors += res.Summary.Errors
		total.Warnings += res.Summary.Warnings
		total.Infos += res.Summary.Infos

		fail := failureOf(res, cerr, o)
		if fail != nil {
			failed++
			if firstFail == nil {
				firstFail = fail
			}
		}

		if o.JSON {
			reports = append(reports, fileResult{File: t.Path, Result: res, Error: errText(cerr)})
			continue
		}
		if multi {
			fmt.Fprintf(stderr, "check: file %s\n", t.Path)
		}
		printDiagnostics(stderr, res.Diagnostics, t.Source, o.Color)
		if cerr != nil {
			// A failed analysis has no trustworthy summary or carrier
			// stack, so neither is printed. With one target the caller
			// prints the returned error; with several it goes out here,
			// under the file's own banner, and the run continues.
			if multi {
				fmt.Fprintf(stderr, "check: %s\n", fail)
			}
			continue
		}
		fmt.Fprintf(stderr, "check: %d error(s), %d warning(s), %d info\n",
			res.Summary.Errors, res.Summary.Warnings, res.Summary.Infos)
		printStack(stdout, t.Path, res.Stack, multi)
	}

	if o.JSON {
		if err := writeJSON(stdout, reports, multi); err != nil {
			return err
		}
	} else if multi {
		fmt.Fprintf(stderr, "check: total %d error(s), %d warning(s), %d info across %d file(s)\n",
			total.Errors, total.Warnings, total.Infos, len(targets))
	}

	if failed == 0 {
		return nil
	}
	if multi {
		return fmt.Errorf("check failed: %d of %d file(s)", failed, len(targets))
	}
	return firstFail
}

// failureOf reports the error that should fail a target's check, or nil
// when the target is acceptable: an analysis error always fails, and
// Error-severity diagnostics fail unless --soft downgraded them.
// gates reports whether sum should drive a non-zero exit, returning the error
// that names why. Soft short-circuits every tier; otherwise errors
// always gate and the advisory tiers gate only under Pedantic. Shared by
// the JSON and text paths so `--json --pedantic` cannot disagree with
// `--pedantic`.
func (o Opts) gates(sum lang.CheckSummary) error {
	if o.Soft {
		return nil
	}
	if sum.Errors > 0 {
		return fmt.Errorf("check failed: %d error(s)", sum.Errors)
	}
	if o.Pedantic && sum.Warnings+sum.Infos > 0 {
		return fmt.Errorf("check failed: %d warning(s), %d info (--pedantic)",
			sum.Warnings, sum.Infos)
	}
	return nil
}

// failureOf is gates plus the analysis-error arm, applied per target. It
// delegates so the multi-file path and `gates` can never drift apart.
func failureOf(res lang.CheckResult, cerr error, o Opts) error {
	if cerr != nil {
		return fmt.Errorf("check error: %s", cerr)
	}
	return o.gates(res.Summary)
}

// writeJSON emits the machine-readable report: the bare CheckResult
// object for a single target (the shape editors and CI already parse),
// an array of {file, result} objects when several were checked.
func writeJSON(stdout io.Writer, reports []fileResult, multi bool) error {
	var payload any = reports
	if !multi {
		payload = reports[0].Result
	}
	out, jerr := jsonMarshalIndent(payload, "", "  ")
	if jerr != nil {
		return fmt.Errorf("json marshal: %s", jerr)
	}
	fmt.Fprintln(stdout, string(out))
	return nil
}

// printStack writes a target's residual carrier stack to stdout. With
// several targets the line carries the file it belongs to; with one it
// is the bare `check: ...` line the command has always printed.
func printStack(stdout io.Writer, path string, stack []string, multi bool) {
	prefix := "check: "
	if multi {
		prefix = "check: " + path + ": "
	}
	if len(stack) > 0 {
		fmt.Fprintln(stdout, prefix+strings.Join(stack, " "))
	} else {
		fmt.Fprintln(stdout, prefix+"(empty stack)")
	}
}

// errText renders err for the JSON report, or "" when there is none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// printDiagnostics writes each diagnostic to w in the `check: row:col:
// [sev] code: detail` form shared by Run and Preflight. The one-liner
// is a parsing contract (editors and CI scripts key on it); the RICH
// block underneath — source excerpt, notes, suggestions — is additive
// (design/DIAGNOSTICS.0.md, phase 6).
func printDiagnostics(w io.Writer, diags []lang.CheckDiagnostic, source string, color bool) {
	for _, d := range diags {
		sev := string(d.Severity)
		if sev == "" {
			sev = "info"
		}
		if d.Row > 0 {
			fmt.Fprintf(w, "check: %d:%d: [%s] %s: %s\n", d.Row, d.Col, sev, d.Code, d.Detail)
		} else {
			fmt.Fprintf(w, "check: [%s] %s: %s\n", sev, d.Code, d.Detail)
		}
		if block := lang.RenderCheckDiagnostic(d, source, "", lang.RenderOpts{Color: color}); block != "" {
			fmt.Fprintln(w, block)
		}
	}
}

// RunWith is Run with every option carried in a struct. It is the
// single-source spelling of RunTargets, kept because it is the shape the
// `--pedantic` suite drives.
func RunWith(stdout, stderr io.Writer, source string, opts Opts) error {
	return RunTargets(stdout, stderr, []Target{{Source: source}}, opts)
}

// Preflight runs the static checker as a pre-execution gate for
// `boru run --check`: it prints any diagnostics to stderr and returns a
// non-nil error when an Error-severity diagnostic is present, so the
// caller aborts before executing. Unlike Run it prints no summary or
// result-stack line — stdout is left entirely for the program.
func Preflight(stderr io.Writer, source, registry string, seed int64, verbose bool) error {
	return PreflightColor(stderr, source, registry, seed, verbose, lang.ResolveColor(nil, stderr, "auto"))
}

// PreflightColor is Preflight with the color decision resolved by the
// caller (the run subcommand's --color flag). Relative file imports
// resolve against the process cwd — exactly how the run that follows
// will resolve them.
func PreflightColor(stderr io.Writer, source, registry string, seed int64, verbose, color bool) error {
	return PreflightColorAt(stderr, source, registry, seed, verbose, color, "")
}

// PreflightColorAt is PreflightColor with relative file imports anchored
// to baseDir instead of the process cwd. `boru build` (NUR044) needs the
// anchor: a built binary resolves `import "./lib.boru"` against the
// build-time entry directory (buildrt.Main sets NativeRegistry().BaseDir
// to cfg.EntryDir), so its pre-flight must ask the same question or a
// perfectly buildable multi-file program invoked from a foreign cwd
// would be refused. An empty baseDir keeps the cwd behaviour run/debug
// want — for them, cwd IS how the subsequent execution resolves imports.
func PreflightColorAt(stderr io.Writer, source, registry string, seed int64, verbose, color bool, baseDir string) error {
	a, err := langNew(lang.Options{Registry: registry, Seed: seed})
	if err != nil {
		return fmt.Errorf("init error: %s", err)
	}
	if baseDir != "" {
		a.NativeRegistry().BaseDir = baseDir
	}
	res, cerr := a.Check(source)
	// Quiet by default (check-by-default runs this on EVERY execution):
	// diagnostics surface only when the run is about to abort, or when
	// the caller asked for the verbose pre-flight (--check).
	if verbose || cerr != nil || res.Summary.Errors > 0 {
		printDiagnostics(stderr, res.Diagnostics, source, color)
	}
	if cerr != nil {
		return fmt.Errorf("check error: %s", cerr)
	}
	if res.Summary.Errors > 0 {
		return fmt.Errorf("check failed: %d error(s)", res.Summary.Errors)
	}
	return nil
}
