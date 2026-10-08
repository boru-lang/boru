package lang

// The last real programs (2026-09-26): the three mechanisms that took the
// real-program gate from 58 to 62 of 62 — each pinned here by a minimal
// both-lane parity row (the interpreter is the oracle) and by the negatives
// that must keep their old answer.

import (
	"fmt"
	"strings"
	"testing"
)

// checkDiagCodes runs the plain check pass and returns "code: detail" for
// every error-severity diagnostic.
func checkDiagCodes(t *testing.T, src string) []string {
	t.Helper()
	res, err := mustNew(t).Check(src)
	if err != nil {
		t.Fatalf("%q: Check: %v", src, err)
	}
	var out []string
	for _, d := range res.Diagnostics {
		if d.Severity == "error" {
			out = append(out, d.Code+": "+d.Detail)
		}
	}
	return out
}

// TestNarrowingSkipsPolymorphicDisjunctSlot — mini-redis's HDEL handler. A
// def-bound DYNAMIC DISJUNCT (`def cur (hashes get k)` over an Any store, the
// union of get's reachable returns) consumed by `(cur get f)` was narrowed to
// the FIRST overload's slot, Module: slotIsPolymorphic measured each sibling
// against the carrier's Parent — the bare Disjunct node, disjoint from every
// container — so no sibling read as reachable. The later `cur set (f) None`
// then declined no_signature and took the `def h2` it fed with it: a false
// `undefined word: h2` on the read `(hashes set (k) h2)`. The reachability
// test now intersects the value's bound, the operand the narrowing itself
// intersects.
func TestNarrowingSkipsPolymorphicDisjunctSlot(t *testing.T) {
	hdel := `def g fn [[k:Any f:Any state:Any][Integer][def hashes state.hashes def cur (hashes get k) if (cur eq None) [0] [def had (if ((cur get f) eq None) [0] [1]) def h2 (cur set (f) None) (hashes set (k) h2) drop had]]] end `
	for _, src := range []string{
		hdel + `def st {hashes:(flex {a:(flex {b:1})})} end g "a" "b" st`,
		hdel + `def st {hashes:(flex {a:(flex {c:1})})} end g "a" "b" st`,
		hdel + `def st {hashes:(flex {})} end g "a" "b" st`,
		hdel + `def st {hashes:(flex {a:(flex {b:1})})} end g "a" "b" st end st.hashes`,
	} {
		if diags := checkDiagCodes(t, src); len(diags) != 0 {
			t.Errorf("%q: plain check reports %v; want none", src, diags)
		}
		requireEngineParity(t, src, true)
	}
	// Negative: a genuinely unbound read in the same position is still the
	// interpreter's undefined_word, on the check and on both lanes.
	bad := `def g fn [[k:Any f:Any state:Any][Integer][def hashes state.hashes def cur (hashes get k) if (cur eq None) [0] [def had (if ((cur get f) eq None) [0] [1]) def h2 (cur set (f) None) (hashes set (k) h3) drop had]]] end g "a" "b" {hashes:(flex {a:(flex {b:1})})}`
	if diags := checkDiagCodes(t, bad); len(diags) == 0 || !strings.Contains(strings.Join(diags, "\n"), "h3") {
		t.Errorf("an unbound h3 must still be reported: %v", diags)
	}
	_, errI := mustNew(t).RunInterp(bad)
	if codeOf(errI) != "undefined_word" {
		t.Errorf("interpreter: %v; want undefined_word", errI)
	}
}

// TestEachOverDynamicCollectionIsNotAStrictMap — kg/ingest.boru's
// ingest-entity. `each [body] xs` over a dynamic xs reaches both the List form
// (a ReturnsFn) and the Map form (Returns Map); matching committed to the Map
// form, and because dynamicReachableReturns abandons a union containing a
// ReturnsFn overload for a partially-known call, applyGradualContagion kept
// the committed return STRICT: a Map the runtime List contradicts. A module
// fn over the result (`KgEnt.distinct-sorted`, xs:List) then did not match,
// stayed as data beside it, and the map literal holding both had no compiled
// home ("fn call operand of unknown provenance"); the checker also reported
// a false `expected List, got Map`. The committed return widens to
// dynamic(Any) there.
func TestEachOverDynamicCollectionIsNotAStrictMap(t *testing.T) {
	mod := `import module [def ds fn xs:List List [sort xs] export "M" {ds:ds/v}] end `
	for _, src := range []string{
		mod + `def f fn raw:Map Map [ {a:(M.ds (each [var [[a] a]] raw.aliases))} ] end f {aliases:["y" "x"]}`,
		mod + `def f fn raw:Map List [ (M.ds (each [a] raw.aliases)) ] end f {aliases:["y" "x"]}`,
		`def f fn raw:Map List [ (each [a] raw.aliases) ] end f {aliases:["x"]}`,
		`def f fn raw:Any List [ (each [var [[a] a]] raw) ] end f ["x"]`,
	} {
		if diags := checkDiagCodes(t, src); len(diags) != 0 {
			t.Errorf("%q: plain check reports %v; want none", src, diags)
		}
		requireEngineParity(t, src, true)
	}
	// Negatives: a Map at run time is still each's Map form — the declared
	// List return raises identically on both lanes — and a CONCRETE Map
	// collection still types as a Map (the checker keeps that precision).
	requireEngineParity(t, `def f fn raw:Map List [ (each [a] raw.aliases) ] end f {aliases:{k:1}}`, true)
	if diags := checkDiagCodes(t, `def f fn [[] [List] [(each [a] {k:1})]] end f`); len(diags) == 0 {
		t.Error("each over a concrete Map declared List must still be reported")
	}
}

