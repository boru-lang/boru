package core

import (
	"strings"
	"testing"
)

// Block scopes (block.go, design/IMMUTABLE-DEF.1.md §2.1, phase 2): the
// unit battery for the kernel's half — the scope handles, the seams, the
// engine's arm, condition and loop-region blocks, the tail-call probe over
// a BlockEnd marker, the type retirement a closing scope performs, and the
// def census's block findings. Each positive case is paired with the
// negative that proves the gate: a body that binds nothing opens no block,
// a binding the block did not make stands.

// blockProbe is the per-registry state the probe vocabulary writes.
type blockProbe struct {
	ticks  int
	minted *Type
}

// blockReg builds a cov registry with the block tests' vocabulary. `var`
// stands in for the language's binder — a native of that NAME, so
// BodyBindsLocals sees it: `var 5` binds `blk` to 5 in the innermost
// scope. `btick` answers true twice, then false (a while condition);
// `bfail` raises; `bbrk` signals a break; `btype` mints a type `Zt` and
// binds it.
func blockReg(t *testing.T) (*Registry, *blockProbe) {
	t.Helper()
	p := &blockProbe{}
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "var",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.Defs.Push("blk", args[0])
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "btick",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					p.ticks++
					return []Value{NewBoolean(p.ticks <= 2)}, nil
				}),
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "bfail",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, &BoruError{Code: "runtime_error", Detail: "boom"}
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "bbrk",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.FlowCtrl = FlowBreak
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "btype",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					p.minted = reg.Types.MintType("Zt", TInteger)
					reg.Defs.PushType("Zt", p.minted, NewTypeLiteral(p.minted))
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
	})
	return r, p
}

func blockBody(toks ...Value) []Value { return toks }

// bind5 is the body `var 5 blk`: binds blk and reads it back.
func bind5() []Value { return blockBody(NewWord("var"), NewInteger(5), NewWord("blk")) }

func assertClosed(t *testing.T, r *Registry, what string) {
	t.Helper()
	if r.Defs.Has("blk") {
		t.Errorf("%s: the block's binding must end with the block", what)
	}
	if d := r.Defs.ScopeDepth(); d != 0 {
		t.Errorf("%s: every scope must be closed, depth %d", what, d)
	}
}

