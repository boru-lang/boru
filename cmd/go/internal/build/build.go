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
	compileFlag := fs.Bool("compile", false, "bake best-effort bytecode compilation into the binary (the default)")
	noCompileFlag := fs.Bool("no-compile", false, "bake interpreter-only execution into the binary; wins over --compile/--force-compile")
	forceCompileFlag := fs.Bool("force-compile", false, "bake REQUIRED bytecode compilation into the binary")
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

	// Validate --options eagerly so a typo fails at build time, not when the
	// produced binary runs.
	if *optionsStr != "" {
		var probe lang.Options
		m, err := lang.ParseOptions(*optionsStr)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err)
			return 1
		}
		if err := lang.ApplyOptions(&probe, m); err != nil {
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

	cfg, err := buildConfig(srcPath, *registry, seed, resolveCompile(*compileFlag, *forceCompileFlag, *noCompileFlag), *optionsStr, prof)
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
	if !*noCheck && os.Getenv("BORU_NO_CHECK") == "" {
		color := lang.ResolveColor(nil, stderr, "auto")
		if cerr := check.PreflightColorAt(stderr, cfg.Source, *registry, seed, false, color, cfg.EntryDir); cerr != nil {
			fmt.Fprintf(stderr, "%s\n", cerr)
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

// resolveCompile maps the three flags to a CompileMode. The default is
// best-effort TRY; noCompile wins over everything, then force wins over try
// (mirrors run.ResolveCompileMode without the env-var rollout knobs, which do
// not apply to a frozen binary — --no-compile is the only opt-out here).
func resolveCompile(compile, force, noCompile bool) buildrt.CompileMode {
	switch {
	case noCompile:
		return buildrt.CompileOff
	case force:
		return buildrt.CompileForce
	case compile:
		return buildrt.CompileTry // the explicit spelling of the default
	default:
		return buildrt.CompileTry
	}
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

// buildConfig reads the entry program, walks its file-import graph, and
// assembles the buildrt.Config baked into the binary. Built-in boru: imports
// are skipped (they are in the runtime); every reachable .boru file is embedded
// under its absolute path so the in-memory file system resolves it at run time.
func buildConfig(srcPath, registry string, seed int64, mode buildrt.CompileMode, optionsBlob string, prof *policy.Profile) (buildrt.Config, error) {
	return buildConfigWithAbs(srcPath, registry, seed, mode, optionsBlob, prof, filepath.Abs)
}

func buildConfigWithAbs(srcPath, registry string, seed int64, mode buildrt.CompileMode, optionsBlob string, prof *policy.Profile, abs func(string) (string, error)) (buildrt.Config, error) {
	entryAbs, err := abs(srcPath)
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
		Compile:     mode,
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
