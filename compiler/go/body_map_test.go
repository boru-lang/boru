package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// body_map_test.go covers body_map.go, the recorded run of a body-map word
// (Rand.map-from — NUR330, NUR353): which signatures take it, what a schema's
// facts are, what the run arms, and what its lowering refuses against the
// loops open around it.

func bodyMapSig() *core.Signature {
	return &core.Signature{
		Args:          []*core.Type{core.TMap},
		NoEvalMapArgs: map[int]bool{0: true},
		CompileEffect: core.CompileDynBody,
	}
}

func bodyMapSchema(vals map[string]core.Value) core.Value {
	m := core.NewOrderedMap()
	for _, k := range []string{"a", "b", "c"} {
		if v, ok := vals[k]; ok {
			m.Set(k, v)
		}
	}
	return core.NewMap(m)
}

func bodyMapReg(t *testing.T) (*core.Registry, *EmitState, func()) {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := r.Check.Begin()
	es := NewEmitState()
	r.Check.Emit = es
	return r, es, done
}

func TestSoleNoEvalMapSlot(t *testing.T) {
	if _, ok := soleNoEvalMapSlot(&core.Signature{}, 2); ok {
		t.Error("no NoEvalMapArgs slot: none")
	}
	if mp, ok := soleNoEvalMapSlot(&core.Signature{NoEvalMapArgs: map[int]bool{1: true}}, 2); !ok || mp != 1 {
		t.Errorf("the one slot: got %d %v", mp, ok)
	}
	if _, ok := soleNoEvalMapSlot(&core.Signature{NoEvalMapArgs: map[int]bool{0: true, 1: true}}, 2); ok {
		t.Error("two slots: no sole one")
	}
}

func TestBodyMapShape(t *testing.T) {
	r, _, done := bodyMapReg(t)
	defer done()
	if f := bodyMapShape(r, core.NewCarrier(core.TMap)); !f.computed {
		t.Error("a carrier schema is computed")
	}
	if f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewCarrier(core.TList)})); !f.computed {
		t.Error("a carrier value is computed")
	}
	reach := core.NewReach(core.ReachInfo{
		Receiver: []core.Value{core.NewWord("rv")},
		Segments: []core.ReachSeg{{Computed: true, KeyExpr: []core.Value{core.NewWord("kx")}}},
	})
	inner := core.NewOrderedMap()
	inner.Set("n", core.NewWord("mv"))
	f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{
		"a": core.NewList([]core.Value{core.NewWord("i"), core.NewParenExpr([]core.Value{core.NewWord("pw")})}),
		"b": core.NewList([]core.Value{reach, core.NewMap(inner), {Data: core.MapPayload{}}}),
		"c": core.NewList([]core.Value{core.NewInteger(1), core.NewWord("break")}),
	}))
	if f.computed || f.opaque {
		t.Errorf("literal bodies of plain words: computed=%v opaque=%v", f.computed, f.opaque)
	}
	for _, n := range []string{"i", "pw", "rv", "kx", "mv", "break"} {
		if !f.names[n] {
			t.Errorf("the walk misses %q: %v", n, f.names)
		}
	}
	if !f.flow {
		t.Error("a body with a break escapes")
	}
	if f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewSplice(core.NewInteger(1))})})); !f.opaque {
		t.Error("a splice reads names no walk lists")
	}
	if f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewInteger(1)})})); f.flow || len(f.names) != 0 {
		t.Errorf("a plain datum body names nothing and never escapes: %+v", f)
	}
}

// A carrier entry marks the schema computed but the scan reads the rest, so
// a later literal body's escape is seen whatever the key order (Codex P2);
// a value the handler refuses as a body runs nothing; a flex list is a body.
func TestBodyMapShapeEveryEntry(t *testing.T) {
	r, _, done := bodyMapReg(t)
	defer done()
	brk := core.NewList([]core.Value{core.NewInteger(1), core.NewWord("break")})
	for _, order := range [][2]string{{"a", "b"}, {"b", "a"}} {
		f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{order[0]: core.NewCarrier(core.TList), order[1]: brk}))
		if !f.computed || !f.flow {
			t.Errorf("carrier at %q: computed=%v flow=%v, want both", order[0], f.computed, f.flow)
		}
	}
	f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{
		"a": core.NewInteger(5),
		"b": core.NewFlexList([]core.Value{core.NewWord("i"), core.NewWord("continue")}),
	}))
	if !f.names["i"] || !f.flow || f.computed {
		t.Errorf("a flex body is read, a non-list value skipped: %+v", f)
	}
}

