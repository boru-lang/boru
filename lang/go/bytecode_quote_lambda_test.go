package lang

import (
	"strings"
	"testing"
)

// bytecode_quote_lambda_test.go pins the quote-polarity screen
// (design/legacy/checker-compiler-completeness-review.0.ignore §2.2). An Atom-typed
// lambda param is a quote-capture slot the runtime never binds from a
// delivered stack value, so the interpreter leaves such a lambda as DATA in
// the HOF callback position. Until 2026-08-02 the compiled path admitted it
// as an APPLYING callback over a COMPUTED collection (`each [[k:Atom] =>
// […]] (keys m)` → compiled signature_error, interpreted `[fn (Atom)]`).
// The screen (lambdaHookCompatible's quote arm + the closure-unit
// quoteParamCarrierBind guard) declines the compile; the shapes refuse with
// faithful interpreter fallback — or compile with byte-identical outcomes.
func TestQuoteLambdaCallbackParity(t *testing.T) {
	// Legacy refusal+fallback-parity contract, like TestApplyOverParamFnCompiles.
	t.Setenv("BORU_COMPILE_FALLBACK", "1")

	fnValueM2Refusal(t, "each: Atom-lambda over computed keys stays data",
		`def doc {meta: 7} each [[k:Atom] => [doc get k]] (keys doc)`,
		"code-body word each")
	fnValueM2Refusal(t, "fold: Atom-lambda over computed keys stays data",
		`fold [[k:Atom acc:Integer] => [acc add 1]] (keys {a:1 b:2}) 0`,
		"code-body word fold")

	// filter's convention delivers a {key,value} pair, so the Atom lambda
	// never matches in either engine: the body COMPILES and both engines
	// raise the identical filter_error — error parity, no refusal.
	{
		src := `filter [[k:Atom] => [true]] (keys {a:1})`
		a, _ := New()
		_, iErr := a.RunInterp(src)
		b, _ := New()
		_, compiled, cErr := b.RunCompiled(src)
		if !compiled {
			t.Errorf("%q: the filter sibling must still compile", src)
		}
		if iErr == nil || cErr == nil || codeOf(iErr) != codeOf(cErr) ||
			!strings.Contains(cErr.Error(), "must produce a Boolean") {
			t.Errorf("%q: raise parity — compiled=[%s]%v interp=[%s]%v",
				src, codeOf(cErr), cErr, codeOf(iErr), iErr)
		}
	}

	// CONTROLS — value-typed lambdas keep compiling natively.
	fnValueM2Native(t, "Integer-lambda each with a capture",
		`def start 10 each [([k:Integer] => [start add k]) apply] [1 2 3]`,
		"[[11 12 13]]")
	fnValueM2Native(t, "Any-lambda over computed keys applies in both engines",
		`each [([k:Any] => [k]) apply] (keys {meta: 7})`,
		"[['meta']]")
}
