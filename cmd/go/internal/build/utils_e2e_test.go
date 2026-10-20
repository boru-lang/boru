package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// End-to-end acceptance for utils/ — the coreutils subset written in Boru.
//
// WHY THIS FILE EXISTS. utils/ is a top-level boru directory with its own
// Makefile, outside the repo's Go MODULES fan-out and outside ci.yml, so
// nothing in `make test` reaches it. kg/ is the standing proof that such a
// directory rots: its `fmt` target sat disabled for months because nothing
// executed it. This test is the tripwire — it builds real utils programs with
// `boru build` and runs the produced executables, so a change that breaks them
// fails the ordinary Go suite.
//
// It is also the ONLY place several claims can be checked at all, because each
// is a property of a BUILT BINARY rather than of a running program:
//
//   - argv reaches a built binary (buildrt.Main -> SetHostScriptArgs)
//   - the process environment reaches one (EnvOps.All, via printenv)
//   - the exit code a program chooses is the process's exit status
//   - stdin arrives as a stream a program can read incrementally
//   - and the BAKED-PERMISSIONS PAIR: the same source built twice, once with
//     permissions that deny the write and once with permissions that allow it
//
// The pair is the acceptance test for design/CLI-PROGRAMS.1.md §1. One build
// failing proves nothing on its own — it could fail for any reason — and one
// build succeeding proves nothing either. Only the pair shows the baked
// profile is what decided.
//
// HOW IT BUILDS. `boru build` without -native copies the RUNNING executable and
// appends a payload, so a plain call from inside a test would copy the test
// binary. osExecutable is a test seam (design/TEST-SEAMS.10.md); these tests
// point it at a freshly compiled boru, which makes the produced binaries real
// standalone tools — the same artifact a user gets.

// boruOnce compiles cmd/go once per test binary; every subtest reuses it.
var (
	boruOnce sync.Once
	boruPath string
	boruDir  string
	boruErr  error
)

// TestMain removes the shared boru build directory once every test in the
// package has finished. Without it each run leaks ~42MB under the system temp
// dir — the binary is ~42MB and the sync.Once means one per test binary, so a
// repeated gauntlet quietly accumulates hundreds of megabytes.
func TestMain(m *testing.M) {
	code := m.Run()
	if boruDir != "" {
		_ = os.RemoveAll(boruDir)
	}
	os.Exit(code)
}

// buildBoruBinary compiles the boru CLI into a temp dir shared by all subtests
// and returns its path. The Go toolchain is required, so callers skip without.
func buildBoruBinary(t *testing.T) string {
	t.Helper()
	boruOnce.Do(func() {
		moduleDir, err := findCmdGoModuleDir()
		if err != nil {
			boruErr = err
			return
		}
		// NOT t.TempDir(): the binary is shared by every test in the file
		// via sync.Once, and t.TempDir() is removed when the FIRST test that
		// happened to trigger the build finishes — leaving later tests
		// pointing at a deleted path. So it is an explicit MkdirTemp with an
		// explicit removal registered below.
		dir, err := os.MkdirTemp("", "boru-utils-e2e-")
		if err != nil {
			boruErr = err
			return
		}
		boruDir = dir
		out := filepath.Join(dir, "boru"+exeSuffix) // runnable on Windows too
		// The main package is ./boru inside the cmd/go module — building "."
		// yields a library archive, which fails much later and much less
		// legibly ("exec format error" from the produced util).
		cmd := exec.Command("go", "build", "-o", out, "./boru")
		cmd.Dir = moduleDir
		if b, err := cmd.CombinedOutput(); err != nil {
			boruErr = err
			t.Logf("go build: %s", b)
			return
		}
		if !looksExecutable(out) {
			boruErr = fmt.Errorf("%s is not an executable image", out)
			return
		}
		boruPath = out
	})
	if boruErr != nil {
		t.Skipf("cannot build boru: %v", boruErr)
	}
	return boruPath
}

