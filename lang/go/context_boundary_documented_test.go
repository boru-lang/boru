package lang_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestDocumentedContextBoundaries pins the boundary list EXPLANATION.md
// publishes ("Which bodies are write boundaries") against the engine.
//
// The list exists because the passage it replaces was wrong in both
// directions: it named `for` as a sub-engine creator (it is not a write
// boundary in either engine) and omitted `for-each` (it is one in both). A
// prose list of which forms scope is exactly the kind of claim that rots, so
// each row here is the assertion the doc makes, and the doc cites this
// behaviour rather than the other way round.
//
// This is the VALUE test, deliberately distinct from
// TestContextBoundaryDifferential, which asserts only that the two engines
// AGREE. Agreement is not correctness: both engines leaking would pass there
// and still make the doc a lie. So every row states what `context has y/q` must
// actually be, and for the forms the compiler does not yet bracket it states
// the two answers separately — which is what the doc's own caveat says.
func TestDocumentedContextBoundaries(t *testing.T) {
	// contained: the body's write did NOT reach the enclosing scope.
	const contained, leaked = false, true
	cases := []struct {
		name string
		src  string
		// interp is the canonical answer; compiled is what the VM gives today.
		// They differ only for the INLINED forms, and the doc says so.
		interp, compiled bool
	}{
		// ---- boundaries in BOTH engines ----
		{"do", `do [ context set y 1 5 ]`, contained, contained},
		{"each", `each [ drop context set y 1 5 ] [1]`, contained, contained},
		{"fold", `fold [ drop drop context set y 1 5 ] [1]`, contained, contained},
		{"filter", `filter [ drop context set y 1 true ] [1]`, contained, contained},
		{"outer", `outer [ drop drop context set y 1 5 ] [1] [2]`, contained, contained},
		{"scan", `scan [ drop drop context set y 1 5 ] [1]`, contained, contained},
		{"for-each", `for-each [ drop context set y 1 5 ] [1]`, contained, contained},
		{"await", `import "boru:time-util" end
TimeUtil.await [ [ context set y 1 5 ] ]`, contained, contained},

		// ---- NOT boundaries in either engine ----
		// `for` and `if` are the two the doc calls out by name: a reader
		// reasonably expects them to scope, and they do not.
		{"for body", `for 1 [ context set y 5 ]`, leaked, leaked},
		{"if branch", `if true [ context set y 1 5 ] [ 6 ]`, leaked, leaked},
		{"named fn body", `def f fn [[] [Any] [ context set y 1 5 ]]
f`, leaked, leaked},
		{"called lambda", `def g ([] => [ context set y 1 5 ])
g`, leaked, leaked},
		// A paren resolves its value and hands it to the signature matcher, so
		// it never opens a scope. This one used to be a boundary COMPILED only
		// (the apply islands, and the island's Run pushed a body frame); the
		// doc's rule and the engine now agree.
		{"paren-grouped apply", `def m {f: ([a:Integer] => [ context set y 1 a ])}
(m.f 1) drop`, leaked, leaked},

		// ---- inlined forms: boundaries on both engines, via compile failure ----
		// The compiler INLINES these into the caller's code, so there is no
		// body to bracket — instead a context write through a handle read
		// inside one DECLINES compilation (NUR054) and the whole program runs
		// on the interpreter, whose answer is canonical. `compiled` is
		// therefore the same answer, delivered by fallback rather than by a
		// bracketed body; the doc's caveat says exactly this.
		{"case clause body", `case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]`, contained, contained},
		{"otherwise list argument", `false otherwise [ context set y 1 5 ]`, contained, contained},
		{"list auto-evaluation", `def b [ context set y 1 5 ]`, contained, contained},
		{"interp-string hole", "`x${context set y 1 5}` drop", contained, contained},
		{"xml child hole", `<p>${context set y 1 5}</p> drop`, contained, contained},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := c.src + "\ncontext has y/q"
			a, _ := lang.New()
			gotC, cErr := a.Run(src)
			b, _ := lang.New()
			gotI, iErr := b.RunInterp(src)
			if lang.NoteCompileDefect(t, src, gotC, cErr) {
				return
			}
			if cErr != nil || iErr != nil {
				t.Fatalf("both engines must run the program — compiled: %v / interpreted: %v",
					cErr, iErr)
			}
			if got := lastBool(t, gotI); got != c.interp {
				t.Errorf("interpreted: `context has y/q` = %v, want %v.\n"+
					"This is the CANONICAL answer and EXPLANATION.md's boundary list "+
					"publishes it. Changing it is a language-semantics change — update "+
					"the doc in the same commit, or this is a regression.", got, c.interp)
			}
			if got := lastBool(t, gotC); got != c.compiled {
				extra := ""
				if c.interp != c.compiled {
					extra = " This row is a KNOWN compiled divergence (an inlined body, " +
						"nothing to bracket) that EXPLANATION.md documents as a caveat. " +
						"If the compiled answer now matches the interpreter, that is the " +
						"fix landing: set compiled to the interpreter's value here and " +
						"delete the form from the doc's caveat."
				}
				t.Errorf("compiled: `context has y/q` = %v, want %v.%s", got, c.compiled, extra)
			}
		})
	}
}

// lastBool reads the boolean the `context has y/q` tail left on top.
func lastBool(t *testing.T, stack []any) bool {
	t.Helper()
	if len(stack) == 0 {
		t.Fatalf("empty residual stack — the `context has y/q` tail produced nothing")
	}
	switch v := stack[len(stack)-1].(type) {
	case bool:
		return v
	default:
		s := fmt.Sprint(v)
		if s == "true" || s == "false" {
			return s == "true"
		}
		t.Fatalf("top of stack is %T (%v), not the boolean `context has y/q` returns; "+
			"full stack: %v", v, v, stack)
		return false
	}
}

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
