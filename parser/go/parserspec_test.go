package parser_test

// The Go half of the parser-level parity corpus (parser/spec/*.tsv).
//
// parser/ts/src/parserspec.test.ts is the other half. The two runners share
// NO code — they read the same files and each decodes the escape notation
// and renders the value stream independently, exactly as core/spec's pair
// does. That independence is the point: shared scaffolding can hide the same
// bug from both engines (design/legacy/CORE-GO-TS-DEFECTS.0.ignore, blind spot 9), and
// a parser corpus with a shared reader would hide precisely the class of
// defect design/legacy/TS-PARITY-AUDIT.0.ignore found.
//
// Seven files, seven contracts (lex.tsv and data.tsv have their own strict
// runners in lexspec_test.go and dataspec_test.go because they render
// tokens and decoded data rather than parsed values):
//
//   parse.tsv      src -> expected. Both engines must produce `expected`.
//   divergent.tsv  src -> go, ts.   The LIVE parity-debt ledger: each row is a
//                                   real measured divergence. This runner
//                                   re-renders every row and must reproduce
//                                   the `go` column exactly, so a fixed
//                                   divergence fails loudly and the row moves
//                                   to parse.tsv. The row count is pinned in
//                                   both runners.
//   lex.tsv        src, EOF status -> exact trivia-preserving token stream,
//                                   including UTF-8 byte offsets.
//   data.tsv       src -> decode.  The safe DATA-decode seam
//                                   (SafeParseData + ConvertParsedNumber).
//   nesting.tsv    kind, depth -> outcome. Each runner independently builds
//                                   the source, checks OK/error code, and walks
//                                   every successful spine iteratively.
//   shape.tsv      case, src -> semantic shape. Positions, flags, modifier
//                                   payloads, nested values, and diagnostics
//                                   omitted by the canonical render.
//   canon-fixpoint.tsv  src, record, why. ADR-015's shrink-only ledger: the
//                                   parse.tsv rows whose canon, parsed again,
//                                   does not yet canon the same
//                                   (TestParserCanonFixpoint, NUR072).

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

const (
	parseSpecRowCount     = 749
	divergentSpecRowCount = 0
	nestingSpecRowCount   = 18
	shapeSpecRowCount     = 28
	// canonFixpointRowCount pins canon-fixpoint.tsv, ADR-015's shrink-only
	// ledger of parse.tsv rows that do not yet reach their canon fixpoint.
	canonFixpointRowCount = 0
)

// specRow is one decoded corpus line: the source plus its columns.
type specRow struct {
	line int
	src  string
	cols []string
}

// nestingSpecRow is deliberately separate from specRow: nesting.tsv is not a
// source/render corpus. Each runner must generate its own source from the
// compact kind/depth contract, and every data row must have exactly 3 columns.
type nestingSpecRow struct {
	line     int
	kind     string
	depth    int
	expected string
}

// shapeSpecRow has its own strict schema: shape.tsv is not an extension of
// parse.tsv, and accepting optional/trailing columns here would let a malformed
// structural oracle silently pass in one implementation.
type shapeSpecRow struct {
	line     int
	name     string
	src      string
	expected string
}

