package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestMakeListUserFnElements pins voxgig leaf L1: a list literal whose elements
// are USER-FN or MODULE call results (`def specs [(Test.test …) …]`, the voxgig
// spec-list pattern; minimally `[(mk) (mk)]`) used to decline force-compile —
// recordMakeListInner blanket-declined any non-builtin-produced element. They now
// assemble via OpMakeList (each element's recorded event RE-RUNS at runtime; it is
// never frozen), exactly as `make` instances and builtin results already did.
func TestMakeListUserFnElements(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"single user-fn element", `def mk fn [[][Integer][42]] [(mk)]`, "[[42]]"},
		{"two user-fn elements", `def mk fn [[][Integer][42]] [(mk) (mk)]`, "[[42 42]]"},
		{"per-element-differing", `def f fn [[n:Integer][Integer][n mul 10]] [(f 1) (f 2) (f 3)]`, "[[10 20 30]]"},
		{"each over fn-element list", `def mk fn [[][Integer][42]] ([(mk)] each [dup])`, "[[42]]"},
		{"fresh map per call (re-runs, not frozen)", `def mk fn [[][Map][{a:1}]] ([(mk) (mk)] get 0)`, "[{a:1}]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