// looksExecutable reports whether path starts with an ELF or Mach-O magic
// rather than, say, a Go archive. It exists so a wrong build target fails
// where it happens instead of as an "exec format error" six calls later.
func looksExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var magic [4]byte
	if _, err := f.Read(magic[:]); err != nil {
		return false
	}
	switch {
	case bytes.Equal(magic[:], []byte{0x7f, 'E', 'L', 'F'}): // ELF
		return true
	case bytes.Equal(magic[:], []byte{0xcf, 0xfa, 0xed, 0xfe}): // Mach-O 64
		return true
	case bytes.Equal(magic[:], []byte{0xce, 0xfa, 0xed, 0xfe}): // Mach-O 32
		return true
	case magic[0] == 'M' && magic[1] == 'Z': // PE
		return true
	}
	return false
}

// utilsDir walks up from the working directory to the repo root and returns
// its utils/ directory.
func utilsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Skipf("no working directory: %v", err)
	}
	for {
		cand := filepath.Join(dir, "utils")
		if st, err := os.Stat(filepath.Join(cand, "cat.boru")); err == nil && !st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("utils/ not found from the test's working directory")
		}
		dir = parent
	}
}

// buildUtil builds utils/<name>.boru into dir and returns the executable path.
// extra carries build flags — the permissions pair is spelled here.
func buildUtil(t *testing.T, dir, name string, extra ...string) string {
	t.Helper()
	self := buildBoruBinary(t)
	src := filepath.Join(utilsDir(t), name+".boru")
	out := filepath.Join(dir, name+exeSuffix)

	// Point the self-embed path at the real boru, not at this test binary.
	orig := osExecutable
	osExecutable = func() (string, error) { return self, nil }
	t.Cleanup(func() { osExecutable = orig })

	args := append([]string{src, "-o", out}, extra...)
	var stdout, stderr bytes.Buffer
	if rc := New().Run(args, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("boru build %v: rc=%d\nstdout=%s\nstderr=%s", args, rc, stdout.String(), stderr.String())
	}
	if st, err := os.Stat(out); err != nil || !runnable(st) {
		t.Fatalf("produced file is not an executable (err=%v)", err)
	}
	return out
}

// runResult is what a produced binary did.
type runResult struct {
	stdout string
	stderr string
	code   int
}

// runUtil executes bin with args, feeding stdin, and reports its exit status
// rather than failing on a non-zero one — the exit code is usually the thing
// under test.
func runUtil(t *testing.T, bin string, stdin string, env []string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if env != nil {
		cmd.Env = env
	}
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errorAs(err, &ee) {
			t.Fatalf("running %s %v: %v\nstderr=%s", bin, args, err, se.String())
		}
		code = ee.ExitCode()
	}
	return runResult{stdout: so.String(), stderr: se.String(), code: code}
}

