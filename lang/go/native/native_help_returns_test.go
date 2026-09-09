package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// native_help_returns_test.go pins two things `describe` promises about a
// word it renders from live registry data: WHAT IT RETURNS, and what the
// synthetic example under it may say when the engine refuses to run it.
//
// Both were walked incidentally until help-example generation became lazy.
// The register-time hook used to call BuildFuncInfo — and evaluate a real
// example — for every word registered after MarkReady, so boru:math-util's
// own sub-registry (newModuleRegistry registers `abs`, `sign`, `ceil`, … as
// plain Go natives) dragged the return-type table through on the way past.
// Nothing walks it now unless someone asks the question it answers, so the
// questions are asked here.

// unaryMathReg registers name as a Go native declaring the Number facade
// over concrete argument slots — the shape boru:math-util's sub-registry
// holds (makeTypedFnDef declares Number; the inner natives carry the
// per-leaf Integer/Float overloads), and the shape a host registering such
// a word through the public API gets. The handler is never called: describe
// reads signatures, it does not run the word.
func unaryMathReg(t *testing.T, name string, args ...*Type) *Registry {
	t.Helper()
	r := seam5Reg(t)
	r.RegisterNativeFunc(NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args: args,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(0)}, nil
			}),
			Returns:    []*Type{TNumber},
			BarrierPos: -1,
		}},
	})
	return r
}

// TestUnaryMathReturnsPerWord pins one row per arm of the unary-math return
// rule. The declared slot type is Number for every row, so every answer that
// is not "Number" is the builtin table speaking — which is the whole point of
// consulting it: a native's declared facade is the weaker statement, and
// `describe abs 2.5` promising Number where the word yields a Float is the
// regression this guards.
//
// The three arms differ in kind, not just in value, which is why each gets
// its own rows: abs/negate echo the ARGUMENT type (so the same word answers
// differently per overload), sign and the rounding family answer Integer
// whatever they were handed, and everything else on the list — the roots,
// exponentials and trig — answers Float. Rows that pair a Float argument
// against an Integer answer (and an Integer argument against a Float one)
// are the discriminating ones: an arm that quietly collapsed into the
// default, or a default that started echoing its argument, still passes
// every same-type row.
func TestUnaryMathReturnsPerWord(t *testing.T) {
	cases := []struct {
		word string
		args []*Type
		want string
		why  string
	}{
		// abs/negate: the argument's own type, per overload.
		{"abs", []*Type{TInteger}, "Integer", "abs of an Integer stays an Integer"},
		{"abs", []*Type{TFloat}, "Float", "abs of a Float stays a Float — not the Integer its sibling overload gives"},
		{"negate", []*Type{TInteger}, "Integer", "negate preserves its argument's type"},
		{"negate", []*Type{TFloat}, "Float", "negate preserves its argument's type"},

		// sign and the rounding family: Integer, whatever came in.
		{"sign", []*Type{TInteger}, "Scalar/Number/Integer", "sign yields -1/0/1"},
		{"sign", []*Type{TFloat}, "Scalar/Number/Integer", "sign of a Float is still one of -1/0/1, NOT a Float"},
		{"ceil", []*Type{TFloat}, "Scalar/Number/Integer", "ceil lands on a whole number"},
		{"floor", []*Type{TFloat}, "Scalar/Number/Integer", "floor lands on a whole number"},
		{"round", []*Type{TFloat}, "Scalar/Number/Integer", "round lands on a whole number"},
		{"trunc", []*Type{TFloat}, "Scalar/Number/Integer", "trunc lands on a whole number"},

		// The default arm: everything else on the unary-math list.
		{"sqrt", []*Type{TInteger}, "Scalar/Number/Float", "sqrt of an Integer is a Float — the default arm does NOT echo its argument"},
		{"log", []*Type{TFloat}, "Scalar/Number/Float", "the logarithms are Float-valued"},
		{"sin", []*Type{TInteger}, "Scalar/Number/Float", "trig is Float-valued even over an Integer angle"},
		{"atan", []*Type{TFloat}, "Scalar/Number/Float", "the inverse trig words are on the list too"},

		// Negatives: what the table must NOT answer for.
		{"root", []*Type{TFloat}, "Number", "the list is closed — a host word that merely looks like unary math keeps its own declared return"},
		{"abs", []*Type{TFloat, TFloat}, "Number", "the rule is one-argument only; a two-argument abs is not the abs the table knows"},
	}

	for _, tc := range cases {
		shape := tc.word
		for _, a := range tc.args {
			shape += "/" + a.String()
		}
		t.Run(shape, func(t *testing.T) {
			r := unaryMathReg(t, tc.word, tc.args...)
			info := BuildFuncInfo(r, tc.word)
			if info == nil || len(info.Sigs) != 1 {
				t.Fatalf("BuildFuncInfo(%s) = %+v, want one signature", tc.word, info)
			}
			got := info.Sigs[0].Returns
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("describe %s reports returns %v, want [%s] — %s", shape, got, tc.want, tc.why)
			}
		})
	}
}

