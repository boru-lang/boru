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
// (design/CORE-GO-TS-DEFECTS.0.md).

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
	return NewWord(tok)
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
func coreSpecAssemble(t *testing.T, toks []string) []Value {
	t.Helper()
	var out []Value
	pos := 0
	for pos < len(toks) {
		var v Value
		v, pos = coreSpecItem(t, toks, pos)
		out = append(out, v)
	}
	return out
}

// coreSpecItem reads ONE item starting at pos, returning it and the index
// just past it. Recursive because the bracket forms nest.
func coreSpecItem(t *testing.T, toks []string, pos int) (Value, int) {
	t.Helper()
	switch toks[pos] {
	case "[", "[q":
		open := toks[pos]
		pos++
		elems := []Value{}
		for pos < len(toks) && toks[pos] != "]" {
			var v Value
			v, pos = coreSpecItem(t, toks, pos)
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
	case "p(":
		pos++
		items := []Value{}
		for pos < len(toks) && toks[pos] != ")" {
			var v Value
			v, pos = coreSpecItem(t, toks, pos)
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
			v, pos = coreSpecItem(t, toks, pos)
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
		return CanonValue(NewDispatchMod(DispatchModInfo{Ref: arg == "r", Quote: arg == "q"}))
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
		return CanonValue(NewList(coreSpecAssemble(t, coreSpecFields(arg))))
	case "run":
		out, err := NewTop(r).Run(coreSpecAssemble(t, coreSpecFields(arg)))
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

func TestCoreSpec(t *testing.T) {
	entries, err := os.ReadDir(coreSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", coreSpecDir, err)
	}
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsv") {
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