// A called boru fn's body is read — transitively, once per name, cycles
// included — so a loop index it reads is seen (Codex P1).
func TestBodyMapShapeReadsCallees(t *testing.T) {
	r, _, done := bodyMapReg(t)
	defer done()
	r.Register("zzbmleaf", core.Signature{Impl: core.Boru([]core.Value{core.NewWord("i")}), BarrierPos: -1})
	r.Register("zzbmmid", core.Signature{Impl: core.Boru([]core.Value{core.NewWord("zzbmleaf"), core.NewWord("zzbmmid")}), BarrierPos: -1})
	lambda := core.Value{Data: core.FnDefInfo{Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewWord("lv")})}}}}
	f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewWord("zzbmmid"), lambda})}))
	for _, n := range []string{"zzbmmid", "zzbmleaf", "i", "lv"} {
		if !f.names[n] {
			t.Errorf("the callee walk misses %q: %v", n, f.names)
		}
	}
}

// Quoted data — a quoted list, the operand of `quote` — reads nothing unless
// a word that runs a value takes it (Codex P2): then its words, and its
// opaque tokens, count.
func TestBodyMapShapeQuotedData(t *testing.T) {
	r, _, done := bodyMapReg(t)
	defer done()
	r.RegisterNativeFunc(core.NativeFunc{Name: "quote", CompileEffect: core.CompileQuoteInert, Signatures: []core.Signature{{
		Args: []*core.Type{core.TAny}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: -1,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return a, nil
		}),
	}}})
	r.RegisterNativeFunc(core.NativeFunc{Name: "zzbmrun", Signatures: []core.Signature{{
		Args: []*core.Type{core.TList}, NoEvalArgs: map[int]bool{0: true}, BarrierPos: -1,
		Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return a, nil
		}),
	}}})
	quotedList := core.NewList([]core.Value{core.NewWord("qi"), core.NewSplice(core.NewInteger(1))})
	quotedList.Quoted = true
	inner := core.NewList([]core.Value{core.NewWord("qj")})
	inner.Quoted = true
	body := []core.Value{
		core.NewWord("quote"), core.NewList([]core.Value{core.NewWord("qk"), inner}),
		quotedList,
		core.NewWord("quote"), core.NewParenExpr([]core.Value{core.NewWord("pw")}),
		core.NewWord("quote"),
	}
	f := bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewList(body)}))
	if f.names["qi"] || f.names["qj"] || f.names["qk"] || f.opaque {
		t.Errorf("quoted data reads nothing when nothing runs it: %+v", f)
	}
	if !f.names["pw"] {
		t.Error("a paren after quote evaluates first: its words are read")
	}
	f = bodyMapShape(r, bodyMapSchema(map[string]core.Value{"a": core.NewList(append([]core.Value{core.NewWord("zzbmrun")}, body...))}))
	if !f.names["qi"] || !f.names["qj"] || !f.names["qk"] || !f.opaque {
		t.Errorf("a word that runs a value reads the quoted data: %+v", f)
	}
}

func TestSigRunsValue(t *testing.T) {
	for _, c := range []struct {
		name string
		sig  core.Signature
		want bool
	}{
		{"a plain native", core.Signature{Args: []*core.Type{core.TAny, nil}}, false},
		{"a callable body", core.Signature{Callable: &core.CallableSpec{}}, true},
		{"a body-running effect", core.Signature{CompileEffect: core.CompileDynBody}, true},
		{"a Function operand", core.Signature{Args: []*core.Type{core.TFunction}}, true},
		{"a code-body slot", core.Signature{NoEvalArgs: map[int]bool{0: true}}, true},
		{"a body-map slot", core.Signature{NoEvalMapArgs: map[int]bool{0: true}}, true},
		{"an inert quoted slot", core.Signature{NoEvalArgs: map[int]bool{0: true}, CompileEffect: core.CompileQuoteInert}, false},
		{"a key slot", core.Signature{NoEvalArgs: map[int]bool{0: true}, CompileEffect: core.CompileQuoteKey}, false},
	} {
		if got := sigRunsValue(&c.sig); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNoteBodyMapRunGuards(t *testing.T) {
	r, es, done := bodyMapReg(t)
	defer done()
	schema := bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewWord("k")})})
	args := []core.Value{schema}
	for _, sig := range []*core.Signature{
		nil,
		{Callable: &core.CallableSpec{}, CompileEffect: core.CompileDynBody, NoEvalMapArgs: map[int]bool{0: true}},
		{NoEvalMapArgs: map[int]bool{0: true}},
		{CompileEffect: core.CompileDynBody},
	} {
		noteBodyMapRun(r, "rand-map-from", sig, args, core.SrcPos{})
		if es.dynEnv {
			t.Fatalf("sig %+v is no body-map word: nothing arms", sig)
		}
	}
	es.Compilable = false
	noteBodyMapRun(r, "rand-map-from", bodyMapSig(), args, core.SrcPos{})
	if es.dynEnv {
		t.Fatal("an inactive recorder arms nothing")
	}
	// A recorder that is no EmitState: nothing to arm.
	r.Check.Emit = nil
	noteBodyMapRun(r, "rand-map-from", bodyMapSig(), args, core.SrcPos{})
}

