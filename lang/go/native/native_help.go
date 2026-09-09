package native

import (
	"io"
	"strings"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native/help"
)

// EnableDynamicHelp sets up the OnRegisterHook so that functions registered
// after MarkReady() can get their help examples computed from the live
// engine. Call this after initial setup and ParseFunc are ready.
//
// The hook only RECORDS the name (core's NoteHelpWord, whose comment carries
// the measurements). Generation happens in FormatWordHelp, when someone
// actually describes the word — because this hook fires from installFnDef on
// every fn installation, including the user's own `def f fn […]` mid-check,
// and evaluating a synthesised example there put a documentation feature
// inside a user's analysis pass.
func EnableDynamicHelp(r *Registry) {
	r.OnRegisterHook = r.NoteHelpWord
}

// FormatWordHelp renders one word's help, synthesising and evaluating its
// examples FIRST if the word was registered after startup (so no build-time
// snapshot exists for it) — the on-demand half of EnableDynamicHelp.
//
// Every `describe` / hover render site goes through here rather than calling
// help.FormatDynamic directly, so there is one place where the synthetic
// evaluation can happen and it is a place the user asked for output. The
// evaluation is still fully hermetic (makeDynamicEval's seven channels): it
// now runs inside the user's RUN rather than their CHECK, so it must not
// disturb the def stack, the budget, the recording or the diagnostics of the
// program that called `describe`.
func FormatWordHelp(r *Registry, info help.FuncInfo) string {
	if r.IsHelpWord(info.Name) {
		if eval := makeDynamicEval(r); eval != nil {
			help.GenerateDynamicExamples(info, eval)
		}
	}
	return help.FormatDynamic(info)
}

