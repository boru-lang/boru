package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// exitCode drives fn with the osExit seam swapped to a panicking stub
// (execution must not continue past an exit point) and returns the code
// it exited with, or -1 when fn returned normally.
func exitCode(t *testing.T, fn func()) (code int) {
	t.Helper()
	prev := osExit
	t.Cleanup(func() { osExit = prev })
	type exited struct{ code int }
	osExit = func(c int) { panic(exited{c}) }
	code = -1
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(exited)
			if !ok {
				panic(r)
			}
			code = e.code
		}
	}()
	fn()
	return code
}

// runMain drives main() with swapped os.Args (flag.CommandLine is
// rebuilt per call because main registers its flags on the global set).
func runMain(t *testing.T, args ...string) int {
	t.Helper()
	prevArgs, prevFlags := os.Args, flag.CommandLine
	t.Cleanup(func() { os.Args, flag.CommandLine = prevArgs, prevFlags })
	flag.CommandLine = flag.NewFlagSet("specgen", flag.ExitOnError)
	os.Args = append([]string{"specgen"}, args...)
	return exitCode(t, main)
}

func TestUsageExits(t *testing.T) {
	// Each mode with missing required flags exits 2.
	if got := runMain(t, "-frontier"); got != 2 {
		t.Errorf("-frontier without flags exited %d, want 2", got)
	}
	if got := runMain(t, "-extend5"); got != 2 {
		t.Errorf("-extend5 without flags exited %d, want 2", got)
	}
	if got := runMain(t, "-extract"); got != 2 {
		t.Errorf("-extract without flags exited %d, want 2", got)
	}
}

func TestGenerateCreateFailureExits(t *testing.T) {
	// -out into a missing directory: the create fails and exits 1.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "m.tsv")
	if got := runMain(t, "-max", "1", "-out", bad); got != 1 {
		t.Errorf("generate with bad -out exited %d, want 1", got)
	}
}
