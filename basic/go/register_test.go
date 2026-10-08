package basic

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

// TestRegisterStandalone drives Register the way an eng-only embedder
// would (the calc/go pattern, one layer up): a bare eng registry, the
// basic layer installed on top, no lang module anywhere. This is the
// executable form of ADR-013's claim that the base language works
// against the kernel alone — one probe per registration group, plus
// the negative half (an unregistered word still errors).
func TestRegisterStandalone(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("core.NewRegistry: %v", err)
	}
	if err := Register(r); err != nil {
		t.Fatalf("basic.Register: %v", err)
	}
	r.SetParseFunc(parser.Parse)
	r.InitRootContext()
	r.MarkReady()

	asInt := func(v core.Value) int64 {
		t.Helper()
		n, err := core.AsInteger(v)
		if err != nil {
			t.Fatalf("AsInteger(%v): %v", v, err)
		}
		return n
	}

	run := func(src string) []core.Value {
		t.Helper()
		values, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		out, err := core.NewTop(r).Run(values)
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out
	}

	// Stack words: dup.
	if out := run("7 dup"); len(out) != 2 || asInt(out[0]) != 7 || asInt(out[1]) != 7 {
		t.Fatalf("7 dup: want [7 7], got %v", out)
	}
	// Definition words: def + resolution.
	if out := run("def x 9 x"); len(out) != 1 || asInt(out[0]) != 9 {
		t.Fatalf("def x 9 x: want [9], got %v", out)
	}
	// Control words: if.
	if out := run("if true [1] [2]"); len(out) != 1 || asInt(out[0]) != 1 {
		t.Fatalf("if true [1] [2]: want [1], got %v", out)
	}
	// fn definition + dispatch (identity — no library words needed).
	if out := run("def ident fn [[a:Integer] [Integer] [a]] ident 5"); len(out) != 1 || asInt(out[0]) != 5 {
		t.Fatalf("fn identity: want [5], got %v", out)
	}

	// Negative: a word outside the basic layer is still an undefined
	// word — Register must not have degraded stepWord's error path.
	values, err := parser.Parse("keys")
	if err != nil {
		t.Fatalf("parse negative probe: %v", err)
	}
	if _, err := core.NewTop(r).Run(values); err == nil || !strings.Contains(err.Error(), "undefined_word") {
		t.Fatalf("expected undefined_word for a lang-layer word, got %v", err)
	}
}

// TestRegisterRefusesOnTypeInitError is the negative half of the entry
// point's contract: an eng-only embedder has nothing above it to check
// the init-time accumulator, so Register must hand the failure back
// rather than install words over a type table already known bad.
func TestRegisterRefusesOnTypeInitError(t *testing.T) {
	boom := errors.New("types_handles: register Ideal/Patrun: duplicate FixedID")
	prev := SwapTypeInitErrs([]error{boom})
	defer SwapTypeInitErrs(prev)

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("core.NewRegistry: %v", err)
	}
	if got := Register(r); !errors.Is(got, boom) {
		t.Fatalf("Register must surface the init-time error, got %v", got)
	}
	// Declined BEFORE installing: the layer's first group is absent, so a
	// caller that ignored the error cannot mistake a half-built registry
	// for a working one.
	if r.Lookup("dup") != nil {
		t.Fatal("Register must not install words after declining")
	}
}
