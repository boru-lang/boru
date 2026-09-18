// Property-based differential gate: instead of the finite curated corpus,
// GENERATE random well-typed boru programs from the compilable subset and assert
// the compiled VM and the interpreter agree on every one (the same invariant
// TestSpecCompiledDifferential checks per spec row). The corpus exercises hand-
// written rows; this exercises COMBINATIONS the corpus never enumerates (a
// computed list whose elements are `if` results, `size` over an assembled list,
// a def-local feeding arithmetic, …) — exactly where the carrier compiler's
// per-construct gates can have holes. A failure is shrunk to a minimal program.
package langspec

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fuzzEnv reads a positive integer from an env var, else returns def. Lets CI /
// a nightly job CRANK the fuzzer (`BORU_FUZZ_SEEDS=20 BORU_FUZZ_ITERS=5000`)
// without touching the lean, deterministic default the standard suite runs.
func fuzzEnv(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// cat is a generated node's value category, so generation and shrinking stay
// well-typed (an Integer slot never gets a Boolean node).
type cat int

const (
	cInt cat = iota
	cBool
	cStr
	cList
	cMap
)

type gnode struct {
	op   string
	cat  cat
	ecat cat      // element category (list) / value category (map -> get's result)
	n    int      // lit value / var index / str-lit index / map-key index
	cmp  string   // for op=="cmp"
	keys []string // for op=="map"
	kids []*gnode
}

var mapKeys = []string{"a", "b", "c"}
var strLits = []string{"x", "y", "z", "w"}
var cmps = []string{"lt", "gt", "eq", "lte", "gte"}

// randVCat / randECat pick a map value / list element category, biased to cInt
// but reaching strings and nested containers.
func randVCat(r *rand.Rand) cat {
	switch r.Intn(4) {
	case 0:
		return cStr
	case 1:
		return cList
	default:
		return cInt
	}
}
func randECat(r *rand.Rand) cat {
	switch r.Intn(5) {
	case 0:
		return cStr
	case 1:
		return cList
	case 2:
		return cMap
	default:
		return cInt
	}
}

// gen builds a node of the requested category within a depth budget; scope is
// the in-scope Integer var names (def-bound). Categories are tracked so `if`
// branches and `get` results stay well-typed.
func gen(r *rand.Rand, c cat, depth int, scope []string) *gnode {
	if depth <= 0 {
		return leaf(r, c, scope)
	}
	switch c {
	case cBool:
		switch r.Intn(5) {
		case 0:
			return &gnode{op: "and", cat: cBool, kids: []*gnode{gen(r, cBool, depth-1, scope), gen(r, cBool, depth-1, scope)}}
		case 1:
			return &gnode{op: "or", cat: cBool, kids: []*gnode{gen(r, cBool, depth-1, scope), gen(r, cBool, depth-1, scope)}}
		case 2:
			return &gnode{op: "not", cat: cBool, kids: []*gnode{gen(r, cBool, depth-1, scope)}}
		case 3:
			return &gnode{op: "cmp", cat: cBool, cmp: "eq", kids: []*gnode{gen(r, cStr, depth-1, scope), gen(r, cStr, depth-1, scope)}}
		default:
			return &gnode{op: "cmp", cat: cBool, cmp: cmps[r.Intn(len(cmps))], kids: []*gnode{gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
		}
	case cStr:
		switch r.Intn(6) {
		case 0:
			return &gnode{op: "add", cat: cStr, kids: []*gnode{gen(r, cStr, depth-1, scope), gen(r, cStr, depth-1, scope)}} // concat
		case 1:
			return &gnode{op: "if", cat: cStr, kids: []*gnode{gen(r, cBool, depth-1, scope), gen(r, cStr, depth-1, scope), gen(r, cStr, depth-1, scope)}}
		case 2:
			m := genMap(r, cStr, depth-1, scope)
			return &gnode{op: "get", cat: cStr, n: r.Intn(len(m.keys)), kids: []*gnode{m}}
		case 3:
			return genCase(r, cStr, depth, scope) // string-bodied dispatch over an int scrutinee
		case 4:
			return genIndex(r, cStr) // `([…] get i)` -> the String element
		default:
			return leaf(r, cStr, scope)
		}
	case cMap:
		switch r.Intn(4) {
		case 0:
			// copy-returning map update: (<map> set <key> <value>) -> a new map.
			vc := randVCat(r)
			m := genMap(r, vc, depth-1, scope)
			return &gnode{op: "setmap", cat: cMap, ecat: vc, n: r.Intn(len(mapKeys)), kids: []*gnode{m, gen(r, vc, depth-1, scope)}}
		case 1:
			// value-filtered copy: (<map{Int}> filter [<cmp> N]) -> a sub-map.
			// Result keeps the input map type (a map of Integer); the predicate
			// runs per value. Quotation `[cmp N]` and KeyVal-lambda `.v` forms.
			return genFilter(r, cMap, depth-1, scope)
		}
		return genMap(r, randVCat(r), depth-1, scope)
	case cList:
		switch r.Intn(9) {
		case 7:
			// `(<litList> each [(<expr>) <lit>])` — a top-taking closure body that
			// leaves a COMPUTED value below a trailing throwaway literal, so the
			// residual is [element, <computed>, <lit>]. The handler reads only the
			// top (the lit), so the compiler drops the element and the computed
			// value below it (trimToTopResult); without that the event-above-inert
			// residual refused. The body uses an empty scope, so it has no captures
			// and compiles as a real closure (PUSH_CLOSURE), exercising the trim.
			ebody := gen(r, cInt, depth-1, nil)
			return &gnode{op: "eachtail", cat: cList, ecat: cInt, n: r.Intn(6),
				kids: []*gnode{genLitList(r), ebody}}
		case 6:
			// `(<litList> each [var [[v] <body using v> <lit>]])` — the var-block
			// closure-body idiom (a let-binding plus a trailing throwaway literal,
			// so each element maps to the literal). `var` SPLICES its body, so it
			// refuses inside the closure probe and the each body bakes as an
			// interpreted const the handler runs per element — the path the var
			// clean-refusal path enabled. The body still executes
			// (side effects / errors are compared for taxonomy parity), but the
			// mapped value is the trailing literal.
			vbody := gen(r, cInt, depth-1, append(append([]string{}, scope...), "v"))
			return &gnode{op: "eachvar", cat: cList, ecat: cInt, n: r.Intn(6),
				kids: []*gnode{genLitList(r), vbody}}
		case 0:
			// `for <count> [<body using i>]` -> list of per-iteration values.
			return &gnode{op: "for", cat: cList, ecat: cInt, kids: []*gnode{
				gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, append(append([]string{}, scope...), "i"))}}
		case 1:
			// `(each [<op> N] <litList>)` -> mapped list.
			return &gnode{op: "each", cat: cList, ecat: cInt, cmp: binOps[r.Intn(len(binOps))], n: r.Intn(6), kids: []*gnode{genLitList(r)}}
		case 2:
			// `(<litList> scan [<binop>])` -> running fold.
			return &gnode{op: "scan", cat: cList, ecat: cInt, cmp: binOps[r.Intn(len(binOps))], kids: []*gnode{genLitList(r)}}
		case 3:
			// `(filter [<cmp> N] <litList>)` -> the kept sub-list. The predicate
			// narrows the result to the INPUT collection type (a list of Integer);
			// quotation `[cmp N]` and `{key value}`-lambda `.value` forms.
			return genFilter(r, cList, depth-1, scope)
		case 4:
			// `(reverse <list>)` -> the same list reversed (element type preserved).
			in := gen(r, cList, depth-1, scope)
			return &gnode{op: "reverse", cat: cList, ecat: in.ecat, kids: []*gnode{in}}
		case 5:
			// `(<scalarList> sort)` -> ascending sort (element type preserved).
			sl := genScalarList(r)
			return &gnode{op: "sort", cat: cList, ecat: sl.ecat, kids: []*gnode{sl}}
		}
		ec := randECat(r)
		k := 1 + r.Intn(3)
		l := &gnode{op: "list", cat: cList, ecat: ec}
		for i := 0; i < k; i++ {
			l.kids = append(l.kids, gen(r, ec, depth-1, scope))
		}
		return l
	default: // cInt
		switch r.Intn(14) {
		case 0, 1:
			ops := []string{"add", "sub", "mul", "div", "mod"}
			return &gnode{op: ops[r.Intn(len(ops))], cat: cInt, kids: []*gnode{gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
		case 2:
			return &gnode{op: "if", cat: cInt, kids: []*gnode{gen(r, cBool, depth-1, scope), gen(r, cInt, depth-1, scope), gen(r, cInt, depth-1, scope)}}
		case 3:
			return &gnode{op: "size", cat: cInt, kids: []*gnode{gen(r, cList, depth-1, scope)}}
		case 4:
			m := genMap(r, cInt, depth-1, scope)
			return &gnode{op: "get", cat: cInt, n: r.Intn(len(m.keys)), kids: []*gnode{m}}
		case 5:
			// `(fold [<binop>] <litList> <init>)` -> accumulated value.
			return &gnode{op: "fold", cat: cInt, cmp: binOps[r.Intn(len(binOps))], n: r.Intn(6), kids: []*gnode{genLitList(r)}}
		case 6:
			return genCase(r, cInt, depth, scope) // multi-way dispatch (N-arm if-join)
		case 7:
			return genIndex(r, cInt) // `([…] get i)` -> the Integer element
		default:
			return leaf(r, cInt, scope)
		}
	}
}

var binOps = []string{"add", "mul", "sub"}

// genIndex builds an integer-index read over a LITERAL list of c-elements:
// `([e0 e1 …] get i)` with i in bounds, so the result is a value of category c.
// The list is pure literals (a computed/var element refuses const-baking), which
// is the shape the checker narrows — "concrete-list integer-index read to the
// element type": Integer for an int list, String for a string list. n is the
// index; kids[0] is the literal list.
func genIndex(r *rand.Rand, c cat) *gnode {
	k := 2 + r.Intn(3)
	l := &gnode{op: "list", cat: cList, ecat: c}
	for i := 0; i < k; i++ {
		l.kids = append(l.kids, idxLeaf(r, c))
	}
	return &gnode{op: "idx", cat: c, n: r.Intn(k), kids: []*gnode{l}}
}

// idxLeaf is a pure-literal element for an index-read list (no vars / no
// computation, so the whole list bakes as a const).
func idxLeaf(r *rand.Rand, c cat) *gnode {
	if c == cStr {
		return &gnode{op: "strlit", cat: cStr, n: r.Intn(len(strLits))}
	}
	return &gnode{op: "lit", cat: cInt, n: r.Intn(6)}
}

// genScalarList builds a list whose elements are all ONE comparable scalar type
// (all Integer or all String) — a sortable list. Either a literal list or a
// computed-but-scalar list (an `each`-mapped int list), so `sort` lowers over
// both a baked-const and a runtime-assembled input.
func genScalarList(r *rand.Rand) *gnode {
	switch r.Intn(3) {
	case 0:
		l := &gnode{op: "list", cat: cList, ecat: cStr}
		for k := 2 + r.Intn(3); k > 0; k-- {
			l.kids = append(l.kids, &gnode{op: "strlit", cat: cStr, n: r.Intn(len(strLits))})
		}
		return l
	case 1:
		return &gnode{op: "each", cat: cList, ecat: cInt, cmp: binOps[r.Intn(len(binOps))], n: r.Intn(6), kids: []*gnode{genLitList(r)}}
	}
	return genLitList(r)
}

// genFilter builds a predicate-`filter` over an all-Integer collection: a
// literal int list (-> cList) or an int-valued map (-> cMap). filter keeps the
// container shape, so the result narrows to the INPUT collection type — the
// checker's "narrow filter to its input collection type" path. Two predicate
// forms exercise distinct lowerings: the quotation block `[<cmp> N]` (the value
// is on the stack) and a Function callback that reads the per-element pair
// (`{key value}.value` over a list, KeyVal `.v` over a map — the ClosureInShape
// closure-arg path). keys marks the lambda form; cmp/n carry the predicate.
func genFilter(r *rand.Rand, c cat, depth int, scope []string) *gnode {
	n := &gnode{op: "filter", cat: c, ecat: cInt, cmp: cmps[r.Intn(len(cmps))], n: r.Intn(6)}
	if r.Intn(2) == 0 {
		n.keys = []string{"L"} // Function (lambda) form
	}
	if c == cMap {
		n.kids = []*gnode{genMap(r, cInt, depth, scope)}
	} else {
		n.kids = []*gnode{genLitList(r)}
	}
	return n
}

// genLitList builds a LITERAL int list (2-4 lits) — higher-order words compile
// over a literal list but fall back over a computed (makelist) one.
func genLitList(r *rand.Rand) *gnode {
	l := &gnode{op: "list", cat: cList, ecat: cInt}
	for k := 2 + r.Intn(3); k > 0; k-- {
		l.kids = append(l.kids, &gnode{op: "lit", cat: cInt, n: r.Intn(6)})
	}
	return l
}

// genCase builds a well-typed `case` dispatch whose result is category c (cInt
// or cStr) — a multi-way branch-join, the N-arm generalization of `if`. The
// scrutinee stays a literal/var Integer (a computed scrutinee refuses); clauses
// mix scalar / [gt N] / [lt N] / Integer-type matches; a default is ALWAYS
// present so the result is a well-typed value of c (embeddable anywhere). Some
// cInt arms are value-consuming blocks ([<binop> N], which see the scrutinee).
func genCase(r *rand.Rand, c cat, depth int, scope []string) *gnode {
	n := &gnode{op: "case", cat: c}
	if r.Intn(2) == 0 {
		n.cmp = "stk" // stack form: <scrutinee> case [clauses]
	}
	n.kids = append(n.kids, leaf(r, cInt, scope)) // scrutinee: lit or in-scope var
	for cl := 1 + r.Intn(3); cl > 0; cl-- {
		n.keys = append(n.keys, genMatchKey(r))
		n.kids = append(n.kids, genArmBody(r, c, depth-1, scope))
	}
	n.kids = append(n.kids, genArmBody(r, c, depth-1, scope)) // default (last kid)
	return n
}

// genCaseStmt is a top-level NO-DEFAULT `case` — it may match nothing and yield
// an empty result, which is only well-typed at the statement tail. Exercises the
// 0-result dispatch path (a case value that isn't re-pushable).
func genCaseStmt(r *rand.Rand) *gnode {
	n := &gnode{op: "case", cat: cInt}
	if r.Intn(2) == 0 {
		n.cmp = "stk"
	}
	n.kids = append(n.kids, &gnode{op: "lit", cat: cInt, n: r.Intn(6)}) // literal scrutinee
	for cl := 1 + r.Intn(3); cl > 0; cl-- {
		n.keys = append(n.keys, genMatchKey(r))
		n.kids = append(n.kids, genArmBody(r, cInt, 2, nil))
	}
	return n // no default kid: len(kids) == 1 + len(keys)
}

// genMatchKey encodes one case match clause: "sN" scalar literal, "gN"/"lN" the
// predicate block [gt N]/[lt N], "tI" the Integer type literal (matches any
// Integer scrutinee).
func genMatchKey(r *rand.Rand) string {
	switch r.Intn(5) {
	case 0:
		return "g" + fmt.Sprint(r.Intn(6))
	case 1:
		return "l" + fmt.Sprint(r.Intn(6))
	case 2:
		return "tI"
	default:
		return "s" + fmt.Sprint(r.Intn(6))
	}
}

// genArmBody is a case arm body: a normal node of category c, or — for cInt — a
// value-consuming block [<binop> <int>] that operates on the scrutinee.
func genArmBody(r *rand.Rand, c cat, depth int, scope []string) *gnode {
	if c == cInt && r.Intn(4) == 0 {
		return &gnode{op: "vblock", cat: cInt, cmp: binOps[r.Intn(len(binOps))], kids: []*gnode{leaf(r, cInt, scope)}}
	}
	if depth < 0 {
		depth = 0
	}
	return gen(r, c, depth, scope)
}

// renderMatch renders a match key (see genMatchKey) as case-clause source.
func renderMatch(key string) string {
	switch key[0] {
	case 'g':
		return "[gt " + key[1:] + "]"
	case 'l':
		return "[lt " + key[1:] + "]"
	case 't':
		return "Integer"
	default: // 's'
		return key[1:]
	}
}

// leaf is a depth-0 terminal of category c.
func leaf(r *rand.Rand, c cat, scope []string) *gnode {
	switch c {
	case cStr:
		return &gnode{op: "strlit", cat: cStr, n: r.Intn(len(strLits))}
	case cBool:
		return &gnode{op: "cmp", cat: cBool, cmp: "lt", kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(3)}, {op: "lit", cat: cInt, n: r.Intn(3)}}}
	case cList:
		return &gnode{op: "list", cat: cList, ecat: cInt, kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(6)}}}
	case cMap:
		return genMap(r, cInt, 0, scope)
	default: // cInt
		if len(scope) > 0 && r.Intn(2) == 0 {
			return &gnode{op: "var", cat: cInt, n: r.Intn(len(scope))}
		}
		return &gnode{op: "lit", cat: cInt, n: r.Intn(6)}
	}
}

