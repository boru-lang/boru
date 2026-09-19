package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The `jp` (JSONPath, github.com/ohler55/ojg) and `jq` (jq filter,
// github.com/itchyny/gojq) mini-languages query a document — the stack subject,
// which may be a Node (Map/List), a class instance, Table or Record. Both ship
// built-in with boru:minilang and return a List of results.

const qImp = `import "boru:minilang"  `

// TestMiniQuerySubjectTypes pins that jp and jq work over every supported
// subject shape (the document conversion path). A Map/Record subject needs an
// explicit {} opts (the mini opts gotcha); List / class-instance / Table do not.
func TestMiniQuerySubjectTypes(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"jp Node", `{store:{book:[{title:'A'} {title:'B'}]}} mini jp '$.store.book[*].title' {}`, "['A' 'B']"},
		{"jq Node", `{a:1 b:2} mini jq '.a + .b' {}`, "[3]"},
		{"jp List", `[{n:1} {n:2} {n:3}] mini jp '$[*].n'`, "[1 2 3]"},
		{"jq List", `[1 2 3 4] mini jq '.[] | select(. > 2)'`, "[3 4]"},
		{"jp Object", `def Pt class {x:Integer y:Integer} end  (make Pt {x:7 y:9}) mini jp '$.x'`, "[7]"},
		{"jq Object", `def Pt class {x:Integer y:Integer} end  (make Pt {x:7 y:9}) mini jq '.x + .y'`, "[16]"},
		{"jp Record", `def R (refine Record [{a:Integer b:String}])  (make R {a:5 b:'hi'}) mini jp '$.b' {}`, "['hi']"},
		{"jq Table", `def Row (refine Record [{a:Integer}])  def T (refine Table Row)  (make T [[1] [2] [3]]) mini jq 'map(.a) | add'`, "[6]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			res, err := runReference(t, a, qImp+c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := strings.TrimSpace(fmt.Sprintf("%v", res[0])); got != c.want {
				t.Errorf("%s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestMiniQueryDesugar proves `mini jp`/`mini jq` are sugar for the standard call.
func TestMiniQueryDesugar(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	sugar := fmt.Sprintf("%v", mustRun(t, a, qImp+`{a:1 b:2} mini jp '$.a' {}`))
	desugared := fmt.Sprintf("%v", mustRun(t, a, qImp+`{a:1 b:2} MiniLang.lang_jp '$.a' {} end`))
	if sugar != desugared {
		t.Fatalf("jp sugar=%q desugared=%q must agree", sugar, desugared)
	}
}

// TestMiniQueryErrors pins the loud parse failures.
func TestMiniQueryErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bad jp", `{a:1} mini jp '$.[' {}`, "mini_parse_error"},
		{"bad jq", `{a:1} mini jq '.[' {}`, "mini_parse_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(qImp + c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q should contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestMiniQueryInKinds confirms jp and jq are listed by MiniLang.kinds.
func TestMiniQueryInKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	listed := fmt.Sprintf("%v", mustRun(t, a, qImp+`MiniLang.kinds`))
	for _, k := range []string{"jp", "jq"} {
		if !strings.Contains(listed, k) {
			t.Errorf("MiniLang.kinds %q should contain %q", listed, k)
		}
	}
}

func mustRun(t *testing.T, a *lang.Boru, src string) any {
	t.Helper()
	res, err := runReference(t, a, src)
	if err != nil {
		t.Fatalf("Run(%q): %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("Run(%q): expected 1 result, got %d", src, len(res))
	}
	return res[0]
}
