package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// vm_dyn_words_test.go pins the whole-frame replay's WORD arms (NUR123): the
// per-unit word table lookup, the region liveness test, the fast path for a
// region of plain data, the no-conversion decline, the frame-binding install
// and teardown around the island, and the closure-as-word bridge's declines.

func TestDynFrameWordsAt(t *testing.T) {
	if dynFrameWordsAt(nil, 0, 0) != nil || dynFrameWordsAt(&compiler.Program{}, -1, 0) != nil ||
		dynFrameWordsAt(&compiler.Program{}, 0, 0) != nil {
		t.Error("a nil program, the main code and an out-of-range unit carry no table")
	}
	p := &compiler.Program{Fns: []compiler.CompiledFn{{DynFrameWords: map[int][]compiler.DynFrameWord{3: {{Name: "g"}}}}}}
	if w := dynFrameWordsAt(p, 0, 3); len(w) != 1 || w[0].Name != "g" {
		t.Errorf("the unit's table reads back by pc: %v", w)
	}
	if dynFrameWordsAt(p, 0, 4) != nil {
		t.Error("an unkeyed pc has no table")
	}
}

// TestDynApplyNameAt pins the trailing apply's head-name lookup
// (CompiledFn.DynApplyName, the nineteenth increment) — same guards as the
// replay's table, one entry rather than a slice.
func TestDynApplyNameAt(t *testing.T) {
	if dynApplyNameAt(nil, 0, 0).Name != "" || dynApplyNameAt(&compiler.Program{}, -1, 0).Name != "" ||
		dynApplyNameAt(&compiler.Program{}, 0, 0).Name != "" {
		t.Error("a nil program, the main code and an out-of-range unit name nothing")
	}
	p := &compiler.Program{Fns: []compiler.CompiledFn{{DynApplyName: map[int]compiler.DynApplyHead{
		3: {Name: "g", Pos: core.SrcPos{Row: 1, Col: 37}, NWritten: 1},
	}}}}
	if w := dynApplyNameAt(p, 0, 3); w.Name != "g" || w.Pos.Col != 37 {
		t.Errorf("the unit's table reads back by pc: %+v", w)
	}
	if dynApplyNameAt(p, 0, 4).Name != "" {
		t.Error("an unkeyed pc names nothing")
	}
}

// TestInstalledSigView pins the barrier reconciliation the head name needed:
// registration resolves BarrierAllForward (-1) to the sig's arg count, and
// the no-match diagnostic reads the RESOLVED value to decide the forward-args
// help. An already-resolved barrier is left alone, and the source fn is never
// mutated — a compiled const is shared program state.
func TestInstalledSigView(t *testing.T) {
	src := core.FnDefInfo{Signatures: []core.Signature{
		{Params: []core.FnParam{{Type: core.TString}}, BarrierPos: core.BarrierAllForward},
		{Params: []core.FnParam{{Type: core.TString}, {Type: core.TString}}, BarrierPos: 0},
	}}
	view := installedSigView(src)
	if view.Signatures[0].BarrierPos != 1 {
		t.Errorf("the sentinel resolves to the arg count: %d", view.Signatures[0].BarrierPos)
	}
	if view.Signatures[1].BarrierPos != 0 {
		t.Errorf("an explicit stack-only barrier is left alone: %d", view.Signatures[1].BarrierPos)
	}
	if !view.HasForwardSigs() {
		t.Error("the resolved view is what gates the forward-args help")
	}
	if src.Signatures[0].BarrierPos != core.BarrierAllForward {
		t.Error("the source fn must not be mutated")
	}
}

func TestReplayRegionLive(t *testing.T) {
	fn := core.NewFunction(core.FnDefInfo{})
	quoted := fn
	quoted.Quoted = true
	five := core.NewInteger(5)
	word := []compiler.DynFrameWord{{Name: "g"}}
	if replayRegionLive([]core.Value{five, five}, nil) {
		t.Error("plain data is not live")
	}
	if !replayRegionLive([]core.Value{five, fn}, nil) {
		t.Error("an unquoted fn is live")
	}
	if replayRegionLive([]core.Value{quoted}, nil) {
		t.Error("a quoted fn at a value entry is data")
	}
	if !replayRegionLive([]core.Value{quoted}, word) {
		t.Error("a quoted fn at a word-read entry dispatches the binding")
	}
	if !replayRegionLive([]core.Value{core.NewWord("add")}, nil) {
		t.Error("a tape-coupled token is live")
	}
}

