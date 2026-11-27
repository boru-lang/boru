package core

// The Go half of the core-level parity corpus (core/spec/*.tsv).
//
// core/ts/src/corespec.test.ts is the other half. The two runners share NO
// code — they read the same files and each implements the tiny expression
// notation independently, which is the point: a bug in one runner cannot
// hide a bug in the other engine, the way shared scaffolding would.
//
// This is a SPEC, not a differential. The `expected` column is written from
// the documented contract (REFERENCE.md, design/TYPES.10.md), so a row can
// legitimately fail on BOTH engines — which is exactly the class of defect
// the engine-level agreement corpus is structurally blind to
// (design/legacy/CORE-GO-TS-DEFECTS.0.ignore).

import (
	"bufio"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	apd "github.com/cockroachdb/apd/v3"
)

const coreSpecDir = "../spec"

type coreSpecRow struct {
	file     string
	line     int
	expr     string
	expected string
	note     string
}

// parseCoreSpec reads one TSV, skipping comments and blank lines.
func parseCoreSpec(t *testing.T, path string) []coreSpecRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var rows []coreSpecRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("%s:%d: want at least 2 tab-separated columns, got %d", path, n, len(parts))
		}
		row := coreSpecRow{file: filepath.Base(path), line: n, expr: parts[0], expected: parts[1]}
		if len(parts) > 2 {
			row.note = parts[2]
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

// coreSpecTypeLit maps a builtin type name to its literal value. Kept
// deliberately short — the corpus only names types every engine must have.
func coreSpecTypeLit(name string) (Value, bool) {
	switch name {
	case "Integer":
		return NewTypeLiteral(TInteger), true
	case "String":
		return NewTypeLiteral(TString), true
	case "Boolean":
		return NewTypeLiteral(TBoolean), true
	case "List":
		return NewTypeLiteral(TList), true
	case "Map":
		return NewTypeLiteral(TMap), true
	case "Any":
		return NewTypeLiteral(TAny), true
	case "None":
		return NewTypeLiteral(TNone), true
	}
	return Value{}, false
}

// coreSpecToken builds one `run` token: a decimal is an Integer, '…' is a
// String, a known builtin type name is that type's literal, anything else is
// a Word.
// coreSpecFnScratch is the throwaway name fn( installs under before the
// caller's def clause rebinds the value under its own name.
const coreSpecFnScratch = "__corespec_fn"

func coreSpecToken(tok string) Value {
	if strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'") && len(tok) >= 2 {
		return NewString(tok[1 : len(tok)-1])
	}
	if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
		return NewInteger(n)
	}
	if tl, ok := coreSpecTypeLit(tok); ok {
		return tl
	}
	switch tok {
	case "(":
		return NewOpenParen()
	case ")":
		return NewCloseParen()
	case ";":
		// The END marker. `end` is its word spelling and `;` its synonym
		// (REFERENCE.md:415); the corpus uses the punctuation so a row can
		// still name a WORD called end.
		return NewEnd()
	}
	// A word with call-site modifiers, as the parser would hand it over, in
	// their one written order: `name/` then an optional forced arity N, at
	// most one of `s` (stack only) or `f` (forward only), `u` (the bound
	// function with its argument order reversed) and `v` (the bound VALUE,
	// never a call). A slash whose suffix is not a modifier
	// (`Scalar/String`) leaves the token a plain word.
	if i := strings.LastIndexByte(tok, '/'); i > 0 && i < len(tok)-1 {
		if w, ok := coreSpecModifiers(tok[:i], tok[i+1:]); ok {
			return NewValueRaw(TWord, w)
		}
	}
	return NewWord(tok)
}

