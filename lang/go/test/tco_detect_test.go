package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// End-to-end battery for the tail-call detection probe
// (design/legacy/TCO-STAGED.10.ignore Stage 2): real programs through the
// production registry, asserting exactly how many fn-body dispatches
// the probe classifies as tail calls. Detection is telemetry-only —
// these counts are the contract the optimisation stages act on, so
// both directions matter: the shapes that MUST fire and the shapes
// that must NOT (a wrong accept here becomes a wrong frame teardown
// in Stage 3).

func detectTailCalls(t *testing.T, src string) int64 {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.NewTop(reg).Run(values); err != nil {
		t.Fatalf("run: %v", err)
	}
	return reg.TCO.Detected
}

func TestDetectTailSelfRecursion(t *testing.T) {
	// s2 5 0: the top-level call is not inside a frame; the 5
	// recursive calls (n=5..1) are tail calls.
	got := detectTailCalls(t,
		`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 5 0`)
	if got != 5 {
		t.Errorf("tail self-recursion detected %d, want 5", got)
	}
}

func TestDetectMutualTailRecursion(t *testing.T) {
	// isev 4 → isod 3 → isev 2 → isod 1 → isev 0: four inner tail
	// dispatches.
	got := detectTailCalls(t,
		`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] `+
			`def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 4`)
	if got != 4 {
		t.Errorf("mutual tail recursion detected %d, want 4", got)
	}
}

func TestDetectValueAccretingBody(t *testing.T) {
	// Each level parks a value below the call — inert for a shell
	// teardown, so the probe accepts (with ValuesBelow flagged; a full
	// frame replacement gates on that flag separately).
	got := detectTailCalls(t,
		`def m fn [[n:Integer] [] [if (n lte 0) [] [n mul 2 m (n sub 1)]]] m 3`)
	if got != 3 {
		t.Errorf("value-accreting recursion detected %d, want 3", got)
	}
}

func TestDetectRejectsNonTailShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			// The pending add Forward below the call consumes the
			// frame's result — the forward half alone would match;
			// the backward half must reject.
			name: "pending forward below (n add (s ...))",
			src:  `def s fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (s (n sub 1))]]] s 5`,
		},
		{
			name: "call feeding a parked def",
			src:  `def g fn [[n:Integer] [Integer] [n add 1]] def x (g 41) x`,
		},
		{
			name: "call feeding a pending word at top level",
			src:  `def inc fn [[n:Integer] [Integer] [n add 1]] 5 add (inc 1)`,
		},
		{
			name: "single top-level call (no enclosing frame)",
			src:  `def g fn [[n:Integer] [Integer] [n add 1]] g 1`,
		},
	}
	for _, tc := range cases {
		if got := detectTailCalls(t, tc.src); got != 0 {
			t.Errorf("%s: detected %d tail calls, want 0", tc.name, got)
		}
	}
}
