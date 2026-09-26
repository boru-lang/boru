package lang

import (
	"fmt"
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
	// shape carries. def's keyword forms are checked by rule below (32
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
	if defForms != 32 {
		t.Errorf("def: %d code-body keyword forms, want 32 (the census's S2b worklist)", defForms)
	}
	// 26 named above + def's 32 = the 58 signatures S2b declared.
	if seen+defForms != 58 {
		t.Errorf("pinned %d code-body signatures, want 58", seen+defForms)
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
	if want := core.CompileRunsBodyOnRegistry | core.CompileDynBody; sig.CompileEffect != want {
		t.Errorf("receive %s: CompileEffect %v, want exactly CompileRunsBodyOnRegistry|CompileDynBody", s2aShape(sig), sig.CompileEffect)
	}
	if sig.RunInCheckMode() || sig.ReturnsFn != nil {
		t.Errorf("receive: the check pass must not run the clause bodies (check mode %v, ReturnsFn %v) — the replay-hazard exemption rests on it",
			sig.RunInCheckMode(), sig.ReturnsFn != nil)
	}
}

// TestS2BReceiveBodyOnRegistryLowering: the declaration's behaviour on both
// lanes. At the top-level statement position the dispatch bakes and answers
// as the interpreter does (the module-scope rule). At a NESTED position — a
// fn body, a loop body, a branch arm — it bakes only when the clause list
// names nothing the program or the registry knows
// (registryBodyNamesNothingKnown): the handler's own keyword `after` and
// literals, as the sweep's seeds are. Every clause body that could read a
// name the VM holds as a frame local — a fn param (bare, in a list, in a map
// member), a loop iterator — or reach interpreter-maintained state (`args`),
// or change a binding the program reads after it, DECLINES. The fn-param,
// iterator and rebind shapes all miscompiled on main before the flag
// (ae17688): `undefined word` for the first two, an internal RESTEP_LANDING
// underflow for the third.
func TestS2BReceiveBodyOnRegistryLowering(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`receive [ {never: 1} [ "msg" ] after 5 [ "timed-out" ] ]`, "[timed-out]"},
		{`def k 4 end receive [ {never: 1} [ 0 ] after 5 [ k add 1 ] ]`, "[5]"},
		{`receive [ {never: 1} [ 0 ] after 5 [ (1 add 2) ] ]`, "[3]"},
		// Nested positions whose clause list names nothing known.
		{`def f fn [[n:Integer][Integer][receive [ {never: 1} [ 0 ] after 5 [ 9 ] ]]] end f 7`, "[9]"},
		{`for 2 [receive [ {} [ 1 ] after 0 [ 7 ] ]]`, "[7 7]"},
		{`if true [receive [ {never: 1} [ 0 ] after 5 [ 9 ] ]] [0]`, "[9]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errC != nil || errI != nil || !compiled {
			t.Errorf("%q: want a compiled run on both lanes, compiled=%v errC=%v errI=%v", c.src, compiled, errC, errI)
			continue
		}
		if fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: compiled %v, interp %v, want %s", c.src, gotC, gotI, c.want)
		}
	}
	for _, c := range []struct{ src, want string }{
		// A clause body reading a fn param inside the fn (the miscompile).
		{`def f fn [[n:Integer][Integer][receive [ {never: 1} [ 0 ] after 5 [ n ] ]]] end f 7`, "[7]"},
		// ... a top-level loop's iterator.
		{`for 2 [receive [ {never: 1} [ 0 ] after 5 [ i ] ]]`, "[0 1]"},
		// ... a body that rebinds a name the program reads after it.
		{`def x 1 end receive [ {never: 1} [ 0 ] after 5 [ def x 2 ] ] x`, "[2]"},
		// ... a fn param one level down, as a map member.
		{`def f fn [[n:Integer][Any][receive [ {never: 1} [ 0 ] after 5 [ {a: n} ] ]]] end f 7`, "[{a:7}]"},
		// ... a registered word reading interpreter-maintained state.
		{`def f fn [[n:Integer][Any][receive [ {never: 1} [ 0 ] after 5 [ args ] ]]] end f 7`, "[[7]]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if compiled {
			t.Errorf("%q: must decline, compiled to %v/%v", c.src, gotC, errC)
		}
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp %v/%v, want %s", c.src, gotI, errI, c.want)
		}
		// The decline is loud and counted, never a wrong compiled answer.
		requireEngineParity(t, c.src, false)
	}
	// A name NEITHER lane knows is admitted at a nested position and raises
	// the same undefined_word on both.
	const unknown = `def f fn [[n:Integer][Any][receive [ {never: 1} [ 0 ] after 5 [ zzq ] ]]] end f 7`
	_, compiled, errC, _, errI := runBothEngines(t, unknown)
	if !compiled || errC == nil || errI == nil || codeOf(errC) != "undefined_word" || codeOf(errI) != "undefined_word" {
		t.Errorf("%q: want a compiled run raising undefined_word on both lanes, compiled=%v errC=%v errI=%v", unknown, compiled, errC, errI)
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

// TestS2BDeclaredWordsKeepParity: programs over the declared words compile
// and answer as the interpreter does — the declaration moved no lowering.
func TestS2BDeclaredWordsKeepParity(t *testing.T) {
	for _, src := range []string{
		// CompileOwnLowering: the structured half.
		`def c true end if c [1] [2]`,
		`if false [1]`,
		`for 3 [1]`,
		`for [0 3] [i]`,
		`def n 0 end while [n lt 2] [def n (n add 1)] end n`,
		// CompileOwnLowering: the check-mode constructors and binders.
		`def f fn [[x:Integer] [Integer] [x add 1]] end f 2`,
		`2 (x:Integer => [x add 1]) apply`,
		`def P fnpred n:Integer [n gt 0] end 5 is P`,
		`def G gen [T] class {v: T} end G deq G`,
		`def m (macro [[e] [ quote [ unquote e add 1 ] ]]) end m 5`,
		`import module [export "M" {x: 1}] end M has x/q`,
		// CompileResteps: the splices.
		`var [[[a 1] [b 2]] a add b]`,
		`def dbl word [dup add] end 5 dbl`,
		// CompileQuoteKey / CompileQuoteInert over a NoEvalArgs list.
		`unpack [a b] {a:1 b:2} a add b`,
		`def f fn [[n:Integer][Any][reach n ['a']]] end f 7`,
		`def Color enum [red green blue] end Color tcmp Color`,
	} {
		requireEngineParity(t, src, true)
	}
}