// errorAs is errors.As, kept local so the import list stays small.
func errorAs(err error, target **exec.ExitError) bool {
	for err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			*target = ee
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// --- stdin, argv, and exit codes ---

func TestUtilsCatEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()
	bin := buildUtil(t, dir, "cat")

	t.Run("stdin is streamed through", func(t *testing.T) {
		got := runUtil(t, bin, "one\ntwo\nthree\n", nil)
		if got.stdout != "one\ntwo\nthree\n" || got.code != 0 {
			t.Fatalf("stdout=%q code=%d, want the input back and 0", got.stdout, got.code)
		}
	})

	t.Run("ARGV reaches the built binary", func(t *testing.T) {
		f := filepath.Join(dir, "in.txt")
		if err := os.WriteFile(f, []byte("alpha\nbravo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := runUtil(t, bin, "", nil, f)
		if got.stdout != "alpha\nbravo\n" || got.code != 0 {
			t.Fatalf("stdout=%q code=%d, want the file's contents and 0", got.stdout, got.code)
		}
	})

	t.Run("a flag in argv is a flag, not an operand", func(t *testing.T) {
		f := filepath.Join(dir, "in.txt")
		got := runUtil(t, bin, "", nil, "-n", f)
		if !strings.Contains(got.stdout, "1\talpha") {
			t.Fatalf("stdout=%q, want -n to number the lines", got.stdout)
		}
	})

	t.Run("a missing operand exits 1 and diagnoses on STDERR", func(t *testing.T) {
		got := runUtil(t, bin, "", nil, filepath.Join(dir, "no-such-file"))
		if got.code != 1 {
			t.Fatalf("code=%d, want 1", got.code)
		}
		if got.stdout != "" {
			t.Fatalf("stdout=%q, want nothing — diagnostics belong on stderr", got.stdout)
		}
		if !strings.Contains(got.stderr, "cat:") {
			t.Fatalf("stderr=%q, want a cat: diagnostic", got.stderr)
		}
	})

	t.Run("--help renders and exits 0", func(t *testing.T) {
		got := runUtil(t, bin, "", nil, "--help")
		if got.code != 0 {
			t.Fatalf("code=%d, want 0", got.code)
		}
		for _, want := range []string{"usage: cat", "--number", "--help"} {
			if !strings.Contains(got.stdout, want) {
				t.Fatalf("--help output %q is missing %q", got.stdout, want)
			}
		}
	})

	t.Run("an unknown flag exits 2", func(t *testing.T) {
		if got := runUtil(t, bin, "", nil, "-Z"); got.code != 2 {
			t.Fatalf("code=%d, want 2", got.code)
		}
	})
}

// --- the exit-code tri-state, with nothing else in the way ---

func TestUtilsTrueFalseExitStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()
	tbin := buildUtil(t, dir, "true")
	fbin := buildUtil(t, dir, "false")

	cases := []struct {
		name string
		bin  string
		args []string
		want int
	}{
		{"true is 0", tbin, nil, 0},
		{"true ignores operands", tbin, []string{"a", "b"}, 0},
		{"true ignores an unknown flag", tbin, []string{"-Z"}, 0},
		{"true --help is 0", tbin, []string{"--help"}, 0},
		{"false is 1", fbin, nil, 1},
		{"false ignores operands", fbin, []string{"a"}, 1},
		{"false --help prints help and is STILL 1", fbin, []string{"--help"}, 1},
		{"false --version is 1 too", fbin, []string{"--version"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runUtil(t, tc.bin, "", nil, tc.args...)
			if got.code != tc.want {
				t.Fatalf("code=%d, want %d", got.code, tc.want)
			}
		})
	}

	// The help really is produced on the arm that still fails — otherwise the
	// case above would pass on a program that printed nothing at all.
	if got := runUtil(t, fbin, "", nil, "--help"); !strings.Contains(got.stdout, "usage: false") {
		t.Fatalf("false --help stdout=%q, want the usage block", got.stdout)
	}
}

// --- the environment reaches a built binary (EnvOps.All) ---

func TestUtilsPrintenvSeesTheProcessEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()
	bin := buildUtil(t, dir, "printenv")
	env := []string{"BORU_E2E_ONE=first", "BORU_E2E_TWO=second"}
	if runtime.GOOS == "windows" {
		// os/exec adds SYSTEMROOT to a Windows child's environment (much of
		// Windows cannot start without it) unless the environment already
		// carries it: pass it, so the listing is still exactly what we gave.
		env = append(env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}

	t.Run("a named variable is printed", func(t *testing.T) {
		got := runUtil(t, bin, "", env, "BORU_E2E_ONE")
		if got.stdout != "first\n" || got.code != 0 {
			t.Fatalf("stdout=%q code=%d, want first and 0", got.stdout, got.code)
		}
	})

	t.Run("an unset variable exits 1 and prints nothing", func(t *testing.T) {
		got := runUtil(t, bin, "", env, "BORU_E2E_MISSING")
		if got.code != 1 || got.stdout != "" {
			t.Fatalf("stdout=%q code=%d, want nothing and 1", got.stdout, got.code)
		}
	})

	// The All() half: no operand lists the WHOLE environment. This is the only
	// place in the repository that seam is exercised — every other use of the
	// environment reads one variable by name.
	t.Run("no operand lists the whole environment", func(t *testing.T) {
		got := runUtil(t, bin, "", env)
		if got.code != 0 {
			t.Fatalf("code=%d, want 0", got.code)
		}
		for _, want := range []string{"BORU_E2E_ONE=first\n", "BORU_E2E_TWO=second\n"} {
			if !strings.Contains(got.stdout, want) {
				t.Fatalf("listing %q is missing %q", got.stdout, want)
			}
		}
		if n := strings.Count(got.stdout, "\n"); n != len(env) {
			t.Fatalf("listing has %d records, want exactly %d — the environment leaked", n, len(env))
		}
	})
}

// --- grep: the exit-code tri-state and --color=auto's pipe behaviour ---

func TestUtilsGrepEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()
	bin := buildUtil(t, dir, "grep")
	const esc = "\x1b["

	t.Run("a match exits 0", func(t *testing.T) {
		got := runUtil(t, bin, "alpha\nbravo\n", nil, "alp")
		if got.stdout != "alpha\n" || got.code != 0 {
			t.Fatalf("stdout=%q code=%d, want the matching line and 0", got.stdout, got.code)
		}
	})

	t.Run("no match exits 1, cleanly", func(t *testing.T) {
		got := runUtil(t, bin, "alpha\n", nil, "zulu")
		if got.stdout != "" || got.code != 1 {
			t.Fatalf("stdout=%q code=%d, want nothing and 1", got.stdout, got.code)
		}
	})

	t.Run("a malformed pattern exits 2", func(t *testing.T) {
		if got := runUtil(t, bin, "alpha\n", nil, "["); got.code != 2 {
			t.Fatalf("code=%d, want 2", got.code)
		}
	})

	// stdout here is a PIPE, never a terminal, which is what makes these two
	// assertable. --color=auto's true arm needs a real tty and is covered by
	// the pure decision fn in utils/tests/grep_test.boru, which takes "is it a
	// tty" as a parameter for exactly this reason.
	t.Run("--color=auto emits NO escapes into a pipe", func(t *testing.T) {
		got := runUtil(t, bin, "alpha\n", nil, "--color=auto", "alp")
		if strings.Contains(got.stdout, esc) {
			t.Fatalf("stdout=%q contains an escape; a pipe must not be coloured", got.stdout)
		}
	})

	t.Run("--color=always does, so the painter is genuinely wired up", func(t *testing.T) {
		got := runUtil(t, bin, "alpha\n", nil, "--color=always", "alp")
		if !strings.Contains(got.stdout, esc) {
			t.Fatalf("stdout=%q has no escape; --color=always did nothing", got.stdout)
		}
	})

	t.Run("an invalid --color value exits 2", func(t *testing.T) {
		if got := runUtil(t, bin, "alpha\n", nil, "--color=purple", "alp"); got.code != 2 {
			t.Fatalf("code=%d, want 2", got.code)
		}
	})
}

