package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestNUR337PendingContainerTrailingApply pins NUR337's close. A map or list
// literal still holding tokens the interpreter evaluates (`{a:(1 add 2)}`,
// `[1 add 2]`) is owed an evaluation when a paren-bounded fn-value apply
// takes it: the interpreter's apply evaluates it at the bind, or its park
// leaves it for the residual sweep. The check pass collapsed the window to a
// carrier without applying the lead, so nothing evaluated the literal and the
// op bound the RAW token — `{a:paren([1 word(add) 2])}` for `{a:3}`. The
// collapse now evaluates it the bind's way under a window that provably fits
// the lead's one signature, folds it where it is a pure constant, and leaves
// anything else to the residual sweep (or the program declines); the op never
// takes one raw.
func TestNUR337PendingContainerTrailingApply(t *testing.T) {
	const lamMap = `def lam ([x:Map] => [x]) end `
	const lamInt = `def lam ([x:Integer] => [x]) end `
	const mk = `def mk fn [[][Integer][3]] end `
	for _, src := range []string{
		// the register's two repros: an apply that fits, a lead that parks
		lamMap + `({a:(1 add 2)} lam/v)`,
		lamInt + mk + `({a:(mk)} lam/v)`,
		// nested containers, a list member, a list argument
		lamMap + `({a:{b:(1 add 2)}} lam/v)`,
		lamMap + `({a:[(1 add 2)]} lam/v)`,
		`def lam ([x:List] => [x]) end ([1 add 2] lam/v)`,
		lamInt + `([1 add 2] lam/v)`,
		// a word, an interpolation, a def-bound read as the member
		`def c 5 ` + lamMap + `({a:(c add 2)} lam/v)`,
		`def x 3 ` + lamMap + `({a:x} lam/v)`,
		`def x 3 ` + lamMap + `({a:"v$(x)"} lam/v)`,
		// a named fn value, a native fn value, a two-param lead
		`def id fn [[x:Map][Any][x]] end ({a:(1 add 2)} id/v)`,
		`def id fn [[x:Any][Any][x]] end ({a:(1 add 2)} id/v)`,
		`([1 add 2] size/v)`,
		`def two ([x:Map y:Integer] => [x]) end (5 {a:(1 add 2)} two/v)`,
		`def two ([x:Map y:Integer] => [x]) end ({a:(1 add 2)} 5 two/v)`,
		`def two ([x:Integer y:Map] => [y]) end (5 {a:(1 add 2)} two/v)`,
		// an under-applied window: the deeper value survives as residual
		lamMap + `({a:(1 add 2)} {b:(2 add 2)} lam/v)`,
		// a later statement, an enclosing list, a consumer, a def
		lamInt + mk + `({a:(mk)} lam/v) 7`,
		lamInt + mk + `[{a:(mk)} lam/v]`,
		`def lam ([x:Map] => [x size]) end ({a:(1 add 2)} lam/v) add 1`,
		lamMap + `9 ({a:(1 add 2)} lam/v)`,
		// the `apply` word, a produced closure lead
		lamMap + `({a:(1 add 2)} lam/v apply)`,
		`def mk fn [[][Function][([m:Map] => [m])]] end ({a:(1 add 2)} (mk))`,
		// inside a fn body: a fitting apply, a parked one, a param read,
		// a fn-typed param lead, a body-local def
		lamMap + `def g fn [[][Any][({a:(1 add 2)} lam/v)]] end g`,
		lamMap + `def g fn [[n:Integer][Any][({a:(n add 2)} lam/v)]] end g 1`,
		lamInt + `def g fn [[n:Integer][Any][({a:(n add 2)} lam/v)]] end g 1`,
		lamInt + `def g fn [[n:Integer][Any][([n add 2] lam/v)]] end g 1`,
		lamInt + mk + `def g fn [[][Any][({a:(mk)} lam/v)]] end g`,
		lamMap + `def g fn [[f:Function][Any][({a:(1 add 2)} f)]] end g lam/v`,
		lamMap + `def g fn [[][Any][def r ({a:(1 add 2)} lam/v) r]] end g`,
		// a gradual argument in the window
		`def two ([x:Map y:Any] => [x]) end def d fn [[][Any][5]] end ((d) {a:(1 add 2)} two/v)`,
		// a raising lead and a raising member
		`def lam fn [[x:Integer][Any][x]] end ({a:(1 add 2)} lam/v)`,
		lamMap + `({a:(1 div 0)} lam/v)`,
		// a lead the collapse cannot prove: overloaded, quoted (inert at
		// the tail), a value pattern the run decides
		`def f fn [[x:Map][Any][x] [x:Integer][Any][x]] end ({a:(1 add 2)} f/v)`,
		`({a:(1 add 2)} (quote ([m:Map] => [m])))`,
		`def s (flex {n:3}) def lam ([0 m:Map] => [m]) end def mk fn [[][Integer][s.n]] end ({a:(1 add 2)} (mk) lam/v)`,
		`def s (flex {n:0}) def lam ([0 m:Map] => [m]) end def mk fn [[][Integer][s.n]] end ({a:(1 add 2)} (mk) lam/v)`,
		// a parked member that is no pure constant: an error, a fresh flex
		lamInt + `({a:(1 div 0)} lam/v)`,
		lamInt + `({a:(flex [1])} lam/v)`,
		// a member whose bind-time evaluation raises
		lamMap + `({a:(raise foo "x")} lam/v)`,
		`def lam ([x:List] => [x]) end ([1 add "s"] lam/v)`,
		// NEGATIVE shapes the change must leave alone: a container with
		// nothing to evaluate, and the plain residual
		`def lam ([x:List] => [x]) end ([1 2] lam/v)`,
		mk + `({a:(mk)} 5)`,
	} {
		requireCompiledParity(t, src)
	}
}