// genMap builds a map whose values are all category vcat (so `get` of any key
// has a known type).
func genMap(r *rand.Rand, vcat cat, depth int, scope []string) *gnode {
	nk := 1 + r.Intn(len(mapKeys))
	m := &gnode{op: "map", cat: cMap, ecat: vcat, keys: append([]string{}, mapKeys[:nk]...)}
	for i := 0; i < nk; i++ {
		d := depth
		if d < 0 {
			d = 0
		}
		m.kids = append(m.kids, gen(r, vcat, d, scope))
	}
	return m
}

// genProgram returns a top-level node and the var names it may reference. With
// some probability it wraps the body in one or two def-bindings to stress
// value-def locals (OpStoreLocal) and local references.
// genArrProg builds an in-place FLEX-LIST MUTATION program — the load-bearing
// "a pooled const must never reach an in-place mutator" territory:
//
//	def a0 (flex [0 0 …]) (def _ (a0 set i v)) … <read of a0>
//
// The array is referenced and mutated repeatedly; if the compiler aliased it to
// a pooled const, hoisted the make, or reordered set/read, the result diverges.
func genArrProg(r *rand.Rand) *gnode {
	length := 2 + r.Intn(3)
	p := &gnode{op: "arrprog", cat: cInt, n: length}
	for s := r.Intn(4); s > 0; s-- {
		set := &gnode{op: "setarr", cat: cInt, n: r.Intn(length), kids: []*gnode{gen(r, cInt, 2, nil)}}
		p.kids = append(p.kids, set)
	}
	p.kids = append(p.kids, genArrRead(r, length))
	return p
}