// --- head reads only what it needs ---

func TestUtilsHeadStopsEarly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()
	bin := buildUtil(t, dir, "head")

	var big strings.Builder
	for i := 0; i < 5000; i++ {
		big.WriteString("line\n")
	}
	got := runUtil(t, bin, big.String(), nil, "-n", "3")
	if got.stdout != "line\nline\nline\n" || got.code != 0 {
		t.Fatalf("stdout=%q code=%d, want three lines and 0", got.stdout, got.code)
	}
}

// --- THE BAKED-PERMISSIONS PAIR ---
//
// The acceptance test for design/CLI-PROGRAMS.1.md §1. tee is the suite's only
// writer, so it is the program whose behaviour a file-write permission can
// actually change. Both halves build the SAME SOURCE with the SAME command;
// only the permission flags differ, so the profile is the only thing that can
// explain the difference in behaviour.
//
// Note what is being claimed. A baked profile constrains the PROGRAM — "this
// tool never writes files" as a property of the tool — and it is a strippable
// default, not a boundary against someone holding the binary: the payload is
// plain JSON behind a magic marker with no integrity check. CLI.md says so and
// must keep saying so.
func TestUtilsBakedPermissionsPair(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	dir := t.TempDir()

	denied := buildUtil(t, filepath.Join(dir), "tee", "-perms", "read-only")

	// The permissive half has to be built under a different name, since both
	// halves are the same source file.
	allowedDir := t.TempDir()
	allowed := buildUtil(t, allowedDir, "tee",
		"-perms", "read-only",
		"-allow-global", "disk.write",
		"-allow", "fileops.write")

	t.Run("the denied build cannot write its file", func(t *testing.T) {
		target := filepath.Join(dir, "denied-out.txt")
		got := runUtil(t, denied, "payload\n", nil, target)
		if got.code == 0 {
			t.Fatalf("code=0: the read-only build wrote the file, so the baked profile did nothing")
		}
		if _, err := os.Stat(target); err == nil {
			t.Fatalf("%s exists: the read-only build wrote it", target)
		}
	})

	t.Run("the read-only build can READ a named file (NUR041 resolved)", func(t *testing.T) {
		// read-only means reads work: the profile allows the read-like
		// fileops (read/stat/list) while writes stay denied. cat is
		// the suite's file reader, baked under the same profile as the
		// denied tee half.
		catDir := t.TempDir()
		reader := buildUtil(t, catDir, "cat", "-perms", "read-only")
		src := filepath.Join(catDir, "ro-in.txt")
		if err := os.WriteFile(src, []byte("readable\n"), 0o644); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		got := runUtil(t, reader, "", nil, src)
		if got.code != 0 {
			t.Fatalf("code=%d stderr=%q, want 0 — read-only must allow file reads", got.code, got.stderr)
		}
		if got.stdout != "readable\n" {
			t.Fatalf("stdout=%q, want %q", got.stdout, "readable\n")
		}
	})

	t.Run("the permissive build writes exactly the same bytes it was given", func(t *testing.T) {
		target := filepath.Join(allowedDir, "allowed-out.txt")
		got := runUtil(t, allowed, "payload\n", nil, target)
		if got.code != 0 {
			t.Fatalf("code=%d stderr=%q, want 0 — the allowed build should write", got.code, got.stderr)
		}
		b, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading %s: %v", target, err)
		}
		if string(b) != "payload\n" {
			t.Fatalf("file=%q, want %q", b, "payload\n")
		}
	})

	// The pair's control: BOTH halves must still copy to standard output. If
	// the denied build failed by doing nothing at all, the first case above
	// would pass for the wrong reason.
	t.Run("both halves still copy to standard output", func(t *testing.T) {
		a := runUtil(t, denied, "payload\n", nil, filepath.Join(dir, "x.txt"))
		b := runUtil(t, allowed, "payload\n", nil, filepath.Join(allowedDir, "y.txt"))
		if !strings.Contains(a.stdout, "payload") {
			t.Fatalf("denied build stdout=%q, want the input echoed", a.stdout)
		}
		if !strings.Contains(b.stdout, "payload") {
			t.Fatalf("allowed build stdout=%q, want the input echoed", b.stdout)
		}
	})
}

