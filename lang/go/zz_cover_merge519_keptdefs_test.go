package lang

import "testing"

// zz_cover_merge519_keptdefs_test.go pins, as whole programs, three compiler
// guards the merged ADR-008 profile left unreached (compiler/go has the
// direct halves in its own zz_cover_merge519_*_test.go files).

// TestMerge519ComputedBodyParenTokens: a computed `do`/`each` body whose
// tokens are paren groups that build no anonymous fn value — a three-item
// paren that is no arrow fold, an arrow fold whose params or body is no list
// — is no parking-lambda run (compiler kept_defs_scan.go anonLambdaArity):
// the interpreter steps each paren, and the compiled run agrees.
func TestMerge519ComputedBodyParenTokens(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def mk fn [[][List][quote [(1 add 2)]]] end do (mk)`, "[3]"},
		{`def mk fn [[][List][quote [(1 add 2)]]] end [1 2] each (mk)`, "[[3 3]]"},
		{`def x 99 end def mk fn [[][List][quote [(1 add 2)]]] end do (mk) end x`, "[3 99]"},
		{`def mk fn [[][List][quote [(a b c)]]] end do (mk)`, "[error(undefined word: a)]"},
		{`def mk fn [[][List][quote [([a] => b)]]] end do (mk)`, "[error(undefined word: b)]"},
		{`def mk fn [[][List][quote [(a => [1])]]] end do (mk)`, "[error(undefined word: a)]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestMerge519RootFnCarrierReadAfterComputedBody: a root read after a root
// computed keep-defs body waits for the tag hook to seat it live
// (noteKeptDefsRead); a read of a Function-typed value is no live seat
// (fnLikeResidual), so the next recorded event makes it an OBSERVER
// (compiler kept_defs.go flushKeptRead) and the latch declines loudly
// (NUR210) — the body may have rebound the name, and the interpreter's
// answer is the rebinding's.
func TestMerge519RootFnCarrierReadAfterComputedBody(t *testing.T) {
	const pre = `def mk fn [[][Function][([] => [7])]] end def x (mk) end def q fn [[][List][quote [def x 5]]] end do (q) end `
	for _, c := range []struct{ src, want string }{
		{pre + `x`, "[5]"},
		{pre + `[x]`, "[[5]]"},
	} {
		requireLoudDecline(t, c.src, "the read of `x` after it would read the check model's binding, which never saw the body (NUR210)", c.want)
	}
}

// TestMerge519StatementIslandUnplacedLeadingValue: a root paren apply over a
// member whose value is a fn at run time plans a statement island
// (compiler landing_restart.go planLandingRestarts) only when the program
// residual's entries before the statement can all be placed
// (rootPreStart). A leading literal the walk cannot place — a map literal, a
// list holding one, a def-bound type — plans no island, and the compiled
// paren apply answers the interpreter's value itself.
func TestMerge519StatementIslandUnplacedLeadingValue(t *testing.T) {
	const pre = `def h fn [[x:Integer] [Any] [x add 1]] end def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	for _, c := range []struct{ src, want string }{
		{pre + `{a:1} (m.f 5)`, "[{a:1} 6]"},
		{pre + `[{a:1}] (m.f 5)`, "[[{a:1}] 6]"},
		{pre + `{a:1} [(m.f 5)]`, "[{a:1} [6]]"},
		{pre + `def k Integer end k (m.f 5)`, "[Integer 6]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}