// coreSpecModifiers reads a modifier suffix — digits, then `s` or `f`,
// then `u`, then `v`, each optional but at least one present — into the
// word it modifies.
func coreSpecModifiers(name, mods string) (WordInfo, bool) {
	w := WordInfo{Name: name, ArgCount: -1}
	digits := 0
	for digits < len(mods) && mods[digits] >= '0' && mods[digits] <= '9' {
		digits++
	}
	if digits > 0 {
		n, err := strconv.Atoi(mods[:digits])
		if err != nil {
			return WordInfo{}, false
		}
		w.ArgCount = n
	}
	rest := mods[digits:]
	if strings.HasPrefix(rest, "s") {
		w.ForceStack, rest = true, rest[1:]
	} else if strings.HasPrefix(rest, "f") {
		w.ForceForward, rest = true, rest[1:]
	}
	if strings.HasPrefix(rest, "u") {
		w.ForceUsurp, rest = true, rest[1:]
	}
	if strings.HasPrefix(rest, "v") {
		w.ForceVal, rest = true, rest[1:]
	}
	return w, rest == ""
}

func coreSpecFields(s string) []string {
	var out []string
	for _, f := range strings.Split(s, " ") {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// coreSpecAssemble folds the flat token list into VALUES, building the
// bracket forms the corpus README describes: `[ … ]` an eval list, `[q … ]`
// a quoted one, `{ k: v … }` a map. So a row can hand the step loop the
// containers a parser would have built, without a parser.
//
// Deliberately NOT shared with core/ts/src/corespec.test.ts's copy: shared
// scaffolding hides the same bug from both engines, which is the whole
// reason this corpus has two runners.
func coreSpecAssemble(t *testing.T, r *Registry, toks []string) []Value {
	t.Helper()
	var out []Value
	pos := 0
	for pos < len(toks) {
		var v Value
		v, pos = coreSpecItem(t, r, toks, pos)
		out = append(out, v)
	}
	return out
}

// coreSpecItem reads ONE item starting at pos, returning it and the index
// just past it. Recursive because the bracket forms nest.
func coreSpecItem(t *testing.T, r *Registry, toks []string, pos int) (Value, int) {
	t.Helper()
	switch toks[pos] {
	case "[", "[q":
		open := toks[pos]
		pos++
		elems := []Value{}
		for pos < len(toks) && toks[pos] != "]" {
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			elems = append(elems, v)
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed %s in %s", open, strings.Join(toks, " "))
		}
		pos++ // the "]"
		if open == "[" {
			return NewEvalList(elems), pos
		}
		return NewList(elems), pos
	case "fn(":
		// fn( NAME PTYPE RTYPE [ body ] ) — one param, one return, a boru
		// body: the smallest shape that DISPATCHES. INSTALLED through
		// InstallFnDef, never assembled by hand — only installation builds
		// each Signature's body-splicing Handler (buildFnBodyHandler), and
		// the engine calls that handler unconditionally, so a hand-built
		// FnDefInfo panics into internal_error.
		pos++
		pname := toks[pos]
		ptype, ok := coreSpecTypeLit(toks[pos+1])
		if !ok {
			t.Fatalf("fn( %s: %q is not a corpus-known type name", pname, toks[pos+1])
		}
		rtype, ok2 := coreSpecTypeLit(toks[pos+2])
		if !ok2 {
			t.Fatalf("fn( %s: %q is not a corpus-known type name", pname, toks[pos+2])
		}
		pos += 3
		var body Value
		body, pos = coreSpecItem(t, r, toks, pos)
		if pos >= len(toks) || toks[pos] != ")" {
			t.Fatalf("unclosed fn( in %s", strings.Join(toks, " "))
		}
		pos++
		bl, err := AsList(body)
		if err != nil {
			t.Fatalf("fn( body must be a list: %v", err)
		}
		// InstallFnDef pushes the binding itself, under a scratch name the
		// caller's `def` clause then rebinds; the VALUE is what the clause
		// wants, so read it back off the def stack.
		InstallFnDef(r, coreSpecFnScratch, FnDefInfo{Signatures: []Signature{{
			// The NAMED type itself, as core/ts's runner declares it. A
			// type literal IS its lattice node, so Parent() here would be
			// the node's PARENT — Number for Integer, and nil for Any,
			// which no slot matches (found by spec/tools/diff-fuzz.py).
			Params:     []FnParam{{Name: pname, Type: TypeNodeOf(ptype)}},
			Returns:    []*Type{TypeNodeOf(rtype)},
			Impl:       Boru(bl.Slice()),
			BarrierPos: 1,
		}}})
		fnVal, _ := r.Defs.Top(coreSpecFnScratch)
		return fnVal, pos
	case "d(":
		// `d( … )` — a DISJUNCT of the listed items. The corpus could
		// name seven builtin type literals and nothing else, so the
		// whole type-CONSTRUCTOR surface — the part of unification that
		// actually differs between the ports — had no expression. Three
		// forms open it: this one, and the two typed containers below.
		pos++
		alts := []Value{}
		for pos < len(toks) && toks[pos] != ")" {
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			alts = append(alts, v)
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed d( in %s", strings.Join(toks, " "))
		}
		pos++
		return NewDisjunct(alts), pos
	case "[:":
		// `[: T e1 e2 … ]` — a typed list: the child constraint, then any
		// retained elements. `[: T ]` alone is the bare carrier.
		pos++
		var child Value
		child, pos = coreSpecItem(t, r, toks, pos)
		elems := []Value{}
		for pos < len(toks) && toks[pos] != "]" {
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			elems = append(elems, v)
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed [: in %s", strings.Join(toks, " "))
		}
		pos++
		if len(elems) == 0 {
			return NewTypedList(child), pos
		}
		return NewTypedListWithElements(child, elems), pos
	case "{:":
		// `{: T k: v … }` — a typed map: the child constraint, then any
		// retained entries.
		pos++
		var child Value
		child, pos = coreSpecItem(t, r, toks, pos)
		entries := []ChildEntry{}
		for pos < len(toks) && toks[pos] != "}" {
			key := toks[pos]
			if !strings.HasSuffix(key, ":") {
				t.Fatalf("typed-map key %q must end with ':'", key)
			}
			pos++
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			entries = append(entries, ChildEntry{Key: strings.TrimSuffix(key, ":"), Value: v})
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed {: in %s", strings.Join(toks, " "))
		}
		pos++
		if len(entries) == 0 {
			return NewTypedMap(child), pos
		}
		return NewTypedMapWithEntries(child, entries), pos
	case "p(":
		pos++
		items := []Value{}
		for pos < len(toks) && toks[pos] != ")" {
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			items = append(items, v)
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed p( in %s", strings.Join(toks, " "))
		}
		pos++ // the ")"
		return NewParenExpr(items), pos
	case "{", "{q":
		open := toks[pos]
		pos++
		m := NewOrderedMap()
		for pos < len(toks) && toks[pos] != "}" {
			key := toks[pos]
			if !strings.HasSuffix(key, ":") {
				t.Fatalf("map key %q must end with ':'", key)
			}
			pos++
			if pos >= len(toks) {
				t.Fatalf("map key %s has no value", key)
			}
			var v Value
			v, pos = coreSpecItem(t, r, toks, pos)
			m.Set(strings.TrimSuffix(key, ":"), v)
		}
		if pos >= len(toks) {
			t.Fatalf("unclosed { in %s", strings.Join(toks, " "))
		}
		pos++ // the "}"
		if open == "{" {
			return NewEvalMap(m), pos
		}
		return NewMap(m), pos
	}
	return coreSpecToken(toks[pos]), pos + 1
}

// coreSpecRegistry is the fixture: a bare registry plus ONE word, so the
// registry, signature matching, dispatch and the step loop are all exercised
// without a word library.
func coreSpecRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	r.Register("addq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsInteger(args[0])
			b, _ := AsInteger(args[1])
			return []Value{NewInteger(a + b)}, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	// One word reaches only the one dispatch shape. These four add the
	// shapes the step loop actually distinguishes — a STACK-form word, a
	// MULTI-return word, a gradual (Any) slot, and a handler that RAISES —
	// so a corpus row can exercise collection, residual layout, gradual
	// matching and error propagation rather than just forward addition.
	// core/ts/src/corespec.test.ts declares the same five independently;
	// any asymmetry here shows up as a false divergence, which is the
	// failure mode this corpus exists to prevent, so keep them in step.
	r.Register("negq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsInteger(args[0])
			return []Value{NewInteger(-a)}, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	r.Register("pairq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}},
		Returns: []*Type{TInteger, TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsInteger(args[0])
			return []Value{NewInteger(a), NewInteger(a)}, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	r.Register("sumq", Signature{
		Params:  []FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsInteger(args[0])
			b, _ := AsInteger(args[1])
			return []Value{NewInteger(a + b)}, nil
		}),
		BarrierPos: 0, // STACK form: both operands come off the stack
	})
	r.Register("boomq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}},
		Returns: []*Type{TAny},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return nil, &BoruError{Code: "fixture_boom", Detail: "the fixture word always raises"}
		}),
		BarrierPos: BarrierAllForward,
	})
	// patq carries a concrete-scalar PATTERN on its only slot: it matches
	// the Integer 7 and nothing else, so a row can pin that a pattern
	// narrows the overload rather than merely typing it.
	r.Register("patq", Signature{
		// The pattern goes on the PARAM, not the Patterns mirror:
		// NormalizeSig rebuilds Patterns from Params, so a directly-set
		// mirror is discarded. (Found by the probe — the two fixtures
		// disagreed, which is precisely the false divergence the
		// two-runner rule warns a fixture asymmetry produces.)
		Params:  []FnParam{{Name: "n", Type: TInteger, Pattern: patqPattern()}},
		Returns: []*Type{TInteger},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(1)}, nil
		}),
		BarrierPos: BarrierAllForward,
	})
	// dupq returns its operand TWICE through the identity-returns
	// mechanism (ReturnsIdentity / returnsIdentity): the returns are the
	// ARGS themselves rather than fresh values, which is what lets a
	// duplicating word keep its operand's identity.
	r.Register("dupq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}},
		Returns: []*Type{TAny, TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0], a[0]}, nil
		}),
		ReturnsFn:  ReturnsIdentity(0, 0),
		BarrierPos: BarrierAllForward,
	})
	// __pa pops the per-call args frame. It is NOT a core word — basic/go
	// registers it (native_definition.go). Every boru fn body's frame tail
	// emitted it until 2026-10-08, which is why a bare core registry could
	// not run a boru-bodied fn without the fixture; the frame's DefCleanup
	// marker does the pop itself now (stepDefCleanup), so the fixture is
	// kept for the dispatch rows that use it as an empty-result word
	// (core/spec/dispatch.tsv's `bothq __pa …` block). A fixture rather
	// than a port: the MECHANISM is core's (PopFrameArgs), only the word
	// is basic's.
	r.Register("__pa", Signature{
		Returns: []*Type{},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			return nil, PopFrameArgs(reg)
		}),
		BarrierPos: 0,
	})
	// undef removes a def binding. Like __pa it is a basic-layer word over
	// a mechanism core already owns (DefTable.Pop); the frame tail emitted
	// one `undef name` pair per named param until 2026-10-08 (the marker
	// tears the names down itself now), and the corpus still calls it as
	// a word.
	//
	// The name slot is TAtom, as basic/go's own `undef` declares it
	// (native_definition.go). That is not decoration: /q capture is
	// CONDITIONAL on the slot, and an Any-typed one collects the raw Word
	// instead of an Atom (measured — see core/spec/dispatch.tsv's /q
	// block). A fixture that took Any would be testing a shape no shipped
	// word has, and its handler would need a name-or-word polymorphism the
	// real one does not.
	r.Register("undef", Signature{
		Params:  []FnParam{{Name: "n", Type: TAtom, Quote: true}},
		Returns: []*Type{},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			n, err := AsAtom(a[0])
			if err != nil {
				return nil, err
			}
			reg.Defs.Pop(n)
			return nil, nil
		}),
		BarrierPos: 1,
	})
	// afn is the door onto CLOSURES: it builds an anonymous one-parameter
	// function AT RUN TIME — `afn n [q body… ]` — so the kernel's capture
	// analysis runs where a function is constructed (ComputeCaptures,
	// against the enclosing frame's baseline), which the `fn(` notation
	// cannot reach: it builds its function before the program starts. A
	// FIXTURE over mechanisms core owns, like __pa and unifyq.
	//
	// It carries the basic word's NAME on purpose. The frame-state analysis
	// keys on the words that can construct an inner function
	// (frameStateWords, fn_capture.go): a body that names none of them is a
	// leaf, enters with no baseline, and so captures nothing. A fixture
	// under another name would construct closures the kernel had been told
	// could not exist.
	r.Register("afn", Signature{
		Params:  []FnParam{{Name: "param", Type: TAtom, Quote: true}, {Name: "body", Type: TList}},
		Returns: []*Type{TFunction},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			name, err := AsAtom(a[0])
			if err != nil {
				return nil, err
			}
			body, err := AsList(a[1])
			if err != nil {
				return nil, err
			}
			sig := Signature{
				Params:     []FnParam{{Name: name, Type: TAny}},
				Returns:    []*Type{TAny},
				Impl:       Boru(body.Slice()),
				BarrierPos: 1,
			}
			return []Value{NewFunction(FnDefInfo{
				Signatures: []Signature{sig},
				Anonymous:  true,
				Captured:   ComputeCaptures(reg, &sig),
				Registry:   reg,
			})}, nil
		}),
		BarrierPos: 2,
	})
	// qanyq is the /q fixture whose name slot is ANY rather than Atom, and
	// it RETURNS what it captured, so a row can read the capture back. The
	// pair (undef's Atom slot, this Any one) is what pins /q's conditional
	// coercion: an Atom slot takes the Word→Atom conversion and an Any slot
	// — which the raw Word already fills — does not, so the handler is
	// handed a live Word that re-enters the tape and dispatches.
	r.Register("qanyq", Signature{
		Params:  []FnParam{{Name: "n", Type: TAny, Quote: true}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	// bothq takes TWO gradual args from the forward side. One-arg words
	// cannot reach the collection loop's middle: a marker that parks and
	// then collects TWICE, and one that meets `end` with its slots still
	// unfilled, both need an arity of two.
	r.Register("bothq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[1]}, nil
		}),
		BarrierPos: 2,
	})
	// tyq's only slot is a TYPE-ARG slot: it admits a type (a bare literal
	// or a structural type body) and declines every concrete value, which is
	// a different rule from "a slot whose declared type is Type". It
	// returns what it was given so a row can read the match back.
	r.Register("tyq", Signature{
		Params:   []FnParam{{Name: "t", Type: TAny}},
		TypeArgs: map[int]bool{0: true},
		Returns:  []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	// tpatq carries a TYPE-LITERAL pattern (rather than patq's concrete
	// scalar one) on a STACK slot. The distinction is load-bearing: a
	// type pattern is enforced only at stack positions, so this is the
	// shape that reaches the pattern matcher's type arm.
	r.Register("tpatq", Signature{
		Params:  []FnParam{{Name: "v", Type: TAny, Pattern: ptrTypeLit(TInteger)}},
		Returns: []*Type{TInteger},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 0,
	})
	// unifyq is the door onto UNIFICATION — the one core mechanism this
	// corpus could not reach at all. Its clients (`is`, `case`, typed defs,
	// record shapes) are basic-layer words, so nothing in a bare core
	// registry called the unifier, and the most fundamental thing core does
	// after dispatch went uncompared between the two ports. The word is a
	// FIXTURE over a mechanism core owns, exactly like __pa over the args
	// stack.
	//
	// It returns the PREDICATE half only — did the two values unify — and
	// not the unified value, because core/ts has a predicate and no meet.
	// Asking a question one engine cannot answer would produce rows that
	// pin a port gap as a language contract; the value half lands with the
	// real port.
	r.Register("unifyq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		Returns: []*Type{TBoolean},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			_, ok := UnifyR(a[0], a[1], reg)
			return []Value{NewBoolean(ok)}, nil
		}),
		BarrierPos: 2,
	})
	// depthq is the FULL-STACK fixture: the handler receives the whole
	// resolved stack of the current paren scope and returns its complete
	// replacement, rather than N args and their replacement. One word is
	// enough to exercise the capability — the scope rule (a paren bounds
	// what it can see) is what the corpus rows actually pin.
	r.Register("depthq", Signature{
		Impl: Go(func(_ []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
			return append(append([]Value(nil), stack...), NewInteger(int64(len(stack)))), nil
		}, FullStack()),
		BarrierPos: 0,
	})
	return r
}

