package lang

import (
	"fmt"
	"testing"
)

// A fn body whose residual is a `/v` reference must RETURN that reference, not
// the result of calling it (design/FUNCTION-VALUE-SCOPE.0.md §12.6, ADR-016).
//
// `def h fn [[f:Function] [Any] [f/v]]` used to return `7` interpreted — the
// call's result — where the compiler correctly returned the function value.
// The `/v` was doing its job at the word step; the call came later, when the
// fn frame's collapse re-stepped the body's residual. A frame collapse
// delivers a RETURN, which is not a fresh use of the value, so it must not
// invoke it (core/go/engine.go's fnReturnPark).
//
// The maintainer ruling this pins: `/v` is not sticky — it deactivates the ONE
// use it is attached to — and arity never changes the answer. Since the BROAD
// park (NUR073 clause 3) a USER paren places its collapsed value exactly as a
// frame does; the remaining negatives are the value conditions — a frame
// returning a non-Function or several values is untouched.
//
// Every case runs on BOTH engines. The whole point of the fix was that they
// disagreed; a repair on one engine alone would just move the divergence.
type fnReturnParkCase struct {
	name string
	src  string
	want string
	// why names the rule the row pins, quoted in the failure message so a
	// regression reads as the rule it broke rather than as a bare mismatch.
	why string
}

func fnReturnParkCases() []fnReturnParkCase {
	const zero = "def z fn [[] [Integer] [ 42 ]]\n"
	const inc = "def inc fn [[n:Integer] [Integer] [ n add 1 ]]\n"
	const hold = "def h fn [[f:Function] [Any] [ f/v ]]\n"

	return []fnReturnParkCase{
		{
			// The break itself: a 0-arg fn reached through a Function param.
			// Pre-fix this was `[42]` interpreted — /v silently doing the
			// opposite of what it says.
			name: "0-arg param returned by /v",
			src:  zero + hold + "typeof (h z/v)",
			want: "[Function]",
			why:  "a /v'd param returned from a body is the fn, not its result",
		},
		{
			// Not param-specific: the same body shape over a module-scope
			// name. If the fix had keyed on the frame's parameter binding
			// rather than on the frame, this row would still be `[42]`.
			name: "module-scope name returned by /v",
			src:  zero + "def h fn [[] [Any] [ z/v ]]\n" + "typeof (h)",
			want: "[Function]",
			why:  "the park is a property of the frame, not of a param binding",
		},
		{
			// ADR-016: arity and origin never change how a function behaves.
			// Arity >= 1 already parked before the fix; it must still park.
			name: "1-arg param returned by /v",
			src:  inc + hold + "typeof (h inc/v)",
			want: "[Function]",
			why:  "ADR-016: arity does not change the answer",
		},
		{
			// BROAD (NUR073 clause 3): a USER paren places its collapsed value
			// exactly as a frame does — `(z/v)` is the same as `z/v`, per the
			// maintainer ruling "parens do not re-step; they place a value or
			// values on the forward stack".
			name: "user paren places too (BROAD)",
			src:  zero + "typeof (z/v)",
			want: "[Function]",
			why:  "a paren places its collapsed value — grouping is not a fresh use (NUR073 clause 3)",
		},
		{
			// `ref` is the same operation spelled long, and it must agree
			// with `/v` in both directions — inert under a paren alike.
			name: "user paren places a ref too",
			src:  zero + "typeof (valof z)",
			want: "[Function]",
			why:  "`ref` and `/v` are one operation in every position",
		},
		{
			// NEGATIVE — a frame returning a NON-Function is untouched. This is
			// the arm that would break every ordinary call if the park test
			// dropped its value check.
			name: "frame returning a scalar is untouched",
			src:  zero + "z",
			want: "[42]",
			why:  "the park applies only to a Function residual",
		},
		{
			// NEGATIVE — a frame returning MORE than one value is untouched:
			// the park is the exactly-one-survivor case, and stepping past a
			// multi-value collapse would strand the rest.
			name: "multi-value frame is untouched",
			src:  "def p fn [[] [Integer Integer] [ 1 2 ]]\n" + "p",
			want: "[1 2]",
			why:  "the park is the exactly-one-survivor case",
		},
	}
}

func TestFnFrameParksFunctionReturn(t *testing.T) {
	for _, tc := range fnReturnParkCases() {
		t.Run(tc.name, func(t *testing.T) {
			engines := []struct {
				name string
				run  func(*Boru) ([]any, error)
			}{
				{"interp", func(a *Boru) ([]any, error) { return a.RunInterp(tc.src) }},
				{"compiled", func(a *Boru) ([]any, error) {
					got, _, err := a.RunCompiled(tc.src)
					return got, err
				}},
			}
			for _, eng := range engines {
				t.Run(eng.name, func(t *testing.T) {
					a, err := New()
					if err != nil {
						t.Fatalf("New: %v", err)
					}
					got, err := eng.run(a)
					// A compiler COMPILE FAILURE is an acceptable outcome: the default
					// driver falls through to the interpreter, so the user
					// still gets the right answer. Compiling to a DIFFERENT
					// answer is what this test exists to forbid.
					if isCompileFailure(err) {
						t.Skipf("compilation declined (%v)", err)
					}
					if err != nil {
						t.Fatalf("run: %v", err)
					}
					if s := fmt.Sprint(got); s != tc.want {
						t.Errorf("got %s, want %s — %s", s, tc.want, tc.why)
					}
				})
			}
		})
	}
}
