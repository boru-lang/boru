package lang

import "testing"

// TestDoCatchMultiValueArity pins the fix for a FALLIBLE multi-value `do` body
// under a catch. `do` catches a body raise into ONE Error value, so a body that
// nets N values with no raise but 1 Error on a raise has a runtime-VARIABLE
// count. The compiler used to seat the static N (via the closure / dyn-body /
// generic RecordCall paths), which UNDERFLOWED on the caught path — a
// STORE_LOCAL underflow (the voxgig-boru/trie `codec-roundtrip` shape:
// `def msg (do [(s.decode) "x"] error […])`). doListReturnsFn now refuses a
// fallible multi-value body so it rides the sound interpreter fallback, while a
// pure / infallible multi-value body still compiles at its exact arity.
//
// The differential corpus is BLIND to this (a fallible body that never raises
// at its test input passes parity while latently unsound), so these are
// hand-pinned off-corpus regressions: engine parity (compiled == interpreter,
// including the raised error) AND the native/fallback expectation per shape.
func TestDoCatchMultiValueArity(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	// A module with a value-dependently-raising fn (map-decode-like) and an
	// always-raising one, reached as `M.dec` / `M.boom` (a Reach dispatch).
	const mod = `import module [
  def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]]
  def boom fn [[x:Any] [Any] [ raise bad_input "always" ]]
  export "M" {dec: dec/v, boom: boom/v}
] end `

	// The COMPILES-natively half (pure/infallible multi-value bodies keeping
	// their exact N) lives in the main corpus now:
	// lang/spec/bytecode-migrated.tsv (WS4 migration) — the census owns
	// native-compile + parity for those rows.

	// FALLS BACK (refuses natively) — fallible multi-value bodies. Parity must
	// still hold: the caught-error path is byte-identical to the interpreter,
	// no STORE_LOCAL underflow. wantCompiled=false (the refusal the interpreter absorbs).
	fallback := []string{
		// The trie codec shape: a Reach (module fn) that RAISES, caught, bound.
		mod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`,
		// The same body that does NOT raise at this input still refuses (the
		// static shape is fallible) — and stays correct via fallback.
		mod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`,
		// An always-raising module fn.
		mod + `do [(M.boom 5) "x"] error [dot code]`,
		// A value-diverging native (`div`, CompileValueDiverges) with a NON-static
		// divisor so the body stays multi-value (div does not statically diverge):
		// refuses on the fallible-native path, correct via fallback (y=5, no raise).
		`def y 5  do [(10 div y) 2]`,
		// A user (boru-body) fn that raises, caught.
		`def f fn [[x:Any] [Any] [raise bad_input "nope"]]  do [(f 5) 2] error [dot code]`,
		// A NATIVE module fn bound to a NAME (Module set, no boru body) — the
		// fnDefMayRaise Module!="" path. StructUtil.parse can raise on bad input.
		`import "boru:struct-util"  def g StructUtil.parse/v  do [(g "x") 2] error [dot code]`,
		// A bare module-NAMESPACE VALUE in the body — the wordMayRaise facet-map path.
		mod + `do [M 3] error [dot code]`,
		// A fallible call nested inside a branch-arm LIST within a paren — the
		// nested-list recursion (`(if … [M.boom] …)`).
		mod + `do [(if true [M.boom 5] [7]) 8] error [dot code]`,
	}
	for _, src := range fallback {
		requireEngineParity(t, src, false)
	}
}

// TestDoScalarFallibleAccessorCompiles pins the #stats fix: a SINGLE-value
// fallible `do` body whose declared happy-path residual is a SCALAR actually
// yields an Error when it raises (do catches the raise into an Error value), so
// a downstream Error accessor (`.code` / `.message`) used to no_signature on the
// bare scalar and force the code-body fallback (`code-body word test-test`, the
// stats_unit_test `((do [Stats.mean [] end]).code)` shape). doListReturnsFn now
// widens the scalar residual to the (scalar tor Error) union, so the accessor
// dispatches its Error overload and the body compiles natively. compile ==
// interpret MUST hold (the caught error is byte-identical).
func TestDoScalarFallibleAccessorCompiles(t *testing.T) {
	native := []string{
		// The stats shape: a scalar-declared always-raising fn, `.code` of the do.
		`def boom fn [[] [Integer] [ (raise bad_input/q "always") ]]
[ ((do [boom]).code) ]`,
		// `.message` over a Float-declared fallible do.
		`def boom fn [[] [Float] [ (raise bad_input/q "x") ]]
[ ((do [boom]).message) ]`,
		// The accessor read INSIDE a fn body (the real Test.test / Assert shape).
		`def ec fn [[] [Atom] [ (do [ (raise bad_input/q "e") ]).code ]]
[ (ec) ]`,
	}
	for _, src := range native {
		requireEngineParity(t, src, true)
	}
}

// TestDoFallibleTypeLiteralResidualBareNode pins the !IsBareTypeNode guard on
// the scalar→Error widening: when a FALLIBLE single-value `do` body's happy-path
// residual is a ROOT type literal (Any/None/Never — `.Parent == nil`), the
// widening must NOT fire. `add 1 2 drop X` leaves exactly the bare `X` node after
// the now-overflow-fallible add, so `doBodyMayRaise` is true and the branch is
// reached. A bare type node is not a scalar value (Any tor Error = Any, and
// None/Never aren't scalars), so widening it to (node tor Error) is meaningless.
// The guard also keeps the call-site nil-safety explicit rather than leaning on
// ConformsTo's receiver-nil handling. These shapes compile natively and stay
// byte-parity with the interpreter (both engines produce the bare node value).
func TestDoFallibleTypeLiteralResidualBareNode(t *testing.T) {
	for _, src := range []string{
		`do [add 1 2 drop Any]`,
		`do [add 1 2 drop None]`,
		`do [add 1 2 drop Never]`,
	} {
		requireEngineParity(t, src, true)
	}
}
