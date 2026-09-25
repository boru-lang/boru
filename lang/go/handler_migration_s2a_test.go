package lang

import (
	"fmt"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	native "github.com/boru-lang/boru/lang/go/native"
)

// handler_migration_s2a_test.go pins S2a of design/FULL-COMPILATION-
// REPLAN.0.md (design/HANDLER-MIGRATION-LINE.0.md, Progress 2026-09-25):
// the 35 declaration-only handler signatures — the quoted class's 28 and
// the fn-operand class's 7 — each carry the declaration the handler
// honestly owes, per signature shape. The census (`TestDeclarationCensus`)
// counts them; this names them, so a registration change that dropped or
// swapped a flag fails here by word and shape rather than as a number.

// s2aShape renders a signature's parameter types the way the census
// worklist prints them, `(Atom Any)`.
func s2aShape(sig *core.Signature) string {
	out := "("
	for i, a := range sig.Args {
		if i > 0 {
			out += " "
		}
		if a == nil {
			out += "?"
			continue
		}
		out += a.Name()
	}
	return out + ")"
}

func TestS2ADeclarationsByWordAndShape(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	key, inert, resteps := core.CompileQuoteKey, core.CompileQuoteInert, core.CompileResteps
	want := map[string]map[string]core.CompileEffect{
		// The binding words: the quoted NAME of a registry write or removal
		// is a key; a String-named def form's only quoted operand is the
		// Pattern-pinned constructor keyword, inert data.
		"def": {
			"(Atom Any)": key, "(Atom Atom Any Node)": key, "(Atom Atom Any)": key, "(Atom Atom Map)": key,
			"(String Atom Any Node)": inert, "(String Atom Any)": inert, "(String Atom Map)": inert,
		},
		"undef":      {"(Atom)": key, "(Atom FunctionSignature)": key},
		"__varundef": {"(Atom)": key},
		// Quoted data the handler consumes verbatim, or a key it reads.
		"describe": {"(Atom)": inert},
		"unpack":   {"(Atom Map)": inert, "(Atom String)": key},
		"xml-attr": {"(Atom Xml)": key},
		// The re-stepping words: a wrapper, a parked binding or a splice the
		// tape runs — the declared refusal, on the quoted AND the fn forms.
		"usurp":        {"(Atom)": resteps},
		"stack-args":   {"(Atom)": resteps},
		"forward-args": {"(Atom)": resteps},
		"force-arity":  {"(Integer Atom)": resteps},
		"valof":        {"(Atom)": resteps},
		"apply":        {"(Function)": resteps},
		"mini":         {"(Atom String Map)": resteps, "(Atom String)": resteps, "(Function String Map)": resteps, "(Function String)": resteps},
		"parse":        {"(Atom Map Any)": resteps, "(Atom Any)": resteps, "(Function Map Any)": resteps, "(Function Any)": resteps},
		"emit":         {"(Atom Any Any)": resteps, "(Atom Any)": resteps, "(Atom)": resteps, "(Function Any Any)": resteps, "(Function Any)": resteps},
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
			// Mirror the census's relevance: a NoEvalArgs sig is the code-body
			// class (S2b, untouched — a def keyword form whose constructor takes
			// a raw body, the gen chain), and an unquoted, fn-free sig of the
			// same shape (describe's plain `[Atom]`) is not relevant at all.
			if len(sig.NoEvalArgs) > 0 {
				continue
			}
			fnSlot := false
			for _, ty := range sig.ArgTypes() {
				if ty != nil && ty.ConformsTo(core.TFunction) {
					fnSlot = true
				}
			}
			if len(sig.QuoteArgs) == 0 && !fnSlot {
				continue
			}
			shape := s2aShape(sig)
			flag, relevant := shapes[shape]
			if !relevant {
				continue
			}
			// A shape may register more than once (def's two Map-keyword
			// forms, one per constructor); every copy declares alike.
			found[shape]++
			if sig.CompileEffect != flag {
				t.Errorf("%s %s: CompileEffect %v, want exactly %v", word, shape, sig.CompileEffect, flag)
			}
			// The one exclusion the new flag carries: never an admission
			// beside the refusal.
			if flag == resteps && (sig.CompileEffect.Has(key) || sig.CompileEffect.Has(inert) || sig.CompileEffect.Has(core.CompileReadsFn) || sig.CompileEffect.Has(core.CompileStoresFn)) {
				t.Errorf("%s %s: CompileResteps must not sit beside an admission", word, shape)
			}
		}
		for shape := range shapes {
			if found[shape] == 0 {
				t.Errorf("%s %s: no signature of that shape is registered — the worklist moved", word, shape)
			}
			seen += found[shape]
		}
	}
	// 28 quoted + 7 fn-operand = the 35 signatures the census counted, under
	// 33 shapes (def's two Map-keyword shapes register twice each, one per
	// constructor).
	if seen != 35 {
		t.Errorf("pinned %d signatures, want 35 (the census's S2a worklist)", seen)
	}
}

// TestS2AWordsStillMatchTheCensusRelevance: every pinned shape is one the
// census calls declaration-relevant (quoted or fn-operand) — so the pin
// above cannot silently drift onto an irrelevant overload.
func TestS2AWordsStillMatchTheCensusRelevance(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"def", "undef", "__varundef", "describe", "unpack", "xml-attr", "usurp", "stack-args", "forward-args", "force-arity", "valof", "apply", "mini", "parse", "emit"} {
		fd := reg.Lookup(word)
		if fd == nil {
			t.Fatalf("%s: not registered", word)
		}
		for i := range fd.Signatures {
			sig := &fd.Signatures[i]
			if sig.CompileEffect == core.CompileDefault {
				continue
			}
			quoted := len(sig.QuoteArgs) > 0
			fnSlot := false
			for _, ty := range sig.ArgTypes() {
				if ty != nil && ty.ConformsTo(core.TFunction) {
					fnSlot = true
				}
			}
			if !quoted && !fnSlot && sig.CompileEffect.Has(core.CompileResteps) {
				t.Errorf("%s %s: CompileResteps on a sig that is neither quoted nor fn-operand", word, s2aShape(sig))
			}
		}
	}
}

// TestS2ADeclarationsThatLowerDoSoWithParity: the two S2a declarers that do
// NOT run in check mode reach the recorder's quoted-operand gates, so their
// declaration is a lowering, not just a census answer — `describe foo`
// bakes a CALL_NATIVE over the inert atom (CompileQuoteInert) and
// `xml-attr x <…/>` rides the key admission (CompileQuoteKey). Both must
// compile AND agree with the interpreter.
func TestS2ADeclarationsThatLowerDoSoWithParity(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"xml-attr quoted key, present", `xml-attr x <a x="1" y="2"/>`, "[1]"},
		{"xml-attr quoted key, missing", `xml-attr z <a x="1"/>`, "[None]"},
		{"describe quoted word", `describe add`, "[]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errC != nil || errI != nil {
			t.Errorf("%s: errors compiled=%v interp=%v", c.name, errC, errI)
			continue
		}
		if !compiled {
			t.Errorf("%s: did not compile — the declaration is meant to lower this shape", c.name)
		}
		if got := fmt.Sprint(gotC); got != c.want || got != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled %v, interp %v, want %s", c.name, gotC, gotI, c.want)
		}
	}
}