// evalCoreSpec turns one row's expression into its rendered result, or into
// an "ERROR:<code>" string.
func evalCoreSpec(t *testing.T, r *Registry, expr string) string {
	t.Helper()
	kind, arg, _ := strings.Cut(expr, " ")
	render := func(vs []Value) string {
		parts := make([]string, 0, len(vs))
		for _, v := range vs {
			parts = append(parts, CanonValue(v))
		}
		return strings.Join(parts, " ")
	}
	switch kind {
	case "int":
		n, err := strconv.ParseInt(arg, 10, 64)
		if err != nil {
			t.Fatalf("int %q: %v", arg, err)
		}
		return CanonValue(NewInteger(n))
	case "bigint":
		n, ok := new(big.Int).SetString(arg, 10)
		if !ok {
			t.Fatalf("bigint %q: not a base-10 integer", arg)
		}
		return CanonValue(NewBigInteger(n))
	case "bigdec":
		d, _, err := apd.NewFromString(arg)
		if err != nil {
			t.Fatalf("bigdec %q: %v", arg, err)
		}
		return CanonValue(NewBigDecimal(d))
	case "float":
		f, err := strconv.ParseFloat(arg, 64)
		if err != nil {
			t.Fatalf("float %q: %v", arg, err)
		}
		return CanonValue(NewFloat(f))
	case "str":
		return CanonValue(NewString(arg))
	case "bool":
		return CanonValue(NewBoolean(arg == "true"))
	case "none":
		return CanonValue(NewNone())
	case "end":
		return CanonValue(NewEnd())
	case "closeparen":
		return CanonValue(NewCloseParen())
	case "dispatchmod":
		return CanonValue(NewDispatchMod(DispatchModInfo{Val: arg == "v", Quote: arg == "q"}))
	case "typedlist":
		toks := coreSpecFields(arg)
		child := coreSpecToken(toks[0])
		var elems []Value
		for _, tok := range toks[1:] {
			elems = append(elems, coreSpecToken(tok))
		}
		return CanonValue(NewTypedListWithElements(child, elems))
	case "typedmap":
		toks := coreSpecFields(arg)
		child := coreSpecToken(toks[0])
		var entries []ChildEntry
		for _, tok := range toks[1:] {
			k, v, _ := strings.Cut(tok, ":")
			entries = append(entries, ChildEntry{Key: k, Value: coreSpecToken(v)})
		}
		return CanonValue(NewTypedMapWithEntries(child, entries))
	case "typelit":
		tl, ok := coreSpecTypeLit(arg)
		if !ok {
			t.Fatalf("typelit %q: not a corpus-known type name", arg)
		}
		return CanonValue(tl)
	case "list":
		return CanonValue(NewList(coreSpecAssemble(t, r, coreSpecFields(arg))))
	case "run":
		// A leading `def NAME <item> ;` clause installs a def BINDING before
		// the program runs, which is the only way the corpus can reach the
		// engine's def-substitution paths — nothing in the fixture
		// vocabulary binds a name. `;` ends the clause; several may stack.
		toks := coreSpecFields(arg)
		for len(toks) > 0 && toks[0] == "def" {
			var bound Value
			bound, pos := coreSpecItem(t, r, toks, 2)
			if pos >= len(toks) || toks[pos] != ";" {
				t.Fatalf("def clause for %q must end with ';'", toks[1])
			}
			r.Defs.Push(toks[1], bound)
			toks = toks[pos+1:]
		}
		out, err := NewTop(r).Run(coreSpecAssemble(t, r, toks))
		if err != nil {
			var be *BoruError
			if errors.As(err, &be) {
				return "ERROR:" + be.Code
			}
			return "ERROR:non_boru:" + err.Error()
		}
		return render(out)
	}
	t.Fatalf("unknown expression kind %q", kind)
	return ""
}

