package lang

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	native "github.com/boru-lang/boru/lang/go/native"
)

// s2b_declarations_test.go pins S2b of design/FULL-COMPILATION-REPLAN.0.md
// (design/HANDLER-MIGRATION-LINE.0.md, the code-body class): the census's
// code-body signatures each carry the declaration true of their handler,
// per word and shape, read from the live registry exactly as
// TestDeclarationCensus reads it. None of the declarations changes a
// recorded program — every recorder gate that reads these flags screens
// NoEvalArgs or RunInCheckMode first — so the parity programs below are
// the same answers, on both lanes, that the words gave before.

func TestS2BDeclarationsByWordAndShape(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	own, resteps, dyn := core.CompileOwnLowering, core.CompileResteps, core.CompileDynBody
	key, inert := core.CompileQuoteKey, core.CompileQuoteInert
	// word -> shape -> the EXACT CompileEffect every NoEvalArgs sig of that
	// shape carries. def's keyword forms are checked by rule below (34
	// forms, synthesized from the constructors' tables).
	want := map[string]map[string]core.CompileEffect{
		// Structured ReturnsFn lowering (RecordBranch / RecordLoop). Two
		// moved on 2026-09-26, each CHANGING lowering: the clause-list `if`
		// (List) left CompileResteps — its ReturnsFn now records the chain
		// as branch events (basic's ifClauseRecord) — and `for` took
		// CompileDynBody, the hosted splice of a computed body.
		"if":    {"(Any Any Any)": own, "(Any Any)": own, "(List)": own},
		"for":   {"(Integer List)": own | dyn, "(List List)": own | dyn},
		"while": {"(List List)": own},
		// Check-mode constructors: the value is built on the check engine.
		"fn":     {"(Any Any List)": own, "(List)": own},
		"afn":    {"(Any Any)": own},
		"fnsig":  {"(Any Any)": own, "(List)": own},
		"fnpred": {"(Any Any)": own, "(List)": own},
		"gen":    {"(List)": own},
		"macro":  {"(List)": own},
		"module": {"(List)": own},
		// import: the rename lists are export names (keys); the inline
		// module forms run their body on the check engine.
		"import": {"(List Module)": key, "(List String)": key, "(Atom List)": own, "(List Atom List)": own, "(Atom Atom List)": own},
		// Splices the tape re-steps.
		"var":  {"(List)": resteps},
		"word": {"(Any)": resteps},
		// A NoEvalArgs list of names / keys / literal data.
		"unpack": {"(List Map)": key},
		"reach":  {"(Any List)": key},
		"enum":   {"(List)": inert},
	}
	var seen int
	for word, shapes := range want {
		fd := reg.Lookup(word)
		if fd == nil {
			t.Errorf("%s: not registered", word)
			continue
		}
		found := map[string]int{}
		for i := range fd.Signatures {
			sig := &fd.Signatures[i]
			if len(sig.NoEvalArgs) == 0 {
				continue
			}
			shape := s2aShape(sig)
			flag, ok := shapes[shape]
			if !ok {
				t.Errorf("%s %s: a code-body signature the pin does not name — the worklist moved", word, shape)
				continue
			}
			found[shape]++
			if sig.CompileEffect != flag {
				t.Errorf("%s %s: CompileEffect %v, want exactly %v", word, shape, sig.CompileEffect, flag)
			}
		}
		for shape := range shapes {
			if found[shape] == 0 {
				t.Errorf("%s %s: no code-body signature of that shape is registered", word, shape)
			}
			seen += found[shape]
		}
	}
	// def's keyword forms: every code-body form declares CompileOwnLowering
	// beside S2a's quoted-operand answer — the Atom-named form's NAME is a
	// key, the String-named form's quoted operands are Pattern-pinned
	// keywords (inert).
	fd := reg.Lookup("def")
	if fd == nil {
		t.Fatal("def: not registered")
	}
	defForms := 0
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if len(sig.NoEvalArgs) == 0 {
			continue
		}
		defForms++
		wantDef := inert | own
		if len(sig.Args) > 0 && sig.Args[0] != nil && sig.Args[0].Equal(core.TAtom) {
			wantDef = key | own
		}
		if sig.CompileEffect != wantDef {
			t.Errorf("def %s: CompileEffect %v, want exactly %v", s2aShape(sig), sig.CompileEffect, wantDef)
		}
	}
	// 32 -> 34 at the merge of main's #510 with the reverse-order NUR run:
	// fn's 0-argument refusal (NUR091, a base signature that always raises)
	// synthesizes the gen chain's `def name gen [T] fn` in both its
	// Atom-named and String-named spellings, code-body forms like the rest.
	if defForms != 34 {
		t.Errorf("def: %d code-body keyword forms, want 34 (the census's S2b worklist)", defForms)
	}
	// 26 named above + def's 34 = the 60 signatures S2b declares.
	if seen+defForms != 60 {
		t.Errorf("pinned %d code-body signatures, want 60", seen+defForms)
	}
}