// TestCallDynFrameWordsArms drives the replay: plain data at the word entry
// returns the stack untouched without an island; a live region whose word
// entry holds no fn declines (handled=false) so the value path decides; a
// fn at the word entry is installed as the frame binding for the island run
// (`5 g` over a 1-arg native collects 5 → 6) and popped afterwards; a
// no-match raises the interpreter's named error.
func TestCallDynFrameWordsArms(t *testing.T) {
	r, _, _ := seam7DelegReg(t)
	vc := seam7VC(r)
	// The native's own FnDefInfo (what `cinc/v` reads), not the delegation
	// wrapper: a frame binding installs the value's OWN overloads.
	inc := core.NewFunction(*r.Lookup("cinc"))
	five := core.NewInteger(5)
	words := []compiler.DynFrameWord{{}, {Name: "g", Pos: core.SrcPos{Row: 1, Col: 4}}}

	st, handled, err := vc.callDynFrameWords(r, words, 0, 0, []core.Value{five, five}, seam7Dbg, 0)
	if err != nil || !handled || len(st) != 2 {
		t.Errorf("plain data: handled with the stack untouched, got %v %v %v", st, handled, err)
	}
	_, handled, err = vc.callDynFrameWords(r, []compiler.DynFrameWord{{Name: "g"}, {}}, 0, 0, []core.Value{five, inc}, seam7Dbg, 0)
	if err != nil || handled {
		t.Errorf("a live region with no fn at its word entry declines: %v %v", handled, err)
	}
	st, handled, err = vc.callDynFrameWords(r, words, 0, 0, []core.Value{five, inc}, seam7Dbg, 0)
	if err != nil || !handled || len(st) != 1 {
		t.Fatalf("`5 g` over a 1-arg fn: got %v %v %v", st, handled, err)
	}
	if n, _ := core.AsInteger(st[0]); n != 6 {
		t.Errorf("the word collects its argument from the region: %v", st[0])
	}
	if r.Defs.Depth("g") != 0 {
		t.Errorf("the frame binding is popped after the island: depth %d", r.Defs.Depth("g"))
	}
	_, handled, err = vc.callDynFrameWords(r, []compiler.DynFrameWord{{Name: "g", Pos: core.SrcPos{Row: 1, Col: 4}}}, 0, 0, []core.Value{inc}, seam7Dbg, 0)
	if !handled || err == nil || !strings.Contains(err.Error(), "cannot call `g`") {
		t.Errorf("a no-match raises the named dispatch error: %v %v", handled, err)
	}
	if r.Defs.Depth("g") != 0 {
		t.Error("the frame binding is popped on the error path too")
	}
}

// TestClosureAsWordDeclines pins the bridge's arms: a FnDefInfo passes
// through unchanged; a closure whose unit this context cannot resolve
// declines (and a word entry holding one is left as it is by the replay);
// a closure minted by a FOREIGN program resolves through that program, and
// an undeclared param type bridges as Any.
func TestClosureAsWordDeclines(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	vc.p = &compiler.Program{}
	fn := core.NewFunction(core.FnDefInfo{})
	if v, ok := vc.closureAsWord(r, fn); !ok || v.ID != fn.ID {
		t.Error("a FnDefInfo is already dispatchable by name")
	}
	cl := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{Unit: 0}}
	if _, ok := vc.closureAsWord(r, cl); ok {
		t.Error("a closure with no unit in this program declines")
	}
	// The replay leaves such an entry alone: nothing converted, not handled.
	words := []compiler.DynFrameWord{{Name: "g"}}
	if _, handled, err := vc.callDynFrameWords(r, words, 0, 0, []core.Value{cl}, seam7Dbg, 0); handled || err != nil {
		t.Errorf("an unbridgeable closure at a word entry declines the words path: %v %v", handled, err)
	}
	// A unit that recorded no declared param contract (a token body) declines
	// too: guessing Any would apply where the interpreter refuses.
	vc.p = &compiler.Program{Fns: []compiler.CompiledFn{{NArgs: 1}}}
	if _, ok := vc.closureAsWord(r, cl); ok {
		t.Error("a unit without its param contract declines")
	}
	// A closure minted by a FOREIGN program resolves through that program,
	// and the bridge declares the unit's contract: a nil type is Any, a
	// pattern-only param carries its pattern, and MatchFnSig over the bridged
	// value rejects a String for the Integer param and admits an Integer.
	pat := core.NewInteger(7)
	foreign := &compiler.Program{Fns: []compiler.CompiledFn{{NArgs: 2, Params: []*core.Type{core.TInteger, nil}, ParamPatterns: []*core.Value{nil, &pat}}}}
	fcl := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{Prog: foreign, Unit: 0}}
	v, ok := vc.closureAsWord(r, fcl)
	if !ok {
		t.Fatal("a foreign program's closure resolves through that program")
	}
	fd, isFn := v.Data.(core.FnDefInfo)
	if !isFn || len(fd.Signatures) != 1 || len(fd.Signatures[0].Args) != 2 ||
		!fd.Signatures[0].Args[0].Equal(core.TInteger) || !fd.Signatures[0].Args[1].Equal(core.TAny) ||
		fd.Signatures[0].Params[1].Pattern == nil {
		t.Errorf("the bridge carries one signature over the unit's declared params: %v", v)
	}
	if core.MatchFnSig(v, []core.Value{core.NewString("s"), core.NewInteger(7)}) != nil {
		t.Error("the declared Integer param rejects a String")
	}
	if core.MatchFnSig(v, []core.Value{core.NewInteger(1), core.NewInteger(7)}) == nil {
		t.Error("the declared params admit their inhabitants")
	}
}
