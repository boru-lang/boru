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
	own, resteps := core.CompileOwnLowering, core.CompileResteps
	key, inert := core.CompileQuoteKey, core.CompileQuoteInert
	// word -> shape -> the EXACT CompileEffect every NoEvalArgs sig of that
	// shape carries. def's keyword forms are checked by rule below (32
	// forms, synthesized from the constructors' tables).
	want := map[string]map[string]core.CompileEffect{
		// Structured ReturnsFn lowering (RecordBranch / RecordLoop).
		"if":    {"(Any Any Any)": own, "(Any Any)": own, "(List)": resteps},
		"for":   {"(Integer List)": own, "(List List)": own},
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

// TestS2BReceiveStaysUndeclared is the negative: `receive`'s clause body runs
// on a sub-engine over the ENCLOSING registry (runClauseBody), whose true
// declaration, CompileRunsBodyOnRegistry, changes lowering — so a
// declaration-only line leaves it at the zero value rather than write a flag
// that is not its contract. When the word is migrated this pin moves with
// it, and the census ceiling goes to 0.
func TestS2BReceiveStaysUndeclared(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fd := reg.Lookup("receive")
	if fd == nil {
		t.Fatal("receive: not registered")
	}
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if sig.CompileEffect != core.CompileDefault || sig.Callable != nil {
			t.Errorf("receive %s: declared %v — update the census ceiling, this pin and the handoff together", s2aShape(sig), sig.CompileEffect)
		}
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