// TestS2BReceiveDeclaresRunsBodyOnRegistry: `receive`'s clause body runs on
// a sub-engine over the ENCLOSING registry (runClauseBody → New(r).Run) in
// both modes, so its declaration is CompileRunsBodyOnRegistry — the census's
// last code-body signature (2026-09-26, the S2b follow-up). The flag's
// module-scope rule drops the replay-hazard screen on the premise that the
// CHECK pass never runs the body; for receive that holds because the word is
// neither check-mode nor carries a ReturnsFn (its result is the declared Any),
// and this pins the premise beside the flag so a later ReturnsFn that ran the
// clause bodies would have to revisit it.
func TestS2BReceiveDeclaresRunsBodyOnRegistry(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fd := reg.Lookup("receive")
	if fd == nil {
		t.Fatal("receive: not registered")
	}
	if len(fd.Signatures) != 1 {
		t.Fatalf("receive: %d signatures, want 1", len(fd.Signatures))
	}
	sig := &fd.Signatures[0]
	// A literal clause list takes the registry-body rule, a COMPUTED one the
	// dyn-body backstop (the sweep's `receive` × module-export cell); the two
	// flags split on the operand, and nothing else rides beside them.
	// Beside them, the observable effect a mailbox read is (NUR356).
	if want := core.CompileRunsBodyOnRegistry | core.CompileDynBody | core.CompileSideEffect; sig.CompileEffect != want {
		t.Errorf("receive %s: CompileEffect %v, want exactly CompileRunsBodyOnRegistry|CompileDynBody|CompileSideEffect", s2aShape(sig), sig.CompileEffect)
	}
	if sig.RunInCheckMode() || sig.ReturnsFn != nil {
		t.Errorf("receive: the check pass must not run the clause bodies (check mode %v, ReturnsFn %v) — the replay-hazard exemption rests on it",
			sig.RunInCheckMode(), sig.ReturnsFn != nil)
	}
}

// TestS2BOwnLoweringShapes: CompileOwnLowering is a CODE-BODY fact of one of
// two shapes — a check-mode handler, or a structured ReturnsFn — and never
// sits beside CompileResteps (a word whose result the tape re-steps has no
// lowering of its own) or on a signature with no NoEvalArgs operand. Checked
// over the WHOLE default registry, so a later declarer is held to the same
// contract.
func TestS2BOwnLoweringShapes(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, name := range reg.RegisteredWordNames() {
		fd := reg.Lookup(name)
		if fd == nil {
			continue
		}
		for i := range fd.Signatures {
			sig := &fd.Signatures[i]
			if !sig.CompileEffect.Has(core.CompileOwnLowering) {
				continue
			}
			n++
			if len(sig.NoEvalArgs) == 0 {
				t.Errorf("%s %s: CompileOwnLowering on a signature with no code body", name, s2aShape(sig))
			}
			if !sig.RunInCheckMode() && sig.ReturnsFn == nil {
				t.Errorf("%s %s: CompileOwnLowering with neither a check-mode handler nor a structured ReturnsFn", name, s2aShape(sig))
			}
			if sig.CompileEffect.Has(core.CompileResteps) {
				t.Errorf("%s %s: CompileOwnLowering beside CompileResteps", name, s2aShape(sig))
			}
		}
	}
	if n == 0 {
		t.Fatal("no signature declares CompileOwnLowering")
	}
}
