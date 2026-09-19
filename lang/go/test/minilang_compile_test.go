package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Compiled mini-languages (design/MINILANG.5.md §13). A BUILT-IN kind may
// carry an expansion-time Go compile hook alongside its runtime transducer;
// when src is concrete, `mini` runs the hook at the call site and splices
// its tokens instead of the standard call. The transducer stays the
// semantic reference, the check-mode target, and the non-concrete-src
// fallback. (The custom-kind hook surfaces — MiniLangSpec.Compile and
// MiniLang.register-compiled — died with the frozen kind namespace;
// mini_hook_compile_test.go drives the Go-hook machinery directly.)

const mcImp = `import "boru:minilang"  `

func mcRun(t *testing.T, src string) any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d: %v", src, len(res), res)
	}
	return res[0]
}

// TestMiniCompileReCarrier: the built-in `re` kind now compiles the pattern at
// the call site into a carrier consumed by run-re — results unchanged.
func TestMiniCompileReCarrier(t *testing.T) {
	if got := mcRun(t, mcImp+`("AbcD" mini re '[a-z]+').fst.m`); got != "bc" {
		t.Errorf("compiled re match = %v, want bc", got)
	}
	if got := mcRun(t, mcImp+`("a1b2c3" mini re '\\d').n`); got != int64(3) {
		t.Errorf("compiled re count = %v, want 3", got)
	}
	if got := mcRun(t, mcImp+`("a1b2c3" mini re '\\d' {limit:2}).n`); got != int64(2) {
		t.Errorf("compiled re opts.limit = %v, want 2", got)
	}
	// The +re/…/ literal desugars to mini re and hits the same compiled path.
	if got := mcRun(t, mcImp+`("AbcD" +re/[a-z]+/).fst.m`); got != "bc" {
		t.Errorf("+re literal compiled = %v, want bc", got)
	}
}

// TestMiniCompileReBadPattern: a malformed pattern still raises mini_parse_error
// — the hook defers to the transducer when it can't build a carrier, so the
// error is identical to (and parity-safe with) the standard call.
func TestMiniCompileReBadPattern(t *testing.T) {
	a, _ := lang.New()
	if _, err := a.Run(mcImp + `"x" mini re '('`); err == nil {
		t.Fatal("expected mini_parse_error for a bad pattern")
	} else if !strings.Contains(err.Error(), "mini_parse_error") {
		t.Fatalf("error = %v, want mini_parse_error", err)
	}
}

// --- negatives ------------------------------------------------------------

// TestMiniCompileRegisterTombstones pins the frozen registry on this
// surface: both register words raise mini_registry_frozen unconditionally
// (the custom-kind hook contract errors died with them).
func TestMiniCompileRegisterTombstones(t *testing.T) {
	for _, c := range []struct{ name, prog string }{
		{"register", mcImp + `MiniLang.register`},
		{"register-compiled", mcImp + `MiniLang.register-compiled ghost (macro [[s o] [ quote [ s ] ]])`},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := lang.New()
			if err := runReferenceErr(t, a, c.prog); err == nil {
				t.Fatalf("%s: expected mini_registry_frozen", c.name)
			} else if !strings.Contains(err.Error(), "mini_registry_frozen") {
				t.Fatalf("%s: error %q does not contain mini_registry_frozen", c.name, err.Error())
			}
		})
	}
}
