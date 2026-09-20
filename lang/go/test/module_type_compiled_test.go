package test

// Escaped module types under the bytecode compiler. A module's exported
// type literal (MatrixUtil.Matrix, TimeUtil.Timezone, …) is minted in
// the module sub-registry; import ADOPTS the canonical node into the
// importing registry's TypeTable (adoptEscapedTypes), which is what the
// VM's OpPushType resolution (r.Types.LookupByID) relies on. Without
// the adoption every force-compiled program using such a literal
// aborts with "unresolvable type operand" — pinned here because the
// TSV differential runner does not compile on compile failure and
// would mask the regression.

import (
	"fmt"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func TestModuleTypeCompiledLiteral(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"matrix make", `import "boru:matrix-util"  make MatrixUtil.Matrix [[1 2] [3 4]]`, "Matrix(2x2)"},
		{"timezone is", `import "boru:time-util"  (TimeUtil.tz "UTC") is TimeUtil.Timezone`, "true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("force-compiled run: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("expected one result, got %v", got)
			}
			if s := fmt.Sprintf("%v", got[0]); s != c.want {
				t.Fatalf("got %q, want %q", s, c.want)
			}
		})
	}
}
