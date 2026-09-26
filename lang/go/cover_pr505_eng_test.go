package lang

import "testing"

// cover_pr505_eng_test.go — real programs for the VM paths PR #505 added in
// eng/go (vm.go, vm_dyn_apply.go) whose only witnesses were the corpus rows
// that pass through them on the answering arm. Each row reaches the named arm
// on the compiled lane and must agree with the interpreter.

// nfQRet is TestNamedFnCandidatesOpenShapes's `/q` fixture with the capturing
// overload's contract broken: the capture of `y` is an Atom where the
// declaration promises an Integer, so the capture itself raises.
const nfQRet = `def z fn [[] [Atom] [(quote z)]] end def y fn [[] [Integer] [42]] end ` +
	`def h fn [[] [Integer] [42]] end def h fn [[x:Atom/q] [Integer] [x]] end ` +
	`def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `

// TestLandingCaptureRaiseAgrees pins the error arms of the landing's `/q`
// claim (NUR190): the capture runs on the interpreter — over the value and
// the word alone where no island can be rebuilt (landingSkip: a literal's
// member), over the body from the word on where it can (landingDeopt: a
// top-level statement) — and a capture that raises raises the interpreter's
// own error, rendered identically (positions included) on both lanes.
func TestLandingCaptureRaiseAgrees(t *testing.T) {
	const want = "ERROR:h: return value 1: expected Integer, got Atom"
	agreeOnBothLanes(t, nfQRet+`[(m.f y)]`, want)
	agreeOnBothLanes(t, nfQRet+`m.f y def w 3 w`, want)
}

// TestValueDeliveredClosureParksAgrees pins the `/v` DELIVERY of a compiled
// CLOSURE the window does not fit (NUR124's fifth witness, the closure arm):
// the interpreter leaves the value as data beside its window, in the order
// the window was written — so a leading `(g/v 5)` inside a fn is the frame's
// count error with the fn beneath the 5, and a trailing `(5 g/v)` is the
// window unchanged, fn on top.
func TestValueDeliveredClosureParksAgrees(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][([s:String] => [k])]] end `
	agreeOnBothLanes(t, `def appv fn [[g:Function][Integer][(g/v 5)]] end `+mk+`appv (mk 1)`,
		"ERROR:appv: expected 1 return value(s), got 2 — [fn g(String) 5]")
	agreeOnBothLanes(t, mk+`def g (mk 1) end (5 g/v)`, "[5 fn g(String)]")
	agreeOnBothLanes(t, `def appv fn [[g:Function][Integer][(5 g/v)]] end appv (z:String => [z])`,
		"ERROR:appv: expected 1 return value(s), got 2 — [5 fn g(String)]")
}

// TestZeroArgLeadIslandRaiseAgrees pins the error arm of the 0-arg lead's
// island (NUR176): `(k bad/v)` over a 0-arg k fires k over nothing and then
// steps bad/v over its result, and a raise from that step is the
// interpreter's own, on both lanes.
func TestZeroArgLeadIslandRaiseAgrees(t *testing.T) {
	const zk = `def z fn [[] [Integer] [7]] end `
	agreeOnBothLanes(t, zk+`def bad fn [[n:Integer] [Integer] ['s']] end def h fn [[k:Function] [Integer] [(k bad/v)]] end h z/v`,
		"ERROR:bad: return value 1: expected Integer, got ProperString")
	agreeOnBothLanes(t, zk+`def bad fn [[n:Integer] [Integer] [n div 0]] end def h fn [[k:Function] [Integer] [(k bad/v)]] end h z/v`,
		"ERROR:division by zero")
}

// TestForeignFnValueBareReadRefusalAgrees pins dynApplyForeign's refusal of
// a module fn whose stamped unit reads a param BARE when the argument there
// is a fn (NUR217's rule on the foreign seam): the interpreter dispatches
// that read as a word, so the apply stays the island's and the fn fires —
// 42 and 7 on both lanes, never the fn value itself.
func TestForeignFnValueBareReadRefusalAgrees(t *testing.T) {
	const mod = `import module [def g fn [[f:Any] [Any] [f]] export "M" {g: g/v}] end `
	const ap = `def ap fn [[k:Function x:Function] [Any] [(k x/v)]] end `
	agreeOnBothLanes(t, mod+ap+`ap M.g/v ([] => [42])`, "[42]")
	agreeOnBothLanes(t, mod+`def z fn [[] [Integer] [7]] end `+ap+`ap M.g/v z/v`, "[7]")
	agreeOnBothLanes(t, `import module [def g fn [[f:Any] [Any] [f]] def run fn [[k:Function x:Function][Any][(k x/v)]] export "M" {g: g/v, run: run/v}] end M.run M.g/v ([] => [42])`, "[42]")
}

// The rows below reach VM arms that predate PR #505 (main-side lines the
// merged gate also needs), each through a real program on both lanes.

// TestTrailingClosureMemberNoMatchAgrees pins callDynamic's TRAILING no-match
// for a compiled fn-value CLOSURE (`5 m.f` over a factory's capturing
// lambda): the window as written stays — the arg beneath, the fn on top —
// where the lowering had rotated the fn under its arg for the apply. A
// window the closure fits applies it.
func TestTrailingClosureMemberNoMatchAgrees(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][([s:String] => [k])]] end def m {f: (mk 1)} end `
	agreeOnBothLanes(t, mk+`5 m.f`, "[5 fn (String)]")
	agreeOnBothLanes(t, mk+`"x" m.f`, "[1]")
}

