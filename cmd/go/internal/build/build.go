// Package build implements `boru build <prog.boru>` — compile a boru program
// into a standalone native executable.
//
// Two mechanisms produce the binary:
//
//   - Self-embedding launcher (default). Copy the running `boru` binary and
//     append the program (plus any bundled file imports and the baked engine
//     settings) as a payload. At startup the copy detects the payload and runs
//     it through the full interpreter (see cmd/go/main.go). No Go toolchain or
//     module resolution is needed, and it runs any program — at the cost of a
//     binary the size of `boru`, for the host OS/arch only.
//
//   - Native (`--native`). Generate a tiny main.go that embeds the program and
//     calls buildrt.Main, then invoke `go build`. Smaller, cross-compilable,
//     but needs the Go toolchain and the boru module graph (see native.go).
//
// Both paths bundle file imports (`import "./lib.boru"`) so the produced binary
// is self-contained; built-in `boru:` modules are already in the runtime and
// need no bundling.
package build

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
	"github.com/boru-lang/boru/cmd/go/internal/flagutil"
	"github.com/boru-lang/boru/cmd/go/internal/pathutil"
	"github.com/boru-lang/boru/cmd/go/internal/permsflags"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/policy"
)

type cmd struct{}

// New returns the build subcommand.
func New() command.Command { return &cmd{} }

func (*cmd) Name() string     { return "build" }
func (*cmd) Synopsis() string { return "compile a program into a standalone executable" }