// makeDynamicEval returns a function that parses and evaluates a boru
// expression, returning the formatted result. Returns nil if ParseFunc
// is not set.
func makeDynamicEval(r *Registry) func(string) (string, error) {
	if r.ParseFunc == nil {
		return nil
	}
	return func(expr string) (string, error) {
		vals, err := r.ParseFunc(expr)
		if err != nil {
			return "", err
		}
		savedOut := r.Output
		r.Output = io.Discard
		defer func() { r.Output = savedOut }()
		// The synthetic example evaluation is DOCUMENTATION, not the user's
		// program, and MUST be fully hermetic — it fires from OnRegisterHook on
		// EVERY fn registration, INCLUDING the program's own `def f fn […]` DURING
		// compilation, so any trace it leaves contaminates that very program's
		// compile. Seven leak channels are closed:
		//   1. EmitState (recording + interned consts + RememberOriginal) — swap in
		//      a FRESH throwaway EmitState (IsolateEmit), not just Suspend: Suspend
		//      keeps the SAME EmitState, so the example's consts (e.g. a generated
		//      `['a' 'b']` sample list for a list-typed param) leaked into the
		//      program's pool and a later operand compiled against the stale value
		//      (a valid `def zs [1 2] end f2 zs` saw f2's arg as `['a' 'b']`).
		//   2. Check diagnostics — snapshot + truncate (the decision.boru false
		//      positives from synthetic stand-in args).
		//   3. Def-stack bindings — snapshot + restore (an example that `def`s).
		//   4. Check-mode step budget — snapshot + restore (IsolateBudget). The
		//      synthetic run shares the registry's StepCount; a doc example that
		//      runs many steps — most acutely a RECURSIVE macro, which loops to
		//      the step ceiling — would otherwise burn the real program's shared
		//      budget and short-circuit its later statements (e.g. a subsequent
		//      `macroexpand (recursive-macro …)` never gets reached).
		//   5. Filesystem — swap the active FileOps for a FRESH seeded
		//      in-memory implementation for this one evaluation (and restore
		//      after). Without this, registering the io words (read/write/
		//      folder — e.g. a test helper installing them bare, or a host
		//      registering fs words) evaluated their generated examples
		//      against the registry's REAL FileOps: `write 'd' 'e'` created
		//      a file named d in the process's cwd. A fresh seed per expr
		//      also keeps results order-independent; the seed matches
		//      genhelp and the help-example validation test, so dynamic and
		//      precomputed results agree.
		//   6. Input — an example of a reading word (stdin) must consume an
		//      empty hermetic reader, never the process's stdin.
		//   7. The fn-body ANALYSIS MEMO family (IsolateFnAnalysis) — the
		//      example's own AnalyseFnBody of the user's real body is
		//      memoised under a key that renders arg TYPE NAMES only, so the
		//      program's own call site took the memo HIT and was handed the
		//      EXAMPLE's residual instead of analysing its concrete args. The
		//      dispatch that should have failed was never attempted, and
		//      channel 2 ate the evidence: `def app fn [[nd:Any m:Map] [Any]
		//      [nd (m get "inc") apply]]  def rules {inc: 42}  app 5 rules`
		//      reported NOTHING on the FIRST check in a process (the example
		//      is evaluated once per process — help's own result memo is
		//      package-level) and its two real errors on every later one.
		// Real construction-time body checking is a first-class pass
		// (checkFnBodyAtConstruction), so suppressing the eval's diagnostics is sound.
		defer r.Check.IsolateEmit()()
		defer r.Check.IsolateBudget()()
		defer r.Check.IsolateFnAnalysis()()
		diagBase := len(r.Check.Diagnostics)
		defer r.Check.TruncateDiagnostics(diagBase)
		// A CONTENT-preserving snapshot (SnapshotEntries), not the depth-based
		// Defs.Snapshot: the depth restore can only truncate, so an example
		// that ran `undef` on a pre-existing name — or an overlapping
		// redefinition — escaped the region while the ledger bracket below
		// suppressed its note (Codex P2, PR #418). The entries restore is IN
		// PLACE and gen-guarded — a table swap (RestoreBindings) breaks every
		// reference the enclosing pass holds across this hook, which fires
		// mid-install.
		defsSnap := r.Defs.SnapshotEntries()
		defer r.Defs.RestoreEntriesSnapshot(defsSnap)
		// The restore above tears down every def the example run installs, so
		// the bind ledger must not record them (the hook fires mid-pass from
		// the program's own fn registrations — an example that expands a macro
		// installed the expansion's `def tmp$g…` as a phantom ledger entry,
		// the live-depth oracle's gensym-temp class).
		defer r.Check.SuppressBindLedger()()

		prevOps, hadOps, _ := r.Capabilities.Get(CapFileOps)
		prevMem, hadMem, _ := r.Capabilities.Get(CapMemFileOps)
		mem := capabilities.NewMem()
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			mem.Files[name] = []byte("file-" + name + "-content")
		}
		_ = r.Capabilities.Set(CapFileOps, capabilities.FileOps(mem))
		_ = r.Capabilities.Set(CapMemFileOps, capabilities.FileOps(mem))
		restoreCap := func(key string, prev any, had bool) {
			if had {
				_ = r.Capabilities.Set(key, prev)
			} else {
				_, _ = r.Capabilities.Delete(key)
			}
		}
		defer restoreCap(CapFileOps, prevOps, hadOps)
		defer restoreCap(CapMemFileOps, prevMem, hadMem)
		savedIn := r.Input
		r.Input = strings.NewReader("")
		defer func() { r.Input = savedIn }()

		eng := NewTop(r)
		result, err := eng.Run(vals)
		if err != nil {
			return "", err
		}
		var parts []string
		for _, v := range result {
			parts = append(parts, v.String())
		}
		return strings.Join(parts, " "), nil
	}
}