func TestNoteBodyMapRunArms(t *testing.T) {
	r, es, done := bodyMapReg(t)
	defer done()
	// A datum body: nothing to mirror, and no event to ride.
	noteBodyMapRun(r, "rand-map-from", bodyMapSig(), []core.Value{bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewInteger(1)})})}, core.SrcPos{})
	if es.dynEnv || es.keptDefsLevel != 0 {
		t.Fatal("a body naming nothing arms nothing")
	}
	// A literal body naming a word: the environment mirror, facts on the call.
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "rand-map-from"}})
	noteBodyMapRun(r, "rand-map-from", bodyMapSig(), []core.Value{bodyMapSchema(map[string]core.Value{"a": core.NewList([]core.Value{core.NewWord("k")})})}, core.SrcPos{})
	frame := es.frames[0]
	if !es.dynEnv || es.keptDefsLevel != 0 || frame[len(frame)-1].call.bodyMap == nil || !frame[len(frame)-1].call.bodyMap.names["k"] {
		t.Fatalf("a named literal body: dynEnv=%v latch=%d facts=%+v", es.dynEnv, es.keptDefsLevel, frame[len(frame)-1].call.bodyMap)
	}
	// Another word's event last: the facts ride nothing.
	es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "other"}})
	noteBodyMapRun(r, "rand-map-from", bodyMapSig(), []core.Value{core.NewCarrier(core.TMap)}, core.SrcPos{})
	frame = es.frames[0]
	if frame[len(frame)-1].call.bodyMap != nil {
		t.Error("the facts ride only the body-map call itself")
	}
	// A computed schema: the kept-defs latch arms and the root leaks.
	if es.keptDefsLevel == 0 || es.keptDefsWord != "rand-map-from" || !es.rootDynLeak {
		t.Errorf("a computed schema arms the latch: level=%d word=%q rootLeak=%v", es.keptDefsLevel, es.keptDefsWord, es.rootDynLeak)
	}
}

func TestBodyMapReason(t *testing.T) {
	named := &bodyMapFacts{names: map[string]bool{"i": true}}
	for _, c := range []struct {
		name  string
		facts *bodyMapFacts
		loops []loopCtx
		fn    bool
		want  string
	}{
		{"no facts", nil, []loopCtx{{iterName: "i"}}, false, ""},
		{"the loop index, named", named, []loopCtx{{iterName: "i"}}, false, "loop index `i`"},
		{"another loop's index", named, []loopCtx{{iterName: "j"}}, false, ""},
		{"a condition loop has no index", named, []loopCtx{{}}, false, ""},
		{"a computed body in a counted loop", &bodyMapFacts{computed: true}, []loopCtx{{iterName: "j"}}, false, "loop index `j`"},
		{"an opaque body in a counted loop", &bodyMapFacts{opaque: true}, []loopCtx{{iterName: "j"}}, false, "loop index `j`"},
		{"an escape at the root", &bodyMapFacts{flow: true}, nil, false, "break/continue outside a compiled loop"},
		{"an escape in a loop", &bodyMapFacts{flow: true}, []loopCtx{{iterName: "j"}}, false, ""},
		{"an escape in a fn unit", &bodyMapFacts{flow: true}, nil, true, ""},
	} {
		lw := &lowerer{loops: c.loops, isFnUnit: c.fn}
		got := lw.bodyMapReason(&emitCall{word: "rand-map-from", bodyMap: c.facts})
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
