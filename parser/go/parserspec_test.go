package parser_test

// The Go half of the parser-level parity corpus (parser/spec/*.tsv).
//
// parser/ts/src/parserspec.test.ts is the other half. The two runners share
// NO code — they read the same files and each decodes the escape notation
// and renders the value stream independently, exactly as core/spec's pair
// does. That independence is the point: shared scaffolding can hide the same
// bug from both engines (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9), and
// a parser corpus with a shared reader would hide precisely the class of
// defect design/TS-PARITY-AUDIT.0.md found.
//
// Two files, two contracts:
//
//   parse.tsv      src -> expected. Both engines must produce `expected`.
//   divergent.tsv  src -> go, ts.   The parity debt. Each runner asserts its
//                                   OWN column, so a divergence stays pinned
//                                   instead of drifting, and fixing one means
//                                   deleting the row.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

// specRow is one decoded corpus line: the source plus its columns.
type specRow struct {
	line int
	src  string
	cols []string
}

// readSpec decodes one corpus file. A row is tab-separated; '#' at the start
// of a line is a comment and blank lines are skipped.
func readSpec(t *testing.T, name string) []specRow {
	t.Helper()
	path := filepath.Join("..", "spec", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

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
		if len(parts) < 2 {
			t.Fatalf("%s:%d: need at least 2 tab-separated columns, got %d", name, n, len(parts))
		}
		// EVERY column is escaped, not just the source: a render can itself
		// contain a newline (XML text spanning lines), which would otherwise
		// split one row across two lines and silently truncate it.
		cols := make([]string, 0, len(parts)-1)
		for _, c := range parts[1:] {
			cols = append(cols, decodeSpecEscapes(c))
		}
		rows = append(rows, specRow{line: n, src: decodeSpecEscapes(parts[0]), cols: cols})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	// divergent.tsv is the parity DEBT, so empty is the goal state, not a
	// broken corpus. Every other file must have rows — an empty one there
	// means the corpus is not being read.
	if len(rows) == 0 && filepath.Base(path) != "divergent.tsv" {
		t.Fatalf("%s: no rows", path)
	}
	return rows
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

// renderSpec is the corpus's `expected` contract: the canon of each parsed
// value, space-joined, or "ERR " and the first line of the error text.
func renderSpec(src string) string {
	vals, err := parser.Parse(src)
	if err != nil {
		return "ERR " + strings.SplitN(err.Error(), "\n", 2)[0]
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, core.CanonValue(v))
	}
	return strings.Join(parts, " ")
}

func TestParserSpecParse(t *testing.T) {
	for _, r := range readSpec(t, "parse.tsv") {
		got := renderSpec(r.src)
		if got != r.cols[0] {
			t.Errorf("parse.tsv:%d: %q\n  want: %s\n  got : %s", r.line, r.src, r.cols[0], got)
		}
	}
}

// TestParserSpecDivergent pins the Go side of every recorded divergence, and
// fails if a row has stopped diverging — a fixed divergence must be MOVED to
// parse.tsv, not left here, or the file stops being an honest debt list.
func TestParserSpecDivergent(t *testing.T) {
	rows := readSpec(t, "divergent.tsv")
	if len(rows) == 0 {
		// The debt is paid. Kept as a live assertion rather than deleted:
		// the file is the ratchet, and a NEW divergence has to be added
		// here deliberately (with the justification the header demands)
		// instead of quietly landing as a changed expectation elsewhere.
		t.Log("divergent.tsv: empty — parser/go and parser/ts agree on every corpus row")
		return
	}
	for _, r := range rows {
		if len(r.cols) < 2 {
			t.Errorf("divergent.tsv:%d: need src, go, ts columns", r.line)
			continue
		}
		wantGo, wantTS := r.cols[0], r.cols[1]
		if wantGo == wantTS {
			t.Errorf("divergent.tsv:%d: %q: go and ts columns are identical — move this row to parse.tsv", r.line, r.src)
			continue
		}
		if got := renderSpec(r.src); got != wantGo {
			t.Errorf("divergent.tsv:%d: %q (go column)\n  want: %s\n  got : %s", r.line, r.src, wantGo, got)
		}
	}
}
