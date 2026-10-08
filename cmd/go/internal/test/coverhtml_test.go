package test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// add merges rows (first sight records order/source; a repeat unions), and rows
// splits the denominator back into covered/uncovered.
func TestCovAccumAddRows(t *testing.T) {
	a := newCovAccum()
	a.add("m", "src", []int{1}, []int{2}) // first sight: covered{1} denom{1,2}
	a.add("m", "src", []int{1}, []int{2}) // repeat (seen): unchanged
	if len(a.order) != 1 || a.order[0] != "m" {
		t.Fatalf("order = %v, want [m]", a.order)
	}
	cov, unc := a.rows("m")
	if len(cov) != 1 || cov[0] != 1 {
		t.Errorf("covered = %v, want [1]", cov)
	}
	if len(unc) != 1 || unc[0] != 2 {
		t.Errorf("uncovered = %v, want [2]", unc)
	}
}

func TestRenderModuleHTML(t *testing.T) {
	// row 1 covered, row 3 uncovered, row 2 non-executable; row 1 has a `<`.
	got := renderModuleHTML("m.boru", "a<b\nplain\nc", []int{1}, []int{3})
	for _, want := range []string{
		`class="line covered"`,   // row 1
		`class="line uncovered"`, // row 3
		`class="line"`,           // row 2 (non-executable)
		"a&lt;b",                 // HTML-escaped source
		"m.boru",                 // header names the module
	} {
		if !strings.Contains(got, want) {
			t.Errorf("module page missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderIndexHTML(t *testing.T) {
	got := renderIndexHTML([]indexRow{{id: "m.boru", page: "mod0.html", covered: 3, total: 4}})
	for _, want := range []string{`href="mod0.html"`, "m.boru", "75.0%", "3 / 4"} {
		if !strings.Contains(got, want) {
			t.Errorf("index missing %q in:\n%s", want, got)
		}
	}
}

// oneModuleAccum returns an accumulator carrying a single partially-covered
// module, for the writeHTML tests.
func oneModuleAccum() *covAccum {
	a := newCovAccum()
	a.add("m.boru", "a\nb\nc", []int{1}, []int{3})
	return a
}

// An empty accumulator writes nothing.
func TestWriteHTMLEmpty(t *testing.T) {
	index, err := newCovAccum().writeHTML(filepath.Join(t.TempDir(), "cov"))
	if err != nil || index != "" {
		t.Fatalf("writeHTML(empty) = (%q, %v), want (\"\", nil)", index, err)
	}
}

// The happy path writes index.html + one page per module.
func TestWriteHTMLSuccess(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cov")
	index, err := oneModuleAccum().writeHTML(dir)
	if err != nil {
		t.Fatal(err)
	}
	if index != filepath.Join(dir, "index.html") {
		t.Errorf("index = %q", index)
	}
	for _, f := range []string{"index.html", "mod0.html"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
}

func TestWriteHTMLMkdirError(t *testing.T) {
	old := mkdirAll
	t.Cleanup(func() { mkdirAll = old })
	mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	if _, err := oneModuleAccum().writeHTML(t.TempDir()); err == nil {
		t.Fatal("expected a mkdir error")
	}
}

func TestWriteHTMLModulePageError(t *testing.T) {
	old := writeFile
	t.Cleanup(func() { writeFile = old })
	writeFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	if _, err := oneModuleAccum().writeHTML(t.TempDir()); err == nil {
		t.Fatal("expected a module-page write error")
	}
}

func TestWriteHTMLIndexError(t *testing.T) {
	old := writeFile
	t.Cleanup(func() { writeFile = old })
	// Module pages succeed; only the index write fails.
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(name, "index.html") {
			return errors.New("index boom")
		}
		return os.WriteFile(name, data, perm)
	}
	if _, err := oneModuleAccum().writeHTML(t.TempDir()); err == nil {
		t.Fatal("expected an index write error")
	}
}

// Run surfaces a coverage-report write failure as a warning (not a hard fail).
func TestRunCoverageWriteWarning(t *testing.T) {
	old := mkdirAll
	t.Cleanup(func() { mkdirAll = old })
	mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir boom") }

	dir := t.TempDir()
	mod := write(t, filepath.Join(dir, "calc.boru"),
		"def one fn [[] [Integer] [1]]\nexport \"Calc\" { one: one/v }\n")
	f := write(t, filepath.Join(dir, "calc_test.boru"),
		"import \"boru:test\"\nimport \""+mod+"\"\nTest.test \"one\" [(Calc.one) 1 Assert.equal]\n")
	code, _, stderr := runCmd("--coverage", "--coverage-dir", filepath.Join(dir, "cov"), f)
	if code != 0 {
		t.Errorf("code = %d, want 0 (a report-write failure must not fail the run)", code)
	}
	if !strings.Contains(stderr, "warning: coverage report") {
		t.Errorf("stderr = %q, want the report-write warning", stderr)
	}
}