// BuildQualifiedFuncInfo builds help.FuncInfo for a dotted module-export
// name (e.g. "ArrayUtil.indices") by resolving the namespace binding that
// `import` installed in the def stack and reading the export's FnDefInfo
// provenance (Module/Doc) and signatures. Returns nil when name is not a
// dotted name, the namespace is unbound (module not imported), or the word
// is not an exported function. This is the runtime `describe` path; it needs
// no access to the modules package because the namespace value is already a
// facet-carrying namespace Map bound in r.Defs.
func BuildQualifiedFuncInfo(r *Registry, name string) *help.FuncInfo {
	dot := strings.IndexByte(name, '.')
	if dot <= 0 || dot >= len(name)-1 {
		return nil
	}
	ns, word := name[:dot], name[dot+1:]
	binding, ok := r.Defs.Top(ns)
	if !ok {
		return nil
	}
	exp, ok := moduleExportGet(binding, word)
	if !ok {
		return nil
	}
	fnDef, ok := exp.Data.(FnDefInfo)
	if !ok {
		return nil
	}
	return FnDefFuncInfo(name, &fnDef)
}

// FnDefFromValue unwraps an FnDefInfo carried by a function Value (e.g. a
// module export's wrapper), for callers outside the engine package that hold
// the Value but not the payload. ok=false when v is not an FnDef.
func FnDefFromValue(v Value) (*FnDefInfo, bool) {
	fn, ok := v.Data.(FnDefInfo)
	if !ok {
		return nil, false
	}
	return &fn, true
}

// FnDefFuncInfo builds a help.FuncInfo from a function's FnDefInfo, under the
// display label (e.g. "ArrayUtil.indices"). It carries module provenance
// (Module/Doc) when present and prefers the signatures' authored Returns —
// module wrappers declare them explicitly — falling back to inferReturns for
// core words whose Returns are derived. Shared by the qualified-name path
// above and any caller that already holds an FnDefInfo (e.g. the CLI).
// sigKeywordSlots returns the KEYWORD-slot render overrides for a
// signature: a position with a /q flag AND a concrete Atom pattern
// admits exactly one literal word, and displays as `<word>/q` (e.g.
// def's `def name fn [...]` form shows position 1 as `fn/q`). Nil when
// the signature has no keyword slots. Shared by the two SigInfo
// builders so describe/help render both def-form paths identically.
func sigKeywordSlots(sig Signature) map[int]string {
	var kw map[int]string
	for i, pat := range sig.Patterns {
		// A keyword slot is a /q position with a CONCRETE Atom pattern.
		// IsConcrete filters bare atom nodes / type literals, so the
		// AsAtom below always yields the name (err path is a plain skip,
		// no separate branch).
		if !sig.QuoteArgs[i] || !IsConcrete(pat) || !pat.Parent.ConformsTo(TAtom) {
			continue
		}
		if name, err := AsAtom(pat); err == nil {
			if kw == nil {
				kw = map[int]string{}
			}
			kw[i] = name + "/q"
		}
	}
	return kw
}

func FnDefFuncInfo(display string, fn *FnDefInfo) *help.FuncInfo {
	info := &help.FuncInfo{
		Name:        display,
		ForwardArgs: fnDefForwardEligible(fn),
		Module:      fn.Module,
		Doc:         fn.Doc,
		Examples:    fn.Examples,
	}
	// A module export has no static help Entry; a bare core word might.
	if fn.Module == "" {
		info.Entry = help.Lookup(fn.Name)
	}
	for i := range fn.Signatures {
		sig := fn.Signatures[i]
		if sig.Fallback {
			continue
		}
		si := help.SigInfo{BarrierPos: sig.BarrierPos, Keywords: sigKeywordSlots(sig)}
		for j, t := range sig.ArgTypes() {
			si.Args = append(si.Args, sigArgDisplay(&sig, j, t))
		}
		if len(sig.Returns) > 0 {
			for _, t := range sig.Returns {
				si.Returns = append(si.Returns, t.String())
			}
		} else {
			si.Returns = inferReturns(fn.Name, sig)
		}
		info.Sigs = append(info.Sigs, si)
	}
	return info
}