// coreSpecDivergentFile is the parity DEBT and has a different column shape,
// so TestCoreSpec skips it and TestCoreSpecDivergent reads it instead.
const coreSpecDivergentFile = "divergent.tsv"

// patqPattern is the concrete-scalar pattern on patq's only slot.
// FnParam.Pattern is a *Value, so it needs an addressable one.
func patqPattern() *Value {
	v := NewInteger(7)
	return &v
}

func TestCoreSpec(t *testing.T) {
	entries, err := os.ReadDir(coreSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", coreSpecDir, err)
	}
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsv") || e.Name() == coreSpecDivergentFile {
			continue
		}
		path := filepath.Join(coreSpecDir, e.Name())
		rows := parseCoreSpec(t, path)
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			for _, row := range rows {
				total++
				got := evalCoreSpec(t, coreSpecRegistry(t), row.expr)
				if got != row.expected {
					t.Errorf("%s:%d  %s\n  got  %s\n  want %s\n  (%s)",
						row.file, row.line, row.expr, got, row.expected, row.note)
				}
			}
		})
	}
	if total == 0 {
		t.Fatal("core/spec produced no rows — the corpus is not being read")
	}
	t.Logf("core/spec: %d rows", total)
}

// TestCoreProbe is the Go half of scripts/core-probe.sh — a developer probe,
// not a gate. It renders each candidate expression exactly as evalCoreSpec
// does, so a probe result can be pasted straight into a corpus file. Without
// CORE_PROBE_IN set it is a no-op, so `go test ./...` is unaffected.
//
// Why a probe at all: a corpus row is only honest if BOTH engines were asked
// before it was written down. The row's `expected` is still the CONTRACT —
// this corpus is a spec, not a differential — but a row neither engine
// produces should be a deliberate red row, not a surprise.
func TestCoreProbe(t *testing.T) {
	in, out := os.Getenv("CORE_PROBE_IN"), os.Getenv("CORE_PROBE_OUT")
	if in == "" || out == "" {
		t.Skip("CORE_PROBE_IN / CORE_PROBE_OUT unset — probe is opt-in")
	}
	src, err := os.ReadFile(in) //nolint:gosec // developer probe, path from env
	if err != nil {
		t.Fatalf("open %s: %v", in, err)
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(src), "\n"), "\n") {
		if line == "" {
			continue
		}
		b.WriteString(evalCoreSpec(t, coreSpecRegistry(t), line))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}
}

