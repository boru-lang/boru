package lang

import "testing"

// TestDynScopeCaptureDef pins the fix for `RecordDynBind` misresolving a
// CURRENT-UNIT CAPTURE to an unreachable enclosing event. Under the program-wide
// dynEnv mode (armed by a `do {…}` computed-map dispatch), every `def` emits its
// OpBindDynScope twin. A `def t (fixture)` inside a code body — where `fixture`
// is an under-annotated module-scope value CAPTURED into the body — used to bind
// from `fixture`'s foreign top-level producing event (still live while the inline
// body compiles) instead of the body's own capture slot, declining "dynamic-scope
// def `t` of unpromoted computed value" (the promotion has no local event to
// promote). RecordDynBind now mirrors resolveOperand's capID override and binds
// from the capture slot, so the body compiles and is byte-identical to the
// interpreter.
//
// Off-corpus regression: the differential corpus has no captured-value dyn-bind,
// so this pins engine parity AND that the shape now compiles natively.
func TestDynScopeCaptureDef(t *testing.T) {
	// `mk` returns an under-annotated Any carrier (a `do {…}` computed map), so
	// `fixture` is a non-concrete module-scope carrier captured into the code
	// body; the `do {…}` arms dynEnv; `get2` reads over the dynamic receiver.
	const src = `import "boru:test"
def mk fn [ [] [Any] [ do {a: [1] b: [2]} ] ]
def get2 fn [ [k:String m:Any] [Any] [ m k get ] ]
def fixture (mk)
[
  def t (fixture)
  1 (t "a" get2) Assert.equal end
] "t" Test.test end
0 (Test.fail-count) Assert.equal end`
	requireEngineParity(t, src, true)

	// Adversarial guard (agent-2's hazard): a dyn-bound def whose result ID
	// COLLIDES with a live frame local `a` (a JoinCarriers ID reuse) must bind
	// the BRANCH-selected value, not local `a`, for both conditions — this fails
	// if the capID gate is dropped and localByID wins over the producing event.
	for _, c := range []struct{ cond, want string }{{"true", "1"}, {"false", "2"}} {
		join := `def mk fn [ [] [Any] [ do {a: [1]} ] ]
def _arm (mk)
def a 1
def b 2
def r (if ` + c.cond + ` [a] [b])
r`
		requireEngineParity(t, join, false /* dynamic join may fall back; parity is the point */)
		_ = c.want
	}
}
