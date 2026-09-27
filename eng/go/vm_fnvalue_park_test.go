package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// vm_fnvalue_park_test.go pins fnValueNoMatchVerdict's fork: the park and the
// raise it answers, and every window it must leave to the island — a guard
// that declines, and a window the interpreter's step WOULD dispatch through
// one of its three matchers (the plan, the /s retry, the legacy pure-stack
// path), each reached here with the others failing.

func parkReg(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// parkFn builds a fn value over the given signatures; barrier -1 is the
// authored all-forward sentinel.
func parkFn(name string, anon bool, sigs ...core.Signature) (core.Value, core.FnDefInfo) {
	fd := core.FnDefInfo{Name: name, Anonymous: anon, Signatures: sigs}
	return core.NewFunction(fd), fd
}

func parkSig(barrier int, params ...core.FnParam) core.Signature {
	return core.Signature{Params: params, BarrierPos: barrier, Returns: []*core.Type{core.TAny}, Impl: core.Boru([]core.Value{core.NewInteger(0)})}
}

func intParam(name string) core.FnParam { return core.FnParam{Name: name, Type: core.TInteger} }
func strParam(name string) core.FnParam { return core.FnParam{Name: name, Type: core.TString} }

func TestFnValueNoMatchVerdictParkAndRaise(t *testing.T) {
	r := parkReg(t)
	s := core.NewString("x")
	anon, _ := parkFn("", true, parkSig(-1, intParam("a")))
	if v, _ := fnValueNoMatchVerdict(r, anon, nil, []core.Value{s}); v != noMatchPark {
		t.Errorf("an anonymous value over a rejected forward arg parks: %v", v)
	}
	if v, _ := fnValueNoMatchVerdict(r, anon, []core.Value{s}, nil); v != noMatchPark {
		t.Errorf("an anonymous value over a rejected stack arg parks: %v", v)
	}
	named, _ := parkFn("g", false, parkSig(-1, intParam("a")))
	v, fd := fnValueNoMatchVerdict(r, named, nil, []core.Value{s})
	if v != noMatchRaise || fd.Name != "g" {
		t.Errorf("a named value with a candidate raises: %v %q", v, fd.Name)
	}
	// With no candidate operand at all the named value is data too: nothing
	// was offered to call it with. (A 2-param value over one rejected arg.)
	if v, _ := fnValueNoMatchVerdict(r, named, nil, nil); v != noMatchPark {
		t.Errorf("a named value with no candidate parks: %v", v)
	}
	err := uncalledAt(r, named, fd, []core.Value{core.WithPosAt(s, core.SrcPos{Row: 1, Col: 9})})
	if err.Code != "uncalled_function" || !strings.Contains(err.Detail, "'g'") || err.Row != 1 || err.Col != 9 {
		t.Errorf("the raise is the interpreter's, at the candidate's position: %+v", err)
	}
}

func TestFnValueNoMatchVerdictMatchesStayOnTheIsland(t *testing.T) {
	r := parkReg(t)
	five, s := core.NewInteger(5), core.NewString("s")
	// The plan picks: a forward arg the param admits.
	fn, _ := parkFn("", true, parkSig(-1, intParam("a")))
	if v, _ := fnValueNoMatchVerdict(r, fn, nil, []core.Value{five}); v != noMatchUnproven {
		t.Errorf("a forward match is not a no-match: %v", v)
	}
	// The /s retry picks: the forward plan binds a='x' and fails b on the
	// stack top; the all-stack plan binds a='s' (top), b=3.
	two, _ := parkFn("", true, parkSig(-1, strParam("a"), intParam("b")))
	stk, fwd := []core.Value{core.NewInteger(3), s}, []core.Value{core.NewString("x")}
	if v, _ := fnValueNoMatchVerdict(r, two, stk, fwd); v != noMatchUnproven {
		t.Errorf("a /s-retry match is not a no-match: %v", v)
	}
	// …and it is the RETRY that matched, not the first plan.
	twoView := fnDispatchView(two.Data.(core.FnDefInfo))
	h := newRegionHostOver(r, append(append(append([]core.Value(nil), stk...), two), fwd...))
	w := core.WordInfo{ArgCount: -1}
	if planPicks(h, r, &twoView, w, stk) {
		t.Error("the forward-first plan must not match this window")
	}
	w.ForceStack = true
	if !planPicks(h, r, &twoView, w, stk) {
		t.Error("the /s retry must match this window")
	}
	// The legacy pure-stack path picks: an all-UNNAMED signature reads its
	// window bottom-up (param 0 = 5), where both plans read the top first.
	unnamed, _ := parkFn("", true, parkSig(-1, core.FnParam{Type: core.TInteger}, core.FnParam{Type: core.TString}))
	if v, _ := fnValueNoMatchVerdict(r, unnamed, []core.Value{five, s}, nil); v != noMatchUnproven {
		t.Errorf("a legacy stack match is not a no-match: %v", v)
	}
	if !core.FnValueStackMatches(unnamed.Data.(core.FnDefInfo), []core.Value{five, s}) {
		t.Error("the legacy pure-stack path must be what matched")
	}
	// An all-stack signature has no forward plan to retry (the /s arm is
	// skipped) and still parks when nothing admits the stack.
	stackOnly, _ := parkFn("", true, parkSig(0, intParam("a")))
	if v, _ := fnValueNoMatchVerdict(r, stackOnly, []core.Value{s}, nil); v != noMatchPark {
		t.Errorf("an all-stack value over a rejected stack arg parks: %v", v)
	}
}

func TestFnValueNoMatchVerdictGuards(t *testing.T) {
	r := parkReg(t)
	s := core.NewString("x")
	sig := parkSig(-1, intParam("a"))
	base, baseFd := parkFn("", true, sig)
	cases := []struct {
		label             string
		fn                core.Value
		resolved, forward []core.Value
	}{
		{"not a fn value", s, nil, []core.Value{s}},
		{"a quoted value", func() core.Value { v := base; v.Quoted = true; return v }(), nil, []core.Value{s}},
		{"a reach-grouped value", func() core.Value { v := base; v.ReachGroup = true; return v }(), nil, []core.Value{s}},
		{"a macro", core.NewFunction(core.FnDefInfo{Anonymous: true, Macro: true, Signatures: []core.Signature{sig}}), nil, []core.Value{s}},
		{"an apply-marked value", core.NewFunction(core.FnDefInfo{Anonymous: true, Applied: true, Signatures: []core.Signature{sig}}), nil, []core.Value{s}},
		{"no signatures", core.NewFunction(core.FnDefInfo{Anonymous: true}), nil, []core.Value{s}},
		{"a modifier wrapper", core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{sig}, Wrap: core.WrapReverse, Wraps: &base}), nil, []core.Value{s}},
		{"a real 0-arg signature", core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{sig, parkSig(-1)}}), nil, []core.Value{s}},
		{"a forward list (stepped: not its own result)", base, nil, []core.Value{core.NewList([]core.Value{s})}},
		{"a word beneath (tape-coupled)", base, []core.Value{core.NewWord("w")}, nil},
		{"a pending list beneath (evaluates at the end)", base, []core.Value{func() core.Value { v := core.NewList([]core.Value{s}); v.Eval = true; return v }()}, nil},
	}
	for _, c := range cases {
		if v, _ := fnValueNoMatchVerdict(r, c.fn, c.resolved, c.forward); v != noMatchUnproven {
			t.Errorf("%s: must keep the island, got %v", c.label, v)
		}
	}
	// A NAMED value from another module is looked up there by the step; an
	// ANONYMOUS one from another module reads its own signatures and parks.
	other := parkReg(t)
	foreign := baseFd
	foreign.Registry = other
	if v, _ := fnValueNoMatchVerdict(r, core.NewFunction(foreign), nil, []core.Value{s}); v != noMatchPark {
		t.Errorf("a foreign anonymous value parks: %v", v)
	}
	foreign.Anonymous, foreign.Name = false, "g"
	if v, _ := fnValueNoMatchVerdict(r, core.NewFunction(foreign), nil, []core.Value{s}); v != noMatchUnproven {
		t.Errorf("a foreign named value must keep the island: %v", v)
	}
}

func TestResidualInert(t *testing.T) {
	s := core.NewString("x")
	pending := core.NewList([]core.Value{s})
	pending.Eval = true
	quotedPending := pending
	quotedPending.Quoted = true
	for _, c := range []struct {
		label string
		v     core.Value
		want  bool
	}{
		{"a scalar", s, true},
		{"a plain list", core.NewList([]core.Value{s}), true},
		{"a pending list", pending, false},
		{"a quoted pending list", quotedPending, true},
		{"a word", core.NewWord("w"), false},
		{"a typed list", core.NewCarrier(core.TList), false},
	} {
		if got := residualInert(c.v); got != c.want {
			t.Errorf("%s: residualInert = %v, want %v", c.label, got, c.want)
		}
	}
}