// readSpec decodes one corpus file. A row is tab-separated; '#' at the start
// of a line is a comment and blank lines are skipped.
func readSpec(t *testing.T, name string) []specRow {
	t.Helper()
	wantColumns := 4 // divergent.tsv: src, go, ts, justification
	if name == "parse.tsv" || name == "canon-fixpoint.tsv" {
		wantColumns = 3 // src, expected, optional note / src, record, why
	}
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	seen := make(map[string]int)
	var rows []specRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		// A line is a COMMENT only when it starts with '#' AND carries no
		// tab. '#' is boru's own comment marker, so sources begin with it;
		// treating every '#' line as a comment silently drops those rows,
		// which is the failure this corpus exists to make impossible.
		if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != wantColumns {
			t.Fatalf("%s:%d: need exactly %d tab-separated columns, got %d", name, n, wantColumns, len(parts))
		}
		// EVERY column is escaped, not just the source: a render can itself
		// contain a newline (XML text spanning lines), which would otherwise
		// split one row across two lines and silently truncate it.
		cols := make([]string, 0, len(parts)-1)
		for _, c := range parts[1:] {
			cols = append(cols, decodeSpecEscapes(c))
		}
		if name == "divergent.tsv" && strings.TrimSpace(cols[2]) == "" {
			t.Fatalf("%s:%d: divergence justification must not be blank", name, n)
		}
		if name == "divergent.tsv" && cols[0] == cols[1] {
			t.Fatalf("%s:%d: go and ts renders are equal; move the row to parse.tsv", name, n)
		}
		src := decodeSpecEscapes(parts[0])
		if first, exists := seen[src]; exists {
			t.Fatalf("%s:%d: duplicate source (first at line %d): %q", name, n, first, src)
		}
		seen[src] = n
		rows = append(rows, specRow{line: n, src: src, cols: cols})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	wantRows := divergentSpecRowCount
	if name == "canon-fixpoint.tsv" {
		wantRows = canonFixpointRowCount
	}
	if name == "parse.tsv" {
		wantRows = parseSpecRowCount
	}
	if len(rows) != wantRows {
		t.Fatalf("%s: got %d data rows, want exact ratchet %d", path, len(rows), wantRows)
	}
	return rows
}

// readNestingSpec reads the compact nesting contract without sharing any
// reader or source-generation code with the TypeScript runner.
func readNestingSpec(t *testing.T) []nestingSpecRow {
	t.Helper()
	const name = "nesting.tsv"
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	seen := make(map[string]int)
	var rows []nestingSpecRow
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("%s:%d: need exactly 3 tab-separated columns (kind, depth, expected outcome), got %d", name, n, len(parts))
		}
		depth, err := strconv.Atoi(parts[1])
		if err != nil || depth < 1 {
			t.Fatalf("%s:%d: depth %q is not a positive integer", name, n, parts[1])
		}
		if parts[2] != "OK" && parts[2] != "ERR evaluation_limit" {
			t.Fatalf("%s:%d: expected outcome %q is not OK or ERR evaluation_limit", name, n, parts[2])
		}
		key := parts[0] + "\x00" + strconv.Itoa(depth)
		if first, exists := seen[key]; exists {
			t.Fatalf("%s:%d: duplicate kind/depth %q/%d (first at line %d)", name, n, parts[0], depth, first)
		}
		seen[key] = n
		rows = append(rows, nestingSpecRow{line: n, kind: parts[0], depth: depth, expected: parts[2]})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(rows) != nestingSpecRowCount {
		t.Fatalf("%s: got %d data rows, want exact ratchet %d", path, len(rows), nestingSpecRowCount)
	}
	return rows
}

