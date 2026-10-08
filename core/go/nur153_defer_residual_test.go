package core

import (
	"strings"
	"testing"
)

// NUR153's rule at the CallBoru seam (core/go/registry.go), ruled 2026-09-18
// and stated by ResidualEvalsInFrame: an anonymous `=>` whose body is a single
// bare container DEFERS that container past the frame, so the seam's sub-run
// holds its end-of-run sweep (Engine.DeferResidual) and the container is swept
// AFTER the teardown — in the scope a tape consumer would have evaluated it in,
// where the frame's own bindings are gone.
func TestCallBoruNamedDefersAnonymousContainerResidual(t *testing.T) {
	r := poolTestRegistry(t)
	// A name the MODULE scope carries, which the deferred container must see.
	r.Defs.Set("zzOuter", []Value{NewInteger(99)})

	body := NewList([]Value{NewWord("zzOuter")})
	body.Eval = true
	sig := &FnSig{Anonymous: true, Impl: Boru([]Value{body})}

	got, err := r.CallBoruNamed(sig, nil, nil, "zz-defer")
	if err != nil {
		t.Fatalf("deferring body: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one residual, got %v", got)
	}
	// Swept after the teardown: the container is evaluated (not left pending),
	// and its bare word resolved where the consumer would have resolved it.
	elems, lerr := AsList(got[0])
	if lerr != nil || elems.Len() != 1 {
		t.Fatalf("the deferred container must be swept to a list of one, got %v (%v)", got[0], lerr)
	}
	if n, nerr := AsInteger(elems.Get(0)); nerr != nil || n != 99 {
		t.Fatalf("the deferred container resolved %v, want the module binding 99", elems.Get(0))
	}

	// A deferred residual that is NOT an evaluating container passes through
	// the sweep untouched — the sweep resolves what a consumer would have
	// resolved and leaves everything else exactly as the frame produced it.
	scalar := &FnSig{Anonymous: true, Impl: Boru([]Value{NewInteger(7)})}
	sv, serr := r.CallBoruNamed(scalar, nil, nil, "zz-defer-scalar")
	if serr != nil {
		t.Fatalf("scalar residual: %v", serr)
	}
	if len(sv) != 1 {
		t.Fatalf("want one residual, got %v", sv)
	}
	if n, nerr := AsInteger(sv[0]); nerr != nil || n != 7 {
		t.Fatalf("the swept scalar = %v, want 7 unchanged", sv[0])
	}

	// …and an EVALUATING value that is not a plain list or map falls through
	// the sweep's container arms to the same pass-through.
	marked := NewString("zz")
	marked.Eval = true
	other := &FnSig{Anonymous: true, Impl: Boru([]Value{marked})}
	ov, oerr := r.CallBoruNamed(other, nil, nil, "zz-defer-other")
	if oerr != nil {
		t.Fatalf("non-container evaluating residual: %v", oerr)
	}
	if len(ov) != 1 {
		t.Fatalf("want one residual, got %v", ov)
	}

	// A NAMED (non-anonymous) sig evaluates in the live frame instead — the
	// same predicate's other arm, so the sweep never runs twice.
	named := &FnSig{Impl: Boru([]Value{body})}
	if _, err := r.CallBoruNamed(named, nil, nil, "zz-inframe"); err != nil {
		t.Fatalf("in-frame body: %v", err)
	}
}

// The deferred sweep PROPAGATES its error: a container the frame's teardown
// leaves unresolvable raises from CallBoruNamed itself, which is how the seam
// answers what the tape answers (both raise `undefined_word`).
func TestCallBoruNamedDeferredSweepPropagatesError(t *testing.T) {
	r := poolTestRegistry(t)
	body := NewList([]Value{NewWord("zzNeverDefined")})
	body.Eval = true
	sig := &FnSig{Anonymous: true, Impl: Boru([]Value{body})}

	_, err := r.CallBoruNamed(sig, nil, nil, "zz-defer-err")
	if err == nil {
		t.Fatal("a deferred container naming an unbound word must raise from the sweep")
	}
	if !strings.Contains(err.Error(), "zzNeverDefined") {
		t.Fatalf("the sweep's error must name the unresolved word, got %v", err)
	}
}

// ResidualEvalsInFrame is the one predicate every seam asks; pin both arms and
// the computing-body escape the migration uses.
func TestResidualEvalsInFrameArms(t *testing.T) {
	bare := NewList([]Value{NewWord("x")})
	for _, c := range []struct {
		name      string
		anonymous bool
		body      []Value
		want      bool
	}{
		{"named fn, single container", false, []Value{bare}, true},
		{"anonymous, single bare container", true, []Value{bare}, false},
		{"anonymous, multi-token body", true, []Value{bare, bare}, true},
		// Vacuous: an empty body leaves no residual to defer, and the
		// predicate answers on the body's SHAPE, not on what it produced.
		{"anonymous, empty body", true, nil, false},
	} {
		if got := ResidualEvalsInFrame(c.anonymous, c.body); got != c.want {
			t.Errorf("%s: ResidualEvalsInFrame = %v, want %v", c.name, got, c.want)
		}
	}
}