// genArrRead reads the mutated array a0: an element, its size, or arithmetic
// over two elements (multiple reads of a mutated instance).
func genArrRead(r *rand.Rand, length int) *gnode {
	switch r.Intn(3) {
	case 0:
		return &gnode{op: "arrsize", cat: cInt}
	case 1:
		return &gnode{op: "arrget", cat: cInt, n: r.Intn(length)}
	default:
		return &gnode{op: "arradd", cat: cInt, kids: []*gnode{
			{op: "arrget", cat: cInt, n: r.Intn(length)}, {op: "arrget", cat: cInt, n: r.Intn(length)}}}
	}
}

// genObjProg builds an OBJECT/CLASS field-mutation program — in-place mutation
// through the object path (a different instance type than Array):
//
//	def P class {a: 0 b: 0} def p (make P {}) (p set a v) … <read of p>
func genObjProg(r *rand.Rand) *gnode {
	nf := 1 + r.Intn(len(mapKeys))
	fields := append([]string{}, mapKeys[:nf]...)
	p := &gnode{op: "objprog", cat: cInt, keys: fields}
	for s := r.Intn(4); s > 0; s-- {
		f := fields[r.Intn(nf)]
		p.kids = append(p.kids, &gnode{op: "setobj", cat: cInt, keys: []string{f}, kids: []*gnode{gen(r, cInt, 2, nil)}})
	}
	p.kids = append(p.kids, genObjRead(r, fields))
	return p
}

func genObjRead(r *rand.Rand, fields []string) *gnode {
	if r.Intn(2) == 0 {
		return &gnode{op: "objget", cat: cInt, keys: []string{fields[r.Intn(len(fields))]}}
	}
	return &gnode{op: "objadd", cat: cInt, kids: []*gnode{
		{op: "objget", cat: cInt, keys: []string{fields[r.Intn(len(fields))]}},
		{op: "objget", cat: cInt, keys: []string{fields[r.Intn(len(fields))]}}}}
}

