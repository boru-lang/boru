package native

// The await RESULT MODEL, exercised directly. The shapes it can prove come
// from doAwait's own statement order, and the two declines (a non-concrete
// parallels list, a mode whose residual is the winning branch's whole
// residual) are not reachable through the spec corpus — a run always has a
// concrete list, and `first` / `any` are non-deterministic. So they are
// driven here, against the same functions the handler dispatches through.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// wantResidual asserts a one-value residual whose carrier node is want.
func wantResidual(t *testing.T, label string, got []Value, want *Type) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("%s: got %d residual values, want 1", label, len(got))
	}
	if got[0].Parent != want {
		t.Errorf("%s: residual %v, want %v", label, got[0].Parent, want)
	}
}

func TestAwaitResidualDeclines(t *testing.T) {
	// A non-concrete parallels list: doAwait raises before it reads a mode,
	// so there is no residual to model.
	wantResidual(t, "carrier parallels", awaitResidual(nil, "all", NewCarrier(TList)), TAny)

	// An unknown mode raises — not modellable.
	wantResidual(t, "mode bogus",
		awaitResidual(nil, "bogus", NewList([]Value{NewList(nil)})), TAny)
}

// first / any hand back the WINNING branch's whole residual — any type at
// ANY ARITY, including zero — so the model is the variadic spread, not a
// one-seat Any (NUR067). The element is the deliberate Any of an unbounded
// winner (the SpreadPayload contract note).
func TestAwaitResidualWinnerModesAreVariadic(t *testing.T) {
	nonEmpty := NewList([]Value{NewList(nil)})
	for _, mode := range []string{"first", "any"} {
		got := awaitResidual(nil, mode, nonEmpty)
		if len(got) != 1 {
			t.Fatalf("%s: got %d residual values, want the one spread", mode, len(got))
		}
		elem, ok := core.IsVariadicSpread(got[0])
		if !ok {
			t.Fatalf("%s: residual %+v, want a variadic-spread carrier", mode, got[0])
		}
		if !ValueType(elem).Equal(TAny) {
			t.Errorf("%s: spread element %v, want the deliberate Any", mode, elem)
		}
		// A NON-CONCRETE parallels list is variadic too: the runtime list
		// could be empty (one List) or not (the winner's residual), so only
		// the spread covers both counts.
		carrier := awaitResidual(nil, mode, NewCarrier(TList))
		if _, ok := core.IsVariadicSpread(carrier[0]); !ok {
			t.Errorf("%s: carrier parallels must model variadic, got %+v", mode, carrier[0])
		}
	}
}

func TestAwaitResidualEmptyListPrecedesTheMode(t *testing.T) {
	// doAwait returns the empty List BEFORE the mode switch, so an empty
	// parallels list models as a List for every mode — including one with
	// no runner at all. This is the ordering the mirror has to honour.
	for _, mode := range []string{"all", "full", "first", "any", "bogus"} {
		wantResidual(t, "empty/"+mode, awaitResidual(nil, mode, NewList(nil)), TList)
	}
}

// The emptiness predicate is shared by the mirror and the result model, and
// this is the test that keeps them shared: two copies of it drifted once
// already, and in the MIRROR the drift flags an error-severity diagnostic on
// a program doAwait runs to completion.
func TestAwaitParallelsEmptyMatchesTheHandler(t *testing.T) {
	// A NIL payload is empty, exactly as doAwait's len(Slice()) reads it.
	if !awaitParallelsEmpty(NewList(nil)) {
		t.Error("nil-backed list: not empty, but doAwait returns the empty List for it")
	}
	if !awaitParallelsEmpty(NewList([]Value{})) {
		t.Error("zero-length list: not empty")
	}
	if awaitParallelsEmpty(NewList([]Value{NewList(nil)})) {
		t.Error("one-branch list reads as empty — the mode switch would be skipped")
	}
	// A non-list cannot be asked for a length; the gate keeps it out, and the
	// predicate must not claim emptiness for it either.
	if awaitParallelsEmpty(NewInteger(5)) {
		t.Error("non-list reads as empty")
	}
}

func TestAwaitResidualKnownModes(t *testing.T) {
	nonEmpty := NewList([]Value{NewList(nil)})

	// full has no error escape: always a List, and a list of MAPS.
	full := awaitResidual(nil, "full", nonEmpty)
	wantResidual(t, "full", full, TList)
	ci, ok := full[0].Data.(ChildTypeInfo)
	if !ok || ci.Child.Parent != TMap {
		t.Errorf("full: element carrier %+v (typed=%v), want a Map", full[0].Data, ok)
	}

	// all is List-or-Error, named as a dynamic union so the claim does not
	// distribute over dispatch (a strict disjunct refuses `… get 0`).
	all := awaitResidual(nil, "all", nonEmpty)
	if len(all) != 1 {
		t.Fatalf("all: got %d residual values, want 1", len(all))
	}
	if !all[0].Dynamic {
		t.Errorf("all: residual is strict; a strict disjunct false-positives on list consumers")
	}
	di, err := AsDisjunct(all[0])
	if err != nil {
		t.Fatalf("all: residual is not a disjunct: %v", err)
	}
	// The alternatives are bare type NODES, so the node is ValueType, not
	// Parent — the same discrimination typeCovered makes before it decomposes
	// a disjunct. Compared with Equal, not pointer identity: ValueType hands
	// back the address of a by-value type literal, which is exactly the
	// non-canonical pointer CanonicalType exists to resolve.
	if len(di.Alternatives) != 2 ||
		!ValueType(di.Alternatives[0]).Equal(TList) ||
		!ValueType(di.Alternatives[1]).Equal(TError) {
		t.Errorf("all: alternatives %v, want [List Error]", di.Alternatives)
	}
}

