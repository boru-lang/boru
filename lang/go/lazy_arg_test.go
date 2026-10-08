package lang_test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// These tests pin the structure-first, lazy forward-argument resolution
// behaviour (design/legacy/LAZY-ARG-RESOLUTION.10.ignore). They are the §11 acceptance
// criteria from that proposal, exercised end-to-end against the default
// language registry (which ships the internal `boru:string-util` module, so
// no filesystem is involved).
//
// The headline case (N1): an UNTERMINATED `import "mod"` immediately
// followed by an expression that references the imported namespace used to
// fail with `undefined word`, because the dispatcher eagerly evaluated the
// trailing paren — dereferencing the namespace — before `import` installed
// it. Under lazy resolution the paren is left raw (no overload consumes it),
// `import` selects `[String]`, installs the namespace, and the expression
// then runs as an ordinary statement.

func runLazy(t *testing.T, src string) []interface{} {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.RunInterp(src)
	if err != nil {
		t.Fatalf("Run(%q) errored: %v", src, err)
	}
	return res
}

// 1. Unterminated import followed by a use of the imported namespace.
func TestLazyArg_UnterminatedImport(t *testing.T) {
	res := runLazy(t, "import \"boru:string-util\"\n(StringUtil.indexof \"B\" \" ABC\") end")
	if len(res) != 1 || res[0] != int64(2) {
		t.Fatalf("got %#v, want [2]", res)
	}
}

// 2. The terminated form keeps working unchanged.
func TestLazyArg_TerminatedImport(t *testing.T) {
	res := runLazy(t, "import \"boru:string-util\" end\n(StringUtil.upper \"hi\") end")
	if len(res) != 1 || res[0] != "HI" {
		t.Fatalf("got %#v, want [HI]", res)
	}
}

// 5. A bare literal after the import is not consumed as a second argument —
// import takes only its string and leaves the literal on the stack.
func TestLazyArg_TrailingLiteralUntouched(t *testing.T) {
	res := runLazy(t, "import \"boru:string-util\" 42\n\"after\" end")
	if len(res) != 2 || res[0] != int64(42) || res[1] != "after" {
		t.Fatalf("got %#v, want [42 after]", res)
	}
}

// 4. A genuine error inside a CLAIMED argument stays loud and located —
// lazy resolution does not swallow it.
func TestLazyArg_GenuineErrorStaysLoud(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.RunInterp("import \"boru:string-util\" end\n(NoSuchThing.nope 1) end")
	if err == nil {
		t.Fatal("expected an error for undefined NoSuchThing, got nil")
	}
	if !strings.Contains(err.Error(), "NoSuchThing") {
		t.Fatalf("error %q does not mention NoSuchThing", err.Error())
	}
}

// Check mode (§11 tail): `boru check` must resolve the import's signature
// WITHOUT evaluating the trailing paren, so it must NOT report the imported
// namespace as undefined.
func TestLazyArg_CheckModeNoFalsePositive(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	cr, err := a.Check("import \"boru:string-util\"\n(StringUtil.indexof \"B\" \" ABC\") end")
	if err != nil {
		t.Fatalf("Check errored: %v", err)
	}
	for _, d := range cr.Diagnostics {
		if strings.Contains(d.Detail, "StringUtil") || d.Word == "StringUtil" {
			t.Fatalf("check reported a false positive on StringUtil: %+v", d)
		}
	}
	if cr.Summary.Errors != 0 {
		t.Fatalf("check reported %d error(s), want 0: %+v", cr.Summary.Errors, cr.Diagnostics)
	}
}