// --- a built binary runs the sources it was built from ---
//
// bundledFirst decides which LAYER answers a read; it does not decide which
// PATH is asked for. Both halves are needed, and only an end-to-end build
// shows that: the unit tests in internal/buildrt passed while a real binary
// still loaded a stranger, because the import was still being resolved
// relative to the run-time cwd. This test is the one that would have caught
// it, so it belongs here rather than beside the layer.
func TestUtilsBuiltBinaryIgnoresSameNamedHostFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping utils end-to-end build in -short mode")
	}
	self := buildBoruBinary(t)

	proj := t.TempDir()
	elsewhere := t.TempDir()
	write := func(dir, name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	lib := write(proj, "lib.boru", `export "Lib" { who: "BUNDLED" }`+"\n")
	src := write(proj, "main.boru", "import \"./lib.boru\"\nprint (Lib.who)\n")
	write(elsewhere, "lib.boru", `export "Lib" { who: "STRANGER" }`+"\n")

	out := filepath.Join(proj, "tool"+exeSuffix)
	orig := osExecutable
	osExecutable = func() (string, error) { return self, nil }
	t.Cleanup(func() { osExecutable = orig })
	var so, se bytes.Buffer
	if rc := New().Run([]string{src, "-o", out}, nil, &so, &se); rc != 0 {
		t.Fatalf("boru build: rc=%d stderr=%s", rc, se.String())
	}

	run := func(cwd string) string {
		cmd := exec.Command(out)
		cmd.Dir = cwd
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("running the built tool in %s: %v\n%s", cwd, err, b)
		}
		return strings.TrimSpace(string(b))
	}

	if got := run(proj); got != "BUNDLED" {
		t.Fatalf("baseline: got %q, want BUNDLED", got)
	}

	// Editing the imported source after the build must not change the tool.
	if err := os.WriteFile(lib, []byte(`export "Lib" { who: "EDITED" }`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run(proj); got != "BUNDLED" {
		t.Errorf("after editing the imported file the tool printed %q — a built "+
			"binary must run the sources it was built from, not the live ones", got)
	}

	// And a same-named file in the directory the tool is STARTED from must
	// never be loaded in place of the bundled module.
	if got := run(elsewhere); got != "BUNDLED" {
		t.Errorf("run from a directory holding its own lib.boru the tool printed %q "+
			"— a stranger's file was executed in place of the bundled module", got)
	}
}