func TestAwaitOptsReturnsDeclinesUnreadableOptions(t *testing.T) {
	// The mode has to be READ before it can select a shape: an options
	// carrier resolves nothing — including the ARITY, since a runtime
	// first/any mode is reachable, so the model is the variadic spread
	// (NUR067), never the one-seat "all" default (that default belongs to a
	// map that is present and simply missing a mode key).
	unread := awaitOptsReturns([]Value{NewCarrier(TOptions), NewList(nil)}, nil)
	if len(unread) != 1 {
		t.Fatalf("options carrier: got %d residual values, want the one spread", len(unread))
	}
	if _, ok := core.IsVariadicSpread(unread[0]); !ok {
		t.Errorf("options carrier: residual %+v, want a variadic-spread carrier", unread[0])
	}

	// Short args cannot reach the reader either. A ReturnsFn is called with
	// the matched operands, so this is defensive — but the rule is that a
	// handler-side index is guarded, not that it is provably safe.
	short := awaitOptsReturns(nil, nil)
	if len(short) != 1 {
		t.Fatalf("short args: got %d residual values, want the one spread", len(short))
	}
	if _, ok := core.IsVariadicSpread(short[0]); !ok {
		t.Errorf("short args: residual %+v, want a variadic-spread carrier", short[0])
	}
	wantResidual(t, "short args (1-arg form)", awaitDefaultReturns(nil, nil), TAny)
}

func TestAwaitModeNameFallsBackToDefault(t *testing.T) {
	if got := awaitModeName(NewString("full"), "all"); got != "full" {
		t.Errorf("String mode: %q, want full", got)
	}
	if got := awaitModeName(NewAtom("first"), "all"); got != "first" {
		t.Errorf("Atom mode: %q, want first", got)
	}
	// Neither String nor Atom: the handler keeps its default rather than
	// raising, and the model has to inherit that.
	if got := awaitModeName(NewInteger(5), "all"); got != "all" {
		t.Errorf("Integer mode: %q, want the default all", got)
	}
}

func TestAwaitRunnerTableMatchesTheMirror(t *testing.T) {
	for _, mode := range []string{"all", "full", "first", "any"} {
		if awaitRunner(mode) == nil {
			t.Errorf("awaitRunner(%q) is nil — the mirror would flag a mode that runs", mode)
		}
	}
	if awaitRunner("bogus") != nil {
		t.Error("awaitRunner(bogus) is non-nil — the mirror would miss a mode that raises")
	}
}

// The winner-takes-all model is ONE model, not two (NUR067's graduation):
// the compile pass records the SAME variadic-spread carrier the plain pass
// returns, and the recorder turns that one out into a runtime-variadic
// REGION event (callVariadicRegion → eventFlags.variadicRegion). The
// wholesale MarkUncompilable this used to assert is gone — a region has a
// representation now, so the fixed-arity consumers refuse individually
// instead of the whole program refusing up front.
func TestAwaitVariadicResultModelsARegionOnBothPasses(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := reg.Check.Begin()
	defer done()
	plain := awaitVariadicResult(reg)
	reg.Check.Compiling = true
	defer func() { reg.Check.Compiling = false }()
	got := awaitVariadicResult(reg)
	if len(got) != 1 {
		t.Fatalf("compile pass: got %d residual values, want 1", len(got))
	}
	elem, spread := core.IsVariadicSpread(got[0])
	if !spread {
		t.Fatalf("compile pass residual %+v, want the variadic-spread carrier", got[0])
	}
	if !core.ValueType(elem).Equal(TAny) {
		t.Errorf("compile pass element %+v, want the deliberate Any", elem)
	}
	if len(plain) != 1 {
		t.Fatalf("plain pass: got %d residual values, want 1", len(plain))
	}
	if _, plainSpread := core.IsVariadicSpread(plain[0]); !plainSpread {
		t.Errorf("plain pass residual %+v, want the same spread carrier", plain[0])
	}
	// The two passes agree by IDENTITY, not by coincidence: one model, so a
	// future divergence (a compile-only seat, the shape this graduated from)
	// fails here rather than at a corpus row.
	if core.Canon(got) != core.Canon(plain) {
		t.Errorf("compile residual %s, plain %s — the two passes must model one region", core.Canon(got), core.Canon(plain))
	}
}