// sigArgDisplay renders the declared type at sig position i for the
// describe listing. A negation Pattern on the slot (e.g. fn's triple
// form, whose input slot is `Any` constrained by `tnot List`) IS the
// dispatch-visible contract, so it displays as `(tnot List)` instead of
// the bare slot type. All other patterns keep the historical bare-type
// rendering.
func sigArgDisplay(sig *Signature, i int, t *Type) string {
	pat, ok := sigDisplayPattern(sig, i)
	if !ok || !IsNegation(pat) {
		return t.String()
	}
	ni, _ := AsNegation(pat) // payload presence guaranteed by IsNegation
	inner := ni.Inner
	if IsBareTypeNode(inner) {
		return "(tnot " + (&inner).Leaf() + ")"
	}
	return "(tnot " + inner.String() + ")"
}

// sigDisplayPattern returns the Pattern declared at sig position i,
// consulting Params first (the normalized store) and falling back to
// the positional Patterns mirror for a sig that has not been
// normalized (mirrors eng's unexported sigPattern).
func sigDisplayPattern(sig *Signature, i int) (Value, bool) {
	if i < len(sig.Params) && sig.Params[i].Pattern != nil {
		return *sig.Params[i].Pattern, true
	}
	if len(sig.Params) == 0 && sig.Patterns != nil {
		p, ok := sig.Patterns[i]
		return p, ok
	}
	return Value{}, false
}

// fnDefForwardEligible reports whether any signature can collect forward
// arguments. Unlike HasForwardSigs (which only counts a positive resolved
// BarrierPos), this also treats the -1 "all-forward" sentinel as forward —
// module wrappers carry BarrierPos: -1 and are called in forward form, so
// describe should show forward precedence for them. Only a sig pinned to 0
// (pure stack) is non-forward.
func fnDefForwardEligible(fn *FnDefInfo) bool {
	for i := range fn.Signatures {
		if fn.Signatures[i].BarrierPos != 0 {
			return true
		}
	}
	return false
}

// BuildFuncInfo extracts dynamic signature data from the registry for a word.
func BuildFuncInfo(r *Registry, name string) *help.FuncInfo {
	fn := r.Lookup(name)
	if fn == nil {
		// Check if it's a simple def (not a function)
		if r.Defs.Has(name) {
			return &help.FuncInfo{
				Name:  name,
				Entry: help.Lookup(name),
			}
		}
		return nil
	}

	info := &help.FuncInfo{
		Name:        fn.Name,
		ForwardArgs: fn.HasForwardSigs(),
		Entry:       help.Lookup(name),
	}

	for _, sig := range fn.Signatures {
		if sig.Fallback {
			continue
		}
		si := help.SigInfo{BarrierPos: sig.BarrierPos, Keywords: sigKeywordSlots(sig)}
		for j, t := range sig.ArgTypes() {
			si.Args = append(si.Args, sigArgDisplay(&sig, j, t))
		}
		si.Returns = SigReturnNames(fn.Name, sig)
		info.Sigs = append(info.Sigs, si)
	}

	return info
}

// SigReturnNames names a signature's return types for the introspection
// surfaces. `describe` and `inspect` both call it, so the two cannot
// disagree about what a function returns.
//
// A DECLARED return wins. Running the handler to observe its result is
// not feasible, so for a native the hand-tuned per-word table comes
// first: inferExact and the category rules know more about a builtin
// than its declared slot type does (`add` declares Number and infers
// Integer for two Integers), and many builtins declare no Returns at
// all. But that table is keyed on the NAME alone, so it answered for a
// user fn that happens to shadow a builtin — `def upper fn x:Integer
// [Integer] [x]` reported `Scalar/String`, a contract the function does
// not have. sigIsDefined draws the line: a signature with a body is
// boru-source, and its declaration is not a guess.
//
// Where a return carries a PATTERN, the pattern is the contract and the
// *Type is the weaker half. A literal return type admits exactly itself
// (design/FN-OUTPUT-SIG.0.md), so `def f fn x:String 22 [22]` returns
// `22`, not `Integer`; a declared union degrades its type to `Any` and
// keeps the whole domain in the pattern. Reporting the type alone named
// a contract wider than the one enforced.
func SigReturnNames(name string, sig Signature) []string {
	if !sigIsDefined(sig) {
		if ret := inferReturns(name, sig); len(ret) > 0 {
			return ret
		}
	}
	out := make([]string, 0, len(sig.Returns))
	for i, t := range sig.Returns {
		if i < len(sig.ReturnPatterns) && sig.ReturnPatterns[i] != nil {
			out = append(out, sig.ReturnPatterns[i].String())
			continue
		}
		out = append(out, t.String())
	}
	return out
}