// genFnProg builds a fn-VALUE INDIRECTION program: define a fn (a named `fn` or
// a `=>` lambda) over 0-2 Integer params with an Integer body, then CALL it
// THROUGH a value — `apply` (`<args> f0/v apply`, or `/uv` usurp) or stored-
// field dispatch (`def m {f: <fnval>} m.f <args>`). This is dispatch machinery
// nothing else the generator emits exercises: every other call is direct. The
// body is a full Integer expression over the params (if / case / fold / for …),
// so the closure unit lowers a rich body. A 0-param paren-def auto-fires, so
// apply is gated to >=1 param; stored dispatch handles 0.
//
// The fn may also CLOSE OVER outer def-locals: with some probability 1-2 `def
// vK <lit>` bindings precede the fn and join its body scope, so the body
// references them as captures (the FnBaselines / ComputeCaptures path). Capture
// values are literals — capturing a computed (carrier) local refuses. keys holds
// the whole body scope (params ++ captured names); n marks the param prefix.
func genFnProg(r *rand.Rand) *gnode {
	nparams := r.Intn(3) // 0, 1, 2
	apply := nparams > 0 && r.Intn(2) == 0
	params := append([]string{}, []string{"a", "b"}[:nparams]...)
	bodyScope := append([]string{}, params...)
	var caps []*gnode
	if r.Intn(2) == 0 {
		for c := 1 + r.Intn(2); c > 0; c-- {
			caps = append(caps, &gnode{op: "def", n: len(caps), kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(6)}}})
			bodyScope = append(bodyScope, fmt.Sprintf("v%d", len(caps)-1))
		}
	}
	form := "F"
	if nparams > 0 && r.Intn(2) == 0 {
		form = "L" // lambda (=>) — needs >=1 param
	}
	op := "fnstored"
	if apply {
		op = "fnapply"
		if r.Intn(3) == 0 {
			op = "fnapplyu" // usurp (/uv): argument-reversed wrapper
		}
	}
	body := gen(r, cInt, 3, bodyScope)
	if len(caps) > 0 && r.Intn(3) != 0 { // bias toward an ACTUAL capture reference
		body = &gnode{op: binOps[r.Intn(len(binOps))], cat: cInt,
			kids: []*gnode{body, {op: "var", cat: cInt, n: nparams + r.Intn(len(caps))}}}
	}
	node := &gnode{op: op, cat: cInt, n: nparams, keys: bodyScope, cmp: form, kids: []*gnode{body}}
	for i := 0; i < nparams; i++ {
		node.kids = append(node.kids, &gnode{op: "lit", cat: cInt, n: r.Intn(6)})
	}
	if len(caps) == 0 {
		return node
	}
	return &gnode{op: "seq", kids: append(caps, node)} // outer captures ++ the fn prog
}

// genUserFnProg builds a DIRECTLY-CALLED named fn — the CALL_USER / RET (and,
// for the recursive form, TAIL_CALL_USER) lowering that nothing else here
// exercises: genFnProg only ever calls a fn THROUGH a value (apply / stored
// dispatch), never `name args` directly. Two shapes:
//
//   - non-recursive: `def f0 fn [[a:Integer b:Integer][Integer][<body>]] (f0 1 2)`
//     — a plain frame call with a rich Integer body over the params.
//   - bounded tail-recursive accumulator: a self-call in the else-arm tail
//     position (TAIL_CALL_USER), driven by a SMALL literal counter (1..5) so it
//     terminates in a handful of frames — far under either engine's step budget,
//     so it can never make the differential flaky.
func genUserFnProg(r *rand.Rand) *gnode {
	if r.Intn(2) == 0 {
		return &gnode{op: "userfnrec", cat: cInt,
			cmp:  binOps[r.Intn(len(binOps))],
			n:    1 + r.Intn(5),                                  // bounded recursion depth (1..5)
			kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(6)}}, // initial accumulator
		}
	}
	nparams := 1 + r.Intn(2) // 1 or 2
	params := append([]string{}, []string{"a", "b"}[:nparams]...)
	node := &gnode{op: "userfn", cat: cInt, n: nparams, keys: params,
		kids: []*gnode{gen(r, cInt, 3, params)}} // body in the param scope
	for i := 0; i < nparams; i++ {
		node.kids = append(node.kids, &gnode{op: "lit", cat: cInt, n: r.Intn(6)})
	}
	return node
}

// genCatchProg — do/error catch frames + the CompileValueDiverges div/mod-by-zero
// divergent terminal (the island-fix path). The body is an Integer expression;
// half the time it is FORCED to raise (`(body) div 0` / `mod 0`) and a consuming
// handler reads the caught Error (`dot code` / `dot message`) or a bind-and-read
// converts it — exercising the native catch-frame closure and the divergent
// terminal. The other half is a non-raising body whose value the handler passes
// through. No prior fuzzer path reached do/error/raise or the div0-in-do shape.
func genCatchProg(r *rand.Rand) *gnode {
	// The bind-and-read handler `convert Map e0` TYPE-CONSUMES the caught value,
	// so it pairs only with a CLEAN literal div/mod-by-zero terminal. A gen body
	// can nest a static-zero div/mod inside an OUTER static-zero mod
	// (`(0 mod 0) mod 0` — dead code); the check then mistypes the caught error
	// under `convert` (a narrow pre-existing precision edge, tracked separately).
	// The error-READING handlers (dot code/message, fallback int) read the Error
	// directly and are robust to that nesting, so they take a full gen body.
	if r.Intn(3) == 0 {
		return &gnode{op: "catchlit", n: 1 + r.Intn(2)} // 1=div0, 2=mod0 → convert Map
	}
	body := gen(r, cInt, 2, nil)
	raise := 0
	if r.Intn(2) == 0 {
		raise = 1 + r.Intn(2) // force a raise: (body) div 0 / mod 0
	}
	return &gnode{op: "catchprog", n: raise, cmp: []string{"code", "message", "int"}[r.Intn(3)], kids: []*gnode{body}}
}

// genGradualAnyProg — the gradual-`Any` boundary (miscompile classes C & D,
// design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore): a fn DECLARED to return `[Any]` whose
// gradual result feeds a polymorphic word (`add`), a higher-order word over a
// list of Any elements (`each` / `size`), a comparison, or a concrete Integer
// param (via a second Any-returning fn). These were CONFIRMED silent
// divergences and the well-typed literal-only generator structurally never
// built them.
func genGradualAnyProg(r *rand.Rand) *gnode {
	body := gen(r, cInt, 2, []string{"x"}) // Integer body in the param scope
	return &gnode{op: "anyprog", n: r.Intn(5), kids: []*gnode{body,
		{op: "lit", cat: cInt, n: r.Intn(6)}, {op: "lit", cat: cInt, n: r.Intn(6)}}}
}

// genInterpProg — template strings (`\`text ${expr} …\“) → OpInterp, an entire
// opcode with ZERO prior generative coverage. 1–2 typed Integer holes woven
// through literal text; the whole thing evaluates to a String on both engines.
func genInterpProg(r *rand.Rand) *gnode {
	kids := make([]*gnode, 1+r.Intn(2))
	for i := range kids {
		kids[i] = gen(r, cInt, 2, nil)
	}
	return &gnode{op: "interpprog", kids: kids}
}