// TestUncalledDispatchDynamicOperandRematches — a module native's fn value
// over a DYNAMIC flex read (`MathUtil.cbrt (zf get 'n')`) was one of
// TestUncalledDispatchTrapDeclinesInexactOperands' negatives: the static tag
// is not a value, so no terminal trap. Since 2026-09-26 the fn value's
// recovery re-matches it at run time instead: the interpreter's
// uncalled_function on a String, its answer on a number.
func TestUncalledDispatchDynamicOperandRematches(t *testing.T) {
	for _, v := range []string{`'x'`, `27`} {
		src := `import "boru:math-util"  def zf (flex {n:` + v + `}) end MathUtil.cbrt (zf get 'n')`
		requireEngineParity(t, src, true)
		if dis := compileDisasm(t, src); !strings.Contains(dis, "cbrt/1 (poly)") || strings.Contains(dis, "TRAP") {
			t.Errorf("%s: want the runtime re-match, not a trap:\n%s", src, dis)
		}
	}
	_, errI := mustNew(t).RunInterp(`import "boru:math-util"  def zf (flex {n:'x'}) end MathUtil.cbrt (zf get 'n')`)
	if codeOf(errI) != "uncalled_function" {
		t.Errorf("interpreter: %v; want uncalled_function", errI)
	}
}

// TestModuleNativeFnValueOverAnyRecovers — the mini-s3 chunk loops. A module
// NATIVE reached as a fn value (`Net.send-bytes (slice i hi body) sock`)
// over an `Any` param found no static signature and PARKED as data, so the
// loop body netted [fn bytes sock] per iteration where the interpreter nets
// nothing ("for: body nets multiple values per iteration"). The value now
// takes the bare word's no-signature recovery — a runtime re-match over the
// module's own registry (recoveryPolyOwner) — and a runtime no-match raises
// the fn value's own
// uncalled_function (PolyNoMatchSpec.Uncalled), not a word's
// signature_error.
func TestModuleNativeFnValueOverAnyRecovers(t *testing.T) {
	pre := `import "boru:net" end ` +
		`def snd fn [[sock:Any body:Bytes][Integer][def total (size body) for [0 total 2] [def hi (if ((add i 2) lt total) [add i 2] [total]) Net.send-bytes (slice i hi body) sock] total]] end ` +
		`def rd fn [[sock:Any][String][(convert String (Net.recv-until sock (convert Bytes "\n") {within:5000}))]] end ` +
		`def l (Net.listen {tcp: 0}) end def a (Net.addr l) end ` +
		`def s (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String a.port)])}) end ` +
		`def c (Net.accept l {within: 5000}) end `
	for _, src := range []string{
		pre + `def n (snd s (convert Bytes "hello")) end def g (convert String (Net.recv-bytes c 5 {within: 5000})) end Net.close s; Net.close c; Net.close l; [n g]`,
		pre + `def n (snd s (convert Bytes "ab\n")) end def g (rd c) end Net.close s; Net.close c; Net.close l; [n g]`,
	} {
		requireEngineParity(t, src, true)
		if dis := compileDisasm(t, src); !strings.Contains(dis, "POLY") {
			t.Errorf("the fn-value dispatch must lower to a poly re-match:\n%s", dis)
		}
	}
	h := `def h fn [[a:Any][Any][a]] end `
	// The runtime no-match: the interpreter raises the fn value's
	// uncalled_function at the value; so does the compiled re-match.
	for _, src := range []string{
		`import "boru:net" end ` + h + `def g fn [[s:Any][Integer][for [0 4 2] [Net.send-bytes (convert Bytes "x") s] 0]] end g (h 1)`,
		`import "boru:net" end ` + h + `def g fn [[s:Any][Any][(Net.recv-until s (convert Bytes "\n") {within:10})]] end g (h 1)`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || codeOf(errI) != "uncalled_function" || fmt.Sprint(gotC, errC) != fmt.Sprint(gotI, errI) {
			t.Errorf("%q: compiled=%v C=%v/%v I=%v/%v; want the interpreter's uncalled_function on both lanes", src, compiled, gotC, errC, gotI, errI)
		}
	}
	// Negatives, both declining loudly with parity through the fallback: a
	// DEFINITE mismatch (a concrete Integer, no operand of unknown type) keeps
	// parking — the interpreter raises at the park — and a window NARROWER
	// than an overload (`recv-until` over two operands, a 3-arity form
	// exists) cannot prove its first match.
	fnValueM2CompileFailure(t, "definite mismatch parks",
		`import "boru:net" end def g fn [[s:Any][Integer][for [0 4 2] [Net.send-bytes (convert Bytes "x") s] 0]] end g 1`,
		"body nets multiple values per iteration")
	fnValueM2CompileFailure(t, "narrow window",
		`import "boru:net" end `+h+`def g fn [[s:Any][Any][(Net.recv-until s (convert Bytes "\n"))]] end g (h 1)`,
		"unmatched dispatch recovered at recv-until")
}