func TestBodyBindsLocals(t *testing.T) {
	for _, c := range []struct {
		name string
		toks []Value
		want bool
	}{
		{"empty", nil, false},
		{"no binder", blockBody(NewWord("x"), NewInteger(1)), false},
		{"def", blockBody(NewWord("def"), NewWord("x"), NewInteger(1)), true},
		{"var", blockBody(NewWord("var"), NewInteger(1)), true},
		{"nested list", blockBody(NewList(blockBody(NewWord("var"), NewInteger(1)))), true},
	} {
		if got := BodyBindsLocals(c.toks); got != c.want {
			t.Errorf("%s: BodyBindsLocals = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBodyBindsLocalsMemo(t *testing.T) {
	r, _ := blockReg(t)
	body := NewList(bind5())
	body.ID = "B1"
	if !r.bodyBindsLocals(body) {
		t.Fatal("a binding body scans true")
	}
	// The memo answers the second time: flipping the stored verdict shows
	// the hit.
	r.blockBinds["B1"] = false
	if r.bodyBindsLocals(body) {
		t.Error("the second call must read the memo")
	}
	// A body with no identity is scanned every time and memoised nowhere.
	anon := NewList(bind5())
	anon.ID = ""
	if !r.bodyBindsLocals(anon) || len(r.blockBinds) != 1 {
		t.Errorf("an identity-less body scans each time: %v, memo %d", r.bodyBindsLocals(anon), len(r.blockBinds))
	}
	// A full memo is dropped before the next entry.
	for i := 0; i < blockBindsMemoCap; i++ {
		r.blockBinds["fill"+string(rune('a'+i%26))+string(rune('a'+(i/26)%26))+string(rune('a'+(i/676)%26))] = true
	}
	other := NewList(blockBody(NewInteger(1)))
	other.ID = "B2"
	if r.bodyBindsLocals(other) || len(r.blockBinds) != 1 {
		t.Errorf("a memo past its cap restarts: %d entries", len(r.blockBinds))
	}
	// A nil memo is created on the first miss.
	r.blockBinds = nil
	if !r.bodyBindsLocals(body) || !r.blockBinds["B1"] {
		t.Error("the first miss creates the memo")
	}
}

func TestEnterLeaveBlock(t *testing.T) {
	r, _ := blockReg(t)
	InstallDef(r, "n", NewInteger(1))
	id := EnterBlock(r)
	r.Defs.Push("n", NewInteger(2))
	r.Defs.Push("m", NewInteger(3))
	LeaveBlock(r, id)
	if v, _ := r.Defs.Top("n"); Canon([]Value{v}) != "1" {
		t.Errorf("the block's shadow pops, the module binding stands: n = %s", Canon([]Value{v}))
	}
	if r.Defs.Has("m") || r.Defs.ScopeDepth() != 0 {
		t.Error("the block's own binding ends with it")
	}
	// A closed id is a no-op (a BlockEnd marker left on the tape by an
	// eager teardown).
	r.Defs.Push("m", NewInteger(4))
	LeaveBlock(r, id)
	if !r.Defs.Has("m") {
		t.Error("leaving a closed block pops nothing")
	}
	r.Defs.Pop("m")
	// Leaving an OUTER block closes the inner one an unwinding left open,
	// and pops both blocks' bindings.
	outer := EnterBlock(r)
	r.Defs.Push("a", NewInteger(1))
	EnterBlock(r)
	r.Defs.Push("b", NewInteger(2))
	LeaveBlock(r, outer)
	if r.Defs.Has("a") || r.Defs.Has("b") || r.Defs.ScopeDepth() != 0 {
		t.Error("an outer block's close takes the inner block's bindings with it")
	}
}

func TestLeaveBlockRetiresMintedTypeAndFreesName(t *testing.T) {
	r, _ := blockReg(t)
	id := EnterBlock(r)
	minted := r.Types.MintType("Zq", TInteger)
	r.Defs.PushType("Zq", minted, NewTypeLiteral(minted))
	ReserveTypeParts(r, "Zq") // as InstallTypeBody does after its install
	if !r.IsKnownPart("Zq") {
		t.Fatal("the install reserves the name part")
	}
	LeaveBlock(r, id)
	if r.Defs.IsType("Zq") || r.Types.LookupByID(minted.ID) != nil || r.IsKnownPart("Zq") {
		t.Error("a type the block minted retires with it, and its name comes free")
	}
	// The next run's mint of the same name is free to proceed.
	again := r.Types.MintType("Zq", TInteger)
	if again == nil || r.Types.LookupByID(again.ID) == nil {
		t.Error("the name is free for the next run")
	}
	r.Types.Retire(again)
}

func TestUninstallTypeFreesNamePartsOnlyWhenUnbound(t *testing.T) {
	r, _ := blockReg(t)
	minted := r.Types.MintType("Zs", TInteger)
	r.Defs.PushType("Zs", minted, NewTypeLiteral(minted))
	ReserveTypeParts(r, "Zs")
	r.Defs.PushTypeAdopted("Zs", minted, NewTypeLiteral(minted)) // a shadow of the same node
	UninstallType(r, "Zs")
	if !r.Defs.IsType("Zs") || !r.IsKnownPart("Zs") || r.Types.LookupByID(minted.ID) == nil {
		t.Error("a binding still live keeps the node and its name part")
	}
	UninstallType(r, "Zs")
	if r.Defs.IsType("Zs") || r.IsKnownPart("Zs") || r.Types.LookupByID(minted.ID) != nil {
		t.Error("the last binding's pop retires the node and frees the name")
	}
	// A builtin part is never a dynamic reservation.
	r.Defs.PushTypeAdopted("Integer", TInteger, NewTypeLiteral(TInteger))
	UninstallType(r, "Integer")
	if !r.IsKnownPart("Integer") {
		t.Error("a builtin name part stays known")
	}
	// The nil guards.
	var nr *Registry
	nr.forgetTypeParts("x")
	(&Registry{}).forgetTypeParts("x")
}

func TestRunBlockSeams(t *testing.T) {
	r, _ := blockReg(t)
	res, err := RunBlockResolved(r, []Value{NewInteger(9)}, bind5())
	if err != nil || Canon(res) != "9 5" {
		t.Fatalf("RunBlockResolved = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "RunBlockResolved")

	// RunBodyResolved: gated on the body binding.
	res, err = RunBodyResolved(r, NewList(bind5()), nil)
	if err != nil || Canon(res) != "5" {
		t.Fatalf("RunBodyResolved (binding) = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "RunBodyResolved")
	res, err = RunBodyResolved(r, NewList(blockBody(NewInteger(7))), []Value{NewInteger(1)})
	if err != nil || Canon(res) != "1 7" {
		t.Fatalf("RunBodyResolved (plain) = %s, %v", Canon(res), err)
	}

	// RunBlockTokens: the same gate on a fresh engine.
	res, err = RunBlockTokens(r, bind5())
	if err != nil || Canon(res) != "5" {
		t.Fatalf("RunBlockTokens (binding) = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "RunBlockTokens")
	res, err = RunBlockTokens(r, blockBody(NewInteger(7)))
	if err != nil || Canon(res) != "7" {
		t.Fatalf("RunBlockTokens (plain) = %s, %v", Canon(res), err)
	}

	// InvokeBody is a block; InvokeBodyKeepDefs (`do`) is not.
	res, err = InvokeBody(r, NewList(bind5()), nil)
	if err != nil || Canon(res) != "5" {
		t.Fatalf("InvokeBody = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "InvokeBody")
	res, err = InvokeBodyKeepDefs(r, NewList(bind5()), nil)
	if err != nil || Canon(res) != "5" || !r.Defs.Has("blk") {
		t.Fatalf("InvokeBodyKeepDefs = %s, %v, kept %v", Canon(res), err, r.Defs.Has("blk"))
	}
	r.Defs.Pop("blk")

	// With an Invoker installed, each seam tells it which it is, and a
	// nested call restores the enclosing seam's answer.
	var saw []bool
	nested := false
	r.Invoker = func(reg *Registry, body Value, inputs []Value) ([]Value, error) {
		saw = append(saw, reg.InvokeKeepsDefs())
		if reg.InvokeKeepsDefs() && !nested {
			nested = true
			_, _ = InvokeBody(reg, body, nil)
			saw = append(saw, reg.InvokeKeepsDefs())
		}
		return nil, nil
	}
	_, _ = InvokeBody(r, NewList(bind5()), nil)
	_, _ = InvokeBodyKeepDefs(r, NewList(bind5()), nil)
	if len(saw) != 4 || saw[0] || !saw[1] || saw[2] || !saw[3] || r.InvokeKeepsDefs() {
		t.Errorf("the seams' answers to the Invoker: %v (flag after %v), want [false true false true] and false", saw, r.InvokeKeepsDefs())
	}
	r.Invoker = nil
	var nr *Registry
	if nr.InvokeKeepsDefs() {
		t.Error("a nil registry keeps nothing")
	}
}

func ifTokens(id string, cond Value, cont *IfCont) []Value {
	return blockBody(NewMark(id), cond, NewMoveIf(id, "if", cont))
}

func TestIfArmBlock(t *testing.T) {
	r, _ := blockReg(t)
	// The taken arm binds: its binding ends with the arm.
	res, err := NewTop(r).Run(ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: bind5(), ThenBlock: true}))
	if err != nil || Canon(res) != "5" {
		t.Fatalf("then arm = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "then arm")
	res, err = NewTop(r).Run(ifTokens(NextMarkID(), NewBoolean(false), &IfCont{Then: blockBody(NewInteger(1)), Else: blockBody(NewWord("var"), NewInteger(6), NewWord("blk")), ElseBlock: true}))
	if err != nil || Canon(res) != "6" {
		t.Fatalf("else arm = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "else arm")
	// An arm flagged but empty splices nothing and opens nothing.
	res, err = NewTop(r).Run(ifTokens(NextMarkID(), NewBoolean(true), &IfCont{ThenBlock: true}))
	if err != nil || len(res) != 0 {
		t.Fatalf("empty arm = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "empty arm")
	// A read after the arm of the name the arm bound is an undefined word.
	toks := append(ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5)), ThenBlock: true}), NewWord("blk"))
	if _, err := NewTop(r).Run(toks); err == nil || !strings.Contains(err.Error(), "blk") {
		t.Errorf("a read after the arm must find the name unbound, got %v", err)
	}
	assertClosed(t, r, "read after the arm")
	// An arm that raises before its BlockEnd marker ran: the fault return
	// closes the block.
	_, err = NewTop(r).Run(ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), NewWord("bfail")), ThenBlock: true}))
	if err == nil {
		t.Fatal("the arm's raise must surface")
	}
	assertClosed(t, r, "raising arm")
}

func TestIfCondBlock(t *testing.T) {
	r, _ := blockReg(t)
	// The condition binds blk and reads it as its decision value.
	cont := &IfCont{Then: blockBody(NewInteger(1)), Else: blockBody(NewInteger(2)), CondBlockID: EnterBlock(r)}
	id := NextMarkID()
	toks := blockBody(NewMark(id), NewWord("var"), NewInteger(7), NewWord("blk"), NewMoveIf(id, "if", cont))
	res, err := NewTop(r).Run(toks)
	if err != nil || Canon(res) != "1" {
		t.Fatalf("binding condition = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "binding condition")
	// A binding condition that nets no value: the error path closes it too.
	cont = &IfCont{Then: blockBody(NewInteger(1)), CondBlockID: EnterBlock(r)}
	id = NextMarkID()
	_, err = NewTop(r).Run(blockBody(NewMark(id), NewWord("var"), NewInteger(7), NewMoveIf(id, "if", cont)))
	if err == nil || !strings.Contains(err.Error(), "condition produced no value") {
		t.Fatalf("want the no-value error, got %v", err)
	}
	assertClosed(t, r, "valueless condition")
}

// forTokens lays out a counted loop as basic's RunForLoop does: the index
// installed, the first iteration's block open when the body binds.
func forTokens(r *Registry, body []Value, end int) ([]Value, *ForCont) {
	InstallDef(r, "i", NewInteger(0))
	cont := &ForCont{Registry: r, IterName: "i", Current: 0, End: int64(end), Step: 1, Body: body, IterDepth: r.Defs.Depth("i"), Block: BodyBindsLocals(body)}
	if cont.Block {
		cont.BlockID = EnterBlock(r)
	}
	id := NextMarkID()
	toks := blockBody(NewMark(id, body...))
	toks = append(toks, body...)
	return append(toks, NewMoveCont(id, "for loop", cont)), cont
}

func TestForBodyBlock(t *testing.T) {
	r, _ := blockReg(t)
	toks, _ := forTokens(r, bind5(), 2)
	res, err := NewTop(r).Run(toks)
	if err != nil || Canon(res) != "5 5" {
		t.Fatalf("for body = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "for body")
	if r.Defs.Has("i") {
		t.Error("the index ends with the loop")
	}
	// A break abandons the iteration: its block ends with it.
	toks, _ = forTokens(r, blockBody(NewWord("var"), NewInteger(5), NewWord("bbrk")), 3)
	res, err = NewTop(r).Run(toks)
	if err != nil || len(res) != 0 {
		t.Fatalf("broken for = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "broken for")
	// An error inside the body: the unwinding closes the open iteration.
	toks, _ = forTokens(r, blockBody(NewWord("var"), NewInteger(5), NewWord("bfail")), 3)
	if _, err = NewTop(r).Run(toks); err == nil {
		t.Fatal("the body's raise must surface")
	}
	assertClosed(t, r, "raising for")
	// A body that binds nothing opens no block.
	toks, cont := forTokens(r, blockBody(NewInteger(1)), 2)
	if cont.Block || cont.BlockID != 0 {
		t.Error("a body that binds nothing is no block")
	}
	if res, err = NewTop(r).Run(toks); err != nil || Canon(res) != "1 1" {
		t.Fatalf("plain for = %s, %v", Canon(res), err)
	}
}

// whileTokens lays out a while loop as basic's RunWhileLoop does: the first
// condition region spliced, its block open when the condition binds.
func whileTokens(r *Registry, cond, body []Value) []Value {
	cont := &ForCont{Registry: r, Body: body, WhileCond: cond, Block: BodyBindsLocals(body), CondBlock: BodyBindsLocals(cond)}
	if cont.CondBlock {
		cont.BlockID = EnterBlock(r)
	}
	id := NextMarkID()
	toks := blockBody(NewMark(id, cond...))
	toks = append(toks, cond...)
	return append(toks, NewMoveCont(id, "while loop", cont))
}

func TestWhileBodyAndCondBlock(t *testing.T) {
	r, p := blockReg(t)
	// The condition binds (and is a block of its own), the body binds.
	res, err := NewTop(r).Run(whileTokens(r, blockBody(NewWord("var"), NewInteger(1), NewWord("btick")), bind5()))
	if err != nil || Canon(res) != "5 5" {
		t.Fatalf("while = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "while")
	// An error inside the body: the unwinding closes the open round.
	p.ticks = 0
	if _, err = NewTop(r).Run(whileTokens(r, blockBody(NewWord("btick")), blockBody(NewWord("var"), NewInteger(5), NewWord("bfail")))); err == nil {
		t.Fatal("the body's raise must surface")
	}
	assertClosed(t, r, "raising while")
	// Neither region binds: no block at all, the plain loop.
	p.ticks = 0
	if res, err = NewTop(r).Run(whileTokens(r, blockBody(NewWord("btick")), blockBody(NewInteger(1)))); err != nil || Canon(res) != "1 1" {
		t.Fatalf("plain while = %s, %v", Canon(res), err)
	}
}

func TestParenGroupBlockEnd(t *testing.T) {
	r, _ := blockReg(t)
	id := NextMarkID()
	toks := blockBody(NewOpenParen(), NewMark(id), NewBoolean(true), NewMoveIf(id, "if", &IfCont{Then: bind5(), ThenBlock: true}), NewCloseParen())
	res, err := NewTop(r).Run(toks)
	if err != nil || Canon(res) != "5" {
		t.Fatalf("arm inside a group = %s, %v", Canon(res), err)
	}
	assertClosed(t, r, "arm inside a group")
}

func TestProbeTailSkipsBlockEnd(t *testing.T) {
	f := newProbeFixture(t)
	be := NewDefCleanup(DefCleanupInfo{Registry: f.r, BlockID: 7})
	// (ₘ 1 f __BE __DC __pa undef n __RC ): the arm's end marker sits among
	// the closers and is stepped over.
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(1), NewWord("f"), be,
		f.dc, NewWord("__pa"), f.und, NewWord("n"), f.rc, NewCloseParen(),
	}
	scan, ok := f.probe(t, tokens, 2, []int{1}, 1)
	if !ok {
		t.Fatal("a tail with a BlockEnd marker among its closers must be detected")
	}
	if scan.TailStart != 4 || scan.RCIdx != 8 || scan.CloseIdx != 9 {
		t.Errorf("scan extent wrong: %+v", scan)
	}
	// The marker alone is no tail: what follows it still has to be the
	// frame's cleanup.
	tokens = []Value{NewFrameOpen(f.meta), NewInteger(1), NewWord("f"), be, NewInteger(7), NewCloseParen()}
	if _, ok := f.probe(t, tokens, 2, []int{1}, 1); ok {
		t.Error("a BlockEnd marker followed by a value is not a tail")
	}
}

func TestIsBlockEnd(t *testing.T) {
	if IsBlockEnd(NewInteger(1)) || IsBlockEnd(NewDefCleanup(DefCleanupInfo{})) {
		t.Error("only a DefCleanup marker with a BlockID is a block end")
	}
	if !IsBlockEnd(NewDefCleanup(DefCleanupInfo{BlockID: 3})) {
		t.Error("a marker with a BlockID is a block end")
	}
}

func TestFrameTeardownRetiresMintedType(t *testing.T) {
	r, p := blockReg(t)
	toks := blockBody(NewWord("btype"), NewDefCleanup(DefCleanupInfo{Snapshot: map[string]int{}, Registry: r}))
	if _, err := NewTop(r).Run(toks); err != nil {
		t.Fatal(err)
	}
	if r.Defs.IsType("Zt") || r.Types.LookupByID(p.minted.ID) != nil || r.IsKnownPart("Zt") {
		t.Error("a type the frame minted retires with the frame, and its name comes free")
	}
}

func TestDefTableScopeHandles(t *testing.T) {
	var nd *DefTable
	if _, ok := nd.LeaveScopeTo(1); ok || nd.ScopeDepth() != 0 || nd.ScopeOpen(5) || !nd.ScopeOpen(0) {
		t.Error("nil table: nothing open but the module scope")
	}
	if _, ok := nd.OutermostBlockAbove(0); ok {
		t.Error("nil table: no block")
	}
	nd.NoteBlockBound("x", BlockBoundNote{})
	if _, ok := nd.BlockBoundSite("x"); ok {
		t.Error("nil table: no record")
	}

	dt := NewDefTable()
	b1 := dt.EnterScope(ScopeBlock)
	dt.Push("a", NewInteger(1))
	fr := dt.EnterScope(ScopeFrame)
	b2 := dt.EnterScope(ScopeBlock)
	dt.Push("b", NewInteger(2))
	if !dt.ScopeOpen(b1) || !dt.ScopeOpen(b2) || dt.ScopeOpen(99) || dt.ScopeDepth() != 3 {
		t.Error("three scopes open")
	}
	if id, ok := dt.OutermostBlockAbove(0); !ok || id != b1 {
		t.Errorf("outermost block from the floor: %d %v, want %d", id, ok, b1)
	}
	if id, ok := dt.OutermostBlockAbove(1); !ok || id != b2 {
		t.Errorf("outermost block above the frame: %d %v, want %d", id, ok, b2)
	}
	if _, ok := dt.OutermostBlockAbove(-1); ok {
		t.Error("a negative floor finds nothing")
	}
	if _, ok := dt.OutermostBlockAbove(3); ok {
		t.Error("a floor past the top finds nothing")
	}
	bound, ok := dt.LeaveScopeTo(fr)
	if !ok || len(bound) != 1 || bound[0] != "b" || dt.ScopeDepth() != 1 || dt.ScopeOpen(b2) {
		t.Errorf("leaving the frame closes the block inside it: %v %v depth %d", bound, ok, dt.ScopeDepth())
	}
	if _, ok := dt.LeaveScopeTo(999); ok {
		t.Error("an unknown id changes nothing")
	}
	// The block-bound record: on the innermost scope, then the root.
	dt.NoteBlockBound("z", BlockBoundNote{Site: SrcPos{Row: 1, Col: 2}, Note: "value"})
	if n, ok := dt.BlockBoundSite("z"); !ok || n.Site.Col != 2 {
		t.Error("the record is read from the innermost scope")
	}
	dt.EnterScope(ScopeFrame)
	if _, ok := dt.BlockBoundSite("z"); ok {
		t.Error("a frame bounds the lexical region")
	}
	dt.LeaveScope()
	dt.LeaveScope()
	if _, ok := dt.BlockBoundSite("z"); ok {
		t.Error("the closed block's record went with its scope")
	}
	dt.NoteBlockBound("y", BlockBoundNote{Site: SrcPos{Row: 3}})
	if n, ok := dt.BlockBoundSite("y"); !ok || n.Site.Row != 3 {
		t.Error("at the module scope the record is the root's")
	}
}

func TestPushFnBaselineTransparentAndFallback(t *testing.T) {
	r, _ := blockReg(t)
	r.Check.NextBaselineTransparent = true
	r.Check.NextBaselineIsBlock = true
	r.PushFnBaseline(nil)
	if r.Defs.ScopeDepth() != 0 || len(r.frameScopes) != 1 || r.frameScopes[0] != transparentScope || r.Check.NextBaselineTransparent || r.Check.NextBaselineIsBlock {
		t.Fatalf("a transparent baseline opens no scope: depth %d, frames %v", r.Defs.ScopeDepth(), r.frameScopes)
	}
	InstallDef(r, "k", NewInteger(1))
	if e, _ := r.Defs.TopEntry("k"); e.Scope != 0 {
		t.Error("an install under a transparent baseline is the enclosing scope's")
	}
	r.PopFnBaseline()
	if len(r.frameScopes) != 0 || r.Defs.ScopeDepth() != 0 {
		t.Error("the transparent baseline pops cleanly")
	}
	UninstallDef(r, "k")
	// A baseline pushed without PushFnBaseline (a test's direct append)
	// falls back to the plain LeaveScope.
	r.FnBaselines = append(r.FnBaselines, map[string]int{})
	r.Defs.EnterScope(ScopeFrame)
	r.PopFnBaseline()
	if r.Defs.ScopeDepth() != 0 || len(r.FnBaselines) != 0 {
		t.Error("the fallback closes the scope")
	}
	r.PopFnBaseline() // empty: a no-op
}

func TestForkResetsBlockState(t *testing.T) {
	r, _ := blockReg(t)
	r.blockBinds = map[string]bool{"x": true}
	r.frameScopes = []int32{1}
	r.invokeKeepDefs = true
	f := r.ForkConcurrent()
	if f.blockBinds != nil || f.frameScopes != nil || f.invokeKeepDefs {
		t.Error("a fork starts with no block state")
	}
}

func TestInstallDefBlockShadowsEnclosingFn(t *testing.T) {
	r, _ := blockReg(t)
	fnOf := func(body Value) Value {
		return NewFunction(FnDefInfo{Signatures: []Signature{{
			Params: []FnParam{{Name: "n", Type: TInteger}}, Returns: []*Type{TInteger},
			Impl: Boru([]Value{body}), BarrierPos: BarrierAllForward,
		}}})
	}
	InstallDef(r, "h", fnOf(NewInteger(1)))
	id := EnterBlock(r)
	InstallDef(r, "h", fnOf(NewInteger(2)))
	if r.Defs.Depth("h") != 2 {
		t.Fatalf("a block's fn def over an enclosing overlapping fn shadows it: depth %d", r.Defs.Depth("h"))
	}
	if res, err := NewTop(r).Run(blockBody(NewWord("h"), NewInteger(0))); err != nil || Canon(res) != "2" {
		t.Errorf("inside the block the shadow dispatches: %s %v", Canon(res), err)
	}
	LeaveBlock(r, id)
	if r.Defs.Depth("h") != 1 {
		t.Fatalf("the shadow ends with the block: depth %d", r.Defs.Depth("h"))
	}
	if res, err := NewTop(r).Run(blockBody(NewWord("h"), NewInteger(0))); err != nil || Canon(res) != "1" {
		t.Errorf("after the block the enclosing fn stands: %s %v", Canon(res), err)
	}
	// In the SAME scope a redefinition still replaces.
	InstallDef(r, "h", fnOf(NewInteger(3)))
	if r.Defs.Depth("h") != 1 {
		t.Errorf("a same-scope redefinition drops the overlapping entry: depth %d", r.Defs.Depth("h"))
	}
}

func TestDefCensusBlockShadowGate(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	es := newS5BEmit()
	prev := r.Check.Emit
	r.Check.Emit = es
	defer func() { r.Check.Emit = prev }()

	siteAt(r, 1, 1)
	InstallDef(r, "n", NewInteger(0))
	blk := EnterBlock(r)
	r.noteAnalysisUseAt("n", SrcPos{Row: 1, Col: 15})
	siteAt(r, 1, 20)
	InstallDef(r, "n", NewInteger(1)) // the counter shape: a shadow that read its name
	LeaveBlock(r, blk)
	var warn *CheckDiagnostic
	for i := range r.Check.Diagnostics {
		if r.Check.Diagnostics[i].Code == "shadow_rebind" {
			warn = &r.Check.Diagnostics[i]
		}
	}
	if warn == nil {
		t.Fatalf("a block-local def that read the name it shadows warns shadow_rebind: %+v", r.Check.Diagnostics)
	}
	if warn.Severity != SeverityWarning || warn.Word != "n" || warn.Row != 1 || warn.Col != 20 || !strings.Contains(warn.Detail, "var n") {
		t.Errorf("the warning: %+v", *warn)
	}
	if len(es.uncompilable) != 1 || !strings.Contains(es.uncompilable[0], "block-local def `n`") {
		t.Errorf("the compiled lane declines the block shadow: %v", es.uncompilable)
	}
	// A plain shadow (no read) warns nothing and still declines.
	blk = EnterBlock(r)
	siteAt(r, 1, 30)
	InstallDef(r, "n", NewInteger(2))
	LeaveBlock(r, blk)
	if len(es.uncompilable) != 2 {
		t.Errorf("a plain block shadow declines too: %v", es.uncompilable)
	}
	nWarn := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "shadow_rebind" {
			nWarn++
		}
	}
	if nWarn != 1 {
		t.Errorf("a plain shadow is no warning: %d warnings", nWarn)
	}
	// A FRAME's shadow is neither.
	r.PushFnBaseline(nil)
	siteAt(r, 2, 1)
	InstallDef(r, "n", NewInteger(3))
	r.Defs.Pop("n")
	r.PopFnBaseline()
	if len(es.uncompilable) != 2 {
		t.Errorf("a frame's shadow is not a block's: %v", es.uncompilable)
	}
}

func TestDefCensusLeakReadAfterBlock(t *testing.T) {
	r, done := censusReg(t)
	defer done()
	// A block at the module scope binds z and ends; a read after it.
	id := EnterBlock(r)
	siteAt(r, 1, 10)
	InstallDef(r, "z", NewInteger(9))
	LeaveBlock(r, id)
	r.noteUnboundReadCensus("z", SrcPos{Row: 1, Col: 20})
	got := classesOf(r)
	if len(got) != 1 || got[0] != "leak-read:z" {
		t.Fatalf("census %v, want the leak read", got)
	}
	if e := r.Check.SortedDefCensus()[0]; e.Site != (SrcPos{Row: 1, Col: 20}) || e.Standing != (SrcPos{Row: 1, Col: 10}) || e.Note != "value" || e.Scope != ScopeModule {
		t.Errorf("the finding names the read and the block's binding: %+v", e)
	}
	// From inside a frame the module's record is out of the lexical region.
	r.PushFnBaseline(nil)
	r.noteUnboundReadCensus("z", SrcPos{Row: 2, Col: 1})
	// A block inside the frame, read after it inside the frame.
	inner := EnterBlock(r)
	siteAt(r, 2, 5)
	InstallDef(r, "w", NewInteger(1))
	LeaveBlock(r, inner)
	r.noteUnboundReadCensus("w", SrcPos{Row: 2, Col: 12})
	r.PopFnBaseline()
	// A name no block bound.
	r.noteUnboundReadCensus("q", SrcPos{Row: 3, Col: 1})
	if got := classesOf(r); len(got) != 2 || got[1] != "leak-read:w" {
		t.Errorf("census %v, want the module's and the frame's leak reads only", got)
	}
	// The engine's check-mode undefined-word diagnostic is the read's site
	// (the analysis continues over a placeholder, so the run returns).
	if _, err := NewTop(r).Run([]Value{NewWord("z")}); err != nil {
		t.Fatalf("the check pass continues past an unbound word: %v", err)
	}
	if got := classesOf(r); len(got) != 3 || strings.Count(strings.Join(got, " "), "leak-read:z") != 2 {
		t.Errorf("census %v, want the engine's read counted", got)
	}
	// Off the check pass nothing is recorded.
	var nr *Registry
	nr.noteUnboundReadCensus("z", SrcPos{})
}

// A pending container literal the arm leaves — a list or a map whose
// elements read the arm's own local — evaluates BEFORE the block closes
// (evalBlockResiduals): the frame's end or the consumer would evaluate it
// after the local is gone (`if true [def h 5 [h]] [0]` met `undefined
// word: h`, the net codec's `{msg: {word: head} rest: tail}` likewise,
// 2026-10-07). A container that reads none of the block's names keeps its
// timing and its Eval mark; a nested paren group inside the literal is
// walked too; an evaluation error inside the literal surfaces and the
// fault return closes the block.
func TestIfArmBlockEvaluatesItsResidualContainers(t *testing.T) {
	r, _ := blockReg(t)
	// [blk] — a list reading the block's local.
	toks := ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), NewEvalList([]Value{NewWord("blk")})), ThenBlock: true})
	res, err := NewTop(r).Run(toks)
	if err != nil || Canon(res) != "[5]" {
		t.Fatalf("list residual = %s, %v, want [5]", Canon(res), err)
	}
	assertClosed(t, r, "list residual")
	// {a: (blk)} — a map whose value is a paren group over the local.
	om := NewOrderedMap()
	om.Set("a", NewValueRaw(TParenExpr, ParenExprPayload{Toks: []Value{NewWord("blk")}}))
	m := NewMap(om)
	m.Eval = true
	toks = ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), m), ThenBlock: true})
	res, err = NewTop(r).Run(toks)
	if err != nil || Canon(res) != "{a:5}" {
		t.Fatalf("map residual = %s, %v, want {a:5}", Canon(res), err)
	}
	assertClosed(t, r, "map residual")
	// A container reading no block name is not the block's to evaluate:
	// it keeps its Eval mark for the run's end to resolve (its value is
	// the same either way).
	toks = ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), NewEvalList([]Value{NewInteger(1), NewInteger(2)})), ThenBlock: true})
	res, err = NewTop(r).Run(toks)
	if err != nil || Canon(res) != "[1 2]" {
		t.Fatalf("unrelated residual = %s, %v, want [1 2]", Canon(res), err)
	}
	assertClosed(t, r, "unrelated residual")
	// An error inside the literal surfaces, and the block is closed.
	toks = ifTokens(NextMarkID(), NewBoolean(true), &IfCont{Then: blockBody(NewWord("var"), NewInteger(5), NewEvalList([]Value{NewWord("blk"), NewWord("bfail")})), ThenBlock: true})
	if _, err := NewTop(r).Run(toks); err == nil {
		t.Fatal("the literal's raise must surface")
	}
	assertClosed(t, r, "raising residual")
	// mentionsAnyName walks every container shape and says no for the rest.
	names := map[string]bool{"blk": true}
	if mentionsAnyName(NewInteger(1), names) || mentionsAnyName(NewMap(nil), names) || mentionsAnyName(NewValueRaw(TMap, MapPayload{}), names) {
		t.Error("a scalar or an empty map mentions nothing")
	}
	if !mentionsAnyName(NewList([]Value{NewList([]Value{NewWord("blk")})}), names) {
		t.Error("a nested list naming the local mentions it")
	}
	// A dot access over the local, a computed key reading it, a template
	// string's hole over it: each mentions it; another receiver, a literal
	// key, a literal part do not.
	if !mentionsAnyName(NewReachFromKeys(NewWord("blk"), []Value{NewString("a")}), names) || mentionsAnyName(NewReachFromKeys(NewWord("m"), []Value{NewString("blk")}), names) {
		t.Error("a dot access mentions its receiver's words, not its literal keys")
	}
	computed := NewReach(ReachInfo{Receiver: []Value{NewWord("m")}, Segments: []ReachSeg{{Computed: true, KeyExpr: []Value{NewWord("blk")}}}})
	if !mentionsAnyName(computed, names) {
		t.Error("a computed key over the local mentions it")
	}
	if !mentionsAnyName(NewInterpString([]InterpPart{{Lit: "x"}, {Expr: []Value{NewWord("blk")}}}), names) || mentionsAnyName(NewInterpString([]InterpPart{{Lit: "blk"}}), names) {
		t.Error("a template string's hole over the local mentions it; a literal part does not")
	}
	if r.Defs.ScopeBoundNames(99) != nil {
		t.Error("a scope that is not open has no bound names")
	}
}
