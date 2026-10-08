package compiler

// NUR037: a fn declared inside another fn's body and named as a
// higher-order body word (`for-each [step] xs`) resolves under the
// interpreter and was undefined under the compiled path — the island
// span, the CALL_NATIVE const-bake, and the closure probe all baked the
// NAME against the check-time registry, which the VM's runtime registry
// never binds. bodyRefsFnLocalFn detects the shape at the
// recordDispatchOutcome seam and declines the whole program ("slow, not
// wrong"). These tests pin the predicate's scope rule — fn-local
// FUNCTION bindings only — and the compile failure wiring, positive and
// negative per the repo's test discipline.

import (
	"testing"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// fnLocalBind pushes a fn-body baseline and then binds name INSIDE it,
// so Depth(name) > baseline[name] — the ComputeCaptures fn-local rule.
func fnLocalBind(r *core.Registry, name string, v core.Value) {
	r.PushFnBaseline(r.Defs.Snapshot())
	r.Defs.Push(name, v)
}

// bodySig builds a for-each-shaped sig: body at position 0 (NoEvalArgs),
// data at position 1.
func bodySig(effect core.CompileEffect, callable *core.CallableSpec) *core.Signature {
	return &core.Signature{
		Args:          []*core.Type{core.TList, core.TList},
		NoEvalArgs:    map[int]bool{0: true},
		CompileEffect: effect,
		Callable:      callable,
	}
}

func bodyArgs(bodyWords ...string) []core.Value {
	toks := make([]core.Value, len(bodyWords))
	for i, w := range bodyWords {
		toks[i] = core.NewWord(w)
	}
	return []core.Value{core.NewList(toks), core.NewList([]core.Value{core.NewInteger(1)})}
}

func TestBodyRefsFnLocalFnPositive(t *testing.T) {
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("step"))
	if !hit || name != "step" {
		t.Errorf("fn-local fn body word: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnCallableGate(t *testing.T) {
	// The Callable disjunct of the family gate: a closure-eligible word
	// with no CompileFallbackBody effect still matches.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	sig := bodySig(0, &core.CallableSpec{BodyPos: 0, BodyOut: 1})
	if name, hit := check.BodyRefsFnLocalFn(r, sig, bodyArgs("step")); !hit || name != "step" {
		t.Errorf("Callable-gated sig: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnRepeatedNameEarlyOut(t *testing.T) {
	// A body naming the fn twice exercises the walker's first-hit
	// early-out; the result is the same single name.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("step", "step"))
	if !hit || name != "step" {
		t.Errorf("repeated body word: got (%q, %v), want (step, true)", name, hit)
	}
}

func TestBodyRefsFnLocalFnModuleScopeBaselineNil(t *testing.T) {
	// Module scope (no enclosing fn): nothing is fn-local, callbacks
	// keep compiling.
	r := newTestRegistry(t)
	r.Defs.Push("step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("step")); hit {
		t.Errorf("module-scope binding with nil baseline matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnModuleScopeBindingUnderBaseline(t *testing.T) {
	// A module-scope fn referenced from INSIDE a fn body: bound before
	// the baseline snapshot, so Depth ≤ baseline — not fn-local.
	r := newTestRegistry(t)
	r.Defs.Push("step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	r.PushFnBaseline(r.Defs.Snapshot())
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("step")); hit {
		t.Errorf("module-scope binding under the baseline matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnValueDefNotMatched(t *testing.T) {
	// A fn-local VALUE def (the mini-redis KEYS accumulator shape) is
	// the closure path's lexical-capture territory — must NOT match.
	r := newTestRegistry(t)
	fnLocalBind(r, "acc", core.NewInteger(7))
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("acc")); hit {
		t.Errorf("fn-local VALUE def matched %q; only Function bindings may match", name)
	}
}

func TestBodyRefsFnLocalFnUnboundWordSkipped(t *testing.T) {
	// A forward ref / registered-native name has no Defs binding: skipped.
	r := newTestRegistry(t)
	r.PushFnBaseline(r.Defs.Snapshot())
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), bodyArgs("nosuch")); hit {
		t.Errorf("unbound body word matched %q; must not", name)
	}
}

func TestBodyRefsFnLocalFnNilSigAndNoBodies(t *testing.T) {
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	if _, hit := check.BodyRefsFnLocalFn(r, nil, bodyArgs("step")); hit {
		t.Error("nil sig must decline")
	}
	plain := &core.Signature{Args: []*core.Type{core.TList}, CompileEffect: core.CompileFallbackBody}
	if _, hit := check.BodyRefsFnLocalFn(r, plain, bodyArgs("step")); hit {
		t.Error("a sig with no NoEvalArgs positions must decline")
	}
}

func TestBodyRefsFnLocalFnStructuredWordExcluded(t *testing.T) {
	// The family gate: a structured-lowering word (if / for — NoEvalArgs
	// but neither CompileFallbackBody nor Callable) records its bodies as
	// inline events (fn-local dispatch lowers to CALL_USER by unit ref),
	// so it must keep compiling.
	r := newTestRegistry(t)
	fnLocalBind(r, "g", core.NewFunction(core.FnDefInfo{Name: "g"}))
	sig := bodySig(0, nil)
	if name, hit := check.BodyRefsFnLocalFn(r, sig, bodyArgs("g")); hit {
		t.Errorf("structured-lowering sig matched %q; the family gate must exclude it", name)
	}
}

func TestBodyRefsFnLocalFnNonBodyArgNotWalked(t *testing.T) {
	// The fn-local fn appears only inside the DATA arg (position 1, no
	// NoEvalArgs): data is evaluated, not name-baked — must not match.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	args := []core.Value{
		core.NewList([]core.Value{core.NewWord("add")}),  // body: registered-word only
		core.NewList([]core.Value{core.NewWord("step")}), // data arg mentions the fn
	}
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), args); hit {
		t.Errorf("non-NoEvalArgs data arg was walked (matched %q); must not", name)
	}
}

func TestBodyRefsFnLocalFnLambdaBodyOpaque(t *testing.T) {
	// A LAMBDA body operand is an FnDefInfo payload — WalkBodyWords
	// treats it as opaque (its own capture analysis handled it), so the
	// predicate must not fire on the lambda value itself.
	r := newTestRegistry(t)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	args := []core.Value{
		core.NewFunction(core.FnDefInfo{Name: ""}),
		core.NewList([]core.Value{core.NewInteger(1)}),
	}
	if name, hit := check.BodyRefsFnLocalFn(r, bodySig(core.CompileFallbackBody, nil), args); hit {
		t.Errorf("lambda body operand matched %q; FnDefInfo payloads are opaque", name)
	}
}

// --- recordDispatchOutcome wiring -------------------------------------------

// BodyRefsFnLocalFns (review of #468): every fn-local fn the bodies name,
// each once, in walk order; a `/v` or `/u` reference marks a value read;
// the same gates as the single-name form.
func TestBodyRefsFnLocalFnsAllNamesAndValueReads(t *testing.T) {
	r := newTestRegistry(t)
	fnLocalBind(r, "f", core.NewFunction(core.FnDefInfo{Name: "f"}))
	r.Defs.Push("h", core.NewFunction(core.FnDefInfo{Name: "h"}))
	r.Defs.Push("n", core.NewInteger(3))
	sig := bodySig(core.CompileFallbackBody, nil)
	names, valueRead := check.BodyRefsFnLocalFns(r, sig, bodyArgs("f", "n", "h", "f", "nosuch"))
	if len(names) != 2 || names[0] != "f" || names[1] != "h" || valueRead {
		t.Fatalf("names=%v valueRead=%v; want [f h] and no value read", names, valueRead)
	}
	if name, hit := check.BodyRefsFnLocalFn(r, sig, bodyArgs("h", "f")); !hit || name != "h" {
		t.Fatalf("the single-name form reports the first: %q %v", name, hit)
	}
	body := []core.Value{core.NewList([]core.Value{core.NewWord("f"), core.NewWordRef("h")}), core.NewList(nil)}
	if names, valueRead := check.BodyRefsFnLocalFns(r, sig, body); len(names) != 2 || !valueRead {
		t.Fatalf("a /v read of a local: names=%v valueRead=%v", names, valueRead)
	}
	body = []core.Value{core.NewList([]core.Value{core.NewWordUsurp("f", false)}), core.NewList(nil)}
	if names, valueRead := check.BodyRefsFnLocalFns(r, sig, body); len(names) != 1 || !valueRead {
		t.Fatalf("a /u read of a local: names=%v valueRead=%v", names, valueRead)
	}
	body = []core.Value{core.NewList([]core.Value{core.NewWordRef("n")}), core.NewList(nil)}
	if names, valueRead := check.BodyRefsFnLocalFns(r, sig, body); len(names) != 0 || valueRead {
		t.Fatalf("a /v read of a VALUE local is not a fn-local read: names=%v valueRead=%v", names, valueRead)
	}
	if names, valueRead := check.BodyRefsFnLocalFns(r, nil, bodyArgs("f")); names != nil || valueRead {
		t.Fatalf("nil sig: names=%v valueRead=%v", names, valueRead)
	}
	r2 := newTestRegistry(t)
	r2.Defs.Push("f", core.NewFunction(core.FnDefInfo{Name: "f"}))
	if names, _ := check.BodyRefsFnLocalFns(r2, sig, bodyArgs("f")); names != nil {
		t.Fatalf("module scope: names=%v", names)
	}
}
