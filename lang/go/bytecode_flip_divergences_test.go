package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The two live miscompiles the Stage-J Run flip's full-tree sweep surfaced
// (2026-07-15) — each pinned compiled-vs-interpreted directly so the pins
// hold regardless of what Run routes to.

// A multi-overload user fn with an fn-PREDICATE-typed param slot must route
// through the user-poly runtime re-match: check-mode sigTypeMatches accepts
// predicate types LENIENTLY (RunPredicate short-circuits true), so a static
// arm commit baked `classify -3` to the Pos arm and raised signature_error
// where the interpreter's runtime predicate run falls through to the Any arm.
func TestPredicateOverloadDispatchCompiledParity(t *testing.T) {
	src := `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def classify fn [
  [x:Pos] [String] ["positive"]
  [x:Any] [String] ["other"]
]
classify 5
classify -3
classify "hi"`
	a := mustNew(t)
	gotC, compiled, errC := a.RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	if fmt.Sprint(gotC) != "[positive other other]" {
		t.Errorf("compiled = %v, want [positive other other]", gotC)
	}
	b := mustNew(t)
	gotI, errI := b.RunInterp(src)
	if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("parity: compiled=%v interp=%v (err=%v)", gotC, gotI, errI)
	}

	// The hazardous sites ride CALL_USER_POLY; the String call has ONE
	// reachable arm (no hazard) and keeps the static CALL_USER commit.
	dis := compileDisasm(t, src)
	if strings.Count(dis, "CALL_USER_POLY") != 2 {
		t.Errorf("want the two Integer calls on CALL_USER_POLY:\n%s", dis)
	}
	if !strings.Contains(dis, "CALL_USER ") {
		t.Errorf("the single-reachable-arm String call must stay static:\n%s", dis)
	}

	// Negative: a SINGLE-overload predicate fn keeps the static unit — its
	// CALL_USER param guard re-validates at entry and raises exactly the
	// interpreter's no-match error.
	one := `def Pos fn [[n:Integer] [Boolean] [n gt 0]] def only fn [[x:Pos] [String] ["p"]] only -3`
	c := mustNew(t)
	_, _, errOC := c.RunCompiled(one)
	if noteCompileDefect(t, one, nil, errOC) {
		return
	}
	d := mustNew(t)
	_, errOI := d.RunInterp(one)
	if codeOf(errOC) != codeOf(errOI) || codeOf(errOC) == "" {
		t.Errorf("single-overload predicate error parity: compiled=[%s]%v interp=[%s]%v",
			codeOf(errOC), errOC, codeOf(errOI), errOI)
	}
	if oneDis := compileDisasm(t, one); strings.Contains(oneDis, "CALL_USER_POLY") {
		t.Errorf("single-overload predicate fn must not go poly:\n%s", oneDis)
	}

	// A ZERO-return overload set BAKES (REFUSAL-CLOSURE.0 §6a): every arm
	// nets zero values, so the call site records a 0-output poly call and
	// the VM's runtime re-match picks the arm — output parity included.
	zeroRet := `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def shout fn [
  [x:Pos] [] ["p" print]
  [x:Any] [] ["o" print]
]
shout -3`
	e := mustNew(t)
	var eOut bytes.Buffer
	e.SetOutput(&eOut)
	gotZ, compiledZ, errZ := e.RunCompiled(zeroRet)
	if noteCompileDefect(t, zeroRet, gotZ, errZ) {
		return
	}
	if !compiledZ || errZ != nil {
		t.Fatalf("zero-return poly run: compiled=%v err=%v", compiledZ, errZ)
	}
	f := mustNew(t)
	var fOut bytes.Buffer
	f.SetOutput(&fOut)
	gotZI, errZI := f.RunInterp(zeroRet)
	if errZI != nil || fmt.Sprint(gotZ) != fmt.Sprint(gotZI) || eOut.String() != fOut.String() {
		t.Errorf("zero-return poly parity: compiled=%v out=%q interp=%v out=%q (err=%v)",
			gotZ, eOut.String(), gotZI, fOut.String(), errZI)
	}
	if eOut.String() != "o\n" {
		t.Errorf("zero-return poly output = %q, want \"o\\n\"", eOut.String())
	}
	if zDis := compileDisasm(t, zeroRet); !strings.Contains(zDis, "CALL_USER_POLY") {
		t.Errorf("zero-return poly call must ride CALL_USER_POLY:\n%s", zDis)
	}

	// When the poly bake DECLINES — a zero-DECLARED-return arm whose body
	// leaves a residual (the interpreter's "residual IS the result" shape,
	// which a 0-output call site cannot carry) — the hazard refuses the
	// program: silently interpreted, so parity holds and the compile failure is hidden.
	declining := `def Pos fn [[n:Integer] [Boolean] [n gt 0]]
def zpick fn [
  [x:Pos] [] [x]
  [x:Any] [] [0]
]
zpick -3`
	g := mustNew(t)
	prog, reason, _, cerr := g.CompileCheck(declining)
	if cerr != nil || prog != nil ||
		!strings.Contains(reason, "fn-predicate-typed overload dispatch at `zpick`") {
		t.Errorf("declining poly bake must refuse with the hazard reason: prog=%v reason=%q err=%v",
			prog != nil, reason, cerr)
	}
	h := mustNew(t)
	if got, ierr := h.RunInterp(declining); ierr != nil || fmt.Sprint(got) != "[0]" {
		t.Errorf("the refused program must interpret cleanly: got=%v err=%v", got, ierr)
	}
}

// Two Xml element literals are two INSTANCES (`eq` is container identity):
// the const pool must never canon-merge identity-bearing payloads — the
// merge pushed ONE instance twice and `(<a/>) eq (<a/>)` answered true.
func TestXmlLiteralIdentityCompiledParity(t *testing.T) {
	a := mustNew(t)
	gotC, compiled, errC := a.RunCompiled(`(<a/>) eq (<a/>)`)
	if noteCompileDefect(t, `(<a/>) eq (<a/>)`, gotC, errC) {
		return
	}
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	if fmt.Sprint(gotC) != "[false]" {
		t.Errorf("compiled identity eq = %v, want [false]", gotC)
	}
	// Positive twin: ONE instance read twice through its binding IS eq to
	// itself — and the def-of-Xml-literal write-back (the stripped-literal
	// rescue in lowerDynBind) keeps the program compiling.
	b := mustNew(t)
	gotS, compiledS, errS := b.RunCompiled(`def x <a/> x eq x`)
	if noteCompileDefect(t, `def x <a/> x eq x`, gotS, errS) {
		return
	}
	if !compiledS || errS != nil {
		t.Fatalf("self-eq run: compiled=%v err=%v", compiledS, errS)
	}
	if fmt.Sprint(gotS) != "[true]" {
		t.Errorf("self identity eq = %v, want [true]", gotS)
	}
	// Structural deq stays value-based across distinct instances.
	c := mustNew(t)
	gotD, _, errD := c.RunCompiled(`(<a x="1"><b/></a>) deq (<a x="1"><b/></a>)`)
	if noteCompileDefect(t, `(<a x="1"><b/></a>) deq (<a x="1"><b/></a>)`, gotD, errD) {
		return
	}
	if errD != nil || fmt.Sprint(gotD) != "[true]" {
		t.Errorf("structural deq = %v (err=%v), want [true]", gotD, errD)
	}
}
