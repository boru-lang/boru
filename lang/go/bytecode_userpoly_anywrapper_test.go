package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestUserPolyAnyWrapperDispatch pins the fix for a multi-overload user fn
// dispatched on a value forwarded through a wrapper fn's `Any` parameter. The
// wrapper's arg generalises to a STRICT Any carrier (core_helpers.go), which
// used to reach only the `Any` overload — so `matchSignature` committed the
// `Any` arm statically and the compiler baked it, diverging from the
// interpreter, which re-dispatches on the true runtime value. The bug showed up
// inside an each/fold body, where a loop-variable key keeps the intermediate
// value gradual instead of constant-folding to a concrete type.
//
// The fix: a strict Any carrier is treated as reaching every same-arity arm
// (isAnyCarrier / dynamicReachableOverloadCount) and arms the user-poly runtime
// re-match (planUserPolyDispatch), so the VM re-runs MatchSignature on the real
// value — exactly like the interpreter. compile == interpret MUST hold, and the
// re-match must pick the RIGHT arm (a Map value → the Map arm, a leaf → the Any
// arm), not merely bias to one.
func TestUserPolyAnyWrapperDispatch(t *testing.T) {
	const defs = `def nk fn [ [v:Map] [String] ["map"] [v:Any] [String] ["leaf"] ]
def hop fn [[cv:Any] [String] [ nk (cv) ]]
`
	cases := []struct{ name, src, want string }{
		// The bug: a Map forwarded through hop's Any param inside an each body.
		// Compiled used to return "leaf"; must be "map" on both surfaces.
		{"map through Any wrapper in each body",
			defs + `def doc {meta: {age: 36}}
(each [ var [[k] (hop (doc get (k))) ] ] (keys doc))`, "map"},
		// The complement — a leaf (Integer) value through the same wrapper must
		// still dispatch to the Any arm, proving the re-match picks the right arm
		// rather than always biasing to Map.
		{"leaf through Any wrapper in each body",
			defs + `def doc {age: 36}
(each [ var [[k] (hop (doc get (k))) ] ] (keys doc))`, "leaf"},
		// Through a fold body (the other higher-order path).
		{"map through Any wrapper in fold body",
			defs + `def doc {meta: {age: 36}}
(fold [ var [[k acc] (push (hop (doc get (k))) acc) ]] (keys doc) [])`, "map"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict declined where it must compile: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter errored: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			gs := fmt.Sprint(got)
			if !strings.Contains(gs, c.want) {
				t.Errorf("got %v, want it to contain %q", got, c.want)
			}
			// Guard against the pre-fix wrong arm leaking through.
			other := map[string]string{"map": "leaf", "leaf": "map"}[c.want]
			if strings.Contains(gs, other) {
				t.Errorf("got %v — wrong overload (%q) selected", got, other)
			}
		})
	}
}
