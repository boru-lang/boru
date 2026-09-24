package lang

import (
	"fmt"
	"testing"
)

// foreign_fn_value_apply_test.go pins the foreign-home fn value at the apply
// seam (2026-09-24, the interp-entry census's callbacks.tsv:L146,
// module-composition.tsv:L100 and module-fnvalue-boundary.tsv:L51): a fn
// VALUE whose matched overload carries a DETACHED compiled unit — a module
// fn's own stamp, its Program not the running one — is hosted nested by the
// VM's dynamic-apply sites (eng dynApplyForeign, runForeignUnit's
// discipline) where dynApplyEnter, which enters only an in-program unit as a
// frame, used to decline it to an island. The unit runs where its Program
// put it, so the module fn's free words resolve at the module (`apply1 A.pub
// 5` beside a main-program `secret` answers the module's 6), a raise inside
// it is the interpreter's error, and a value the window does not fit, or a
// site's result-count claim the contract does not promise, keeps the island
// and its parity.

const modInc = `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end `

var foreignFnValueApplyRows = []struct {
	label, src, want string
	native           bool
}{
	{"one export passed into another as its callback (L146)", `import module [def inc fn [[n:Integer][Integer][n add 1]] def run fn [[f:Function x:Integer][Integer][(f x)]] export "M" {inc: inc/v, run: run/v}] end M.run M.inc 5`, "[6]", true},
	{"an export fetched by get and applied (L100)", modInc + `def m {f: M.inc/v} end 5 (m 'f' get) apply`, "[6]", true},
	{"a named Function param keeps the argument's scope (L51)", `import module [def secret fn [[n:Integer] [Integer] [n add 1]] def pub fn [[n:Integer] [Integer] [secret n]] export "A" {pub: pub/v}] end def secret fn [[n:Integer] [Integer] [n mul 100]] def apply1 fn [[f:Function n:Integer] [Integer] [f n]] apply1 A.pub 5`, "[6]", true},
	{"a paren apply of a Function param", modInc + `def run fn [[f:Function x:Integer][Integer][(f x)]] end run M.inc 5`, "[6]", true},
	{"a callback lambda applying its Function param over a module value", modInc + `each ([f:Function] => [(f 5)]) [M.inc/v]`, "[[6]]", true},
	{"the module value under a Function param, twice", modInc + `def twice fn [[f:Function x:Integer][Integer][(f (f x))]] end twice M.inc 5`, "[7]", true},
	{"a callback lambda applying a module fn fetched from an exported map (L75)", `import module [def h1 fn n:Integer Integer [n add 1] def h2 fn n:Integer Integer [n mul 10] def tbl {inc: h1/v ten: h2/v} export "M" {tbl: tbl}] end each ([k:String] => [((M.tbl k get) 4)]) ['inc' 'ten']`, "[[5 40]]", true},
}

// TestForeignFnValueApplyClaimKeepsIsland: a two-return module fn where
// `apply` claims ONE result. The contract does not promise the claim, so the
// arm stands aside before running anything (a hosted unit has run, effects
// and all, before its results could be counted) and the island answers as
// it always did — which, for this shape, is the pre-existing gradual-lead
// bail (`apply over a gradual lead netted 2 value(s), not the one the model
// committed`, the same on main before this seam: the apply-one model's
// defect, counted in the lang ledger's bail line, not this arm's). The row
// exists to witness the decline; the interpreter's `[5 6]` is the answer
// the bail owes.
func TestForeignFnValueApplyClaimKeepsIsland(t *testing.T) {
	src := `import module [def two fn [[n:Integer][Integer Integer][n (n add 1)]] export "M" {two: two/v}] end def m {f: M.two/v} end 5 (m 'f' get) apply`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[5 6]" {
		t.Fatalf("interp %v/%v, want [5 6]", gotI, errI)
	}
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil || fmt.Sprint(gotC) != "[5 6]" {
		t.Errorf("compiled %v/%v (%v), want [5 6]", gotC, errC, compiled)
	}
}

func TestForeignFnValueApplyParity(t *testing.T) {
	for _, row := range foreignFnValueApplyRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		var entries []string
		disarm := b.ArmInterpEntryHook(func(ev InterpEntry) {
			if ev.Attribution == "" {
				entries = append(entries, ev.Seam)
			}
		})
		gotC, compiled, errC := b.RunCompiled(row.src)
		disarm()
		if errI != nil || errC != nil || !compiled || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != row.want {
			t.Errorf("%s: compiled %v/%v (%v) interp %v/%v, want %s\n  %s", row.label, gotC, errC, compiled, gotI, errI, row.want, row.src)
		}
		if row.native && len(entries) != 0 {
			t.Errorf("%s: the compiled lane entered the interpreter via %v\n  %s", row.label, entries, row.src)
		}
		if !row.native && len(entries) == 0 {
			t.Errorf("%s: measured open — the row now runs natively; move it to the native rows\n  %s", row.label, row.src)
		}
	}
}

// TestForeignFnValueApplyRaisesAlike: a raise inside the hosted unit, and the
// no-match of a value the window does not fit, are the interpreter's errors —
// named, detailed and positioned alike.
func TestForeignFnValueApplyRaisesAlike(t *testing.T) {
	for _, src := range []string{
		`import module [def boom fn [[n:Integer][Integer][raise bad_input "boom"]] export "M" {boom: boom/v}] end def run fn [[f:Function x:Integer][Integer][(f x)]] end run M.boom 5`,
		modInc + `def run fn [[f:Function x:Any][Any][(f x)]] end run M.inc 's'`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errI == nil || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
			t.Errorf("%q: compiled %v [%s] %s (%v), interp %v [%s] %s", src, gotC, codeOf(errC), detailOf(errC), compiled, gotI, codeOf(errI), detailOf(errI))
		}
	}
}
