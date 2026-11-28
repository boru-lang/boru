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
	"fmt"
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

// coreSpecFnType names a type a `fn(` parameter or return declares: the
// seven the notation spells as literals, and the two nodes between Any and
// Integer, so a row can declare a chain of three.
func coreSpecFnType(name string) (*Type, bool) {
	switch name {
	case "Number":
		return TNumber, true
	case "Scalar":
		return TScalar, true
	}
	if lit, ok := coreSpecTypeLit(name); ok {
		return TypeNodeOf(lit), true
	}
	return nil, false
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

// coreSpecNotAList is a fixture's refusal of a List-typed argument with no
// elements to read: a TYPED list that retains none (`[: Integer ]`) is
// list-shaped to dispatch — it fills a List slot — and is a type, not a
// sequence, to the handler. A coded error, so the corpus can pin it: a raw
// Go error would surface as no boru error at all.
func coreSpecNotAList(word string, v Value) error {
	return &BoruError{Code: "fixture_not_a_list", Detail: word + ": no elements to read in " + CanonValue(v)}
}

// coreSpecEach is eachq's driver: an iteration per element, the element as
// the region's inert input, every residual value kept.
type coreSpecEach struct {
	elems []Value
	body  Value
	out   []Value
}

func (d *coreSpecEach) Next(it int) ([]Value, Value, bool, error) {
	if it >= len(d.elems) {
		return nil, Value{}, false, nil
	}
	return []Value{d.elems[it]}, d.body, true, nil
}

func (d *coreSpecEach) Collect(_ int, residual []Value) error {
	// The residual is valid only for the call: keep copies.
	d.out = append(d.out, residual...)
	return nil
}

func (d *coreSpecEach) Finish() ([]Value, error) { return []Value{NewList(d.out)}, nil }
func (d *coreSpecEach) Describe() string         { return "eachq" }

// A body's error is reported as the body's: the corpus reads error CODES.
func (d *coreSpecEach) WrapError(_ int, err error) error { return err }

// coreSpecFor builds a counted loop's first round — the index bound, then
// `mark body… move` — as basic's RunForLoop does: an empty range runs
// nothing, a zero step is refused, and the move's Loop counts from there.
func coreSpecFor(r *Registry, word string, start, end, step int64, body Value) ([]Value, error) {
	bl, err := AsList(body)
	if err != nil {
		return nil, coreSpecNotAList(word, body)
	}
	if step == 0 {
		return nil, &BoruError{Code: "fixture_zero_step", Detail: word + ": the step cannot be zero"}
	}
	if (step > 0 && start >= end) || (step < 0 && start <= end) {
		return nil, nil
	}
	InstallDef(r, "i", NewInteger(start))
	lp := &Loop{
		Registry: r, Word: word, Iter: 1, Count: -1,
		IterName: "i", Current: start, End: end, Step: step,
		Body: bl.Slice(), IterDepth: r.Defs.Depth("i"),
	}
	id := NextMarkID()
	toks := append([]Value{NewMark(id)}, lp.Body...)
	return append(toks, NewMoveCont(id, word+" loop", lp)), nil
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
		// fn( NAME PTYPE … RTYPE [ body ] ) — the parameters in signature
		// order, one return, a boru body. INSTALLED through
		// InstallFnDef, never assembled by hand — only installation builds
		// each Signature's body-splicing Handler (buildFnBodyHandler), and
		// the engine calls that handler unconditionally, so a hand-built
		// FnDefInfo panics into internal_error.
		pos++
		head := pos
		for pos < len(toks) && toks[pos] != "[" && toks[pos] != "[q" {
			pos++
		}
		spec := toks[head:pos]
		// `fn( RTYPE [ body ] )` — the return alone — is a function of NO
		// parameters.
		if len(spec)%2 == 0 {
			t.Fatalf("fn( wants NAME PTYPE … RTYPE before its body, got %q", spec)
		}
		// The NAMED type itself, as core/ts's runner declares it. A type
		// literal IS its lattice node, so Parent() here would be the node's
		// PARENT — Number for Integer, and nil for Any, which no slot
		// matches (found by spec/tools/diff-fuzz.py).
		params := make([]FnParam, 0, len(spec)/2)
		for i := 0; i+1 < len(spec); i += 2 {
			pt, ok := coreSpecFnType(spec[i+1])
			if !ok {
				t.Fatalf("fn( %s: %q is not a corpus-known type name", spec[i], spec[i+1])
			}
			// `_` declares an UNNAMED parameter: its argument is not bound,
			// it is spliced at the head of the call frame as stack data
			name := spec[i]
			if name == "_" {
				name = ""
			}
			params = append(params, FnParam{Name: name, Type: pt})
		}
		rtype, ok := coreSpecFnType(spec[len(spec)-1])
		if !ok {
			t.Fatalf("fn( return: %q is not a corpus-known type name", spec[len(spec)-1])
		}
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
			Params:     params,
			Returns:    []*Type{rtype},
			Impl:       Boru(bl.Slice()),
			BarrierPos: len(params),
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
	case "s(":
		// `s( item )` — a SPLICE MARKER (`__SP`) wrapping one item: stepped
		// with nothing waiting for it, it is replaced by its payload — a
		// plain list's elements, any other value itself.
		pos++
		var payload Value
		payload, pos = coreSpecItem(t, r, toks, pos)
		if pos >= len(toks) || toks[pos] != ")" {
			t.Fatalf("s( wants one item and a closing ) in %s", strings.Join(toks, " "))
		}
		pos++
		return NewSplice(payload), pos
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
	// def binds a name AT RUN TIME — `def name value` — which is what a
	// function's body needs to make a definition of its OWN: the leading
	// `def NAME item ;` clauses of a row bind before the program starts, so
	// they can never be inside a frame. The mechanism is core's
	// (InstallDef — the kernel's definition install — and the frame's
	// truncation of what its body bound);
	// the word is basic's, and it carries basic's NAME on purpose, as afn
	// and undef do: the kernel's frame analysis decides by name whether a
	// body can bind (frameStateWords), and a body that cannot gets a frame
	// with no snapshot to truncate to.
	r.Register("def", Signature{
		Params:  []FnParam{{Name: "n", Type: TAtom, Quote: true}, {Name: "v", Type: TAny}},
		Returns: []*Type{},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			n, err := AsAtom(a[0])
			if err != nil {
				return nil, err
			}
			// The kernel's own definition install: a plain value is
			// pushed; a function is compiled under the name — its optional
			// parameters expanded, its body given the frame runner — and
			// an older definition of the name whose parameter types it
			// repeats is taken off first.
			InstallDef(reg, n, a[1])
			return nil, nil
		}),
		BarrierPos: 2,
	})
	// fn is the door onto THE FUNCTION-SPEC READER (ParseFnDef and the
	// readers beneath it): `fn [q [q params ] [q returns ] [q body ] … ]`
	// answers the function the specification describes — triples, one per
	// overload. The value is as the reader leaves it: a function VALUE,
	// applied where it stands, and compiled when `def` binds it. Named as
	// the shipped word is, for the frame analysis (a body that builds a
	// function needs the baseline). The Go runner's and the Lean runner's
	// only: core/ts has no reader, and its rows are in the ledger.
	r.Register("fn", Signature{
		Params:  []FnParam{{Name: "spec", Type: TList}},
		Returns: []*Type{TFunction},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			l, err := AsList(a[0])
			if err != nil {
				return nil, coreSpecNotAList("fn", a[0])
			}
			elems := l.Slice()
			if len(elems) == 0 || len(elems)%3 != 0 {
				return nil, &BoruError{Code: "fn_invalid_spec", Detail: "fn: list length must be a non-zero multiple of 3"}
			}
			info, err := ParseFnDef(reg, elems)
			if err != nil {
				return nil, &BoruError{Code: "fn_invalid_spec", Detail: err.Error()}
			}
			return []Value{NewFunction(info)}, nil
		}),
		BarrierPos: 1,
	})
	// bindq is def under a name the frame analysis does NOT know. A body
	// that names no binding word is a LEAF — its frame takes no snapshot —
	// so what bindq binds there is not the frame's to truncate, and stays
	// bound after the call. It stands for every word that binds from the
	// host without being on the kernel's list (lang's `unpack` is the
	// shipped one; NUR384).
	//
	// It binds as the kernel's own binders do — through InstallDef. A raw
	// Defs.Push of a FUNCTION value leaves a body with no runner under a
	// name, a state no kernel binder makes at run time: called by that name
	// it is refused as an internal error (execMatch's nil-handler arm). The
	// fixture pushed raw until 2026-11-28, and a fuzzer found the arm.
	r.Register("bindq", Signature{
		Params:  []FnParam{{Name: "n", Type: TAtom, Quote: true}, {Name: "v", Type: TAny}},
		Returns: []*Type{},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			n, err := AsAtom(a[0])
			if err != nil {
				return nil, err
			}
			InstallDef(reg, n, a[1])
			return nil, nil
		}),
		BarrierPos: 2,
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
				return nil, coreSpecNotAList("afn", a[1])
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
	// typeq is the door onto TYPE INSTALLATION (InstallType, the one entry
	// basic's `def Name body` goes through for a capitalised name): it
	// validates the name and the body, mints the type's lattice node — or,
	// for a bare type, adopts the node it aliases — and binds the name to
	// it. Every refusal of the installer is the installer's own error.
	r.Register("typeq", Signature{
		Params:  []FnParam{{Name: "n", Type: TAtom, Quote: true}, {Name: "body", Type: TAny}},
		Returns: []*Type{},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			n, err := AsAtom(a[0])
			if err != nil {
				return nil, err
			}
			return nil, InstallType(reg, n, a[1])
		}),
		BarrierPos: 2,
	})
	// afn0q builds an anonymous function of NO parameters — `afn0q [q body…
	// ]` — which afn, taking one parameter name, cannot. Such a function is
	// the one value whose call needs nothing beside it, and the kernel
	// treats it as DATA where it stands: only an explicit application calls
	// it. (Under this name the frame-state analysis does not know it as a
	// constructing word: built in a function's body it captures nothing.)
	r.Register("afn0q", Signature{
		Params:  []FnParam{{Name: "body", Type: TList}},
		Returns: []*Type{TFunction},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			body, err := AsList(a[0])
			if err != nil {
				return nil, coreSpecNotAList("afn0q", a[0])
			}
			sig := Signature{
				Params:     []FnParam{},
				Returns:    []*Type{TAny},
				Impl:       Boru(body.Slice()),
				BarrierPos: 0,
			}
			return []Value{NewFunction(FnDefInfo{
				Signatures: []Signature{sig},
				Anonymous:  true,
				Captured:   ComputeCaptures(reg, &sig),
				Registry:   reg,
			})}, nil
		}),
		BarrierPos: 1,
	})
	// applyq is the APPLICATION MARK (MarkApplied, basic's `apply`): it
	// stamps the one-shot signal on an anonymous function whose signatures
	// all take nothing and hands it back — and the re-step, finding the
	// mark, calls it. Any other value is handed back as it is.
	r.Register("applyq", Signature{
		Params:  []FnParam{{Name: "f", Type: TAny}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{MarkApplied(a[0])}, nil
		}),
		BarrierPos: 1,
	})
	// eachq is the door onto DRIVEN LOOPS: the protocol a looping native
	// uses to run its iterations on the tape (LoopDriver, loop.go). Its
	// handler hands StartLoop a driver; the engine splices one sealed
	// region per element — `mark (ₗ element body… ) move` — steps the body
	// where the word stands, and hands each region's residual back.
	//
	// The driver is the simplest that shows the protocol whole: one
	// iteration per element, the element the region's one inert input, and
	// EVERYTHING a region leaves collected, in order, into the one list
	// the loop answers. So a row reads off what a body saw (its input, and
	// nothing beneath it), what it left, and what was never re-stepped.
	r.Register("eachq", Signature{
		Params:  []FnParam{{Name: "xs", Type: TList}, {Name: "body", Type: TList}},
		Returns: []*Type{TList},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			xs, err := AsList(a[0])
			if err != nil {
				return nil, coreSpecNotAList("eachq", a[0])
			}
			// out starts EMPTY, not nil: the list the loop answers is then a
			// list whatever the rounds left. A list built over a nil slice is
			// "no backing data" to a dozen kernel readers (ReadList.IsNil) —
			// as a body it is one token, not none (NUR403) — and the corpus
			// is about lists, not about Go slices.
			return StartLoop(reg, "eachq", &coreSpecEach{elems: xs.Slice(), body: a[1], out: []Value{}})
		}),
		BarrierPos: 2,
	})
	// forq, rangeq and whileq are the doors onto the COUNTED and the
	// CONDITIONAL loop: a mark, a body, and a move whose continuation
	// (core.Loop) replays the body — `mark body… move`, with NO sealing
	// paren, so a body runs on the caller's own stack. The words that build
	// those tokens are basic's (`for`, `while`); the continuation and its
	// stepping are the kernel's.
	//
	// forq n [q body ] runs the body for i = 0 … n-1, the index bound as
	// `i`; rangeq start end step [q body ] counts any range; whileq
	// [q cond ] [q body ] alternates a condition region and a body region.
	// Each leaves, where it stood, every value its completed rounds left.
	r.Register("forq", Signature{
		Params: []FnParam{{Name: "n", Type: TInteger}, {Name: "body", Type: TList}},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			n, _ := AsInteger(a[0])
			return coreSpecFor(reg, "forq", 0, n, 1, a[1])
		}),
		BarrierPos: 2,
	})
	r.Register("rangeq", Signature{
		Params: []FnParam{
			{Name: "start", Type: TInteger}, {Name: "end", Type: TInteger},
			{Name: "step", Type: TInteger}, {Name: "body", Type: TList},
		},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			start, _ := AsInteger(a[0])
			end, _ := AsInteger(a[1])
			step, _ := AsInteger(a[2])
			return coreSpecFor(reg, "rangeq", start, end, step, a[3])
		}),
		BarrierPos: 4,
	})
	r.Register("whileq", Signature{
		Params: []FnParam{{Name: "cond", Type: TList}, {Name: "body", Type: TList}},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			cond, err := AsList(a[0])
			if err != nil {
				return nil, coreSpecNotAList("whileq", a[0])
			}
			body, err := AsList(a[1])
			if err != nil {
				return nil, coreSpecNotAList("whileq", a[1])
			}
			lp := &Loop{Registry: reg, Word: "whileq", Count: -1, Body: body.Slice(), WhileCond: cond.Slice()}
			id := NextMarkID()
			toks := append([]Value{NewMark(id)}, lp.WhileCond...)
			return append(toks, NewMoveCont(id, "whileq loop", lp)), nil
		}),
		BarrierPos: 2,
	})
	// ifq and replayq are the doors onto the two OTHER things a move does.
	//
	// ifq [q cond ] [q then ] [q else ] is the `if` continuation (IfCont): a
	// condition region, whose last value picks the branch that takes the
	// region's place. The branches are laid out as basic's `if` lays them
	// out — each a paren group of its own.
	//
	// replayq [q body ] is the plain mark and move, with no continuation at
	// all: the mark carries the body, the body runs once between the two,
	// and the move puts the mark's body where the region stood — to run
	// again. No shipped word builds one; the step loop still steps it.
	r.Register("ifq", Signature{
		Params: []FnParam{{Name: "cond", Type: TList}, {Name: "then", Type: TList}, {Name: "else", Type: TList}},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			parts := make([][]Value, 3)
			for i := range parts {
				l, err := AsList(a[i])
				if err != nil {
					return nil, coreSpecNotAList("ifq", a[i])
				}
				parts[i] = l.Slice()
			}
			group := func(body []Value) []Value {
				out := append([]Value{NewOpenParen()}, body...)
				return append(out, NewCloseParen())
			}
			id := NextMarkID()
			toks := append([]Value{NewMark(id)}, parts[0]...)
			return append(toks, NewMoveIf(id, "ifq", &IfCont{Then: group(parts[1]), Else: group(parts[2])})), nil
		}),
		BarrierPos: 3,
	})
	r.Register("replayq", Signature{
		Params: []FnParam{{Name: "body", Type: TList}},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			l, err := AsList(a[0])
			if err != nil {
				return nil, coreSpecNotAList("replayq", a[0])
			}
			body := l.Slice()
			id := NextMarkID()
			toks := append([]Value{NewMark(id, body...)}, body...)
			return append(toks, NewMove(id, "replayq")), nil
		}),
		BarrierPos: 1,
	})
	// argsq is the door onto the ARGS STACK (Registry.Args): every call of a
	// boru-bodied function pushes the list of its arguments, in signature
	// order, and its frame's cleanup pops it. argsq answers the list of the
	// call it runs in. The word that reads it is basic's (`args`); the stack,
	// its push and its pop are the kernel's. (`__pa`, above, is the pop as a
	// word.)
	r.Register("argsq", Signature{
		Returns: []*Type{TList},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			top, ok, err := reg.Args.Top()
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, &BoruError{Code: "fixture_no_args", Detail: "argsq: not inside a function"}
			}
			return []Value{top}, nil
		}),
		BarrierPos: 0,
	})
	// breakq and contq raise the two FLOW SIGNALS (Registry.FlowCtrl): not
	// errors, a redirection the run resolves after the step — the nearest
	// counted or conditional loop ahead on the tape ends, or starts its
	// next round. With no loop to take it the signal is a flow_error.
	r.Register("breakq", Signature{
		Returns: []*Type{},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			reg.FlowCtrl = FlowBreak
			return nil, nil
		}),
		BarrierPos: 0,
	})
	r.Register("contq", Signature{
		Returns: []*Type{},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			reg.FlowCtrl = FlowContinue
			return nil, nil
		}),
		BarrierPos: 0,
	})
	// callq is the door onto the CALL REGION (CallRegion, loop.go): what a
	// native returns when its last act is running a body — `case`'s matched
	// block. It is a driven loop of ONE round: the input enters sealed and
	// inert, the body steps where the word stands, and the region's
	// residual replaces it AS THE WORD'S RESULTS — stepped again where the
	// word stood, where eachq's are handed to its driver and never stepped.
	r.Register("callq", Signature{
		Params: []FnParam{{Name: "x", Type: TAny}, {Name: "body", Type: TList}},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			return CallRegion(reg, "callq", "callq body", []Value{a[0]}, a[1])
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
	// The RAW-CAPTURE slots: four ways a signature takes an operand as it
	// was written rather than as it evaluates. rawlq's slot leaves a pending
	// LIST unevaluated and rawmq's a pending MAP — the two are separate
	// fields, so a map at rawlq's slot evaluates and a list at rawmq's does.
	// Each returns what it was handed.
	r.Register("rawlq", Signature{
		Params:     []FnParam{{Name: "x", Type: TAny}},
		NoEvalArgs: map[int]bool{0: true},
		Returns:    []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	r.Register("rawmq", Signature{
		Params:        []FnParam{{Name: "x", Type: TAny}},
		NoEvalMapArgs: map[int]bool{0: true},
		Returns:       []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{a[0]}, nil
		}),
		BarrierPos: 1,
	})
	// rawpq's slot captures a forward PAREN EXPRESSION as code, and formq's
	// captures ANY form — a word stays a word. A captured word or paren
	// handed back bare would re-enter the tape and run, so each returns its
	// capture inside a one-element list, where a row can read it.
	r.Register("rawpq", Signature{
		Params:    []FnParam{{Name: "x", Type: TAny}},
		RawParens: map[int]bool{0: true},
		Returns:   []*Type{TList},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList([]Value{a[0]})}, nil
		}),
		BarrierPos: 1,
	})
	r.Register("formq", Signature{
		Params:   []FnParam{{Name: "x", Type: TAny}},
		FormArgs: map[int]bool{0: true},
		Returns:  []*Type{TList},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList([]Value{a[0]})}, nil
		}),
		BarrierPos: 1,
	})
	// raw2q and form2q put the raw slot SECOND of two, so a row can tell
	// which position a capture belongs to; each returns both arguments in
	// signature order.
	r.Register("raw2q", Signature{
		Params:     []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		NoEvalArgs: map[int]bool{1: true},
		Returns:    []*Type{TList},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList([]Value{a[0], a[1]})}, nil
		}),
		BarrierPos: 2,
	})
	r.Register("form2q", Signature{
		Params:   []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		FormArgs: map[int]bool{1: true},
		Returns:  []*Type{TList},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewList([]Value{a[0], a[1]})}, nil
		}),
		BarrierPos: 2,
	})
	// spliceq makes a SPLICE MARKER of its argument, as basic's `word` does.
	// Handed back, the marker is at the pointer with nothing waiting for
	// it, and fires: `spliceq [q 1 2 ]` leaves 1 and 2.
	r.Register("spliceq", Signature{
		Params:  []FnParam{{Name: "v", Type: TAny}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewSplice(a[0])}, nil
		}),
		BarrierPos: 1,
	})
	// THE PATTERN'S PENDING OPERAND (NUR235): a pattern that reads a
	// container's contents — a map shape, a typed list — judges the value
	// the callee receives, so a pending list or map standing at its slot is
	// EVALUATED by the dispatch before the pattern reads it. mpatq (a map
	// that must hold a:1) and lpatq (a list of Integers) are that rule;
	// rawmpatq and rawlpatq put the same patterns on slots that take their
	// operand RAW, which are exempt — the pattern judges what was written.
	// Each returns what it was handed.
	for _, fx := range []struct {
		name    string
		typ     *Type
		pattern Value
		rawList bool
		rawMap  bool
	}{
		{"mpatq", TMap, mapA1Pattern(), false, false},
		{"rawmpatq", TMap, mapA1Pattern(), false, true},
		{"lpatq", TList, NewTypedList(NewTypeLiteral(TInteger)), false, false},
		{"rawlpatq", TList, NewTypedList(NewTypeLiteral(TInteger)), true, false},
	} {
		pattern := fx.pattern
		sig := Signature{
			Params:  []FnParam{{Name: "x", Type: fx.typ, Pattern: &pattern}},
			Returns: []*Type{TAny},
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{a[0]}, nil
			}),
			BarrierPos: 1,
		}
		if fx.rawList {
			sig.NoEvalArgs = map[int]bool{0: true}
		}
		if fx.rawMap {
			sig.NoEvalMapArgs = map[int]bool{0: true}
		}
		r.Register(fx.name, sig)
	}
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
	// It returns the PREDICATE half — did the two values unify. The VALUE
	// half is `meetq` below: the two were one word short while core/ts had
	// a predicate and no meet, and a question one engine cannot answer
	// pins a port gap as a language contract.
	r.Register("unifyq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		Returns: []*Type{TBoolean},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			_, ok := UnifyR(a[0], a[1], reg)
			return []Value{NewBoolean(ok)}, nil
		}),
		BarrierPos: 2,
	})
	// meetq is unifyq's VALUE half: the unified value itself — the meet of
	// the two — or the fixture error `fixture_no_meet` when they do not
	// unify. The predicate cannot tell `5 tor 6` from `5` as the answer to
	// `d( 5 6 )` against `Integer`; this can.
	r.Register("meetq", Signature{
		Params:  []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		Returns: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
			meet, ok := UnifyR(a[0], a[1], reg)
			if !ok {
				return nil, &BoruError{Code: "fixture_no_meet", Detail: "the two values do not unify"}
			}
			return []Value{meet}, nil
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
		eng := NewTop(r)
		if os.Getenv("CORE_PROBE_TRACE") != "" {
			// the probe's own view of a run: the tape and the pointer at
			// every step, to stderr
			eng.SetTrace(func(step, pointer int, tape []Value, note string) {
				cells := make([]string, len(tape))
				for i := range tape {
					cells[i] = CanonValue(tape[i])
				}
				fmt.Fprintf(os.Stderr, "%4d @%d  %s   %s\n", step, pointer, strings.Join(cells, " · "), note)
			})
		}
		out, err := eng.Run(coreSpecAssemble(t, r, toks))
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

// mapA1Pattern is the map pattern {a:1}: a map that holds a:1 (and, the
// match being open, anything else beside it).
func mapA1Pattern() Value {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	return NewMap(m)
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
		// A line the notation cannot read fails ITS subtest and answers
		// NOTATION, as the Lean probe answers it; the lines after it are
		// still evaluated, and the file is still written.
		got := "NOTATION"
		t.Run("line", func(st *testing.T) {
			// a truncated form can run the notation's reader off its tokens
			defer func() {
				if recover() != nil {
					got = "NOTATION"
				}
			}()
			got = evalCoreSpec(st, coreSpecRegistry(st), line)
		})
		b.WriteString(got)
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

// TestCoreSpecTailCallsUnobservable holds the kernel's tail-call elision to
// its contract: it changes what a deep chain of calls COSTS, never what a
// program answers. Every corpus row is evaluated a second time with elision
// switched off (Registry.TCO.Disable, the diagnostic switch) and must give
// the answer the row records — the answer of plain nesting, which is also
// what the Lean specification computes: its machine nests every call and
// has no elision to get wrong.
//
// The second assertion keeps the first from going vacuous: the corpus must
// actually reach both forms of elision — the full replacement of a clean
// frame and the shell variant that keeps the caller's parens and return
// check — or the comparison is between two runs that nested alike.
//
// NUR407 is where the contract does not hold today: a callee that unbinds
// its own parameter reads the caller's binding of that name under nesting,
// and the caller's is already gone under elision. The ledger pins those
// rows with the nested answer in its ts column (core/ts only nests), and
// this test holds them to exactly that — elided, the go column; nested,
// the ts column — and to their number, so the fix has to take them out.
const coreSpecNUR407Rows = 2

func TestCoreSpecTailCallsUnobservable(t *testing.T) {
	entries, err := os.ReadDir(coreSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", coreSpecDir, err)
	}
	var detected, elided, replaced int64
	total, observable := 0, 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		ledger := e.Name() == coreSpecDivergentFile
		// parseCoreSpec puts column 2 in expected: the contract in a shared
		// file, the go column in the ledger — where column 3, its note
		// field, is the ts column.
		for _, row := range parseCoreSpec(t, filepath.Join(coreSpecDir, e.Name())) {
			total++
			on := coreSpecRegistry(t)
			if got := evalCoreSpec(t, on, row.expr); got != row.expected {
				// TestCoreSpec and TestCoreSpecDivergent report this row;
				// here it would only be noise
				continue
			}
			detected += on.TCO.Detected
			elided += on.TCO.Elided
			replaced += on.TCO.Replaced
			off := coreSpecRegistry(t)
			off.TCO.Disable = true
			if got := evalCoreSpec(t, off, row.expr); got != row.expected {
				if ledger && got == row.note {
					// NUR407: nested, the row answers as its ts column
					observable++
				} else {
					t.Errorf("%s:%d  %s\n  with elision    %s\n  without elision %s",
						row.file, row.line, row.expr, row.expected, got)
				}
			}
			if off.TCO.Elided+off.TCO.Replaced != 0 {
				t.Errorf("%s:%d  %s: elision ran with the switch off", row.file, row.line, row.expr)
			}
		}
	}
	if total == 0 {
		t.Fatal("core/spec produced no rows — the corpus is not being read")
	}
	if observable != coreSpecNUR407Rows {
		t.Errorf("elision changes the answer of %d ledger rows, want %d (NUR407): a row it no longer changes leaves %s, and a new one is a new defect",
			observable, coreSpecNUR407Rows, coreSpecDivergentFile)
	}
	if elided == 0 || replaced == 0 {
		t.Errorf("the corpus no longer reaches both forms of elision (shell %d, full replacement %d, of %d tail calls detected): the comparison above proves nothing",
			elided, replaced, detected)
	}
	t.Logf("core/spec: %d rows; %d tail calls detected, %d elided to a shell, %d replaced", total, detected, elided, replaced)
}

// ptrTypeLit is the addressable type literal FnParam.Pattern wants.
func ptrTypeLit(t *Type) *Value {
	v := NewTypeLiteral(t)
	return &v
}