// genHofProg — the higher-order / fn-value AXES the review's two live
// divergences lived in (checker-compiler-completeness-review.0.md §8.1(3)),
// none of which the prior families could spell:
//
//   - apply SPELLING: the forward call `f (g x)`, the `apply` word over a
//     `/v` reference, and the def-split `def r (f x) f r`;
//   - apply DEPTH: 1–3 chained applications of Function-typed params;
//   - lambda param POLARITY: value-typed (Integer) vs quote-typed (Atom —
//     a /q capture slot the runtime never binds from a delivered value);
//   - collection PROVENANCE for a HOF callback: literal list, computed
//     (a filter result / `keys`), gradual (laundered through an [Any] fn).
//
// Many of these shapes REFUSE compilation by design — the oracle skips a
// refusal (comp=false), so the gate is exactly the contract: whatever
// COMPILES must agree with the interpreter. The compose miscompile and the
// Atom-lambda callback divergence both COMPILED wrongly; either would have
// been caught at seed time had this family existed.
func genHofProg(r *rand.Rand) *gnode {
	n := &gnode{op: "hofprog", cat: cInt, n: r.Intn(5)}
	// Three shared Integer literals parameterise every variant (and give the
	// shrinker something to reduce).
	for i := 0; i < 3; i++ {
		n.kids = append(n.kids, &gnode{op: "lit", cat: cInt, n: 1 + r.Intn(5)})
	}
	switch n.n {
	case 0: // chained forward apply, depth 1-3
		n.cmp = []string{"d1", "d2", "d3"}[r.Intn(3)]
	case 1: // apply-word spelling, single or double window
		n.cmp = []string{"one", "two"}[r.Intn(2)]
	case 2: // def-split spelling
		n.cmp = "split"
	case 3: // HOF callback: param polarity x collection provenance
		n.cmp = []string{"int", "atom"}[r.Intn(2)] +
			"-" + []string{"lit", "computed", "gradual"}[r.Intn(3)]
	default: // factory / curried chain, single or double apply
		n.cmp = []string{"c1", "c2"}[r.Intn(2)]
	}
	return n
}

// renderHofProg renders genHofProg's variants. Every program is closed (no
// free names) and Integer-resulting where it succeeds; error outcomes are
// fine — the oracle compares taxonomy.
func renderHofProg(n *gnode) string {
	k1, k2, k3 := fmt.Sprint(n.kids[0].n), fmt.Sprint(n.kids[1].n), fmt.Sprint(n.kids[2].n)
	lamAdd := "([za:Integer] => [za add " + k1 + "])"
	lamMul := "([zb:Integer] => [zb mul " + k2 + "])"
	switch n.n {
	case 0:
		body := "f x"
		switch n.cmp {
		case "d2":
			body = "f (g x)"
		case "d3":
			body = "f (g (f x))"
		}
		return "def zh fn [[f:Function g:Function x:Integer] [Integer] [" + body + "]] zh " +
			lamAdd + " " + lamMul + " " + k3
	case 1:
		if n.cmp == "two" {
			return "def zh fn [[c1:Function c2:Function v:Integer] [Integer] [v c1/v apply c2/v apply]] zh (" +
				lamAdd + "/v) (" + lamMul + "/v) " + k3
		}
		return "def zh fn [[c1:Function v:Integer] [Integer] [v c1/v apply]] zh (" + lamAdd + "/v) " + k3
	case 2:
		return "def zh fn [[f:Function x:Integer] [Integer] [def zr (f x) f zr]] zh " + lamAdd + " " + k3
	case 3:
		pol, prov, _ := strings.Cut(n.cmp, "-")
		var lam, coll string
		if pol == "atom" {
			lam = "[[zk:Atom] => [zk]]"
			switch prov {
			case "lit":
				coll = "['a' 'b']"
			case "computed":
				coll = "(keys {a:" + k1 + " b:" + k2 + "})"
			default:
				coll = "(zid (keys {a:" + k1 + "}))"
			}
		} else {
			lam = "[[zk:Integer] => [zk add " + k1 + "]]"
			switch prov {
			case "lit":
				coll = "[" + k1 + " " + k2 + " " + k3 + "]"
			case "computed":
				coll = "(filter [gt 0] [" + k1 + " " + k2 + " " + k3 + "])"
			default:
				coll = "(zid [" + k1 + " " + k2 + "])"
			}
		}
		if prov == "gradual" {
			return "def zid fn [[zx:Any] [Any] [zx]] each " + lam + " " + coll
		}
		return "each " + lam + " " + coll
	default:
		if n.cmp == "c2" {
			return "def zmk fn [[zx:Integer] [Function] [([zy:Integer] => [([zz:Integer] => [zx add zy add zz])])]] (((zmk " +
				k1 + ") " + k2 + ") " + k3 + ")"
		}
		return "def zmk fn [[zx:Integer] [Function] [([zy:Integer] => [zx add zy])]] ((zmk " + k1 + ") " + k2 + ")"
	}
}

// renderFnDef renders a fn VALUE: a named-fn `(fn [[params][Integer][body]])` or
// an afn lambda `([params] => [body])`. The body is already rendered in the
// param scope.
func renderFnDef(form string, params []string, bodySrc string) string {
	sig := make([]string, len(params))
	for i, p := range params {
		sig[i] = p + ":Integer"
	}
	psig := strings.Join(sig, " ")
	if form == "L" {
		return "([" + psig + "] => [" + bodySrc + "])"
	}
	return "(fn [[" + psig + "][Integer][" + bodySrc + "]])"
}

// genStrProg builds a STRING-OPERATIONS program: the `import "boru:string-util"
// end` preamble (transparent — a program compiles identically with or without
// it) followed by a string-flavoured expression from genS. Exercises the
// `StringUtil.*` module ops and, crucially, COMPUTED strings flowing through
// maps / comparisons / `size` — the scalar-carrier-keep path (keeping a concrete
// inert string concrete through check mode) that the rest of the generator
// barely stresses.
func genStrProg(r *rand.Rand) *gnode {
	c := cStr
	switch r.Intn(5) {
	case 0:
		c = cInt
	case 1:
		c = cBool
	case 2:
		c = cList
	case 3:
		c = cMap
	}
	return &gnode{op: "strprog", cat: c, kids: []*gnode{genS(r, c, 3)}}
}

// genS builds a node of category c from StringUtil ops plus basic int/bool/list/
// map composition; result categories stay well-typed (upper/lower/trim/concat/
// replace/repeat -> String, contains -> Boolean, indexof -> Integer, split ->
// List). Self-contained: it never calls gen, so it cannot perturb the other
// grammar paths. concat takes LITERAL string elements (computed strings in a
// list literal refuse); every other op nests freely.
func genS(r *rand.Rand, c cat, depth int) *gnode {
	if depth <= 0 {
		return sleaf(r, c)
	}
	switch c {
	case cStr:
		switch r.Intn(6) {
		case 0:
			return &gnode{op: "strop", cat: cStr, cmp: "upper", kids: []*gnode{genS(r, cStr, depth-1)}}
		case 1:
			return &gnode{op: "strop", cat: cStr, cmp: "lower", kids: []*gnode{genS(r, cStr, depth-1)}}
		case 2:
			return &gnode{op: "strop", cat: cStr, cmp: "trim", kids: []*gnode{genS(r, cStr, depth-1)}}
		case 3:
			l := &gnode{op: "list", cat: cList, ecat: cStr}
			for k := 2 + r.Intn(2); k > 0; k-- {
				l.kids = append(l.kids, sleaf(r, cStr)) // literal elements -> compiles
			}
			return &gnode{op: "strop", cat: cStr, cmp: "concat", kids: []*gnode{l}}
		case 4:
			return &gnode{op: "strop", cat: cStr, cmp: "repeat", kids: []*gnode{genS(r, cInt, depth-1), genS(r, cStr, depth-1)}}
		default:
			return &gnode{op: "strop", cat: cStr, cmp: "replace", kids: []*gnode{genS(r, cStr, depth-1), genS(r, cStr, depth-1), genS(r, cStr, depth-1)}}
		}
	case cBool:
		if r.Intn(2) == 0 {
			return &gnode{op: "strop", cat: cBool, cmp: "contains", kids: []*gnode{genS(r, cStr, depth-1), genS(r, cStr, depth-1)}}
		}
		return &gnode{op: "cmp", cat: cBool, cmp: "eq", kids: []*gnode{genS(r, cStr, depth-1), genS(r, cStr, depth-1)}}
	case cList:
		return &gnode{op: "strop", cat: cList, cmp: "split", kids: []*gnode{genS(r, cStr, depth-1), genS(r, cStr, depth-1)}}
	case cMap:
		nk := 1 + r.Intn(len(mapKeys))
		m := &gnode{op: "map", cat: cMap, keys: append([]string{}, mapKeys[:nk]...)}
		for i := 0; i < nk; i++ {
			vc := cStr
			if r.Intn(2) == 0 {
				vc = cInt
			}
			m.kids = append(m.kids, genS(r, vc, depth-1))
		}
		return m
	default: // cInt
		switch r.Intn(4) {
		case 0:
			return &gnode{op: "strop", cat: cInt, cmp: "indexof", kids: []*gnode{genS(r, cStr, depth-1), genS(r, cStr, depth-1)}}
		case 1:
			return &gnode{op: "size", cat: cInt, kids: []*gnode{genS(r, cList, depth-1)}}
		case 2:
			return &gnode{op: "add", cat: cInt, kids: []*gnode{genS(r, cInt, depth-1), genS(r, cInt, depth-1)}}
		default:
			return sleaf(r, cInt)
		}
	}
}