func (*cmd) Run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output binary path (default: source basename without .boru)")
	native := fs.Bool("native", false, "use the Go toolchain (go build) instead of the self-embedding launcher")
	keep := fs.Bool("keep", false, "(native) retain the generated temp dir and print its path")
	registry := fs.String("r", "", "registry path baked into the binary")
	var seed int64
	fs.Int64Var(&seed, "s", 0, "random seed baked into the binary")
	optionsStr := fs.String("options", "", "engine options as jsonic, baked in (e.g. tape:initial:65536)")
	noCheck := fs.Bool("no-check", false, "skip the static pre-flight check (also: BORU_NO_CHECK=1)")
	// -perms/-allow/-deny bake the resolved policy into the binary, so a
	// shipped tool carries its author's declared permissions.
	var pf permsflags.Flags
	permsflags.Register(fs, &pf)

	// flag.Parse stops at the first non-flag token, so flags placed after the
	// script (the natural `boru build prog.boru -o prog` form) would be missed.
	// flagutil.ParseInterleaved re-parses after each positional so flags work
	// in any position; `boru check` reuses the same helper.
	positionals, err := flagutil.ParseInterleaved(fs, args)
	if err != nil {
		return 1
	}

	if len(positionals) != 1 {
		fmt.Fprintf(stderr, "error: boru build requires exactly one <prog.boru>\n")
		return 1
	}
	srcPath := pathutil.Expand(positionals[0])

	// Parse --options eagerly so a typo fails at build time, not when the
	// produced binary runs — and keep the parsed value: the compile preflight
	// below must answer the compile question under the SAME engine options
	// the artifact bakes (buildrt.Main applies this blob at launch). A
	// preflight on default options would pass a program that the baked
	// `steps:` ceiling makes uncompilable, and ship exactly the silent
	// fallback the gate exists to close (a Codex review of #471).
	preflightOpts := lang.Options{Registry: *registry, Seed: seed}
	if *optionsStr != "" {
		m, err := lang.ParseOptions(*optionsStr)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if err := lang.ApplyOptions(&preflightOpts, m); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
	}

	pol, err := pf.Resolve()
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	var prof *policy.Profile
	if pol != nil {
		prof = permsflags.ProfileFromPolicy(pol)
	}

	cfg, err := buildConfig(srcPath, *registry, seed, *optionsStr, prof)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// CHECK-BY-DEFAULT (NUR044): the same static pre-flight `boru run`
	// enforces, so `build` refuses to ship a binary whose first execution
	// would abort on a check error. -no-check / BORU_NO_CHECK=1 opt out,
	// mirroring run's escape hatch. The gate is quiet: diagnostics print
	// only when the build is about to be refused. Relative file imports
	// are anchored to cfg.EntryDir — the directory the BUILT binary will
	// resolve them against (buildrt.Main sets BaseDir to EntryDir) — not
	// the build-time cwd, so a multi-file program builds from anywhere.
	// Like `boru check`, the pre-flight executes imported file-module
	// bodies (with modelled writes) to learn their exports.
	checkSkipped := *noCheck || os.Getenv("BORU_NO_CHECK") != ""
	if !checkSkipped {
		color := lang.ResolveColor(nil, stderr, "auto")
		if cerr := check.PreflightColorAt(stderr, cfg.Source, *registry, seed, false, color, cfg.EntryDir); cerr != nil {
			fmt.Fprintf(stderr, "%s\n", cerr)
			return 1
		}
	}

	// COMPILE-BY-DEFAULT, ENFORCED AT BUILD TIME. The check gate above
	// refuses to ship a binary whose first execution would abort on a check
	// error; this is its compile-side twin, and it exists because the
	// asymmetry was a hole. A program the emitter could not lower used to
	// build silently: the baked try mode meant the shipped binary dropped to
	// the interpreter at run time and said nothing. The author shipped a
	// compile failure and was never told.
	//
	// Failure to compile is a failure (design/COMPILABLE-SUBSET.md §1), so
	// `build` names it and stops. There is no opt-out any more, because there
	// is no interpreter binary to opt into: the fallback that made one
	// possible is gone.
	{

		reason, cerr := compilePreflight(cfg.Source, preflightOpts, cfg.EntryDir)
		// -no-check / BORU_NO_CHECK opts out of being gated on the CHECKER, and
		// "check diagnostics" is the checker's verdict reaching the emitter as a
		// sentinel rather than a named construct. Refusing on it here would make
		// the compile gate a second check gate and defeat the opt-out, which
		// `build` documents as "must still produce the artefact". A genuine
		// construct refusal still stops the build either way.
		//
		// In the DEFAULT flow this carve-out is not a loophole: a program whose
		// checker findings are errors never reaches here, because the gate above
		// already returned 1. What reaches here with the sentinel has non-error
		// findings and still does not compile — a real defect, and the single
		// largest blocker for real programs (TestRealProgramsCompile).
		if checkSkipped && reason == "check diagnostics" {
			reason = ""
		}
		if cerr != nil {
			fmt.Fprintf(stderr, "error: %s\n", cerr)
			return 1
		} else if reason != "" {
			fmt.Fprintf(stderr, "error: bytecode compilation FAILED: %s\n", reason)
			fmt.Fprintf(stderr, "  A program that does not compile is an ERROR in need of fixing, not a\n")
			fmt.Fprintf(stderr, "  slower run: there is no interpreter for the binary to fall back to.\n")
			fmt.Fprintf(stderr, "  Fix the construct, or report it as the compiler defect it is.\n")
			return 1
		}
	}

	outPath := *out
	if outPath == "" {
		outPath = defaultOutput(srcPath)
	}

	if *native {
		if err := buildNative(cfg, outPath, *keep, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", outPath)
		return 0
	}

	if err := buildSelfEmbed(cfg, outPath); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", outPath)
	return 0
}

