package lang_test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestNur054InlineCtxRefusal pins the NUR054 refusal in both directions.
//
// The compiler INLINES some bodies into the caller's unit — the `case`
// desugar's clause fragments, auto-evaluated list arguments, interp-string
// holes — where the interpreter runs each in a fresh sub-engine with its own
// context layer. There is no call to bracket (vm.go's enterBodyUnit map,
// path 5), so the region's layer has no compiled twin, and the refusal fires
// AT THE MINT: a `context` read inside the region refuses compilation and
// the program falls back to the interpreter (a refusal the interpreter absorbs,
// design/COMPILABLE-SUBSET.md). Refusing the read — not just a set/del
// through it — is what closes every consumption that can tell the region's
// layer from the ambient one: aliases (`dup`), identity probes (`eq`),
// renders, handles baked into containers. A handle bound OUTSIDE the region
// (an in-place layer write both engines scope identically) and a `context`
// inside a closure unit within the region (the VM brackets those bodies
// with its own context frame) both keep compiling.
//
// TestContextBoundaryDifferential / TestDocumentedContextBoundaries assert
// the resulting AGREEMENT and the canonical values; this test pins the
// mechanism, so agreement can never silently become "both engines leak".
func TestNur054InlineCtxRefusal(t *testing.T) {
	const refusalMark = "NUR054"

	refuse := []struct{ name, src string }{
		{"set in case clause body", "case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]\ncontext has y/q"},
		{"del in case clause body", "context set y 9\ncase 1 [ 1 [ context del y 5 ] 2 [ 6 ] ]\ncontext has y/q"},
		{"set in otherwise list argument", "false otherwise [ context set y 1 5 ]\ncontext has y/q"},
		{"set in def list auto-evaluation", "def b [ context set y 1 5 ]\ncontext has y/q"},
		{"set in fn list argument", "def run fn [[b:List] [Any] [ 0 ]]\nrun [ context set y 1 5 ]\ncontext has y/q"},
		{"set in each collection list", "each [ drop 0 ] [ context set y 1 1 ]\ncontext has y/q"},
		{"set in interp-string hole", "`x${context set y 1 5}`\ncontext has y/q"},
		{"set in nested list element", "false otherwise [ [ context set y 1 1 ] ]\ncontext has y/q"},
		// The refusal is AT THE MINT — `context` read inside the region —
		// which is what closes the consumption paths a write-site rule cannot
		// chase (the 2026-08-14 Codex review round confirmed all three live):
		// a re-IDed alias, an identity probe, an xml child hole.
		{"aliased write via dup in otherwise list", "false otherwise [ context dup drop set y 1 5 ]\ncontext has y/q"},
		{"identity eq in interp hole", "def s (context)\n`x${context eq s}`"},
		{"set in xml child hole", "<p>${context set y 1 5}</p> drop\ncontext has y/q"},
	}
	for _, c := range refuse {
		t.Run("refuses/"+c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("CompileCheck: %v", cerr)
			}
			if prog != nil || !strings.Contains(reason, refusalMark) {
				t.Errorf("must refuse with the NUR054 reason; prog=%v reason=%q",
					prog != nil, reason)
			}
		})
	}

	compile := []struct{ name, src string }{
		// Bracketed closure-unit bodies: contained by enterBodyUnit.
		{"set in do body", "do [ context set y 1 5 ]\ncontext has y/q"},
		{"set in each body", "each [ drop context set y 1 5 ] [1]\ncontext has y/q"},
		// Non-boundaries in either engine: the write leaks identically.
		{"set in if branch", "if true [ context set y 1 5 ] [ 6 ]\ncontext has y/q"},
		{"set in for body", "for 1 [ context set y 1 5 ]\ncontext has y/q"},
		{"set in named fn body", "def f fn [[] [Any] [ context set y 1 5 ]]\nf\ncontext has y/q"},
		{"set in if code-body condition", "if [ context set y 1 true ] [ 5 ] [ 6 ]\ncontext has y/q"},
		// Provenance precision: the handle was read OUTSIDE the inline
		// region, so the write persists identically on both engines.
		{"outside-bound handle written in case arm",
			"def s (context)\ncase 1 [ 1 [ s set y 1 5 ] 2 [ 6 ] ]\ncontext has y/q"},
		// A branch-join handle written at the TOP level: `if` branches are
		// not context boundaries in EITHER engine, so the joined handle is
		// the ambient layer on both — measured agreeing ([5 true] / [5 true])
		// against the Codex round's contrary claim.
		{"branch-joined handle written at top level",
			"if true [ context ] [ context ] set y 1 5\ncontext has y/q"},
		// A case with no context write at all must keep its desugar.
		{"case without context write", "case 1 [ 1 [ 5 ] 2 [ 6 ] ]"},
	}
	for _, c := range compile {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("CompileCheck: %v", cerr)
			}
			if prog == nil {
				t.Errorf("must keep compiling; refused with reason=%q", reason)
			}
		})
	}
}
