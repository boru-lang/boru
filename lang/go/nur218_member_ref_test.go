package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR218MemberRefIsItsWordTwin pins NUR218's close: `/v` yields the
// binding's VALUE whoever reads it, so a member read `m.f/v` is the value its
// word twin `inc/v` is — delivered unquoted and stepped past, inert only by
// where it sits — on both lanes. The peek that consumes a group's `/v`
// marker used to QUOTE the member value, and the quote rode on into a
// paren's survivor, a callback slot and a branch result as data: `each
// (m.f/v) [1 2 3]` was three fn values interpreted and [2 3 4] compiled,
// `(m.f/v 5)` `fn 5` for 6, `def g (m.f/v) end g 4` 5 interpreted and `fn 4`
// compiled. Every shape below answers what the word twin answers, for a
// member of a map literal, of a flex (the check pass's dynamic carrier) and
// of a module export, compiled.
func TestNUR218MemberRefIsItsWordTwin(t *testing.T) {
	const fns = `def inc fn [[n:Integer][Integer][n add 1]] end def s2 fn [[a:Integer b:Integer][Integer][a add b]] end def one fn [[][Integer][1]] end `
	twins := []struct {
		name, pre, x, g, h string
	}{
		{"word", fns, "inc/v", "s2/v", "one/v"},
		{"map", fns + `def m {f: inc/v g: s2/v h: one/v} end `, "m.f/v", "m.g/v", "m.h/v"},
		{"flex", fns + `def m (flex {f: inc/v g: s2/v h: one/v}) end `, "m.f/v", "m.g/v", "m.h/v"},
		{"module", fns + `import module [def inc fn [[n:Integer][Integer][n add 1]] def s2 fn [[a:Integer b:Integer][Integer][a add b]] def one fn [[][Integer][1]] export "M" {f: inc/v g: s2/v h: one/v}] end `, "M.f/v", "M.g/v", "M.h/v"},
	}
	shapes := []struct{ src, want string }{
		{"each (X) [1 2 3]", "[[2 3 4]]"},
		{"fold (G) [1 2 3] 0", "[6]"},
		{"if true (H) [2]", "[1]"},
		{"if true H [2]", "[1]"},
		{"(X 5)", "[6]"},
		{"def g (X) end g 4", "[5]"},
		{"def g X end g 4", "[5]"},
		{"each X [1 2 3]", "[[2 3 4]]"},
		{"[1 2 3] each X", "[[2 3 4]]"},
		{"(X) typeof", "[Function]"},
	}
	for _, tw := range twins {
		for _, sh := range shapes {
			src := tw.pre + strings.NewReplacer("X", tw.x, "G", tw.g, "H", tw.h).Replace(sh.src)
			gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
			if errI != nil || fmt.Sprint(gotI) != sh.want {
				t.Errorf("%s · %s: interpreter %v / %v, want %s", tw.name, sh.src, gotI, errI, sh.want)
			}
			if !compiled || errC != nil || fmt.Sprint(gotC) != sh.want {
				t.Errorf("%s · %s: compiled %v / %v (compiled=%v), want %s", tw.name, sh.src, gotC, errC, compiled, sh.want)
			}
		}
	}
	// Negative: a member `/v` a code body leaves is DATA — the pointer
	// stepped past it inside the body, as it steps past `inc/v` — three fn
	// values, never the applied [2 3 4] the compiled body's replay produced.
	// The compiled lane declines it loudly now (a result above a literal in
	// the body's residual); the word twin compiles and agrees.
	src := twins[1].pre + `[1 2 3] each [m.f/v]`
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[[fn inc(Integer) fn inc(Integer) fn inc(Integer)]]" {
		t.Errorf("a body's member /v is data: interpreter %v / %v", gotI, errI)
	}
	if !noteCompileDefect(t, src, gotC, errC) {
		requireParity(t, src, gotC, errC, gotI, errI)
	}
}

// TestNUR218DynamicBranchArmLands pins the container twin of NUR159 found
// closing NUR218: a branch arm that is a member read over a container the
// check pass cannot see into (a flex) — bare or `/v` — may hold a fn, and
// the interpreter re-steps whatever `if` returned: a 0-arg member fires (1),
// an arg-taking one applies over the value beneath (6) or stays data with
// nothing to take. The merge lands it (OpReStepLanding) on the computed-arm
// path as the general merge does; it compiled to the member itself. A plain
// data member is untouched.
func TestNUR218DynamicBranchArmLands(t *testing.T) {
	const one = `def one fn [[][Integer][1]] end def m (flex {h: one/v}) end `
	const inc = `def inc fn [[n:Integer][Integer][n add 1]] end def m (flex {h: inc/v}) end `
	for _, c := range []struct{ src, want string }{
		{one + `if true m.h [2]`, "[1]"},
		{one + `if (1 eq 1) m.h [2]`, "[1]"},
		{one + `if false [3] m.h`, "[1]"},
		{one + `if true m.h/v [2]`, "[1]"},
		{inc + `5 if true m.h [2]`, "[6]"},
		{inc + `if true m.h [2]`, "[fn inc(Integer)]"},
		{`def m (flex {h: 7}) end if true m.h [2]`, "[7]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s: interpreter %v / %v, want %s", c.src, gotI, errI, c.want)
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.want {
			t.Errorf("%s: compiled %v / %v (compiled=%v), want %s", c.src, gotC, errC, compiled, c.want)
		}
	}
}
