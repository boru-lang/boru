package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The arm-residency bridge's contract, driven directly (the corpus
// exercises the end-to-end path; these arms pin each fence in
// isolation): a total name+order match between the guard bracket's
// twins and the fresh unit's def events stamps and places; EVERY
// mismatch — name, kind, leftover on either side, stale memoized unit,
// wrong body identity — adopts nothing, leaving the twins unplaced and
// the program refused (the sound direction the parity oracle pins). Adopted names fence later root reads —
// NoteDefRead poisons the placement gate (armReadRefusal, refused at
// Finalize's seam, NOT a recorder-layer MarkUncompilable: the
// refusal-site census counts that layer and its count only falls) —
// until a live root install re-binds them.
func TestAdoptResidentTwinsFences(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Begin a check pass so minted values carry compile identities — the
	// latch's bodyID fence needs a real ID, exactly as production has one
	// (runtime mints elide IDs, value.go checkPassDepth).
	defer r.Check.Begin()()
	body := core.NewInteger(1)
	pos := core.SrcPos{Row: 1, Col: 9}

	dynEv := func(name string) EmitEvent {
		return EmitEvent{kind: evDynBind, dyn: &emitDynBind{
			name: name, srcSeq: -1, val: core.NewInteger(5), pos: pos, residentTwin: -1}}
	}
	// build wires a state with a bracketed twin for each noted name and a
	// fresh synthetic unit whose def events carry the given names.
	build := func(twinNames, eventNames []string) *EmitState {
		es := NewEmitState()
		es.BindRegistry(r)
		end := es.MultiRunBodyGuard(r, body.ID)
		for _, n := range twinNames {
			es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: n, Depth: 1, Pos: pos},
				core.DefEntry{Body: core.NewInteger(5)})
		}
		end()
		frag := &EmitFragment{}
		for _, n := range eventNames {
			frag.events = append(frag.events, dynEv(n))
		}
		es.fnRecs = append(es.fnRecs, &fnUnitRec{reg: r, frag: frag})
		es.lastClosure = closureLatch{unit: 0, fresh: true}
		return es
	}
	placed := func(es *EmitState) int {
		n := 0
		for _, p := range es.twinPlaced {
			if p {
				n++
			}
		}
		return n
	}

	// Total match: stamped, placed, read-fenced.
	es := build([]string{"x", "y"}, []string{"x", "y"})
	es.AdoptResidentTwins(body)
	if placed(es) != 2 || es.fnRecs[0].frag.events[0].dyn.residentTwin != 0 ||
		es.fnRecs[0].frag.events[1].dyn.residentTwin != 1 {
		t.Fatalf("total match must stamp both events and place both twins (placed=%d)", placed(es))
	}
	if !es.armBoundNames["x"] || !es.armBoundNames["y"] {
		t.Fatal("adopted names must join the read fence")
	}
	// The read fence poisons the placement gate (the recorder itself stays
	// Compilable — the refusal is Finalize's seam), and a live root
	// install lifts it.
	es.NoteDefRead("some-id", "x")
	if !es.Compilable || !strings.Contains(es.armReadRefusal, "read of `x` after a multi-run body binds it") ||
		!strings.HasPrefix(es.armReadRefusal, "twin regime: ") {
		t.Fatalf("a root read of an arm-bound name must poison the placement gate (Compilable=%v, refusal %q)",
			es.Compilable, es.armReadRefusal)
	}
	es2 := build([]string{"z"}, []string{"z"})
	es2.AdoptResidentTwins(body)
	es2.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "z", Depth: 1, Pos: pos},
		core.DefEntry{Body: core.NewInteger(7)}) // live root install re-binds
	es2.NoteDefRead("some-id", "z")
	if !es2.Compilable || es2.armReadRefusal != "" {
		t.Fatalf("a live root install must lift the read fence (refusal %q)", es2.armReadRefusal)
	}

	// Name mismatch: nothing adopted.
	es = build([]string{"x"}, []string{"y"})
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a name mismatch must adopt nothing")
	}

	// Leftover twin (the var-pair class: an undef kind in the bracket).
	es = build([]string{"x"}, []string{"x"})
	end := es.MultiRunBodyGuard(r, body.ID)
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "x", Depth: 1, Pos: pos},
		core.DefEntry{Body: core.NewInteger(5)})
	es.RecordBindTwin(core.BindTransition{Kind: core.BindUndef, Name: "r"}, core.DefEntry{})
	end()
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a twin with no def site of its own must decline the whole bridge")
	}

	// A def-replace twin is a shape the bridge carries no op for.
	es = build([]string{}, []string{"x"})
	end = es.MultiRunBodyGuard(r, body.ID)
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDefReplace, Name: "x", Depth: 1, Pos: pos},
		core.DefEntry{Body: core.NewInteger(5)})
	end()
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a def-replace twin in the bracket must decline the whole bridge")
	}

	// A VALUE def whose captured entry carries a TYPE node: the install arm
	// would install a runtime value under a name the entry says is a type.
	es = build([]string{}, []string{"x"})
	end = es.MultiRunBodyGuard(r, body.ID)
	es.RecordBindTwin(core.BindTransition{Kind: core.BindDef, Name: "x", Depth: 1, Pos: pos},
		core.DefEntry{Body: core.NewInteger(5), TypeDef: core.TInteger})
	end()
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a BindDef twin carrying a type node must decline the whole bridge")
	}

	// Leftover event: more def sites than bracket twins.
	es = build([]string{"x"}, []string{"x", "extra"})
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a leftover unit event must decline the whole bridge")
	}

	// Stale memoized unit.
	es = build([]string{"x"}, []string{"x"})
	es.lastClosure.fresh = false
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a memo-hit unit must decline (its events belong to another dispatch)")
	}

	// Wrong body identity (a nested body's analysis overwrote the latch).
	es = build([]string{"x"}, []string{"x"})
	other := core.NewInteger(2)
	es.AdoptResidentTwins(other)
	if placed(es) != 0 {
		t.Fatal("a bodyID mismatch must decline the bridge")
	}

	// A bracket twin ALREADY placed (an earlier adoption or a placed op):
	// the bridge declines rather than stamping a second placement.
	es = build([]string{"x", "y"}, []string{"x", "y"})
	es.twinPlaced[0] = true
	es.AdoptResidentTwins(body)
	if placed(es) != 1 || es.fnRecs[0].frag.events[0].dyn.residentTwin != -1 {
		t.Fatal("an already-placed bracket twin must decline the bridge without stamping")
	}

	// Kind crossed: a BindDef twin against a TEARDOWN site (or the reverse)
	// never pairs.
	es = build([]string{"x"}, []string{"x"})
	es.fnRecs[0].frag.events[0].dyn.undef = true
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a def twin against an undef site must decline the bridge")
	}

	// Position crossed: both sides positioned, at different sites.
	es = build([]string{"x"}, []string{"x"})
	es.fnRecs[0].frag.events[0].dyn.pos = core.SrcPos{Row: 2, Col: 2}
	es.AdoptResidentTwins(body)
	if placed(es) != 0 {
		t.Fatal("a twin and a def site at different positions must decline the bridge")
	}

	// Nil receiver: no-op.
	var nilES *EmitState
	nilES.AdoptResidentTwins(body)
}