// describedLine returns the one line of a `describe` render that starts with
// prefix, with runs of spaces collapsed. The renderer pads its columns to the
// widest entry, so an assertion on the raw line would be pinning the padding
// rather than the sentence.
func describedLine(t *testing.T, out, prefix string) string {
	t.Helper()
	for _, ln := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(ln); strings.HasPrefix(s, prefix) {
			return strings.Join(strings.Fields(s), " ")
		}
	}
	t.Fatalf("no line starting %q in:\n%s", prefix, out)
	return ""
}

// describeRegisteredWord renders one word from a registry that holds it
// as a native.
func describeRegisteredWord(t *testing.T, word string, args ...*Type) string {
	t.Helper()
	var out strings.Builder
	DescribeName(unaryMathReg(t, word, args...), &out, word)
	return out.String()
}

// TestDescribeRendersInferredUnaryMathReturn is the same contract at the
// surface a reader actually meets: the signature line of `describe`. It is
// here because BuildFuncInfo's strings are an internal shape — this is the
// sentence a user is told, and the pair below is the one they would notice
// was wrong.
func TestDescribeRendersInferredUnaryMathReturn(t *testing.T) {
	if got := describedLine(t, describeRegisteredWord(t, "abs", TFloat), "[ "); got != "[ [Float] Float ]" {
		t.Errorf("describe abs over a Float renders %q, want a Float return", got)
	}

	// The pair: same argument type, different answer. If the sign arm were
	// lost to the default, this line would read `[ [Float] Float ]` too and
	// the two words would become indistinguishable in the docs.
	if got := describedLine(t, describeRegisteredWord(t, "sign", TFloat), "[ "); got != "[ [Float] Integer ]" {
		t.Errorf("describe sign over a Float renders %q, want an Integer return", got)
	}

	// Negative: an off-list word is rendered from its own declaration, so
	// the table cannot invent a narrower promise than the word made.
	if got := describedLine(t, describeRegisteredWord(t, "root", TFloat), "[ "); got != "[ [Float] Number ]" {
		t.Errorf("describe root renders %q, want its declared Number return", got)
	}
}

