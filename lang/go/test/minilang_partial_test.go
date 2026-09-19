package test

// Minilang partial-function semantics — the parked shapes.
//
// Conceptually every mini expansion produces a function in place with
// src and opts bound; a FILTER kind's function auto-dispatches when a
// matching subject is on the stack and stays a VALUE when not (see
// lang/spec/module-minilang.tsv §fn for the spec-level battery). The
// two PARKED shapes below are pinned here as interpreter tests rather
// than spec rows, because the compiled pipeline does not yet lower an
// unconsumed / /v-parked spliced partial (the check-mode dispatch
// analysis consumes what the runtime parks — the same pre-existing
// /v modeling gap a plain lambda shows: `"AbcD" ([x:String] => [x])/v
// typeof` checks to [String] while the runtime leaves the string and
// the Function). Lowering them is follow-up compiler work; when it
// lands, these can graduate to spec rows.

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestMiniPartialParked pins the two parked shapes:
//  1. /v parks the partial as data even with a matching subject on the
//     stack — the subject is untouched;
//  2. a bare partial with an empty stack is simply a Function value.
func TestMiniPartialParked(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	got, err := runReference(t, a, miniImp+`"AbcD" (+re/[a-z]+/)/v typeof`)
	if err != nil {
		t.Fatalf("/v park: %v", err)
	}
	// Residual is [subject, typeof(partial)] — the subject survived
	// the park and the parked value is a Function.
	if len(got) != 2 {
		t.Fatalf("/v park: expected [subject, Function], got %v", got)
	}
	if s, _ := got[0].(string); s != "AbcD" {
		t.Fatalf("/v park: the subject must be untouched, got %v", got[0])
	}
	if s, _ := got[1].(string); s != "Function" {
		t.Fatalf("/v park: expected a Function on top, got %v", got[1])
	}

	b, _ := lang.New()
	got, err = b.Run(miniImp + `+re/[a-z]+/ typeof`)
	if err != nil {
		t.Fatalf("bare partial: %v", err)
	}
	if len(got) != 1 || got[0] != "Function" {
		t.Fatalf("bare partial: expected one Function value, got %v", got)
	}
}

// TestMiniPartialTypedParam pins the full typed-param flow: a fn whose
// param is the per-kind member type MiniLang.Re receives a re partial
// and CALLS it in the body. The spec row (module-minilang.tsv §fnty)
// keeps a non-calling body because the static checker does not yet
// model a call on a param-bound Function carrier (the fn-value-call
// frontier); the runtime flow is pinned here instead.
func TestMiniPartialTypedParam(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	got, err := runReference(t, a, miniImp+
		`def find-with fn [[m:(MiniLang.Re) s:String] [String] [(s m).fst.m]]  `+
		`find-with (+re/[a-z]+/) "AbcD"`)
	if err != nil {
		t.Fatalf("typed-param call: %v", err)
	}
	if len(got) != 1 || got[0] != "bc" {
		t.Fatalf("typed-param call: expected ['bc'], got %v", got)
	}

	// The negative half: a gex partial is refused by the Re slot.
	b, _ := lang.New()
	_, err = b.Run(miniImp +
		`def find-with fn [[m:(MiniLang.Re) s:String] [String] [(s m).fst.m]]  ` +
		`find-with (+gex/a*/) "AbcD"`)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("typed-param negative: expected a signature error, got %v", err)
	}
}

// TestMiniPartialStoredReuse pins that a parked partial is a real,
// reusable function: bind it, apply it to several subjects, and the
// bound pattern rides inside — with the negative half that a stored
// partial does NOT leak its source binding (a fresh literal with a
// different pattern is a different function).
func TestMiniPartialStoredReuse(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	got, err := a.Run(miniImp +
		`def digits (+re/[0-9]+/)  def letters (+re/[a-z]+/)  ` +
		`[ (("a1b22" digits).fst.m) (("a1b22" letters).fst.m) ]`)
	if err != nil {
		t.Fatalf("stored reuse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored reuse: expected one list, got %v", got)
	}
	s := fmt.Sprintf("%v", got[0])
	if !strings.Contains(s, "1") || !strings.Contains(s, "a") {
		t.Fatalf("stored partials leaked bindings: %s", s)
	}
}
