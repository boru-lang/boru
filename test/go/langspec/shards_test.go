package langspec

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// readShards parses shards.tsv into shard → test names, and the set of
// every assigned test.
func readShards(t *testing.T) (map[int][]string, map[string]int) {
	t.Helper()
	f, err := os.Open("shards.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	byShard := map[int][]string{}
	assigned := map[string]int{}
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			t.Fatalf("shards.tsv:%d: want <shard>\\t<TestName>, got %q", lineNo, line)
		}
		n, err := strconv.Atoi(parts[0])
		if err != nil || n < 1 {
			t.Fatalf("shards.tsv:%d: shard must be a positive integer, got %q", lineNo, parts[0])
		}
		name := parts[1]
		if prev, dup := assigned[name]; dup {
			t.Errorf("shards.tsv:%d: %s is in shard %d and shard %d", lineNo, name, prev, n)
		}
		assigned[name] = n
		byShard[n] = append(byShard[n], name)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return byShard, assigned
}

// packageTests lists every top-level `func TestX(t *testing.T)` in this
// package's directory, by parsing the sources — the same population
// `go test -list` reports.
func packageTests(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Type.Params.NumFields() != 1 {
				continue
			}
			names = append(names, fn.Name.Name)
		}
	}
	sort.Strings(names)
	return names
}

// TestLangspecShardsPartition pins that shards.tsv is a PARTITION of this
// package's tests: every Test function is in exactly one shard, and every
// name in the file is a test that exists. A test added without a shard
// would otherwise never run in CI; a renamed one would leave a stale row.
func TestLangspecShardsPartition(t *testing.T) {
	byShard, assigned := readShards(t)
	tests := packageTests(t)
	known := map[string]bool{}
	for _, name := range tests {
		known[name] = true
		if _, ok := assigned[name]; !ok {
			t.Errorf("%s has no shard — add a `<shard>\\t%s` line to shards.tsv or CI never runs it", name, name)
		}
	}
	for name, n := range assigned {
		if !known[name] {
			t.Errorf("shards.tsv assigns %s to shard %d but no such test exists — delete the line", name, n)
		}
	}
	// Shards are numbered 1..N with no gap, so the ci.yml matrix and
	// `make langspec-shard-count` can enumerate them.
	for i := 1; i <= len(byShard); i++ {
		if len(byShard[i]) == 0 {
			t.Errorf("shard %d is empty — shards must be numbered 1..%d without gaps", i, len(byShard))
		}
	}
	t.Logf("langspec shards: %d shards over %d tests", len(byShard), len(tests))
}
