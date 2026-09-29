package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Merged-coverage wave 521: the vm.go arms no suite reached — a raising
// handler behind a modifier wrapper and a raising zero-argument run at
// CALL_DYNAMIC, a raising trailing native at CALL_DYNAMIC_MIXED, and the
// def-read lead of a shaped method's island. Each is reached at the helper
// it lives in, and paired with the same fixture's non-raising answer.

// pr521Sub is a self-contained two-param Go fn value (scFn's shape) that
// answers args[0] - args[1] in sig order.
func pr521Sub() core.Value {
	return scFn("pr521sub", 2, func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		x, _ := core.AsInteger(a[0])
		y, _ := core.AsInteger(a[1])
		return []core.Value{core.NewInteger(x - y)}, nil
	})
}

// pr521Boom is the same shape, whose handler raises.
func pr521Boom() core.Value {
	return scFn("pr521boom", 2, func(_ []core.Value, _ map[string]core.Value, _ []core.Value, r *core.Registry) ([]core.Value, error) {
		return nil, r.BoruError("value_error", "pr521boom: boom", "pr521boom")
	})
}

// TestPR521CallDynamicWrappedNativeError pins callDynamic's modifier-wrapper
// arm over a NATIVE fn value: a usurp wrapper resolves to what it wraps and
// that native applies VM-native over the reversed args (the wrapper itself
// is no native-applicable value), and a raising handler behind it surfaces
// its own error rather than islanding or answering a result.
func TestPR521CallDynamicWrappedNativeError(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	ten, three := core.NewInteger(10), core.NewInteger(3)

	flipped, ok := core.UsurpFunction(pr521Sub())
	if !ok {
		t.Fatal("usurp over a self-contained Go fn value")
	}
	if vmNativeApplicable(r, flipped.Data.(core.FnDefInfo)) {
		t.Fatal("the wrapper itself must not be native-applicable, or the unwrap arm is not what runs")
	}
	// Leading form: (flipped 10 3) — the wrapped sub sees [3 10].
	got, ent, err := vc.callDynamic(r, 2, false, []core.Value{core.NewInteger(99), flipped, ten, three}, seam7Dbg, 0)
	if err != nil || ent != nil {
		t.Fatalf("flipped apply: ent=%v err=%v", ent, err)
	}
	if !sameInts(intsOf(t, got), []int64{99, -7}) {
		t.Errorf("flipped sub over [10 3] = %v, want [99 -7] (sub 3 10, the value beneath kept)", got)
	}

	boom, _ := core.UsurpFunction(pr521Boom())
	got, ent, err = vc.callDynamic(r, 2, false, []core.Value{core.NewInteger(99), boom, ten, three}, seam7Dbg, 0)
	if got != nil || ent != nil {
		t.Errorf("a raising wrapped native answers nothing, got %v / %v", got, ent)
	}
	wantErr(t, err, "pr521boom: boom")
}

// TestPR521CallDynamicZeroArgError pins callDynamic's zero-argument arm
// when the run raises: a named fn value whose only signature takes nothing
// runs its unit over no args, and its return-contract error comes back from
// callDynamic itself — the contrast is the same value with a contract the
// unit keeps, which answers its result after the window's value.
func TestPR521CallDynamicZeroArgError(t *testing.T) {
	vc, r := foreignVC(t, oneConstProg(1))
	ref := &compiler.CompiledFnRef{Prog: oneConstProg(42), Unit: 0}
	one := core.NewInteger(1)

	good := zeroArgFn("g", false, zeroArgUnitSig(ref, core.TInteger))
	got, ent, err := vc.callDynamic(r, 1, false, []core.Value{good, one}, seam7Dbg, 0)
	if err != nil || ent != nil || !sameInts(intsOf(t, got), []int64{42, 1}) {
		t.Fatalf("a kept contract: got %v ent=%v err=%v, want [42 1]", got, ent, err)
	}

	bad := zeroArgFn("g", false, zeroArgUnitSig(ref, core.TString))
	got, ent, err = vc.callDynamic(r, 1, false, []core.Value{bad, one}, seam7Dbg, 0)
	if got != nil || ent != nil {
		t.Errorf("a broken contract answers nothing, got %v / %v", got, ent)
	}
	wantErr(t, err, "g: return value 1: expected String, got Integer")
}

// TestPR521DynMethodIslandLead pins dynMethodIslandLead: a def-read method
// leads its island with its NAME, positioned at the read's debug entry when
// pc is in range and bare when it is not; any other method leads with its
// value.
func TestPR521DynMethodIslandLead(t *testing.T) {
	fnVal := pr521Sub()
	dbg := []core.SrcPos{{Row: 4, Col: 9}}

	if lead := dynMethodIslandLead(&compiler.DynMethodSpec{Word: "j"}, fnVal, dbg, 0); !core.IsAppliableFn(lead) || core.IsWord(lead) {
		t.Errorf("a non-def-read method leads with its fn value, got %v", lead)
	}

	spec := &compiler.DynMethodSpec{Word: "j", DefRead: true}
	lead := dynMethodIslandLead(spec, fnVal, dbg, 0)
	if name, isWord := pr521WordName(lead); !isWord || name != "j" {
		t.Fatalf("a def-read method leads with its word, got %v", lead)
	}
	if p := lead.Pos(); p.Row != 4 || p.Col != 9 {
		t.Errorf("the word is placed at the read's position 4:9, got %v", p)
	}
	for _, pc := range []int{-1, 1} {
		bare := dynMethodIslandLead(spec, fnVal, dbg, pc)
		if name, isWord := pr521WordName(bare); !isWord || name != "j" {
			t.Fatalf("pc %d: a def-read method leads with its word, got %v", pc, bare)
		}
		if p := bare.Pos(); p.Row != 0 || p.Col != 0 {
			t.Errorf("pc %d out of range: the word carries no position, got %v", pc, p)
		}
	}
}

// TestPR521MixedTrailingNativeError pins callDynamicMixed's trailing-native
// lane: an inert window under one trailing native fn applies it VM-native,
// the top value filling its first param, and a raising handler surfaces its
// error from the op rather than falling to the island.
func TestPR521MixedTrailingNativeError(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	ten, three := core.NewInteger(10), core.NewInteger(3)

	got, err := vc.callDynamicMixed(r, 3, []core.Value{core.NewInteger(99), ten, three, pr521Sub()}, seam7Dbg, 0)
	if err != nil || !sameInts(intsOf(t, got), []int64{99, -7}) {
		t.Fatalf("10 3 sub-value: got %v / %v, want [99 -7] (sub 3 10, top first)", got, err)
	}

	got, err = vc.callDynamicMixed(r, 3, []core.Value{core.NewInteger(99), ten, three, pr521Boom()}, seam7Dbg, 0)
	if got != nil {
		t.Errorf("a raising trailing native answers nothing, got %v", got)
	}
	wantErr(t, err, "pr521boom: boom")
}

// pr521WordName reports v's word name, if v is a word.
func pr521WordName(v core.Value) (string, bool) {
	wi, ok := v.Data.(core.WordInfo)
	return wi.Name, ok && core.IsWord(v)
}
