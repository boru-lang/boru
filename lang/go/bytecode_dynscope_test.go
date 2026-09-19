package lang

import (
	"fmt"
	"strings"
	"testing"
)

// bytecode_dynscope_test.go pins the compiled dynamic-scope reads
// (recursion.tsv 71/72): a fn body's word that resolves only through the
// runtime def stack lowers to OpLookupDynScope, and every frame binding such
// a name installs a registry-visible OpBindDynScope twin. Negatives: a read
// with no reachable binder still refuses (a typo never becomes a runtime
// lookup), and the refusal's interpreter fallback stays faithful.

func TestDynScopeRowsCompileWithParity(t *testing.T) {
	rows := []struct{ src, want, op string }{
		// The base branch reads the PREVIOUS frame's body-local: the def
		// installs per frame, bindings stack through the recursion, and the
		// frame's RET pops them — the lookup finds the caller's acc2.
		{`def f fn [[n:Integer] [Integer] [if (n lte 0) [acc2] [def acc2 n f (n sub 1)]]] f 2`,
			"[1]", "BIND_DYN_SCOPE"},
		// A callee reads the CALLER's param: the param installs at frame
		// entry, exactly where the interpreter's InstallFrameBinding makes
		// it visible.
		{`def g fn [[] [Integer] [n]] def f2 fn [[n:Integer] [Integer] [g]] f2 42`,
			"[42]", "LOOKUP_DYN_SCOPE"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: expected a native compile, refused: reason=%q err=%v", c.src, reason, cerr)
		}
		dis := prog.Disassemble()
		if !strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
			t.Errorf("%q: compiled without the dynamic-scope lookup:\n%s", c.src, dis)
		}
		if !strings.Contains(dis, c.op) {
			t.Errorf("%q: disassembly missing %s:\n%s", c.src, c.op, dis)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if !compiled || errC != nil {
			t.Fatalf("%q: compiled run: compiled=%v err=%v", c.src, compiled, errC)
		}
		d, _ := New()
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: parity: compiled=%v interp=%v (errI=%v), want %s", c.src, gotC, gotI, errI, c.want)
		}
	}
}

// A fn-body read with NO reachable binder (a genuine typo) must NOT lower to
// a runtime lookup — the refusal stands and the interpreter owns the
// undefined_word error.
func TestDynScopeUnreachableNameStaysRefused(t *testing.T) {
	src := `def g fn [[] [Integer] [nosuchbinding]] g`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		// A check-level error is equally sound — the interpreter owns it.
		return
	}
	if prog != nil && strings.Contains(prog.Disassemble(), "LOOKUP_DYN_SCOPE") {
		t.Fatalf("a binder-less name must not compile to a dynamic-scope lookup:\n%s", prog.Disassemble())
	}
	b, _ := New()
	gotC, _, errC := b.RunCompiled(src)
	c, _ := New()
	gotI, errI := c.RunInterp(src)
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("fallback parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
}

// A branch-arm dynamic read with no reachable binder keeps the arm's
// provenance refusal (the resolveArm decline), with faithful fallback.
func TestDynScopeUnreachableBranchArmStaysRefused(t *testing.T) {
	src := `def g fn [[] [Integer] [if (1 lte 0) [nosucharm] [2]]] g`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		return // a check-level error is equally sound
	}
	if prog != nil && strings.Contains(prog.Disassemble(), "LOOKUP_DYN_SCOPE") {
		t.Fatalf("a binder-less arm read must not compile to a lookup (reason %q):\n%s", reason, prog.Disassemble())
	}
	b, _ := New()
	gotC, _, errC := b.RunCompiled(src)
	c, _ := New()
	gotI, errI := c.RunInterp(src)
	if fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("fallback parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
}

// A dyn-read name whose per-frame def is a COMPUTED value the unit did not
// promote refuses at lowering (no way to duplicate the value for the
// registry install) — sound interpreter fallback.
func TestDynScopeUnpromotedComputedDefRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	src := `def f fn [[n:Integer] [Integer] [if (n lte 0) [acc3] [def acc3 (n add 1) f (n sub 1)]]] f 2`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	if prog != nil {
		// If promotion machinery later learns this shape, parity is the bar.
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(src)
		c, _ := New()
		gotI, errI := c.RunInterp(src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("computed dyn-def compiled without parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
		}
		return
	}
	if !strings.Contains(reason, "acc3") {
		t.Errorf("refusal %q should name the computed dyn-scope def", reason)
	}
	b, _ := New()
	gotC, compiled, errC := b.RunCompiled(src)
	c, _ := New()
	gotI, errI := c.RunInterp(src)
	if compiled || fmt.Sprint(errC) != fmt.Sprint(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("fallback parity: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
}