// sigIsDefined reports whether this signature belongs to a boru-defined
// word rather than a Go-registered native — the test for whether the
// name-keyed builtin table may speak for it.
//
// A body is what a native never has. The synthetic 0-arg fallback has
// no body either, but `Fallback` is only ever set on the catch-all
// injected for a boru word (Registry.fnFallbackSig), so it counts as
// defined too: it declares no returns, and rendering none is right,
// where letting the table answer would have it report the return type
// of the builtin the word shadows.
func sigIsDefined(sig Signature) bool {
	return sig.Fallback || len(sig.Body()) > 0
}

// inferReturns attempts to determine return types for a signature.
// Uses known patterns for builtin words.
func inferReturns(name string, sig Signature) []string {
	nArgs := len(sig.ArgTypes())

	// Exact overrides first (word → return types per sig shape).
	if ret := inferExact(name, sig); ret != nil {
		return ret
	}

	// Category-based inference.
	switch {
	case nArgs == 2 && isArithWord(name):
		return inferArithReturns(name, sig)
	case nArgs == 1 && isUnaryMathWord(name):
		return inferUnaryMathReturns(name, sig)
	case nArgs == 2 && isCompareWord(name):
		return []string{"Scalar/Boolean"}
	case nArgs == 2 && isBoolWord(name):
		return []string{"Scalar/Boolean"}
	case nArgs == 1 && name == "not":
		return []string{"Scalar/Boolean"}
	}
	return nil
}

