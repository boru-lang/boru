package lang_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExplanationDocumentsTheBoundaryList keeps the doc and the test above
// from drifting apart in the one way the value assertions cannot catch: the
// doc silently dropping a form, or keeping a caveat for a form that has since
// been fixed. It reads EXPLANATION.md rather than trusting that someone
// remembered to edit it.
func TestExplanationDocumentsTheBoundaryList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "EXPLANATION.md"))
	if err != nil {
		t.Fatalf("read EXPLANATION.md: %v", err)
	}
	doc := string(data)
	if !strings.Contains(doc, "Which bodies are write boundaries") {
		t.Fatal("EXPLANATION.md no longer carries the boundary list — it was added " +
			"because the passage it replaced named `for` as a scoping form and " +
			"omitted `for-each`. If it is being removed, remove this test with it " +
			"and say where the list moved")
	}
	// Every form the test above exercises must appear in the doc, so a new
	// boundary cannot be pinned in Go and left undocumented.
	for _, form := range []string{
		"`do`", "`each`", "`fold`", "`filter`", "`outer`", "`scan`",
		"`for-each`", "`await`", "`case`", "`for`", "`if`", "paren",
	} {
		if !strings.Contains(doc, form) {
			t.Errorf("EXPLANATION.md's boundary list does not mention %s, which "+
				"TestDocumentedContextBoundaries pins", form)
		}
	}
}
