package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestDynamicCallbackPolyReMatches pins S1a of design/FULL-COMPILATION-REPLAN.0.md:
// a higher-order word whose callback the check pass cannot type — read from
// a class field or a map field, picked by a dynamic key, built by a factory,
// or a code body stored in a flex — no longer fails to compile at the
// ambiguous-overload gate. each/fold/scan/filter declare CompileDynBody, so
// the dispatch lowers to a poly re-match over the word's own overloads and
// the LIVE value picks the one the interpreter's dispatch would: a Function
// value takes the fn form, a quoted list the token form, a Map or a List
// collection its own iteration. These are the nineteen corpus rows the
// re-plan carved out as S1a, in the shapes real code writes them, pinned
// here in hundredths of a second where the corpus takes a minute.
func TestDynamicCallbackPolyReMatches(t *testing.T) {
	for _, tc := range []struct{ label, src string }{
		{"a class-field callback at each (callbacks.tsv:57)",
			`def Handler class {cb: Function} def h (make Handler {cb: (fn [[n:Integer][Integer][n add 1]])}) each h.cb [1 2 3]`},
		{"a map-field fn value at each (callbacks.tsv:58)",
			`def inc fn [[n:Integer][Integer][n add 1]]  def hold {f: inc/v}  each hold.f [1 2 3]`},
		{"a callback picked by a dynamic key (each-variants.tsv:196)",
			`def tbl {d:([a:Integer] => [mul 2 a])} end def k 'd' end each (tbl get k) [1 2]`},
		{"a code body stored in a flex at run time (code-bodies.tsv:123)",
			`def s (flex {}) end s set 'body' (quote [add 1]) drop end each (s get 'body') [1 2 3]`},
		{"a code body read off a Map parameter inside a fn (code-bodies.tsv:156)",
			`def m {f:(quote [add 1])} end def g fn [[c:Map][List][each c.f [1 2 3]]] end g m`},
		{"a factory-built callback at each",
			`def mk fn [[][Function][([n:Integer] => [n add 1])]] end each (mk) [1 2 3]`},
		{"a class-field reducer at fold with an init (fold-map-filter.tsv:75)",
			`def Reducer class {step: Function} end def r (make Reducer {step: (fn [[a:Integer e:Integer][Integer][a add e]])}) end 0 fold r.step [1 2 3]`},
		{"a class-field predicate at filter (fold-map-filter.tsv:117)",
			`def Pred class {ok: Function} end def p (make Pred {ok: (fn [[kv:Map][Boolean][kv.value gt 2]])}) end filter p.ok [1 2 3 4]`},
		{"a class-field callback at scan (fold-map-filter.tsv:213)",
			`def Handler class {cb: Function} def h (make Handler {cb: (fn [[a:Integer e:Integer][Integer][a add e]])}) scan h.cb [1 2 3]`},
		{"a map-field reducer picked by a key inside a fn (fold-map-filter.tsv:217)",
			`def fs {sum: (fn [[a:Integer e:Integer][Integer][a add e]])} def use fn [[k:String][Integer][0 fold (fs k get) [1 2 3]]]  use 'sum'`},
		{"a module class's field callback (module-composition.tsv:101)",
			`import module [def Handler class {cb: Function} export "M" {Handler: Handler}] end def h (make M.Handler {cb: (fn n:Integer Integer [n add 1])}) end each h.cb [1 2 3]`},
		// The gradual COLLECTION side of the same gate: a typed lambda over an
		// Any parameter that is a List at run time — and one that is a Map,
		// where both lanes raise the same no-matching-lambda error, because the
		// re-match picked the Map overload exactly as the interpreter did.
		{"a lambda over a gradual collection that is a List",
			`def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f [1 2]`},
		{"a lambda over a gradual collection that is a Map (both lanes raise)",
			`def f fn [[c:Any][Any][each ([x:Integer] => [x add 1]) c]] end f {a:1 b:2}`},
		{"a capturing lambda over a gradual collection",
			`def k 10 end def f fn [[c:Any][Any][each ([x:Integer] => [x add k]) c]] end f [1 2]`},
		{"fold with a lambda over a gradual collection",
			`def f fn [[c:Any][Any][fold ([a:Integer b:Integer] => [a add b]) c 0]] end f [1 2]`},
		{"filter with a lambda over a gradual collection",
			`def f fn [[c:Any][Any][filter ([p:Any] => [p.value gt 1]) c]] end f [1 2 3]`},
	} {
		compiledEqualsInterp(t, tc.label, tc.src)
	}
}

