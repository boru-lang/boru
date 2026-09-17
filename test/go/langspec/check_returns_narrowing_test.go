// Landing tests for the Phase-4.3 G7 return-annotation sweep
// (design/legacy/CHECKER-BYTECODE-COMPLETION-PLAN.0.ignore): per-family positive
// pins (a precise Returns/ReturnsFn flows downstream, so a former
// declared-Any frontier row now checks clean with a NARROW residual) and
// negative pins (the same precision makes a wrong-typed downstream use
// FLAG where the old dynamic(Any) silently matched everything). Plus the
// unit pins for Signature.DeclaresCheckReturns, the predicate the
// coverage gate (check_returns_gate_test.go) is built on.
package langspec

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestReturnsAnnotationNarrowing(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// wantFlagged: the checker reports an error-severity diagnostic.
		wantFlagged bool
		// wantFrontier: a clean row's residual still ends at an
		// Any-bounded carrier (the metric TestCheckAnyFrontier gates).
		// Only meaningful when wantFlagged is false.
		wantFrontier bool
	}{
		// ---- mini re: record-shaped ReturnsFn ({ok ms fst lst n}) ----
		{"re-n-narrows", `import "boru:minilang"  ("a1b2c3" mini re '\\d').n`, false, false},
		{"re-nested-fst-m-narrows", `import "boru:minilang"  def r ("AbcD" mini re '[a-z]+') end  r.fst.m`, false, false},
		{"re-compiled-hook-narrows", `import "boru:minilang"  ("a1b2c3" +re/\d/).n`, false, false},
		// dynamic(Boolean) is now PROVABLY disjoint from add's operands —
		// the old dynamic(Any) result matched silently.
		{"re-ok-into-add-flags", `import "boru:minilang"  ("a1b2c3" mini re '\\d').ok add 1`, true, false},

		// ---- mini gex: subject-shaped ReturnsFn ----
		{"gex-scalar-narrows", `import "boru:minilang"  "ax" mini gex 'a*'`, false, false},
		{"gex-list-narrows", `import "boru:minilang"  ["ab" "zz" "ac"] +gex/a*/`, false, false},

		// ---- parselang: pure concrete-source fold ----
		{"parse-json-field-narrows", `import "boru:parselang"  (parse json '{"a":1,"b":"hi"}') get 'b'`, false, false},
		{"parse-aontu-field-narrows", `import "boru:parselang"  (parse aontu 'a:1 b:2') get 'b'`, false, false},
		{"parse-json-field-into-keys-flags", `import "boru:parselang"  ((parse json '{"a":1}') get 'a') keys`, true, false},

		// ---- StructUtil: shaped/annotated returns ----
		{"struct-parse-folds", `import "boru:struct-util" (StructUtil.parse "{a:1, b:[2,3]}") get 'a'`, false, false},
		{"struct-merge-maps-narrows", `import "boru:struct-util" StructUtil.merge {a:1 b:2} {b:3 c:4}`, false, false},
		{"struct-reify-class-field-narrows", `import "boru:struct-util"  def Point class {x:1, y:2} (StructUtil.reify Point {x:9}) dot y`, false, false},
		{"struct-setpath-keeps-map", `import "boru:struct-util" StructUtil.setpath {a:1} "b" 99`, false, false},
		// getpath is genuinely unknowable — the declared Any must STAY
		// gradual (no false precision).
		{"struct-getpath-stays-gradual", `import "boru:struct-util" StructUtil.getpath "a.b" {a:{b:42}}`, false, true},

		// ---- make over a record type: schema rides the instance carrier ----
		{"make-record-field-narrows", `import "boru:test"  (make Test.TestCase {name: "a" in: [1] out: 2}) get "name"`, false, false},
		// The `out` field is DECLARED Any in the TestCase schema — the
		// honest bound stays dynamic(Any).
		{"make-record-any-field-stays", `import "boru:test"  (make Test.TestCase {name: "a" in: [1] out: 2}) get "out"`, false, true},

		// ---- NUR068: a record schema means the same thing as a fn RETURN.
		// A declared `refine Record` return keeps its schema on the result
		// carrier (recordReturnCarrier), so the three declaration positions
		// — make, param, RETURN — now agree, and a schema carrier passes a
		// record param's field-bag pattern by the runtime's open rule
		// (unifyRecordSchemaCarrierVsMap).
		{"record-return-field-narrows", `def R refine Record [name:String n:Integer] def mk fn [[] [R] [make R {name:"a" n:1}]] (mk) get "name"`, false, false},
		{"record-return-into-same-param-clean", `def R refine Record [name:String n:Integer] def mk fn [[] [R] [make R {name:"a" n:1}]] def use fn [[c:R] [String] [c get "name"]] use (mk)`, false, false},
		{"make-into-record-param-clean", `def R refine Record [name:String n:Integer] def p fn [[c:R] [String] [c get "name"]] p (make R {name:"a" n:1})`, false, false},
		// NEGATIVE (the precision now flags what dynamic(Any) matched):
		// a String field read is not a map, and a DIFFERENT record's
		// param is provably disjoint (exact-key record membership).
		{"record-return-field-into-keys-flags", `def R refine Record [name:String n:Integer] def mk fn [[] [R] [make R {name:"a" n:1}]] keys ((mk) get "name")`, true, false},
		{"record-return-disjoint-param-flags", `def R refine Record [name:String n:Integer] def S refine Record [tag:Atom v:List] def mk fn [[] [R] [make R {name:"a" n:1}]] def use fn [[s:S] [Atom] [s get "tag"]] use (mk)`, true, false},
		// NEGATIVE (does-not-over-claim): a field the schema does not
		// declare keeps the honest dynamic(Any).
		{"record-return-unknown-field-stays", `def R refine Record [name:String n:Integer] def mk fn [[] [R] [make R {name:"a" n:1}]] (mk) get "zz"`, false, true},
		// The boru:test constructor family the NUR068 survey traced. The
		// constructors deliberately stay [Map] (NUR069 — a record
		// annotation would be a false runtime contract for none-valued
		// Any fields); the narrowing comes from BuildFnBodyReturnsFn's
		// record-residual surfacing plus Test.prop's shape ReturnsFn.
		{"test-case-name-narrows", `import "boru:test"  (Test.case 6 [3] "d3") get "name"`, false, false},
		{"test-case-any-field-stays", `import "boru:test"  (Test.case 6 [3] "d3") get "out"`, false, true},
		{"test-spec-name-narrows", `import "boru:test"  (Test.spec [] double/q "doubling") get "name"`, false, false},
		{"test-prop-runs-narrows", `import "boru:test"  def p (Test.prop "nonneg" [r.int 0 100] [0 gte]) end p get "runs"`, false, false},
		{"test-run-property-ok-narrows", `import "boru:test"  def p (Test.prop "nonneg" [r.int 0 100] [0 gte]) end (p Test.run-property) get "ok"`, false, false},

		// ---- Vm.check / Vm.compile report shapes ----
		{"vm-check-ok-narrows", `import "boru:vm" (Vm.check "1 add 2").ok`, false, false},
		{"vm-compile-reason-narrows", `import "boru:vm" (Vm.compile "1 add (((").reason`, false, false},
		{"vm-check-ok-into-add-flags", `import "boru:vm" (Vm.check "1 add 2").ok add 1`, true, false},

		// ---- module descriptor reads ----
		{"module-inst-name-narrows", `import "boru:math-util"  MathUtil.$module.name`, false, false},

		// ---- pop/shift edge-element narrowing ----
		{"pop-elem-narrows", `pop [1 2 3]`, false, false},
		{"pop-elem-into-keys-flags", `pop [1 2 3] keys`, true, false},

		// ---- unify / tany concrete folds ----
		{"unify-fail-folds", `unify 2 3`, false, false},
		{"tany-folds-dynamic-disjunct", `tany ['a','b']`, false, false},

		// ---- istype annotation ----
		{"istype-boolean", `istype 5`, false, false},

		// ---- Error field reads: literal-key narrowing (errorFieldReturns) ----
		// A direct read off a bound Error carrier narrows the two well-known
		// keys off the Any-frontier; the result is gradual (dynamic), so it
		// does not add strictness (no "now flags" counterpart).
		{"err-code-narrows", `def e (do [raise bad_input "nope"])  e dot code`, false, false},
		{"err-message-narrows", `def e (do [raise "boom"])  e.message`, false, false},
		// NEGATIVE (does-not-over-claim): a payload key is an arbitrary
		// raise-spec value the checker cannot type, so it STAYS on the
		// frontier — the narrowing is confined to code/message.
		{"err-payload-key-stays-frontier", `def e (do [raise {code: bad_input/q, message: "m", got: 42}])  e dot got`, false, true},

		// ---- selector: the CONTAINER is knowable even when the elements
		// are not. voxgigstruct.Select's Go return type is []any.
		{"selector-container-narrows", `import "boru:struct-util" StructUtil.selector {color:'red'} {a:{color:'red'}}`, false, false},
		// NEGATIVE: a plain List is not a FlexList, so the narrowed return
		// makes append REFUSE where the Any residual matched optimistically.
		{"selector-into-append-flags", `import "boru:struct-util" append 1 (StructUtil.selector {color:'red'} {a:{color:'red'}})`, true, false},
		// …and the ELEMENTS stay honest: a selected child is any shape.
		{"selector-element-stays-frontier", `import "boru:struct-util" (StructUtil.selector {color:'red'} {a:{color:'red'}}) get 0`, false, true},

		// ---- await: modelled per mode, from doAwait's own order ----
		{"await-all-narrows", `import "boru:time-util" TimeUtil.await [[1 add 2] [3 add 4]]`, false, false},
		{"await-full-elem-narrows", `import "boru:time-util" (TimeUtil.await {mode:'full'} [[1 add 2]]) get 0`, false, false},
		{"await-empty-list-narrows", `import "boru:time-util" TimeUtil.await []`, false, false},
		// NEGATIVE (does-not-over-claim): first hands back the winning
		// branch's whole residual, so it must STAY on the frontier.
		{"await-first-stays-frontier", `import "boru:time-util" TimeUtil.await {mode:'first'} [[1 add 2]]`, false, true},
		// The unknown-mode mirror flags — but only where doAwait reaches the
		// raise. The empty-list row above is the same bad mode, unflagged.
		{"await-bad-mode-flags", `import "boru:time-util" TimeUtil.await {mode:'bogus'} [[1 add 2]]`, true, false},
		{"await-bad-mode-empty-list-clean", `import "boru:time-util" TimeUtil.await {mode:'bogus'} []`, false, false},

		// ---- quote: /q hands the handler a CONCRETE Atom, so running it in
		// check mode yields the quoted symbol a key reader can actually read.
		{"quote-dot-key-narrows", `def m {a:1} m dot (quote a)`, false, false},
		{"quote-get-key-narrows", `def m {a:1 b:2} m get (quote a)`, false, false},
		// NEGATIVE: the key is real enough to be MISSED — a quoted name that
		// is not in the map narrows to None, not to a permissive Any.
		{"quote-missing-key-narrows-none", `def m {a:1 b:2} m get (quote zz)`, false, false},
		// codequote's bare-word form IS quote's — same /q capture, same
		// handler — so it narrows identically. Its paren form stays raw.
		{"codequote-dot-key-narrows", `def m {a:1} m dot (codequote a)`, false, false},
		{"codequote-into-keys-flags", `def m {a:1} keys (m dot (codequote a))`, true, false},
		// ---- date arithmetic: every arm builds a Date ----
		{"add-days-narrows", `import "boru:time-util" TimeUtil.add-days 10 (TimeUtil.to-date (TimeUtil.unix 1700000000))`, false, false},
		{"earliest-narrows", `import "boru:time-util" TimeUtil.earliest (TimeUtil.to-date (TimeUtil.unix 0)) (TimeUtil.to-date (TimeUtil.unix 1700000000))`, false, false},
		{"start-of-narrows", `import "boru:time-util" TimeUtil.start-of "month" (TimeUtil.to-date (TimeUtil.unix 1700000000))`, false, false},
		{"start-of-chains-into-date-word", `import "boru:time-util" TimeUtil.is-leap-year (TimeUtil.start-of "month" (TimeUtil.to-date (TimeUtil.unix 1700000000)))`, false, false},
		// NEGATIVE: a Date is not a Map, and not an Instant either — both
		// uses ran into a runtime error while the Any residual matched.
		{"add-days-into-keys-flags", `import "boru:time-util" keys (TimeUtil.add-days 10 (TimeUtil.to-date (TimeUtil.unix 1700000000)))`, true, false},
		{"add-days-into-to-unix-flags", `import "boru:time-util" TimeUtil.to-unix (TimeUtil.add-days 10 (TimeUtil.to-date (TimeUtil.unix 1700000000)))`, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checked, flagged := checkRow(t, tc.src)
			if flagged != tc.wantFlagged {
				t.Fatalf("flagged=%v want %v for %q (stack %s)",
					flagged, tc.wantFlagged, tc.src, stackTypes(checked))
			}
			if !flagged {
				if got := residualHasAnyFrontier(checked); got != tc.wantFrontier {
					t.Fatalf("frontier=%v want %v for %q (stack %s)",
						got, tc.wantFrontier, tc.src, stackTypes(checked))
				}
			}
		})
	}
}