// TestCoreSpecDivergent pins the Go side of every recorded core-level
// divergence, and fails a row that has STOPPED diverging — a fixed divergence
// must move to the spec file it belongs in, not sit here looking like debt.
// core/ts/src/corespec.test.ts asserts the other column from the same file,
// so both suites stay green while the difference stays visible. Same
// discipline as parser/spec/divergent.tsv.
func TestCoreSpecDivergent(t *testing.T) {
	path := filepath.Join(coreSpecDir, coreSpecDivergentFile)
	rows := parseCoreSpec(t, path)
	if len(rows) == 0 {
		// The debt is paid. Kept as a live assertion rather than deleted:
		// the file is the ratchet, so a NEW divergence has to be added here
		// deliberately instead of quietly landing as a changed expectation
		// somewhere else.
		t.Log("divergent.tsv: empty — core/go and core/ts agree on every corpus expression")
		return
	}
	for _, row := range rows {
		// parseCoreSpec puts column 2 in expected and column 3 in note; for
		// this file those are the go and ts columns.
		wantGo, wantTS := row.expected, row.note
		if wantGo == wantTS {
			t.Errorf("%s:%d: %q: go and ts columns are identical — move this row out of divergent.tsv",
				row.file, row.line, row.expr)
			continue
		}
		// A registry of its own per row, as TestCoreSpec gives each: one
		// shared across the file let a row's `def` clauses stack onto the
		// next row's, so a name defined in two rows had two definitions in
		// the second.
		if got := evalCoreSpec(t, coreSpecRegistry(t), row.expr); got != wantGo {
			t.Errorf("%s:%d: %q (go column)\n  want: %s\n  got : %s",
				row.file, row.line, row.expr, wantGo, got)
		}
	}
	t.Logf("core/spec divergent: %d rows", len(rows))
}

// ptrTypeLit is the addressable type literal FnParam.Pattern wants.
func ptrTypeLit(t *Type) *Value {
	v := NewTypeLiteral(t)
	return &v
}