// compilePreflight reports the emitter's refusal reason for src, or "" when the
// whole program compiles. It COMPILES ONLY — CompileCheck runs the checker with
// the recording pass and linearises the trace; it never executes the program, so
// a build cannot trigger the program's side effects. A non-nil error is an
// init/parse failure, which is distinct from a refusal: the first means we could
// not answer the question, the second is the answer.
func compilePreflight(source string, o lang.Options, baseDir string) (string, error) {
	a, err := lang.New(o)
	if err != nil { //covergate:allow lang.New returns a non-nil error only for an unreadable engine image, not for a bad -r path: a nonexistent or non-registry Registry resolves lazily and New succeeds (verified against /nonexistent and /etc/passwd), so no build invocation can reach this arm; kept because New's signature returns an error and swallowing it would hide a future failure (§misc)
		return "", fmt.Errorf("init error: %s", err)
	}
	if baseDir != "" {
		a.NativeRegistry().BaseDir = baseDir
	}
	prog, reason, _, cerr := a.CompileCheck(source)
	if cerr != nil {
		// CompileCheck reserves this for a source it could not PARSE, so we
		// could not answer the compile question at all. Report it rather than
		// swallow it: returning "no refusal" here would let an unparseable
		// program through the gate under -no-check and ship a binary that
		// cannot run. In the default flow the check pre-flight has already
		// returned 1 on such a source, so this arm is the -no-check path.
		return "", cerr
	}
	if prog != nil {
		return "", nil
	}
	if reason == "" { //covergate:allow unreachable trio: CompileCheck returns (nil program, non-empty reason) on every refusal and (nil, "parse error", err) on a parse failure, which the cerr arm above already took, so a nil program with an empty reason and no error cannot occur; kept as a belt so a future emitter path that forgets to set reason reports something actionable instead of an empty refusal (§misc)
		reason = "no program produced (reason not reported)"
	}
	return reason, nil
}

// defaultOutput is the source basename without its extension, in the cwd:
// prog.boru -> prog, a/b/c.boru -> c.
func defaultOutput(srcPath string) string {
	name := filepath.Base(srcPath)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

func buildConfig(srcPath, registry string, seed int64, optionsBlob string, prof *policy.Profile) (buildrt.Config, error) {
	entryAbs, err := filepath.Abs(srcPath)
	if err != nil {
		return buildrt.Config{}, err
	}
	src, err := os.ReadFile(entryAbs)
	if err != nil {
		return buildrt.Config{}, err
	}

	files := map[string][]byte{entryAbs: src}
	if err := collectImports(entryAbs, src, files); err != nil {
		return buildrt.Config{}, err
	}

	cfg := buildrt.Config{
		Source:      string(src),
		EntryDir:    filepath.Dir(entryAbs),
		Registry:    registry,
		Seed:        seed,
		OptionsBlob: optionsBlob,
		// A policy given at build time is baked in, so the shipped tool
		// carries the author's declared permissions rather than running
		// wide open. See buildrt.Config.Profile for why this is a default
		// and not a boundary.
		Profile: prof,
	}
	// Only attach Files when there is more than the entry itself — a
	// single-file program needs no in-memory file system.
	if len(files) > 1 {
		cfg.Files = files
	}
	return cfg, nil
}

// osExecutable is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildSelfEmbed's locate/read/nested-payload arms — os.Executable
// does not fail on a healthy host and always names a stock test binary.
var osExecutable = os.Executable

// encodePayload is a test seam (design/TEST-SEAMS.10.md); tests swap it to
// drive buildSelfEmbed's encode-failure arm, which is unreachable with
// the plain buildrt.Config shape.
var encodePayload = buildrt.EncodePayload

// buildSelfEmbed produces the standalone binary by copying the running boru
// executable and appending the encoded payload.
func buildSelfEmbed(cfg buildrt.Config, outPath string) error {
	self, err := osExecutable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	image, err := os.ReadFile(self)
	if err != nil {
		return fmt.Errorf("read self: %w", err)
	}
	// Guard against building from an already-built binary, which would nest
	// payloads and run the wrong program.
	if _, ok, _ := buildrt.DecodePayload(image); ok {
		return fmt.Errorf("the running boru binary is itself a built executable; run `boru build` with a stock boru binary")
	}
	payload, err := encodePayload(cfg)
	if err != nil {
		return err
	}
	combined := append(image, payload...)
	if err := os.WriteFile(outPath, combined, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}