// TestDeclaresCheckReturnsPredicate pins the four annotation surfaces the
// coverage gate accepts — and that an unannotated Go sig is refused.
func TestDeclaresCheckReturnsPredicate(t *testing.T) {
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	unannotated := core.Signature{Args: []*core.Type{core.TAny}, Impl: core.Go(noop)}
	if unannotated.DeclaresCheckReturns() {
		t.Fatalf("unannotated Go sig must NOT declare check returns")
	}
	declared := core.Signature{Args: []*core.Type{core.TAny}, Impl: core.Go(noop),
		Returns: []*core.Type{core.TInteger}}
	if !declared.DeclaresCheckReturns() {
		t.Fatalf("declared Returns must count")
	}
	nothing := core.Signature{Impl: core.Go(noop), Returns: []*core.Type{}}
	if !nothing.DeclaresCheckReturns() {
		t.Fatalf("empty non-nil Returns (produces nothing) must count")
	}
	withFn := core.Signature{Impl: core.Go(noop),
		ReturnsFn: func(_ []core.Value, _ *core.Registry) []core.Value { return nil }}
	if !withFn.DeclaresCheckReturns() {
		t.Fatalf("ReturnsFn must count")
	}
	fullStack := core.Signature{Impl: core.Go(noop, core.FullStack(),
		core.CheckFullStack(func(_ []core.Value, stack []core.Value, _ *core.Registry) []core.Value {
			return stack
		}))}
	if !fullStack.DeclaresCheckReturns() {
		t.Fatalf("CheckFullStack shape fn must count")
	}
	boruBody := core.Signature{Impl: core.Boru([]core.Value{core.NewWord("dup")})}
	if !boruBody.DeclaresCheckReturns() {
		t.Fatalf("a boru body (analyser-derived returns) must count")
	}
}
