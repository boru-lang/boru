package test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Phase 0 (design/legacy/MACROS-PHASE1.10.ignore §7): gensym mints fresh, never-colliding
// atoms for capture-free temporaries.
func TestGensymUniqueAndMonotonic(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`[gensym gensym gensym]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	l, err := core.AsList(res[0])
	if err != nil || l.Len() != 3 {
		t.Fatalf("expected 3 gensyms, got %v", res[0])
	}
	seen := map[string]bool{}
	for i := 0; i < l.Len(); i++ {
		v := l.Get(i)
		if !core.IsAtom(v) {
			t.Fatalf("gensym[%d] should be an Atom, got %s", i, v.Parent)
		}
		name, _ := core.AsAtom(v)
		if !strings.HasPrefix(name, "tmp$g") {
			t.Errorf("gensym name %q should start with tmp$g", name)
		}
		if seen[name] {
			t.Errorf("gensym produced a duplicate name %q", name)
		}
		seen[name] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 distinct gensym names, got %d", len(seen))
	}

	// Negative: two gensyms never compare equal.
	res, err = runNativeSteps(t, nil, []string{`(gensym) eq (gensym)`})
	if err != nil {
		t.Fatalf("eq run: %v", err)
	}
	if b, _ := core.AsBoolean(res[0]); b {
		t.Error("(gensym) eq (gensym) should be false (always distinct)")
	}
}

// Phase 1c+1d: a macro definer + expander. unquote inserts operand forms;
// splice flattens; manual hygiene via gensym keeps an introduced binder from
// capturing a same-named user var.
func TestMacroExpandAndHygiene(t *testing.T) {
	// unless: cond false → runs the body; cond true → the then branch.
	res, err := runNativeSteps(t, nil, []string{
		`def unless (macro [[c body] [ quote [ if unquote c [0] unquote body ] ]])`,
		`unless false [42]`,
	})
	if err != nil {
		t.Fatalf("unless: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 42 {
		t.Errorf("unless false [42] = %v, want 42", res[0])
	}

	// splice flattens a list operand into the call.
	res, err = runNativeSteps(t, nil, []string{
		`def callit (macro [[f xs] [ quote [ unquote f splice xs ] ]])`,
		`def add3 fn [[a:Integer b:Integer c:Integer][Integer][a add b add c]]`,
		`callit add3 [1 2 3]`,
	})
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 6 {
		t.Errorf("callit add3 [1 2 3] = %v, want 6", res[0])
	}

	// Manual hygiene: the gensym'd temp does NOT capture the user's `tmp`.
	res, err = runNativeSteps(t, nil, []string{
		`def myor (macro [[a b] [ def g (gensym)  quote [ def unquote g unquote a  if unquote g [unquote g] [unquote b] ] ]])`,
		`def tmp 42`,
		`myor false tmp`,
	})
	if err != nil {
		t.Fatalf("myor: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 42 {
		t.Errorf("hygienic myor false tmp = %v, want 42 (user tmp untouched)", res[0])
	}

	// Negative: unquote outside a template errors.
	if _, err := runNativeSteps(t, nil, []string{`unquote 5`}); err == nil {
		t.Error("unquote outside a macro should error, got nil")
	}
	// Negative: too few operands.
	if _, err := runNativeSteps(t, nil, []string{
		`def m (macro [[a b] [quote [unquote a]]])`, `m 1`,
	}); err == nil {
		t.Error("macro with too few operands should error, got nil")
	}
}

// Phase 4: automatic hygiene. A literal `def <name>` binder in a template is
// renamed to a fresh gensym, so it can't capture a same-named use-site var —
// no manual gensym needed. `def unquote <name>` stays user-controlled.
func TestMacroAutoHygiene(t *testing.T) {
	// Literal `def tmp` in the template must NOT capture the user's tmp.
	res, err := runNativeSteps(t, nil, []string{
		`def myor2 (macro [[a b] [ quote [ def tmp unquote a  if tmp [tmp] [unquote b] ] ]])`,
		`def tmp 42`,
		`myor2 false tmp`,
	})
	if err != nil {
		t.Fatalf("auto-hygiene myor2: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 42 {
		t.Errorf("auto-hygienic myor2 false tmp = %v, want 42 (user tmp untouched)", res[0])
	}

	// `def unquote name` is the escape: an intentional user-visible binding.
	res, err = runNativeSteps(t, nil, []string{
		`def defconst (macro [[name val] [ quote [ def unquote name unquote val ] ]])`,
		`defconst answer 42`,
		`answer`,
	})
	if err != nil {
		t.Fatalf("defconst: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 42 {
		t.Errorf("defconst answer 42; answer = %v, want 42 (user-visible binding)", res[0])
	}
}

// Phase 5 (interpreter staging): a macro is define-before-USE; a macro in a fn
// body expands at call time (so it need only be defined before the call). The
// compiled-mode expander (expansion moved to compile time) awaits the IR
// backend — see design/legacy/MACROS-PHASE5.5.ignore.
func TestMacroStaging(t *testing.T) {
	// Use before definition errors.
	if _, err := runNativeSteps(t, nil, []string{`nope 1`, `def nope (macro [[a] [quote [unquote a]]])`}); err == nil {
		t.Error("using a macro before defining it should error, got nil")
	}
	// A macro defined after the fn but before the call expands at call time.
	res, err := runNativeSteps(t, nil, []string{
		`def f fn [[n:Integer] [Integer] [ twice n ]]`,
		`def twice (macro [[e] [quote [unquote e add unquote e]]])`,
		`f 5`,
	})
	if err != nil {
		t.Fatalf("call-time staging: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 10 {
		t.Errorf("f 5 = %v, want 10 (twice expands at call time)", res[0])
	}
}