// The TYPE twin's half of the same bridge: a BindTypeInstall twin pairs
// ONLY with a type-install site, and the pairing is crossed in both
// directions to prove neither kind can stand in for the other. The
// admitted case needs a real token body, because the screen reads the def
// site's type expression out of it (typeInstallElementIndependent).
func TestAdoptResidentTwinsTypeTwins(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Check.Begin()()

	defPos := core.SrcPos{Row: 1, Col: 1}
	at := func(v core.Value, row, col int) core.Value {
		v.SetPos(core.SrcPos{Row: row, Col: col})
		return v
	}
	word := func(name string, row, col int) core.Value { return at(core.NewWord(name), row, col) }
	// `def Big (Integer gt 5)` — one def site, an element-independent bound.
	body := core.NewList([]core.Value{
		word("def", 1, 1), word("Big", 1, 5),
		at(core.NewParenExpr([]core.Value{
			word("Integer", 1, 10), word("gt", 1, 18), at(core.NewInteger(5), 1, 21),
		}), 1, 9),
	})
	body.SetPos(core.SrcPos{Row: 1, Col: 0})

	build := func(kind core.BindKind, evTypeInstall bool) *EmitState {
		es := NewEmitState()
		es.BindRegistry(r)
		end := es.MultiRunBodyGuard(r, body.ID)
		es.RecordBindTwin(core.BindTransition{Kind: kind, Name: "Big", Depth: 1, Pos: defPos},
			core.DefEntry{Body: core.NewInteger(5), TypeDef: core.TInteger, Minted: true})
		end()
		frag := &EmitFragment{events: []EmitEvent{{kind: evDynBind, dyn: &emitDynBind{
			name: "Big", srcSeq: -1, pos: defPos, residentTwin: -1, typeInstall: evTypeInstall,
		}}}}
		es.fnRecs = append(es.fnRecs, &fnUnitRec{reg: r, frag: frag})
		es.lastClosure = closureLatch{unit: 0, fresh: true}
		return es
	}

	es := build(core.BindTypeInstall, true)
	es.AdoptResidentTwins(body)
	if !es.twinPlaced[0] || es.fnRecs[0].frag.events[0].dyn.residentTwin != 0 {
		t.Fatal("a type twin over an element-independent expression must be stamped and placed")
	}
	if !es.armBoundNames["Big"] {
		t.Fatal("an adopted type name must join the read fence too — its DEPTH is body-run-dependent")
	}

	es = build(core.BindTypeInstall, false)
	es.AdoptResidentTwins(body)
	if es.twinPlaced[0] {
		t.Fatal("a type twin against a plain def site must decline the bridge")
	}

	es = build(core.BindDef, true)
	es.AdoptResidentTwins(body)
	if es.twinPlaced[0] {
		t.Fatal("a value-def twin against a type-install site must decline the bridge")
	}

	// The screen declines: the body binds the word the bound reads. The
	// event and the twin agree on everything else, so only the screen can
	// be what refuses.
	esDep := NewEmitState()
	esDep.BindRegistry(r)
	depBody := core.NewList([]core.Value{
		word("def", 1, 1), word("Big", 1, 5),
		at(core.NewParenExpr([]core.Value{
			word("Integer", 1, 10), word("gt", 1, 18), word("Big", 1, 21),
		}), 1, 9),
	})
	depBody.SetPos(core.SrcPos{Row: 1, Col: 0})
	end := esDep.MultiRunBodyGuard(r, depBody.ID)
	esDep.RecordBindTwin(core.BindTransition{Kind: core.BindTypeInstall, Name: "Big", Depth: 1, Pos: defPos},
		core.DefEntry{Body: core.NewInteger(5), TypeDef: core.TInteger, Minted: true})
	end()
	esDep.fnRecs = append(esDep.fnRecs, &fnUnitRec{reg: r, frag: &EmitFragment{events: []EmitEvent{
		{kind: evDynBind, dyn: &emitDynBind{name: "Big", srcSeq: -1, pos: defPos, residentTwin: -1, typeInstall: true}},
	}}})
	esDep.lastClosure = closureLatch{unit: 0, fresh: true}
	esDep.AdoptResidentTwins(depBody)
	if esDep.twinPlaced[0] {
		t.Fatal("a type expression reading a name the body binds must decline the bridge")
	}
}
