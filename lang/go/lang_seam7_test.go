package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// TestS7Lang_NewSeed drives New's Seed arm (boru.go:144-146): a non-zero
// Options.Seed calls native.SetIDSeed so ID minting is deterministic.
func TestS7Lang_NewSeed(t *testing.T) {
	a, err := New(Options{Seed: 42})
	if err != nil {
		t.Fatalf("New(Seed:42): %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil instance")
	}
	// A second seeded instance run reproduces the first (determinism holds).
	b, err := New(Options{Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp("1 add 2"); err != nil {
		t.Fatalf("seeded instance run: %v", err)
	}
	if _, err := b.RunInterp("1 add 2"); err != nil {
		t.Fatalf("seeded instance run: %v", err)
	}
}

// TestS7Lang_NewRegistryError drives New's registry-construction error arm
// (boru.go:149-151) by swapping the newDefaultRegistryWithPolicy seam to
// fail. The default construction cannot fail once the process is running,
// so only the seam observes the propagation.
func TestS7Lang_NewRegistryError(t *testing.T) {
	orig := newDefaultRegistryWithPolicy
	t.Cleanup(func() { newDefaultRegistryWithPolicy = orig })

	sentinel := errors.New("seam: registry construction failed")
	newDefaultRegistryWithPolicy = func(_ policy.Policy, _ ...func(*native.Registry)) (*native.Registry, error) {
		return nil, sentinel
	}

	a, err := New()
	if err == nil {
		t.Fatalf("expected registry-construction error, got instance %v", a)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the seam's sentinel error, got: %v", err)
	}
	if a != nil {
		t.Errorf("expected nil instance on construction failure, got %v", a)
	}
}

// TestS7Lang_RegisterFormatLazyInit drives RegisterFormat's lazy-init guard
// (boru.go:380-382): when the instance has no formats map, RegisterFormat
// allocates one before registering. A fresh instance ships with formats
// installed, so the guard is exercised by clearing them first — this pins
// the contract that RegisterFormat works even with no prior formats map.
func TestS7Lang_RegisterFormatLazyInit(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// Force the no-formats-map precondition the guard defends against.
	native.SetHostFormats(a.registry, nil)
	if native.HostFormats(a.registry) != nil {
		t.Fatal("precondition: formats map should be nil")
	}

	a.RegisterFormat("s7fmt", &s7BracketFormat{}, ".s7")

	formats := native.HostFormats(a.registry)
	if formats == nil {
		t.Fatal("RegisterFormat must lazily allocate the formats map")
	}
	if _, ok := formats["s7fmt"]; !ok {
		t.Errorf("registered format not present: %v", formats)
	}
}

// s7BracketFormat is a trivial Format used only to exercise RegisterFormat.
type s7BracketFormat struct{}

func (*s7BracketFormat) Decode(string) ([]native.Value, error) { return nil, nil }
func (*s7BracketFormat) Encode(native.Value) (string, error)   { return "", nil }

// TestS7Lang_DefineType drives DefineType (boru.go:425-427): the direct
// embedding-API type installation, plus a negative (an invalid body errors).
func TestS7Lang_DefineType(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tp, err := a.DefineType("S7Alias", NewTypeLiteral(TInteger))
	if err != nil {
		t.Fatalf("DefineType alias: %v", err)
	}
	if tp == nil {
		t.Fatal("DefineType returned nil handle")
	}
	// The installed type is usable from source (a.Run renders results as
	// strings, so 5 is S7Alias comes back as "true").
	out, err := a.RunInterp("5 is S7Alias")
	if err != nil {
		t.Fatalf("run is S7Alias: %v", err)
	}
	if fmt.Sprint(out) != "[true]" {
		t.Errorf("5 is S7Alias = %v, want [true]", out)
	}
	// Negative: a non-Integer is not an S7Alias.
	out, err = a.RunInterp("'x' is S7Alias")
	if err != nil {
		t.Fatalf("run is S7Alias (negative): %v", err)
	}
	if fmt.Sprint(out) != "[false]" {
		t.Errorf("'x' is S7Alias = %v, want [false]", out)
	}
}

// TestS7Lang_DefineTypeFromSourceInvalidName drives the name-validation
// reject arm (boru.go:459-462) and validTypeIdent's two false arms
// (boru.go:478-480 first-char, 486-487 later-char). Names must be a
// capitalised identifier of letters/digits/_/-//.
func TestS7Lang_DefineTypeFromSourceInvalidName(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	badNames := []string{
		"",              // empty: first-char guard (478-480)
		"lower",         // lowercase first char: first-char guard (478-480)
		"1Bad",          // digit first char: first-char guard (478-480)
		"A b",           // space (invalid later char): switch default (486-487)
		"X end (raise)", // injection attempt: switch default (486-487)
		"Foo!",          // punctuation later char: switch default (486-487)
	}
	for _, name := range badNames {
		tp, err := a.DefineTypeFromSource(name, "Integer")
		if err == nil {
			t.Errorf("DefineTypeFromSource(%q): expected invalid-name error, got handle %v", name, tp)
			continue
		}
		if !strings.Contains(err.Error(), "invalid type name") {
			t.Errorf("DefineTypeFromSource(%q): expected invalid-name error, got: %v", name, err)
		}
		if tp != nil {
			t.Errorf("DefineTypeFromSource(%q): expected nil handle, got %v", name, tp)
		}
	}

	// Positive: a valid capitalised name with the allowed separators
	// installs and resolves (validTypeIdent accepts the full char set).
	tp, err := a.DefineTypeFromSource("Ok-Name_2", "Integer")
	if err != nil {
		t.Fatalf("DefineTypeFromSource(valid): %v", err)
	}
	if tp == nil {
		t.Fatal("valid name produced nil handle")
	}
}

// TestS7Lang_DefineTypeFromSourceNoResolve drives the
// installed-but-did-not-resolve arm (boru.go:469): a body whose program
// runs cleanly yet leaves the name unbound (it defines then immediately
// undefs the type) reaches the final defensive error.
func TestS7Lang_DefineTypeFromSourceNoResolve(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// `def Ghost Integer  undef Ghost` runs without error but LookupTypeName
	// finds nothing afterward.
	tp, err := a.DefineTypeFromSource("Ghost", "Integer  undef Ghost")
	if err == nil {
		t.Fatalf("expected did-not-resolve error, got handle %v", tp)
	}
	if !strings.Contains(err.Error(), "installed but did not resolve") {
		t.Fatalf("expected did-not-resolve error, got: %v", err)
	}
	if tp != nil {
		t.Errorf("expected nil handle, got %v", tp)
	}
}

// TestS7Lang_RunCompiledStrictCompileError drives RunCompiledStrict's
// CompileCheck-error arm (boru.go:723-726): a syntax error surfaces from
// CompileCheck and RunCompiledStrict rolls back and returns it.
func TestS7Lang_RunCompiledStrictCompileError(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, err := a.RunCompiledStrict("((")
	if err == nil {
		t.Fatalf("expected syntax error, got %v", out)
	}
	if !strings.Contains(err.Error(), "syntax_error") {
		t.Fatalf("expected syntax_error, got: %v", err)
	}

	// Negative sibling: a program that does not compile takes the prog==nil
	// arm — a distinct path that must NOT be the CompileCheck-error arm.
	// `undefinedword123` is statically INVALID, so its own verdict is the
	// error: the undefined word, with its position and hints intact.
	_, err = a.RunCompiledStrict("undefinedword123")
	if err == nil {
		t.Fatal("expected an error for an uncompilable program")
	}
	if !strings.Contains(err.Error(), "undefined word") {
		t.Fatalf("expected the program's own verdict, got: %v", err)
	}
}

// TestS7Lang_CompileFailureReason drives compileFailureReason (the
// empty-reason default, plus the pass-through else): the empty reason maps
// to a generic message; a non-empty reason passes through verbatim.
func TestS7Lang_CompileFailureReason(t *testing.T) {
	if got := compileFailureReason(""); got != "program is not compilable" {
		t.Errorf("compileFailureReason(\"\") = %q, want the generic default", got)
	}
	if got := compileFailureReason("some reason"); got != "some reason" {
		t.Errorf("compileFailureReason non-empty = %q, want pass-through", got)
	}
}

// TestS7Lang_AmbiguousGradualSplit drives CompileCheck's ambiguous-gradual-
// split compile failure (boru.go:345-347). An `if` join produces a genuinely MIXED
// gradual carrier — Disjunct(Integer,Boolean) — and feeding it to `not`
// (which has a Boolean overload that rejects the mixed carrier at the stack
// top, plus a forward arm that grabs the trailing 0) makes the static
// forward/stack split diverge from the runtime one. CompileCheck declines to
// compile (prog==nil) and reports the reason, rather than emitting bytecode
// that would diverge from the interpreter.
func TestS7Lang_AmbiguousGradualSplit(t *testing.T) {
	src := `def f fn [[c:Boolean] [Any] [(if c [5] [true]) not 0]] (f true)`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck returned an error, want a clean compile failure: %v", cerr)
	}
	if prog != nil {
		t.Fatalf("expected nil program (uncompilable), got %v", prog)
	}
	if !strings.Contains(reason, "gradual operand") {
		t.Fatalf("expected the gradual-operand compile failure reason, got: %q", reason)
	}
}

// TestS7Lang_ApplyOptionsCoercions drives the optInt / optFloat coercion
// arms in options.go through the public ApplyOptions surface:
//   - optInt case int   (79-80) and case int64 (81-82)
//   - optInt default    (88-89) for a non-numeric value
//   - applyTapeOptions grows error propagation (59-61)
//   - optFloat case int (96-97) and case int64 (98-99)
func TestS7Lang_ApplyOptionsCoercions(t *testing.T) {
	// optInt case int + optFloat case int.
	var o Options
	if err := ApplyOptions(&o, map[string]any{
		"tape": map[string]any{"initial": int(4096), "factor": int(3)},
	}); err != nil {
		t.Fatalf("int coercions: %v", err)
	}
	if o.Tape.InitialSize != 4096 {
		t.Errorf("InitialSize = %d, want 4096", o.Tape.InitialSize)
	}
	if o.Tape.GrowthFactor != 3 {
		t.Errorf("GrowthFactor = %v, want 3", o.Tape.GrowthFactor)
	}

	// optInt case int64 + optFloat case int64.
	var o2 Options
	if err := ApplyOptions(&o2, map[string]any{
		"tape": map[string]any{"grows": int64(9), "factor": int64(2)},
	}); err != nil {
		t.Fatalf("int64 coercions: %v", err)
	}
	if o2.Tape.MaxGrows != 9 {
		t.Errorf("MaxGrows = %d, want 9", o2.Tape.MaxGrows)
	}
	if o2.Tape.GrowthFactor != 2 {
		t.Errorf("GrowthFactor = %v, want 2", o2.Tape.GrowthFactor)
	}

	// optInt default arm (88-89): a non-numeric initial value errors.
	var o3 Options
	err := ApplyOptions(&o3, map[string]any{"tape": map[string]any{"initial": "nope"}})
	if err == nil {
		t.Fatal("expected error for non-numeric initial")
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("initial:string error = %v, want 'must be an integer'", err)
	}

	// applyTapeOptions grows error propagation (59-61): a non-integral
	// float for grows makes optInt fail and the error propagates.
	var o4 Options
	err = ApplyOptions(&o4, map[string]any{"tape": map[string]any{"grows": 3.5}})
	if err == nil {
		t.Fatal("expected error for non-integral grows")
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("grows:3.5 error = %v, want 'must be an integer'", err)
	}

	// optFloat default arm (a non-numeric factor errors) — negative pair.
	var o5 Options
	err = ApplyOptions(&o5, map[string]any{"tape": map[string]any{"factor": "nope"}})
	if err == nil {
		t.Fatal("expected error for non-numeric factor")
	}
	if !strings.Contains(err.Error(), "must be a number") {
		t.Errorf("factor:string error = %v, want 'must be a number'", err)
	}
}