// sleaf is a depth-0 terminal for the string sub-grammar.
func sleaf(r *rand.Rand, c cat) *gnode {
	switch c {
	case cStr:
		return &gnode{op: "strlit", cat: cStr, n: r.Intn(len(strLits))}
	case cBool:
		return &gnode{op: "cmp", cat: cBool, cmp: "eq", kids: []*gnode{sleaf(r, cStr), sleaf(r, cStr)}}
	case cList:
		return &gnode{op: "strop", cat: cList, cmp: "split", kids: []*gnode{sleaf(r, cStr), sleaf(r, cStr)}}
	case cMap:
		return &gnode{op: "map", cat: cMap, keys: []string{"a"}, kids: []*gnode{sleaf(r, cStr)}}
	default: // cInt
		return &gnode{op: "lit", cat: cInt, n: r.Intn(6)}
	}
}

func genProgram(r *rand.Rand, depth int) (*gnode, []string) {
	switch r.Intn(14) {
	case 0:
		return genArrProg(r), nil
	case 1:
		return genObjProg(r), nil
	case 2:
		return genCaseStmt(r), nil // top-level no-default case (0-result tail)
	case 3:
		return genFnProg(r), nil // fn-value indirection (apply / stored dispatch)
	case 4:
		return genStrProg(r), nil // StringUtil ops (computed strings, indexof, split…)
	case 5:
		return genUserFnProg(r), nil // directly-called named fn (CALL_USER / TAIL_CALL_USER / RET)
	case 6:
		return genCatchProg(r), nil // do/error catch frames + div/mod-by-zero terminal
	case 7:
		return genGradualAnyProg(r), nil // gradual-Any boundary (miscompile classes C & D)
	case 8:
		return genInterpProg(r), nil // template strings → OpInterp
	case 9:
		return genHofProg(r), nil // HOF/fn-value axes (apply spelling/depth, quote polarity, collection provenance)
	}
	scope := []string{}
	var stmts []*gnode
	// Optionally seed the CONTEXT STORE: `context set 'KEY' <lit> end` …, then
	// expose each KEY as an Integer var that renders `(context get 'KEY')`, so
	// the whole body generator weaves strict-key store reads through arithmetic,
	// `if`, `for`, `case`, defs, … (a get inside a computed container refuses —
	// harmlessly). Keys are '@'-tagged in scope to distinguish them from defs.
	if r.Intn(3) == 0 {
		nk := 1 + r.Intn(len(ctxKeys))
		for i := 0; i < nk; i++ {
			stmts = append(stmts, &gnode{op: "ctxset", keys: []string{ctxKeys[i]}, kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(6)}}})
			scope = append(scope, "@"+ctxKeys[i])
		}
		if r.Intn(2) == 0 { // shadow the first key (latest value wins)
			stmts = append(stmts, &gnode{op: "ctxset", keys: []string{ctxKeys[0]}, kids: []*gnode{{op: "lit", cat: cInt, n: r.Intn(6)}}})
		}
	}
	var defs []*gnode
	for nDefs := 0; r.Intn(3) == 0 && nDefs < 2; nDefs++ {
		name := fmt.Sprintf("v%d", len(scope))
		defs = append(defs, &gnode{op: "def", n: len(scope), kids: []*gnode{gen(r, cInt, depth-1, append([]string{}, scope...))}})
		scope = append(scope, name)
	}
	bodyCat := cInt
	switch r.Intn(7) {
	case 0:
		bodyCat = cList
	case 1:
		bodyCat = cMap
	case 2:
		bodyCat = cStr
	case 3:
		bodyCat = cBool
	}
	body := gen(r, bodyCat, depth, scope)
	prefix := append(stmts, defs...)
	if len(prefix) == 0 {
		return body, scope
	}
	return &gnode{op: "seq", kids: append(prefix, body)}, scope
}

// ctxKeys is the pool of context-store keys the generator may seed and read.
var ctxKeys = []string{"n", "p", "q"}

// renderProgOp renders the added whole-program shapes (catch frames, the
// gradual-Any boundary, template strings) — split out of render so its
// cyclomatic complexity stays under the linter budget.
func renderProgOp(n *gnode, scope []string) string {
	switch n.op {
	case "catchlit": // clean literal div/mod-by-zero terminal, bound and type-consumed
		op := "div"
		if n.n == 2 {
			op = "mod"
		}
		return "def e0 (do [5 " + op + " 0]) convert Map e0"
	case "catchprog":
		body := render(n.kids[0], nil)
		switch n.n {
		case 1:
			body = "(" + body + ") div 0"
		case 2:
			body = "(" + body + ") mod 0"
		}
		switch n.cmp {
		case "message":
			return "do [" + body + "] error [dot message]"
		case "int": // handler ignores the error (nets 2 → island; still parity)
			return "do [" + body + "] error [99]"
		default: // "code" — Atom or None; a non-raising body passes through
			return "do [" + body + "] error [dot code]"
		}
	case "anyprog":
		fdef := "def f0 fn [[x:Integer] [Any] [" + render(n.kids[0], []string{"x"}) + "]] "
		a, b := render(n.kids[1], scope), render(n.kids[2], scope)
		switch n.n {
		case 0:
			return fdef + "add (f0 " + a + ") " + b // Any + Integer into a polymorphic word
		case 1:
			return fdef + "[(f0 " + a + ") (f0 " + b + ")] each [add 1]" // list of Any → each
		case 2:
			return fdef + "if ((f0 " + a + ") gt 0) [1] [2]" // Any into a comparison
		case 3:
			return fdef + "size [(f0 " + a + ") (f0 " + b + ")]" // list of Any → size
		default: // gradual Any into a CONCRETE Integer param (a second boundary)
			return "def g0 fn [[y:Any] [Any] [y]] " + fdef + "f0 (g0 " + a + ")"
		}
	default: // "interpprog"
		var sb strings.Builder
		sb.WriteString("`v")
		for i, k := range n.kids {
			sb.WriteString(fmt.Sprintf("%d=${", i))
			sb.WriteString(render(k, nil))
			sb.WriteString("} ")
		}
		sb.WriteString("end`")
		return sb.String()
	}
}