// TestNUR337PendingContainerDeclines: a pending container the collapse can
// not evaluate exactly — a PARKED lead over a member with an effect, whose
// evaluation the interpreter defers past the next statement — must not
// compile to the raw token either. The program declines (loudly), or — where
// another path now lowers it (the effectful parked row compiles, with parity,
// once the other 2026-09-28 fixes are merged beside this one) — compiles to
// the interpreter's exact answer; never to `{a:paren(...)}`.
func TestNUR337PendingContainerDeclines(t *testing.T) {
	for _, src := range []string{
		`def lam ([x:Integer] => [x]) end def s (flex {n:1}) ({a:(s set n 2)} lam/v) s.n`,
		`def mk fn [[][Function][([m:Map] => [m])]] end def s (flex {n:1}) ({a:(s.n)} (mk))`,
		// the standing declines of an under-applied and a 0-arg lead
		`def two ([x:Map y:Integer] => [x]) end ({a:(1 add 2)} two/v)`,
		`def z ([] => [1]) end ({a:(1 add 2)} z/v)`,
	} {
		gotC, compiled, errC := mustNew(t).RunCompiled(src)
		gotI, errI := mustNew(t).RunInterp(src)
		if compiled && (fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI)) {
			t.Errorf("%q must decline or agree, but compiled: %v [%v] (interp %v [%v])", src, gotC, errC, gotI, errI)
		}
		if strings.Contains(fmt.Sprint(gotI), "paren(") || errI != nil {
			t.Errorf("%q: interpreter answered %v [%v]", src, gotI, errI)
		}
	}
}

// TestNUR338TrapRenderedErrorParity pins NUR338's close with the FULL
// rendered error, not a substring (TestEmitTrap's substring match could not
// see a caret): a compiled terminal trap must raise the interpreter's error
// byte for byte — code, message, position AND caret width. Two traps
// recorded the wrong position: the `/u` illegal_ref took the whole `x/u`
// token's text (a `^^^` for the interpreter's `^` under the bare name), and
// unpack's missing key took the NAME token (1:9) where the interpreter's
// unpositioned handler error is stamped at the dispatching `unpack` word
// (1:1). The same handler-stamp rule held for case, mini, parse and emit.
// Every program here is a top-level trap: it must compile, raise, and render
// identically on both lanes.
func TestNUR338TrapRenderedErrorParity(t *testing.T) {
	for _, src := range []string{
		`def x 5 end x/u`,
		`def x 5 x/u`,
		`def x 5 end [x/u]`,
		`def x 5 end 1 x/u`,
		`def x 5 end (x/u)`,
		"def x 5 end\n  x/u",
		`def x 5 end   x/u 7`,
		`def x 5 end def m {f:[1 2]} each m.f [x/u]`,
		`def x 5 end def mk fn [[][List][[1]]] end each (for 1 [mk]) [x/u]`,
		`def x 5 end def mk fn [[][List][[1]]] end each (mk) [x/u]`,
		`unpack [z] {a:1}`,
		`unpack [a z] {a:1}`,
		`unpack {a:q z:w} {a:1}`,
		`1 unpack [z] {a:1}`,
		`[unpack [z] {a:1}]`,
		`def m2 {x:1} unpack [y] m2`,
		"def m2 {x:1}\nunpack [y] m2",
		`def mk fn [[][Any][[1]]] end each (mk) [unpack [z] {a:1}]`,
		`def mk fn [[][Integer][1]] end each (mk) [unpack [z] {a:1}]`,
		`case 1 Integer`,
		`  case 1 Integer`,
		`case [1 drop] [5 "five" "other"]`,
		`mini foo 'x'`,
		`mini   foo 'x'`,
		`parse foo 'x'`,
		`emit foo {a:1}`,
		// traps whose position was already right stay right
		`gen [T]`,
		`while [] [1]`,
		`def v:Integer "s"`,
		`import "boru:math-util"  MathUtil!.nope`,
		`def loopy (macro [[a] [quote [loopy unquote a]]])  macroexpand (loopy 1)`,
	} {
		if _, errI := mustNew(t).RunInterp(src); errI == nil {
			t.Errorf("%q: the interpreter raised nothing — not a trap program", src)
			continue
		}
		requireCompiledParity(t, src)
	}
}
