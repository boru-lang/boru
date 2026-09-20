package lang

import "testing"

// COMPILE FAILURE-CLOSURE §9.2c (landed 2026-07-17) — an interpolated XML literal
// with runtime-computed ${...} holes compiles to OpInterpXml: the hole
// expressions record their own dispatch events in traversal order
// (attributes first, then children left-to-right, depth-first) and the op
// pops their results and rebuilds the element via rebuildXmlFromTmpl —
// byte-identical to the interpreter's buildXmlFromTmpl over the same
// values. The string sibling landed earlier as OpInterp; this is its tree
// twin. A 0-or-many-valued hole keeps the compile failure (one operand-stack
// slot per hole), pinned below.
func TestXmlInterpComputedCompiles(t *testing.T) {
	// Child hole — the reported §9.2 fixture family.
	mustCompileWithParity(t, `def n 7 <p>${n add 1}</p>`, "[<p>8</p>]")
	// Inside a fn body (the hole is a param read).
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p>${x}</p>]] f 5`, "[<p>5</p>]")
	// Attribute hole.
	mustCompileWithParity(t, `def n 5 def f fn [[x:Integer] [Xml] [<p a=${x}></p>]] f n`, `[<p a="5"/>]`)
	// NESTED child template — the recursion consumes from the same hole run.
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p><q>${x}</q></p>]] f 3`, "[<p><q>3</q></p>]")
	// Literal text around the hole (the coalescing arms).
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p>a${x}b</p>]] f 4`, "[<p>a4b</p>]")
	// Two holes in one element.
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p>${x}-${x add 1}</p>]] f 1`, "[<p>1-2</p>]")
	// A LIST-valued hole splices one level (the addChild rule) — the list
	// arrives as a param (an inline computed list literal inside the hole
	// declines operand resolution and keeps the compile failure, fenced below).
	mustCompileWithParity(t, `def f fn [[xs:List] [Xml] [<p>${xs}</p>]] f [7 8]`, "[<p>78</p>]")
	// An XML-valued hole becomes a child ELEMENT (the addOne IsXmlValue arm).
	mustCompileWithParity(t, `def f fn [[x:Xml] [Xml] [<p>${x}</p>]] f <q></q>`, "[<p><q/></p>]")
	// A literal segment beside the attr hole; an empty-string hole value.
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p a="v${x}"></p>]] f 9`, `[<p a="v9"/>]`)
	mustCompileWithParity(t, `def f fn [[x:String] [Xml] [<p>${x}</p>]] f ""`, "[<p/>]")
	// Attr + child holes in one element (traversal order: attrs first).
	mustCompileWithParity(t, `def f fn [[x:Integer] [Xml] [<p a=${x}>${x add 1}</p>]] f 2`, `[<p a="2">3</p>]`)

	// Decline fences. A 0-valued hole beside a dynamic one cannot map to
	// one stack slot per hole — the compile failure (and the compile failure)
	// stand. (A PURELY 0-valued hole with no dynamic sibling const-folds
	// with parity — the empty element — so it is not a fence.)
	mustFailToCompileWithParity(t,
		`def f fn [[x:Integer] [] []] def g fn [[x:Integer] [Xml] [<p>${x}${f x}</p>]] g 1`,
		"interpolated XML with a runtime-computed part")
	// An inline COMPUTED LIST literal inside the hole declines operand
	// resolution — compile failure, absorbed by the interpreter and owed a fix.
	mustFailToCompileWithParity(t,
		`def f fn [[x:Integer] [Xml] [<p>${[x (x add 1)]}</p>]] f 7`,
		"interpolated XML with a runtime-computed part")
	// A FUNCTION-valued hole GRADUATED (2026-07-17): the compiled closure
	// carries its source fn's interpreter rendering (CompiledFn.Render →
	// ClosurePayload.Render via OpPushClosure), so stringification is
	// byte-identical (`fn (Integer)`, not the former VM-internal
	// `Function({…})` the PR #279 review flagged) and both interpolation
	// paths compile.
	mustCompileWithParity(t,
		`def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]] <p>${mk 1}</p>`,
		"[<p>fn (Integer)</p>]")
	mustCompileWithParity(t,
		"def mk fn [[a:Integer] [Function] [([b:Integer] => [a add b])]] `${mk 1}`",
		"[fn (Integer)]")
}