// fnUtilMod imports fn-util and a module of three fns for `FnUtil.flip` to
// wrap: the wrapper's inner is then a module export, whose unit is its own
// load-time stamp (a DETACHED unit, hosted by dynApplyForeign).
const fnUtilMod = `import "boru:fn-util" import module [def sub2 fn [[a:Integer b:Integer][Integer][a sub b]] ` +
	`def dv fn [[a:Integer b:Integer][Integer][a div b]] def inc fn [[n:Integer][Integer][n add 1]] ` +
	`export "M" {sub2: sub2/v, dv: dv/v, inc: inc/v}] end `

// TestFlippedForeignFnAppliesAgree pins the modifier-wrapper arms of the two
// dynamic applies over a module fn: the wrapper resolves to what it wraps (a
// `FnUtil.flip` usurp, the args reversed) and the module fn's detached unit
// is hosted — the trailing member apply (callDynamic) and the def-read
// shaped apply (callDynMethod), its answer and its raise alike.
func TestFlippedForeignFnAppliesAgree(t *testing.T) {
	agreeOnBothLanes(t, fnUtilMod+`def m {f: (FnUtil.flip M.inc/v)} end 5 m.f`, "[6]")
	agreeOnBothLanes(t, fnUtilMod+`def fs (FnUtil.flip M.sub2/v) end (fs 3 10)`, "[7]")
	agreeOnBothLanes(t, fnUtilMod+`def fs (FnUtil.flip M.dv/v) end (fs 5 10)`, "[2]")
	agreeOnBothLanes(t, fnUtilMod+`def fs (FnUtil.flip M.dv/v) end (fs 0 10)`, "ERROR:division by zero")
}

// TestDynamicApplyRaisesAgree pins three dynamic applies' error arms against
// their answering twins: a 0-arg capturing closure under `apply` fires over
// nothing above the window (callDynApply), a module fn read from a map as a
// shaped method (callDynMethod over dynApplyForeign), and a member read at a
// quotation body's tail taking the element beneath it (callDynFrame's lone
// token) — each raise is the interpreter's own.
func TestDynamicApplyRaisesAgree(t *testing.T) {
	agreeOnBothLanes(t, `def mk0 fn [[k:Integer][Function][([] => [k div 0])]] end 5 (mk0 1)/v apply`, "ERROR:division by zero")
	agreeOnBothLanes(t, `def mk0 fn [[k:Integer][Function][([] => [k])]] end 5 (mk0 1)/v apply`, "[5 1]")
	const bad = `import module [def bad fn [[n:Integer][Integer][n div 0]] def ok fn [[n:Integer][Integer][n add 1]] export "M" {bad: bad/v, ok: ok/v}] end `
	agreeOnBothLanes(t, bad+`def m {f: M.bad/v} end m.f 5`, "ERROR:division by zero")
	agreeOnBothLanes(t, bad+`def m {f: M.ok/v} end m.f 5`, "[6]")
	agreeOnBothLanes(t, `def bad fn [[n:Integer][Integer][n div 0]] end def ops {inc: bad/v} end each [ops.inc] [1 2]`, "ERROR:division by zero")
	agreeOnBothLanes(t, `def inc fn [[n:Integer][Integer][n add 1]] end def ops {inc: inc/v} end each [ops.inc] [1 2]`, "[[2 3]]")
}