func render(n *gnode, scope []string) string {
	switch n.op {
	case "lit":
		return fmt.Sprint(n.n)
	case "var":
		if n.n < len(scope) {
			name := scope[n.n]
			if strings.HasPrefix(name, "@") { // context-store key -> strict get
				return "(context get '" + name[1:] + "')"
			}
			return name
		}
		return "0"
	case "strlit":
		return `"` + strLits[n.n] + `"`
	case "add", "sub", "mul", "div", "mod", "and", "or":
		return "(" + render(n.kids[0], scope) + " " + n.op + " " + render(n.kids[1], scope) + ")"
	case "not":
		return "(not " + render(n.kids[0], scope) + ")"
	case "cmp":
		return "(" + render(n.kids[0], scope) + " " + n.cmp + " " + render(n.kids[1], scope) + ")"
	case "if":
		return "(if " + render(n.kids[0], scope) + " [" + render(n.kids[1], scope) + "] [" + render(n.kids[2], scope) + "])"
	case "size":
		return "(size " + render(n.kids[0], scope) + ")"
	case "get":
		key := mapKeys[0]
		if n.n < len(n.kids[0].keys) {
			key = n.kids[0].keys[n.n]
		}
		return "(" + render(n.kids[0], scope) + " get " + key + ")"
	case "for":
		return "(for " + render(n.kids[0], scope) + " [" + render(n.kids[1], append(append([]string{}, scope...), "i")) + "])"
	case "list":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = render(k, scope)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case "map":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = n.keys[i] + ": " + render(k, scope)
		}
		return "{" + strings.Join(parts, " ") + "}"
	case "setmap":
		// copy-returning map update: (<map> set <key> <value>)
		key := mapKeys[n.n%len(mapKeys)]
		return "(" + render(n.kids[0], scope) + " set " + key + " " + render(n.kids[1], scope) + ")"
	case "arrprog":
		zeros := strings.TrimSpace(strings.Repeat("0 ", n.n))
		var sb strings.Builder
		sb.WriteString("def a0 (flex [" + zeros + "])")
		for _, k := range n.kids {
			sb.WriteString(" " + render(k, scope))
		}
		return sb.String()
	case "setarr":
		// flex `set` returns the node (chaining); bind it away so the mutation
		// is a pure statement, matching the former 0-result Array `set`.
		return "def _ (a0 set " + fmt.Sprint(n.n) + " " + render(n.kids[0], scope) + ")"
	case "arrget":
		return "(a0 get " + fmt.Sprint(n.n) + ")"
	case "arrsize":
		return "(size a0)"
	case "arradd":
		return "(" + render(n.kids[0], scope) + " add " + render(n.kids[1], scope) + ")"
	case "objprog":
		fdefs := make([]string, len(n.keys))
		for i, f := range n.keys {
			fdefs[i] = f + ": 0"
		}
		var sb strings.Builder
		sb.WriteString("def P class {" + strings.Join(fdefs, " ") + "} def p (make P {})")
		for _, k := range n.kids {
			sb.WriteString(" " + render(k, scope))
		}
		return sb.String()
	case "setobj":
		return "(p set " + n.keys[0] + " " + render(n.kids[0], scope) + ")"
	case "objget":
		return "(p get " + n.keys[0] + ")"
	case "objadd":
		return "(" + render(n.kids[0], scope) + " add " + render(n.kids[1], scope) + ")"
	case "fold":
		return "(fold [" + n.cmp + "] " + render(n.kids[0], scope) + " " + fmt.Sprint(n.n) + ")"
	case "each":
		return "(each [" + n.cmp + " " + fmt.Sprint(n.n) + "] " + render(n.kids[0], scope) + ")"
	case "eachtail":
		// kids[0] = the literal list; kids[1] = a computed Integer body expression
		// (no scope → no captures); n = the trailing throwaway literal each element
		// maps to. The body leaves [computed, lit] above the element; the top-taking
		// trim keeps only what the handler reads.
		return "(" + render(n.kids[0], scope) + " each [" + render(n.kids[1], nil) +
			" " + fmt.Sprint(n.n) + "])"
	case "eachvar":
		// kids[0] = the literal list to iterate; kids[1] = the var-block body
		// (an Integer expression that may reference the bound `v`); n = the
		// trailing throwaway literal each element maps to.
		vscope := append(append([]string{}, scope...), "v")
		return "(" + render(n.kids[0], scope) + " each [var [[v] " +
			render(n.kids[1], vscope) + " " + fmt.Sprint(n.n) + "]])"
	case "scan":
		return "(" + render(n.kids[0], scope) + " scan [" + n.cmp + "])"
	case "filter":
		// keys==["L"] selects the Function (lambda) form; the per-element field is
		// `.value` over a list and `.v` over a map.
		pred := "[" + n.cmp + " " + fmt.Sprint(n.n) + "]"
		if len(n.keys) > 0 {
			field := "value"
			if n.cat == cMap {
				field = "v"
			}
			pred = "([p:Any] => [p." + field + " " + n.cmp + " " + fmt.Sprint(n.n) + "])"
		}
		if n.cat == cMap {
			return "(" + render(n.kids[0], scope) + " filter " + pred + ")"
		}
		return "(filter " + pred + " " + render(n.kids[0], scope) + ")"
	case "idx":
		// integer-index read of a literal list: (<list> get <i>)
		return "(" + render(n.kids[0], scope) + " get " + fmt.Sprint(n.n) + ")"
	case "reverse":
		return "(reverse " + render(n.kids[0], scope) + ")"
	case "sort":
		return "(" + render(n.kids[0], scope) + " sort)"
	case "case":
		nClause := len(n.keys)
		clauses := make([]string, 0, nClause*2+1)
		for i := 0; i < nClause; i++ {
			clauses = append(clauses, renderMatch(n.keys[i]), render(n.kids[1+i], scope))
		}
		if len(n.kids) > 1+nClause { // default present (always the last kid)
			clauses = append(clauses, render(n.kids[len(n.kids)-1], scope))
		}
		body := "[" + strings.Join(clauses, " ") + "]"
		if n.cmp == "stk" {
			return "(" + render(n.kids[0], scope) + " case " + body + ")"
		}
		return "(case " + render(n.kids[0], scope) + " " + body + ")"
	case "vblock":
		return "[" + n.cmp + " " + render(n.kids[0], scope) + "]"
	case "ctxset":
		return "context set '" + n.keys[0] + "' " + render(n.kids[0], scope) + " end"
	case "strprog":
		return `import "boru:string-util" ` + render(n.kids[0], scope)
	case "strop":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = render(k, scope)
		}
		return "(StringUtil." + n.cmp + " " + strings.Join(parts, " ") + ")"
	case "fnapply", "fnapplyu":
		// sig = params (keys[:n]); body scope = params ++ captured locals (keys).
		fndef := renderFnDef(n.cmp, n.keys[:n.n], render(n.kids[0], n.keys))
		args := ""
		for _, k := range n.kids[1:] {
			args += render(k, scope) + " "
		}
		suffix := "/v"
		if n.op == "fnapplyu" {
			suffix = "/uv"
		}
		return "def f0 " + fndef + " " + args + "f0" + suffix + " apply"
	case "fnstored":
		out := "def m {f: " + renderFnDef(n.cmp, n.keys[:n.n], render(n.kids[0], n.keys)) + "} m.f"
		for _, k := range n.kids[1:] {
			out += " " + render(k, scope)
		}
		return out
	case "userfn":
		// def f0 fn [[a:Integer b:Integer][Integer][<body>]] (f0 <arg> …)
		sig := make([]string, len(n.keys))
		for i, p := range n.keys {
			sig[i] = p + ":Integer"
		}
		args := ""
		for _, k := range n.kids[1:] {
			args += " " + render(k, scope)
		}
		return "def f0 fn [[" + strings.Join(sig, " ") + "][Integer][" + render(n.kids[0], n.keys) + "]] (f0" + args + ")"
	case "userfnrec":
		// Bounded tail-recursive accumulator: the f0 self-call is the else-arm
		// tail (-> TAIL_CALL_USER); n decreases to the (n lte 0) base case.
		init := render(n.kids[0], scope)
		return "def f0 fn [[n:Integer acc:Integer][Integer][if (n lte 0) [acc] [f0 (n sub 1) (acc " +
			n.cmp + " n)]]] (f0 " + fmt.Sprint(n.n) + " " + init + ")"
	case "catchlit", "catchprog", "anyprog", "interpprog":
		return renderProgOp(n, scope)
	case "hofprog":
		return renderHofProg(n)
	case "def":
		return "def v" + fmt.Sprint(n.n) + " " + render(n.kids[0], scope)
	case "seq":
		parts := make([]string, len(n.kids))
		for i, k := range n.kids {
			parts[i] = render(k, scope)
		}
		return strings.Join(parts, " ")
	}
	return "0"
}