// TestSigReturnNamesPrefersReturnPattern pins the half of SigReturnNames
// that reads the declared RETURN PATTERN rather than the *Type beside it.
// A literal return type admits exactly itself (design/FN-OUTPUT-SIG.0.md),
// so reporting the type alone — Integer for a word that only ever yields 22
// — names a contract wider than the one the checker enforces.
func TestSigReturnNamesPrefersReturnPattern(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def only22 fn x:String 22 [22]`); err != nil {
		t.Fatalf("def only22: %v", err)
	}
	info := BuildFuncInfo(r, "only22")
	if info == nil || len(info.Sigs) != 1 {
		t.Fatalf("BuildFuncInfo(only22) = %+v, want one signature", info)
	}
	if got := info.Sigs[0].Returns; len(got) != 1 || got[0] != "22" {
		t.Errorf("returns = %v, want [22] — the literal pattern is the contract, not its type", got)
	}

	// Negative: where the declaration names a plain type there IS no
	// pattern to prefer, and the type must still be reported. A pattern
	// branch that fired unconditionally would render something else here.
	if _, err := seam5Run(r, `def anyint fn x:String Integer [22]`); err != nil {
		t.Fatalf("def anyint: %v", err)
	}
	info = BuildFuncInfo(r, "anyint")
	if got := info.Sigs[0].Returns; len(got) != 1 || got[0] != "Integer" {
		t.Errorf("returns = %v, want [Integer]", got)
	}
}

// TestDefinedWordKeepsItsOwnReturns is the negative that guards the
// builtin table's REACH. The table is keyed on the name alone, so without
// the has-a-body test it answers for a user fn that merely shares a
// builtin's spelling — `def sign fn x:Float Float [x]` reported
// Scalar/Number/Integer, a contract that function does not have.
func TestDefinedWordKeepsItsOwnReturns(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def sign fn x:Float Float [x]`); err != nil {
		t.Fatalf("def sign: %v", err)
	}
	info := BuildFuncInfo(r, "sign")
	if info == nil || len(info.Sigs) != 1 {
		t.Fatalf("BuildFuncInfo(sign) = %+v, want one signature", info)
	}
	if got := info.Sigs[0].Returns; len(got) != 1 || got[0] != "Float" {
		t.Errorf("returns = %v, want [Float] — a boru-defined word declares its own contract", got)
	}
}

// TestSyntheticExampleDocumentsRefusal pins what happens when the example
// `describe` synthesises does not survive the engine: the evaluation's error
// is HANDLED where it is raised, never propagated out of the render. The
// word is still described; only the example line changes.
//
// The two outcomes are different on purpose. A CODED refusal is documentable
// — it is a true statement about the word, and the reader should see it. An
// undefined word is not: it is a fact about the registry doing the
// evaluating, not about the word (this is why a word living in a module
// nobody imported keeps its `;# ...` placeholder), so nothing is recorded
// and the placeholder survives.
func TestSyntheticExampleDocumentsRefusal(t *testing.T) {
	r := seam5Reg(t)
	EnableDynamicHelp(r)
	r.MarkReady()
	r.RegisterNativeFunc(NativeFunc{
		Name: "zzhelp-refuse",
		Signatures: []Signature{{
			Args: []*Type{TBoolean},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, &BoruError{Code: "type_error", Detail: "zzhelp-refuse: not on a Boolean"}
			}),
			Returns:    []*Type{TBoolean},
			BarrierPos: -1,
		}},
	})

	var out strings.Builder
	DescribeName(r, &out, "zzhelp-refuse")
	if got := describedLine(t, out.String(), "zzhelp-refuse true"); got != "zzhelp-refuse true ;# error [boru/type_error]" {
		t.Errorf("the example line reads %q, want the refusal documented as itself", got)
	}
	if !strings.Contains(out.String(), "zzhelp-refuse x") {
		t.Errorf("the word must still be described around the refused example:\n%s", out.String())
	}

	// Negative: the evaluating registry cannot resolve the word at all.
	// EncodeExampleResult declines to record that, so the render keeps the
	// placeholder — asserting `error [boru/undefined_word]` here would ship
	// documentation claiming the word does not exist.
	unknown := help.FuncInfo{
		Name:        "zzhelp-unimported",
		ForwardArgs: true,
		Sigs:        []help.SigInfo{{Args: []string{"Boolean"}, BarrierPos: -1}},
	}
	r.NoteHelpWord(unknown.Name)
	rendered := FormatWordHelp(r, unknown)
	if got := describedLine(t, rendered, "zzhelp-unimported true"); got != "zzhelp-unimported true ;# ..." {
		t.Errorf("the example line reads %q, want the placeholder kept", got)
	}
	if strings.Contains(rendered, "error [boru/") {
		t.Errorf("an unresolvable word must not be documented as a refusal:\n%s", rendered)
	}
}