// inferExact handles words with specific, known return types.
func inferExact(name string, sig Signature) []string {
	nArgs := len(sig.ArgTypes())
	switch name {
	// String ops
	case "upper", "lower":
		return []string{"Scalar/String"}
	case "concat":
		return []string{"Scalar/String"}
	case "split":
		return []string{"Node/List"}
	case "trim", "changecase", "normalize", "escape", "repeat", "pad", "replace":
		return []string{"Scalar/String"}
	case "contains":
		return []string{"Scalar/Boolean"}
	case "indexof":
		return []string{"Scalar/Number/Integer"}
	case "match":
		return []string{"Node/Map"}
	case "slice":
		if nArgs > 0 {
			last := sig.ArgTypes()[nArgs-1].String()
			if last == "Node/List" { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return []string{"Node/List"}
			}
		}
		return []string{"Scalar/String"}

	// *Type ops
	case "typeof":
		return []string{"Type"}
	case "is":
		return []string{"Scalar/Boolean"}
	case "inspect":
		return []string{"Node/Map"}
	case "convert":
		return []string{"Scalar"}
	case "base":
		return []string{"Any"}
	case "make":
		return []string{"Any"}

	// Storage
	case "set", "context-set":
		return nil // no return
	case "get", "dot", "context-get":
		return []string{"Any"}

	// Definition
	case "def", "undef":
		return nil
	case "fn":
		return []string{"Word/Function"}
	case "args":
		return []string{"Node/List"}
	case "var":
		return []string{"Any"}

	// Control flow
	case "do":
		return []string{"Any"}
	case "if":
		return []string{"Any"}
	case "for":
		return []string{"Any"}
	case "quote":
		return []string{"Any"}
	case "error":
		return []string{"Any"}

	// Accessors
	case "getr", "dotr", "!.":
		return []string{"Any"}

	// I/O
	case "print", "printstr":
		return nil
	case "read":
		return []string{"Any"}
	case "write":
		return []string{"Scalar/String"}
	case "trace":
		return []string{"Any"}
	case "stdin", "stdout", "stderr":
		return []string{"Scalar/String"}

	// Module
	case "module":
		return []string{"Ideal/Module"}
	case "import":
		return nil

	// Unify
	case "unify":
		return []string{"Scalar/String", "Scalar/Boolean"}

	// or/and short-circuit, returning the winning operand. The
	// [Boolean, Boolean] sig keeps the result narrowed to Boolean;
	// the [Any, Any] coerce sig returns the operand value as-is.
	case "or", "and":
		if nArgs == 2 && sig.ArgTypes()[0].String() == "Any" {
			return []string{"Any"}
		}
		return []string{"Scalar/Boolean"}

	// *Type union: tor builds a disjunct from any two values.
	case "tor":
		return []string{"Any"}

	// *Type conjunction: tand merges/unifies two values.
	case "tand":
		return []string{"Any"}

	// List quantifiers: any/all return the winning element value;
	// tany/tall return a folded disjunct or merged value.
	case "any", "all", "tany", "tall":
		return []string{"Any"}

	// Help / describe
	case "help", "describe":
		return nil

	// Constants
	case "math-pi", "math-e":
		return []string{"Scalar/Number/Float"}

	// Stack ops
	case "depth":
		return []string{"Scalar/Number/Integer"}
	case "stack":
		return []string{"Node/List"}
	case "dup":
		return []string{"Any", "Any"}
	case "swap":
		return []string{"Any", "Any"}
	case "drop":
		return nil
	case "over":
		return []string{"Any", "Any", "Any"}
	case "rot":
		return []string{"Any", "Any", "Any"}
	case "nip":
		return []string{"Any"}
	case "tuck":
		return []string{"Any", "Any", "Any"}
	case "dup2":
		return []string{"Any", "Any", "Any", "Any"}
	case "swap2":
		return []string{"Any", "Any", "Any", "Any"}
	case "drop2":
		return nil
	case "over2":
		return []string{"Any", "Any", "Any", "Any", "Any", "Any"}
	case "pick", "roll":
		return []string{"Any"}
	case "break", "continue":
		return nil
	}
	return nil
}

func isArithWord(name string) bool {
	switch name {
	case "add", "sub", "mul", "div", "mod", "min", "max", "pow",
		"atan2", "hypot":
		return true
	}
	return false
}

func isUnaryMathWord(name string) bool {
	switch name {
	case "abs", "negate", "sign", "ceil", "floor", "round", "trunc",
		"sqrt", "cbrt", "exp", "log", "log2", "log10",
		"sin", "cos", "tan", "asin", "acos", "atan":
		return true
	}
	return false
}

func isCompareWord(name string) bool {
	switch name {
	case "lt", "gt", "lte", "gte", "eq", "neq", "deq":
		return true
	}
	return false
}

func isBoolWord(name string) bool {
	switch name {
	case "and", "or", "xor", "nand", "nor", "iff", "xnor", "implies":
		return true
	}
	return false
}

func inferArithReturns(name string, sig Signature) []string {
	if len(sig.ArgTypes()) != 2 {
		return nil
	}
	a0 := sig.ArgTypes()[0].String()
	a1 := sig.ArgTypes()[1].String()

	if name == "add" && a0 == "Scalar" && a1 == "Scalar" {
		return []string{"Scalar/String"}
	}
	if a0 == "Scalar/Number/Integer" && a1 == "Scalar/Number/Integer" { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return []string{"Scalar/Number/Integer"}
	}
	return []string{"Scalar/Number/Float"}
}

func inferUnaryMathReturns(name string, sig Signature) []string {
	a0 := sig.ArgTypes()[0].String()
	switch name {
	case "abs", "negate":
		return []string{a0}
	case "sign":
		return []string{"Scalar/Number/Integer"}
	case "ceil", "floor", "round", "trunc":
		return []string{"Scalar/Number/Integer"}
	default:
		return []string{"Scalar/Number/Float"}
	}
}