// diverges runs the program on both engines and reports whether the COMPILED
// path was taken and whether the two results disagree (value or error presence
// — the TestSpecCompiledDifferential invariant).
func diverges(t testing.TB, src string) (compiled, bad bool) {
	ac := newDifferentialInstance(t)
	gotC, comp, errC := ac.RunCompiled(src)
	if !comp {
		return false, false
	}
	ai := newDifferentialInstance(t)
	gotI, errI := ai.RunInterp(src)
	if (errC == nil) != (errI == nil) {
		return true, true
	}
	if errC != nil {
		return true, errCode(errC) != errCode(errI) // both errored — compare taxonomy
	}
	return true, fmt.Sprint(gotC) != fmt.Sprint(gotI)
}

func TestPropertyDifferential(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("property fuzz: skipped in -short")
	}
	// Lean DETERMINISTIC default (a regression guard, ~10s) — the same programs
	// every run, so a failure always reproduces. Override the budget for a deep
	// exploratory hunt: `BORU_FUZZ_SEEDS=20 BORU_FUZZ_ITERS=5000 go test -run
	// TestPropertyDifferential`.
	const depth = 4
	iters := fuzzEnv("BORU_FUZZ_ITERS", 1500)
	seeds := fuzzEnv("BORU_FUZZ_SEEDS", 2)
	var compiled, failures int
	for seed := 1; seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(int64(seed)))
		for i := 0; i < iters; i++ {
			root, scope := genProgram(r, depth)
			src := render(root, scope)
			comp, bad := diverges(t, src)
			if comp {
				compiled++
			}
			if bad {
				failures++
				min := shrink(t, root, scope)
				t.Errorf("DIVERGENCE (seed %d iter %d):\n  full : %s\n  min  : %s", seed, i, src, render(min, scope))
				if failures >= 10 {
					t.Fatalf("stopping after %d divergences", failures)
				}
			}
		}
	}
	t.Logf("property fuzz: %d seeds x %d programs, %d took the compiled path, %d divergences", seeds, iters, compiled, failures)
}

// shrink greedily replaces sub-nodes with the minimal node of their category
// (and drops list elements) while the divergence persists, yielding a small
// reproducer. Bounded so a pathological case can't loop forever.
func shrink(t *testing.T, root *gnode, scope []string) *gnode {
	cur := root
	for round := 0; round < 200; round++ {
		progressed := false
		for _, cand := range simplifications(cur) {
			if _, bad := diverges(t, render(cand, scope)); bad {
				cur = cand
				progressed = true
				break
			}
		}
		if !progressed {
			return cur
		}
	}
	return cur
}

func minNode(c cat) *gnode {
	switch c {
	case cBool:
		return &gnode{op: "cmp", cat: cBool, cmp: "lt", kids: []*gnode{{op: "lit", cat: cInt}, {op: "lit", cat: cInt}}}
	case cStr:
		return &gnode{op: "strlit", cat: cStr}
	case cList:
		return &gnode{op: "list", cat: cList, ecat: cInt, kids: []*gnode{{op: "lit", cat: cInt}}}
	case cMap:
		return &gnode{op: "map", cat: cMap, ecat: cInt, keys: []string{"a"}, kids: []*gnode{{op: "lit", cat: cInt}}}
	default:
		return &gnode{op: "lit", cat: cInt}
	}
}

// simplifications returns every single-point reduction of n: each reducible
// sub-node replaced by the minimal node of its category, plus list-element drops.
func simplifications(n *gnode) []*gnode {
	var out []*gnode
	var walk func(path []int)
	walk = func(path []int) {
		sub := at(n, path)
		// Replace sub with the minimal node of its category (if not already minimal).
		if !isMinimal(sub) {
			out = append(out, replace(n, path, minNode(sub.cat)))
		}
		// Drop one element from a multi-element list.
		if sub.op == "list" && len(sub.kids) > 1 {
			for d := range sub.kids {
				nl := &gnode{op: "list", cat: cList}
				for i, k := range sub.kids {
					if i != d {
						nl.kids = append(nl.kids, k)
					}
				}
				out = append(out, replace(n, path, nl))
			}
		}
		// Drop one SET statement from an arr/obj prog (the last kid is the read).
		if (sub.op == "arrprog" || sub.op == "objprog") && len(sub.kids) > 1 {
			for d := 0; d < len(sub.kids)-1; d++ {
				np := &gnode{op: sub.op, cat: cInt, n: sub.n, keys: sub.keys}
				for i, k := range sub.kids {
					if i != d {
						np.kids = append(np.kids, k)
					}
				}
				out = append(out, replace(n, path, np))
			}
		}
		for i := range sub.kids {
			walk(append(append([]int{}, path...), i))
		}
	}
	walk(nil)
	return out
}

func isMinimal(n *gnode) bool {
	switch n.op {
	case "lit", "var":
		return true
	case "strlit":
		return true
	case "cmp":
		return n.kids[0].op == "lit" && n.kids[1].op == "lit"
	case "list":
		return len(n.kids) == 1 && n.kids[0].op == "lit"
	case "map":
		return len(n.kids) == 1 && n.kids[0].op == "lit"
	}
	return false
}

func at(n *gnode, path []int) *gnode {
	for _, i := range path {
		n = n.kids[i]
	}
	return n
}

// replace returns a deep copy of root with the node at path swapped for repl.
func replace(root *gnode, path []int, repl *gnode) *gnode {
	if len(path) == 0 {
		return repl
	}
	cp := *root
	cp.kids = append([]*gnode{}, root.kids...)
	cp.kids[path[0]] = replace(root.kids[path[0]], path[1:], repl)
	return &cp
}