// readShapeSpec independently reads the Go half of the semantic-shape oracle.
// Every data row is exactly: unique case name, escaped source, escaped shape.
func readShapeSpec(t *testing.T) []shapeSpecRow {
	t.Helper()
	const name = "shape.tsv"
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	seen := make(map[string]bool)
	var rows []shapeSpecRow
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("%s:%d: need exactly 3 tab-separated columns (case, source, expected shape), got %d", name, n, len(parts))
		}
		caseName := parts[0]
		if caseName == "" || strings.TrimSpace(caseName) != caseName {
			t.Fatalf("%s:%d: case name %q must be non-empty with no surrounding whitespace", name, n, caseName)
		}
		if seen[caseName] {
			t.Fatalf("%s:%d: duplicate case name %q", name, n, caseName)
		}
		seen[caseName] = true
		expected := decodeSpecEscapes(parts[2])
		if expected == "" {
			t.Fatalf("%s:%d: expected shape must not be empty", name, n)
		}
		rows = append(rows, shapeSpecRow{
			line:     n,
			name:     caseName,
			src:      decodeSpecEscapes(parts[1]),
			expected: expected,
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(rows) != shapeSpecRowCount {
		t.Fatalf("%s: got %d data rows, want exact ratchet %d", path, len(rows), shapeSpecRowCount)
	}
	return rows
}

// nestingSource independently expands one compact row into boru source.
// mixed is one spine whose outermost node is always a list and whose levels
// cycle through list, map, and paren; this makes depth count conversion frames
// exactly, including at 10,000.
func nestingSource(t *testing.T, kind string, depth int) string {
	t.Helper()
	switch kind {
	case "list":
		return strings.Repeat("[", depth) + "1" + strings.Repeat("]", depth)
	case "map":
		return strings.Repeat("{a:", depth) + "1" + strings.Repeat("}", depth)
	case "paren":
		return strings.Repeat("(", depth) + "1" + strings.Repeat(")", depth)
	case "typed-list":
		return strings.Repeat("[:", depth) + "Integer" + strings.Repeat("]", depth)
	case "typed-map":
		return strings.Repeat("{:", depth) + "Integer" + strings.Repeat("}", depth)
	case "mixed":
		var opens, closes strings.Builder
		opens.Grow(depth * 2)
		closes.Grow(depth)
		close := make([]byte, depth)
		for i := 0; i < depth; i++ {
			switch i % 3 {
			case 0:
				opens.WriteByte('[')
				close[i] = ']'
			case 1:
				opens.WriteString("{a:")
				close[i] = '}'
			case 2:
				opens.WriteByte('(')
				close[i] = ')'
			}
		}
		for i := len(close) - 1; i >= 0; i-- {
			closes.WriteByte(close[i])
		}
		return opens.String() + "1" + closes.String()
	default:
		t.Fatalf("nesting.tsv: unknown kind %q", kind)
		return ""
	}
}

// nestingOutcome intentionally does not call core.CanonValue: recursively
// rendering a 10,000-deep success would test the renderer's host stack rather
// than the parser's nesting contract. Successful values are returned for the
// independent iterative spine check below.
func nestingOutcome(src string) ([]core.Value, string) {
	values, err := parser.Parse(src)
	if err == nil {
		return values, "OK"
	}
	var ae *core.BoruError
	if errors.As(err, &ae) {
		return nil, "ERR " + ae.Code
	}
	return nil, "ERR unstructured"
}

// assertNestingSpine verifies every successful generated container without
// recursion. Merely accepting a 10,000-deep source is not enough: truncating,
// flattening, or changing one container kind must fail the shared contract too.
func assertNestingSpine(t *testing.T, values []core.Value, kind string, depth int) {
	t.Helper()
	if len(values) != 1 {
		t.Fatalf("%s depth %d: parsed %d root values, want exactly 1", kind, depth, len(values))
	}
	cur := values[0]
	for level := 0; level < depth; level++ {
		wantKind := kind
		if kind == "mixed" {
			wantKind = []string{"list", "map", "paren"}[level%3]
		}
		switch wantKind {
		case "list":
			if core.IsTypedList(cur) || cur.Parent == nil || !cur.Parent.Equal(core.TList) {
				t.Fatalf("%s depth %d: level %d is not an untyped List", kind, depth, level+1)
			}
			items, err := core.AsList(cur)
			if err != nil || items.Len() != 1 {
				t.Fatalf("%s depth %d: level %d List does not contain exactly one item", kind, depth, level+1)
			}
			cur = items.Get(0)
		case "map":
			if core.IsTypedMap(cur) || cur.Parent == nil || !cur.Parent.Equal(core.TMap) {
				t.Fatalf("%s depth %d: level %d is not an untyped Map", kind, depth, level+1)
			}
			entries, err := core.AsMap(cur)
			if err != nil || entries.Len() != 1 || len(entries.Keys()) != 1 || entries.Keys()[0] != "a" {
				t.Fatalf("%s depth %d: level %d Map is not the single entry a:<child>", kind, depth, level+1)
			}
			next, ok := entries.Get("a")
			if !ok {
				t.Fatalf("%s depth %d: level %d Map child is missing", kind, depth, level+1)
			}
			cur = next
		case "paren":
			if !core.IsParenExpr(cur) {
				t.Fatalf("%s depth %d: level %d is not a ParenExpr", kind, depth, level+1)
			}
			items, err := core.AsParenExpr(cur)
			if err != nil || len(items) != 1 {
				t.Fatalf("%s depth %d: level %d ParenExpr does not contain exactly one item", kind, depth, level+1)
			}
			cur = items[0]
		case "typed-list", "typed-map":
			isRightKind := core.IsTypedList(cur)
			if wantKind == "typed-map" {
				isRightKind = core.IsTypedMap(cur)
			}
			if !isRightKind {
				t.Fatalf("%s depth %d: level %d has the wrong typed-container kind", kind, depth, level+1)
			}
			child, err := core.AsChildType(cur)
			if err != nil || len(child.Elements) != 0 || len(child.Entries) != 0 {
				t.Fatalf("%s depth %d: level %d typed container has unexpected concrete children", kind, depth, level+1)
			}
			cur = child.Child
		default:
			t.Fatalf("nesting.tsv: unknown kind %q", wantKind)
		}
	}

	if kind == "typed-list" || kind == "typed-map" {
		word, err := core.AsWord(cur)
		if err != nil || word.Name != "Integer" {
			t.Fatalf("%s depth %d: terminal is not the word Integer", kind, depth)
		}
		return
	}
	n, err := core.AsInteger(cur)
	if err != nil || n != 1 {
		t.Fatalf("%s depth %d: terminal is not the Integer value 1", kind, depth)
	}
}

// decodeSpecEscapes turns the corpus's \n, \t and \\ back into the bytes
// they stand for, so one source can span a line of the file.
func decodeSpecEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// renderSpec is the corpus's `expected` contract: the canon of the parsed
// stream (core.CanonValues — each value's canon, space-joined, a group
// modifier spelled after its group), or "ERR " and the first line of the
// error text.
func renderSpec(src string) string {
	vals, err := parser.Parse(src)
	if err != nil {
		return "ERR " + strings.SplitN(err.Error(), "\n", 2)[0]
	}
	return core.CanonValues(vals)
}

func shapeBit(v bool) byte {
	if v {
		return '1'
	}
	return '0'
}

func renderShapeStrings(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconv.Quote(item)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func renderShapeValues(items []core.Value) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = renderShapeValue(item)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func renderShapeMapEntries(entries core.ReadMap) string {
	keys := entries.Keys()
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if item, ok := entries.Get(key); ok {
			parts = append(parts, strconv.Quote(key)+"=>"+renderShapeValue(item))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func renderShapeChildEntries(entries []core.ChildEntry) string {
	parts := make([]string, len(entries))
	for i, entry := range entries {
		parts[i] = strconv.Quote(entry.Key) + "=>" + renderShapeValue(entry.Value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func renderShapeOptionalValue(v core.Value) string {
	if v.Parent == nil {
		return "none"
	}
	return renderShapeValue(v)
}

// renderShapeValue is deliberately local to the Go runner. Unlike CanonValue,
// it includes every parser-observable field selected by shape.tsv and descends
// into the structural payloads used by the corpus. The TS runner implements the
// same text contract independently rather than sharing a serializer.
func renderShapeValue(v core.Value) string {
	pos := v.Pos()
	var b strings.Builder
	b.WriteString("v(c=")
	b.WriteString(strconv.Quote(core.CanonValue(v)))
	b.WriteString(",p=")
	b.WriteString(strconv.Itoa(pos.Row))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(pos.Col))
	b.WriteByte(':')
	b.WriteString(strconv.Quote(pos.Src))
	b.WriteString(",e=")
	b.WriteByte(shapeBit(v.Eval))
	b.WriteString(",q=")
	b.WriteByte(shapeBit(v.Quoted))

	if core.IsWord(v) {
		w, err := core.AsWord(v)
		if err == nil {
			b.WriteString(",w={name=")
			b.WriteString(strconv.Quote(w.Name))
			b.WriteString(",argc=")
			b.WriteString(strconv.Itoa(w.ArgCount))
			b.WriteString(",stack=")
			b.WriteByte(shapeBit(w.ForceStack))
			b.WriteString(",forward=")
			b.WriteByte(shapeBit(w.ForceForward))
			b.WriteString(",val=")
			b.WriteByte(shapeBit(w.ForceVal))
			b.WriteString(",usurp=")
			b.WriteByte(shapeBit(w.ForceUsurp))
			b.WriteByte('}')
		}
	} else if core.IsAtom(v) {
		if atom, err := core.AsAtom(v); err == nil {
			b.WriteString(",atom=")
			b.WriteString(strconv.Quote(atom))
		}
	}
	if sugar, ok := core.AsSugar(v); ok {
		b.WriteString(",sugar={kind=")
		b.WriteString(strconv.Quote(string(sugar.Kind)))
		b.WriteString(",n=")
		b.WriteString(strconv.FormatInt(sugar.N, 10))
		b.WriteString(",name=")
		b.WriteString(strconv.Quote(sugar.Name))
		b.WriteString(",src=")
		b.WriteString(strconv.Quote(sugar.Src))
		b.WriteString(",head=")
		b.WriteString(renderShapeOptionalValue(sugar.Head))
		b.WriteString(",headErr=")
		b.WriteString(strconv.Quote(sugar.HeadErr))
		b.WriteString(",items=")
		b.WriteString(renderShapeValues(sugar.Items))
		b.WriteByte('}')
	}

	if core.IsTypedList(v) || core.IsTypedMap(v) {
		if child, err := core.AsChildType(v); err == nil {
			kind := "list"
			if core.IsTypedMap(v) {
				kind = "map"
			}
			b.WriteString(",typed={kind=")
			b.WriteString(strconv.Quote(kind))
			b.WriteString(",child=")
			b.WriteString(renderShapeValue(child.Child))
			b.WriteString(",elements=")
			b.WriteString(renderShapeValues(child.Elements))
			b.WriteString(",entries=")
			b.WriteString(renderShapeChildEntries(child.Entries))
			b.WriteByte('}')
		}
	} else if core.IsParenExpr(v) {
		if items, err := core.AsParenExpr(v); err == nil {
			b.WriteString(",paren=")
			b.WriteString(renderShapeValues(items))
		}
	} else if core.IsReach(v) {
		if reach, err := core.AsReach(v); err == nil {
			b.WriteString(",reach={eval=")
			b.WriteByte(shapeBit(reach.Eval))
			b.WriteString(",recv=")
			b.WriteString(renderShapeValues(reach.Receiver))
			b.WriteString(",segs=[")
			segs := make([]string, len(reach.Segments))
			for i, seg := range reach.Segments {
				op := "dot"
				if seg.Getr {
					op = "getr"
				}
				if seg.Computed {
					segs[i] = op + "(expr=" + renderShapeValues(seg.KeyExpr) + ")"
				} else {
					segs[i] = op + "(lit=" + renderShapeValue(seg.KeyLit) + ")"
				}
			}
			b.WriteString(strings.Join(segs, ","))
			b.WriteString("]}")
		}
	} else if items, err := core.AsList(v); err == nil {
		b.WriteString(",list=")
		b.WriteString(renderShapeValues(items.Slice()))
	} else if entries, err := core.AsMap(v); err == nil {
		implicit := false
		var computed []string
		if om, ok := entries.(*core.OrderedMap); ok {
			implicit = om.Implicit
			if ck, ok := om.Meta["ck"].(map[string]bool); ok {
				for key, yes := range ck {
					if yes {
						computed = append(computed, key)
					}
				}
				sort.Strings(computed)
			}
		}
		b.WriteString(",map={implicit=")
		b.WriteByte(shapeBit(implicit))
		b.WriteString(",computed=")
		b.WriteString(renderShapeStrings(computed))
		b.WriteString(",entries=")
		b.WriteString(renderShapeMapEntries(entries))
		b.WriteByte('}')
	}

	b.WriteByte(')')
	return b.String()
}

func renderShape(src string) string {
	values, err := parser.Parse(src)
	if err == nil {
		return "values=" + renderShapeValues(values)
	}
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		return "error(unstructured=" + strconv.Quote(err.Error()) + ")"
	}
	help := make([]string, len(ae.Suggestions))
	for i, suggestion := range ae.Suggestions {
		help[i] = suggestion.Message
	}
	return "error(code=" + strconv.Quote(ae.Code) +
		",detail=" + strconv.Quote(ae.Detail) +
		",p=" + strconv.Itoa(ae.Row) + ":" + strconv.Itoa(ae.Col) + ":" + strconv.Quote(ae.Src) +
		",full=" + strconv.Quote(ae.FullSource) +
		",hint=" + strconv.Quote(ae.Hint) +
		",notes=" + renderShapeStrings(ae.Notes) +
		",help=" + renderShapeStrings(help) + ")"
}

func TestParserSpecParse(t *testing.T) {
	for _, r := range readSpec(t, "parse.tsv") {
		got := renderSpec(r.src)
		if got != r.cols[0] {
			t.Errorf("parse.tsv:%d: %q\n  want: %s\n  got : %s", r.line, r.src, r.cols[0], got)
		}
	}
}

// TestParserSpecDivergent keeps the parity-debt ledger LIVE rather than
// merely counted: this side re-renders every row's source and must reproduce
// the `go` column exactly (the TS runner does the same against its `ts`
// column). A divergence that gets fixed therefore fails here loudly — the
// row must then move to parse.tsv — and a row can never drift into recording
// behavior neither port has. readSpec validates the four-column/provenance
// form and the exact-count ratchet first, so debt cannot grow silently.
func TestParserSpecDivergent(t *testing.T) {
	rows := readSpec(t, "divergent.tsv")
	for _, r := range rows {
		got := renderSpec(r.src)
		if got != r.cols[0] {
			t.Errorf("divergent.tsv:%d: %q\n  recorded go: %s\n  measured go: %s\n(if the ports now agree here, move the row to parse.tsv)",
				r.line, r.src, r.cols[0], got)
		}
	}
}

func TestParserSpecNesting(t *testing.T) {
	for _, r := range readNestingSpec(t) {
		r := r
		t.Run(r.kind+"/"+strconv.Itoa(r.depth), func(t *testing.T) {
			src := nestingSource(t, r.kind, r.depth)
			values, got := nestingOutcome(src)
			if got != r.expected {
				t.Errorf("nesting.tsv:%d: %s depth %d: want %s, got %s", r.line, r.kind, r.depth, r.expected, got)
				return
			}
			if got == "OK" {
				assertNestingSpine(t, values, r.kind, r.depth)
			}
		})
	}
}

func TestParserSpecShape(t *testing.T) {
	for _, r := range readShapeSpec(t) {
		r := r
		t.Run(r.name, func(t *testing.T) {
			if got := renderShape(r.src); got != r.expected {
				t.Errorf("shape.tsv:%d (%s): %q\n  want: %s\n  got : %s", r.line, r.name, r.src, r.expected, got)
			}
		})
	}
}

// TestParserCanonFixpoint is ADR-015's fixpoint diagnostic over the parser
// corpus (design/CANON-ROUNDTRIP.0.md §1, §4): for every parse.tsv row that
// parses, the canon of its stream, parsed again, must canon to the same text
// — a renderer that is consistently wrong can pass a fixpoint, but one that
// fails it is always wrong. A row that does not reach its fixpoint must be
// on canon-fixpoint.tsv, with the record that closes it, and a ledgered row
// that reaches it fails until removed: the ledger only shrinks. The TS
// runner applies the same gate to the same ledger (NUR072).
func TestParserCanonFixpoint(t *testing.T) {
	ledger := map[string]string{}
	for _, row := range readSpec(t, "canon-fixpoint.tsv") {
		ledger[row.src] = row.cols[0]
	}
	seen := map[string]bool{}
	for _, row := range readSpec(t, "parse.tsv") {
		r1 := renderSpec(row.src)
		if strings.HasPrefix(r1, "ERR") {
			continue
		}
		r2 := renderSpec(r1)
		rec, ledgered := ledger[row.src]
		if ledgered {
			seen[row.src] = true
		}
		switch {
		case r2 != r1 && !ledgered:
			t.Errorf("parse.tsv:%d %q: canon %q re-parses to %q — not a fixpoint, and not on canon-fixpoint.tsv", row.line, row.src, r1, r2)
		case r2 == r1 && ledgered:
			t.Errorf("parse.tsv:%d %q reaches its fixpoint now (%s): remove it from canon-fixpoint.tsv and lower canonFixpointRowCount", row.line, row.src, rec)
		}
	}
	for src := range ledger {
		if !seen[src] {
			t.Errorf("canon-fixpoint.tsv lists %q, which is no parse.tsv row that parses", src)
		}
	}
}
