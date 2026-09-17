package lang

import (
	"fmt"
	"testing"
)

// A Map-typed enclosing local, merely READ (narrowed-through-use) by a
// user-fn call inside BOTH `if` arms, then read AGAIN by a user-fn call
// after the join, used to refuse "fn call operand of unknown provenance"
// — even with identical arms. Root cause: the arms narrow the binding
// identity-preserving (narrowDynamicUses keeps the value's ID), but
// InstallJoinedDefs re-pushed the name as a fresh-ID JoinCarriers result,
// stranding every later reference as an unseated dynamic carrier. The fix
// (joinBranchDef in eng/go/carrier.go) preserves the shared ID when both
// arm carriers are the same identity — an identity no-op merge — so the
// binding's compile seat survives the branch.
//
// Reduced from the aless viewer's render fn (voxgig-boru/aless viewer/,
// dx-report 2026-07-21 "bytecode status"), which had to branch on a whole
// widget list rather than bind its middle pane to compile before this fix.
//
// Every case COMPILES and returns the interpreter's result on the VM. The
// controls (scalar local, no-if, no-post-reuse) already compiled before
// the fix and pin that the narrow-preservation did not regress the
// genuine-merge path.
func TestIfBranchMapOperandCompiles(t *testing.T) {
	cases := []struct {
		name, src string
		want      string
	}{
		{"both arms narrow the local, reused after join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab (state get "tabs")
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`, "[2]"},
		{"one arm narrows the local, reused after join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ 5 ])
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`, "[6]"},
		{"control: scalar local, same shape", `
def mkm fn [[m:Integer] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "n") otherwise 0)
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add (mkm (tab))
]]
render {mode: "x", n: 3}`, "[2]"},
		{"control: no if between the uses", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (mkm (tab))
  (pane) add (mkm (tab))
]]
render {mode: "x", tabs: {a: 1}}`, "[2]"},
		{"control: no reuse after the join", `
def mkm fn [[m:Map] [Integer] [ 1 ]]
def render fn [[state:Map] [Integer] [
  def tab ((state get "tabs") otherwise {})
  def pane (if ((state get "mode") eq "help") [ mkm (tab) ] [ mkm (tab) ])
  (pane) add 1
]]
render {mode: "x", tabs: {a: 1}}`, "[2]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New()
			if err != nil {
				t.Fatal(err)
			}
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck: %v", err)
			}
			if prog == nil {
				t.Fatalf("refused (%q) — the if-branch narrow-preservation regressed", reason)
			}

			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			got, compiled, err := b.RunCompiled(c.src)
			if err != nil {
				t.Fatalf("RunCompiled: %v", err)
			}
			if !compiled {
				t.Fatal("must run on the VM, not fall back")
			}
			if fmt.Sprintf("%v", got) != c.want {
				t.Errorf("compiled result %v, want %s", got, c.want)
			}

			// Compilation is sound by construction, but pin fallback-parity
			// anyway: the VM result equals the interpreter's.
			i, err := New()
			if err != nil {
				t.Fatal(err)
			}
			gotI, err := i.RunInterp(c.src)
			if err != nil {
				t.Fatalf("RunInterp: %v", err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", gotI) {
				t.Errorf("VM %v != interp %v", got, gotI)
			}
		})
	}
}

// A GENUINE reassignment inside an arm (a real `def` of a DIFFERENT value,
// not a narrow) gives the arms DIFFERING IDs, so joinBranchDef keeps the
// fresh-ID JoinCarriers merge — the shared-ID shortcut must NOT fire here.
// The shape may REFUSE (the merged binding read after the branch
// is its own known gap), but it must never MISCOMPILE: whenever it does
// compile, the VM result equals the interpreter's. This pins that the
// narrow-preservation did not weaken the genuine-merge path into a
// wrong-value collision.
func TestIfBranchGenuineReassignNoMiscompile(t *testing.T) {
	src := `
def render fn [[state:Map] [Integer] [
  def tab (state get "n")
  def _ (if ((state get "mode") eq "help") [ def tab 99  0 ] [ 0 ])
  tab
]]
render {mode: "x", n: 7}`
	i, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotI, err := i.RunInterp(src)
	if err != nil {
		t.Fatalf("RunInterp: %v", err)
	}
	if fmt.Sprintf("%v", gotI) != "[7]" {
		t.Fatalf("interp %v, want [7] (else path keeps tab)", gotI)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck: %v", err)
	}
	if prog == nil {
		return // a refusal (its own separate gap) — not a miscompile
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := b.RunCompiled(src)
	if err != nil {
		t.Fatalf("RunCompiled: %v", err)
	}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", gotI) {
		t.Errorf("genuine-reassign miscompile: VM %v != interp %v", got, gotI)
	}
}