// TestDynamicCallbackStillDoesNotLowerWithoutTheFlag is the negative: a word that
// does NOT declare CompileDynBody keeps the ambiguous-overload compile failure for the
// same shapes, and the message names the gap. for-each is the case in point
// (its handler reads the collection as a list, and it nets no result, which
// the dyn-body seat also requires) and walk keeps its own code-body compile failure.
func TestDynamicCallbackStillDoesNotLowerWithoutTheFlag(t *testing.T) {
	for _, tc := range []struct{ label, src, want string }{
		{"for-each with a map-field callback",
			`def acc (flex []) end def m {f: ([e:Integer] => [acc push e])} end for-each m.f [1 2 3] end size acc`,
			"gradual-Any operand"},
		{"walk with a map-field hook",
			`def acc (flex []) end def hs {h: (m:Any => [acc push m.path])} end walk {mode: "depth"} {a:1 b:[2 3]} hs.h end size acc`,
			"code-body word walk"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(tc.src)
		if cerr != nil {
			t.Fatalf("%s: check: %v", tc.label, cerr)
		}
		if prog != nil {
			t.Errorf("%s: must still fail to compile without the declaration:\n%s", tc.label, prog.Disassemble())
			continue
		}
		if !strings.Contains(reason, tc.want) {
			t.Errorf("%s: failed with %q, want a reason containing %q", tc.label, reason, tc.want)
		}
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, ierr := b.RunInterp(tc.src); ierr != nil {
			t.Errorf("%s: the interpreter must still answer: %v", tc.label, ierr)
		}
	}
}

// TestStrictAnyOperandReMatches is the negative the S1a review asked for
// (a Codex P1 on #474): a collection that reaches a higher-order word through
// a STRICT Any carrier — a fn's declared `Any` result, not a gradual
// widening — is as unknown at run time as a gradual one, so the dyn-body
// seat must poly re-match rather than bake the checker's pick. Baked, the
// (Reach, List) arm ran over a runtime Integer and answered `[[]]` where
// the interpreter raises signature_error; re-matched, no arm matches and
// the compiled lane raises (or defers to) the interpreter's own verdict.
// Every row compiles; the two lanes must agree on the value and the error
// code — a runtime defer that re-runs the program on the interpreter is
// parity, so the compiled flag is not asserted.
func TestStrictAnyOperandReMatches(t *testing.T) {
	for _, tc := range []struct{ label, src, wantCode string }{
		{"a Reach body over a strict-Any Integer",
			`def get fn [[][Any][1]] end each $.x (get)`, "signature_error"},
		{"a Reach body over a strict-Any Map",
			`def get fn [[][Any][{a:1}]] end each $.x (get)`, "signature_error"},
		{"a Reach body over a strict-Any List (the only arm that matches)",
			`def get fn [[][Any][[{x:1} {x:2}]]] end each $.x (get)`, ""},
		{"a code body over a strict-Any Integer",
			`def get fn [[][Any][1]] end each [add 1] (get)`, "signature_error"},
		{"a code body over a strict-Any String",
			`def get fn [[][Any]['s']] end each [add 1] (get)`, "signature_error"},
		{"a code body over a strict-Any List",
			`def get fn [[][Any][[1 2]]] end each [add 1] (get)`, ""},
		{"a code body over a strict-Any Map (the token form sees the value)",
			`def get fn [[][Any][{a:1 b:2}]] end each [add 1] (get)`, ""},
		{"a lambda over a strict-Any Integer",
			`def get fn [[][Any][1]] end each ([x:Integer] => [x add 1]) (get)`, "signature_error"},
		{"a lambda over a strict-Any Map",
			`def get fn [[][Any][{a:1}]] end each ([x:Integer] => [x add 1]) (get)`, "signature_error"},
		{"fold over a strict-Any Integer",
			`def get fn [[][Any][1]] end fold [add] (get) 0`, "signature_error"},
		{"scan over a strict-Any Integer",
			`def get fn [[][Any][1]] end scan [add] (get)`, "signature_error"},
		{"filter over a strict-Any Integer",
			`def get fn [[][Any][1]] end filter [gt 0] (get)`, "filter_error"},
		{"do over a strict-Any Integer",
			`def get fn [[][Any][1]] end do (get)`, "signature_error"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(tc.src)
		if cerr != nil || prog == nil {
			t.Errorf("%s: must compile, declined %q / %v\n  %s", tc.label, reason, cerr, tc.src)
			continue
		}
		b, _ := New()
		gotC, _, errC := b.RunCompiled(tc.src)
		if noteCompileDefect(t, tc.src, gotC, errC) {
			continue
		}
		c, _ := New()
		gotI, errI := c.RunInterp(tc.src)
		if codeOf(errC) != codeOf(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%s: compiled/interp disagree\n  compiled %v / [%s] %v\n  interp   %v / [%s] %v\n  %s",
				tc.label, gotC, codeOf(errC), errC, gotI, codeOf(errI), errI, tc.src)
		}
		if tc.wantCode == "" && errI != nil {
			t.Errorf("%s: the interpreter must run it: %v", tc.label, errI)
		}
		if tc.wantCode != "" && codeOf(errI) != tc.wantCode {
			t.Errorf("%s: interpreter code [%s], want [%s]", tc.label, codeOf(errI), tc.wantCode)
		}
	}
}
