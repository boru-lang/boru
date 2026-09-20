package parser_test

// The Go half of parser/spec/data.tsv — the safe DATA-decode seam contract
// (SafeParseData + ConvertParsedNumber, the parsers behind StructUtil.parse).
// The TypeScript runner has its own reader and renderer: keeping both
// implementations independent prevents a shared harness bug from making two
// different decodes look equal.
//
// Two expected forms:
//
//   ERR <code>   the number converter declines a claimed spelling with that
//                BoruError code — the loud half of the contract.
//   ERR          the underlying jsonic parse itself rejects the source. Its
//                message is deliberately dependency-native (SafeParseData
//                returns raw jsonic errors), so only the compile failure is pinned.
//
// Everything else is the canonical decode render defined in the README:
// insertion-ordered `{k:v ...}` maps, `[v ...]` lists, `'...'` strings,
// `true`/`false`, `none` for null, and every numeric token rendered through
// ConvertParsedNumber so int/float kind and precision are part of the pin.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

const dataSpecRowCount = 65

type dataSpecRow struct {
	line     int
	src      string
	expected string
}

func readDataSpec(t *testing.T) []dataSpecRow {
	t.Helper()
	const name = "data.tsv"
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	seen := make(map[string]int)
	var rows []dataSpecRow
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || (strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("%s:%d: need exactly 3 tab-separated columns (source, expected, note), got %d", name, n, len(parts))
		}
		src := decodeSpecEscapes(parts[0])
		if first, exists := seen[src]; exists {
			t.Fatalf("%s:%d: duplicate source (first at line %d): %q", name, n, first, src)
		}
		seen[src] = n
		rows = append(rows, dataSpecRow{line: n, src: src, expected: decodeSpecEscapes(parts[1])})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(rows) != dataSpecRowCount {
		t.Fatalf("%s: got %d data rows, want exact ratchet %d", path, len(rows), dataSpecRowCount)
	}
	return rows
}

// renderDataNode renders one decoded node in the canonical data form. A
// BoruError from the number converter aborts the walk; any other shape is a
// harness failure, not a render.
func renderDataNode(v any) (string, error) {
	if ev, claimed, cerr := parser.ConvertParsedNumber(v); claimed {
		if cerr != nil {
			return "", cerr
		}
		return core.CanonValue(ev), nil
	}
	switch x := v.(type) {
	case nil:
		return "none", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case string:
		return "'" + x + "'", nil
	case []any:
		items := make([]string, 0, len(x))
		for _, item := range x {
			s, err := renderDataNode(item)
			if err != nil {
				return "", err
			}
			items = append(items, s)
		}
		return "[" + strings.Join(items, " ") + "]", nil
	case *jsonic.OrderedMap:
		pairs := make([]string, 0, len(x.Keys))
		for _, k := range x.Keys {
			child, _ := x.Get(k)
			s, err := renderDataNode(child)
			if err != nil {
				return "", err
			}
			pairs = append(pairs, k+":"+s)
		}
		return "{" + strings.Join(pairs, " ") + "}", nil
	}
	return "", fmt.Errorf("unsupported data node %T", v)
}

func renderDataSpec(t *testing.T, src string) string {
	t.Helper()
	v, err := parser.SafeParseData(src)
	if err != nil {
		return "ERR"
	}
	s, cerr := renderDataNode(v)
	if cerr != nil {
		var be *core.BoruError
		if !errors.As(cerr, &be) {
			t.Fatalf("data render of %q failed outside the BoruError contract: %v", src, cerr)
		}
		return "ERR " + be.Code
	}
	return s
}

func TestParserSpecData(t *testing.T) {
	for _, r := range readDataSpec(t) {
		if got := renderDataSpec(t, r.src); got != r.expected {
			t.Errorf("data.tsv:%d: %q\n  want: %s\n  got : %s", r.line, r.src, r.expected, got)
		}
	}
}
