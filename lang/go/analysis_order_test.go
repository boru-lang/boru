package lang

// ANALYSIS ORDER IS PROGRAM ORDER — the end-to-end pins for Stage 4b
// (compiler/go/unit_memo.go). Every row here was MEASURED on the default
// lane before the fix — `boru run`, no flags, exit 0 — and the "before"
// column in each comment is what it answered against the interpreter.
//
// Two mechanisms were behind them, and this file pins both:
//
//   - the unit MEMO was binding-insensitive, so one unit baked at one program
//     point served every later call site whatever the bindings were there
//     (the frozen-read class, previously REFUSED; now re-recorded per site);
//   - a leaking body's compile RE-RUN began from the state its analysis run
//     LEAKED, so every read before the body's own rebind baked the rebound
//     value (`do [ k  def k 9  k ]` → `9 9` against `5 9`).
//
// The rows are parity rows: the compiled lane must COMPILE (a refusal here is
// the fix regressing to the old hammer) and agree with the interpreter. The
// second test pins the shapes the fix deliberately leaves to the interpreter,
// each with the reason it refuses under, so a silent graduation or a drift of
// the refusal the interpreter absorbs is visible.

import (
	"strings"
	"testing"
)

func TestAnalysisOrderShapesCompileWithParity(t *testing.T) {
	rows := []struct{ src, before string }{
		// --- the leaked-state class (a body's re-run started from its leak) ---
		{`def k 5  do [ k  def k 9  k ]`, "9 9 for 5 9"},
		{`def m {a:1}  do [ m  def m {a:2}  m ]`, "{a:2} {a:2} for {a:1} {a:2}"},
		{`def k 5  do [ def t k  def k 9  t ]`, "9 for 5"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ f  def k 9  f ]`, "11 11 for 7 11"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ f  def k 9 ]  f`, "11 11 for 7 11"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ def k 9  f ]  f`, "11 11 (already right; the control)"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  [1 2] each [ f  def k 9 ]`, "[11 11] for [7 11]"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  [1 2] each [ f  def k 9 ]  f`, "[11 11] 11 for [7 11] 11"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  [1 2] fold [ f  def k 9  add ] 0`, "13 0 for 9 0"},
		{`def k 5  [1 2] fold [ k  add  def k 9 ] 0`, "11 0 for 7 0"},
		{`def k 5  [1 2] scan [ k  add  def k 9 ] 0`, "[1 11] 0 for [1 7] 0"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  def g fn [[] [Integer] [do [ f  def k 9  f ] add]]  g`, "22 for 18"},
		// --- the caller-frame shadowing class (one unit, two frames) ---
		{`def k 5  def f fn [[] [Integer] [k add 2]]  def g fn [[] [Integer] [def k 9  f]]  g  f`, "11 11 for 11 7"},
		// --- the frozen-read class, previously refused to the interpreter ---
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  def k 9  f`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  def k fn [[] [Integer] [9]]  f`, "refused (a kind change)"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  def k 9  f  def k 11  f`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  [1 2] each [ f  def k 9 ]`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  for 2 [ f  def k 9 ]`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ f  def k 9 ]  f  def k 12  f`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ f ]  def k 9  do [ f ]`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  do [ f ]  def k 9  f`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  do [ f ]  do [ def k 9 ]  do [ f ]`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  if true [ f  def k 9  f ] [0]`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  if true [ f  def k 9  f ] []`, "refused"},
		// The type twin in a `do`, and the minted-type body the re-run must
		// re-define over its surviving lattice part.
		{`def x 3  do [ def P (refine Integer)  def y:P 5  y is P ]  y is P`, "compiled (the control the type carry-over keeps)"},
		// A unit whose start binding is concrete keeps baking per program
		// point — the residual-order hazard is for LIVE reads only.
		{`def k 5  def f fn [[] [Integer Integer] [k  def k 9  k]]  f`, "5 9 (already right; the control)"},
		// The stored-ref and container spellings of a fn value: their rebind
		// safety is the per-ref poisoning, untouched by this increment.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  def h f/v  h/v apply  def k 9  h/v apply`, "refused"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  def m {g: f/v}  (m.g)  def k 9  (m.g)`, "7 11 (control)"},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  [f/v] each [apply]  def k 9  [f/v] each [apply]`, "[7] [11] (control)"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: must compile natively (before the fix: %s); compiled=false err=%v", c.src, c.before, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// The shapes the fix leaves to the interpreter, and why. Each is a SOUND
// fallback — the interpreter's answer is the answer — pinned with the reason
// so a drift is visible. A row that starts compiling here has graduated: move
// it to the parity test above with its interpreter answer.
func TestAnalysisOrderSoundFallbacks(t *testing.T) {
	// The fallback rows run under the one-release silent-fallback hatch so
	// the compiled entry point answers through the interpreter instead of
	// surfacing compile_failed (the same contract
	// TestModuleReadRebindRefusesAndMatches pins).
	rows := []struct{ src, reason string }{
		// The residual-order hazard: a re-pushable residual read (a live
		// lookup, a loop-carried slot) that precedes a rebind of the same
		// name in the same body is re-pushed AFTER the rebind. Measured
		// before this refusal: `9 9` for `5 9` on the first two.
		{`def k (1 add 4)  def f fn [[] [Integer Integer] [k  def k 9  k]]  f`,
			"fn f: residual read of `k` precedes its rebind in the same body (Stage 4b)"},
		{`def k 5  for 2 [ k  def k 9 ]`,
			"for: residual read of `k` precedes its rebind in the same body (Stage 4b)"},
		{`def k 5  for 2 [ k  def k (k add 1) ]`,
			"for: residual read of `k` precedes its rebind in the same body (Stage 4b)"},
		// The multi-out arm: the hazard sits BELOW the top of a body that
		// nets two values per iteration (interpreter: `5 1 9 1`).
		{`def k 5  for 2 [ k  def k 9  1 ]`,
			"for: residual read of `k` precedes its rebind in the same body (Stage 4b)"},
		// A `do` result is a computed value, so the second body's read of
		// it is live, and the same hazard applies inside that body.
		{`def k 5  do [ k  def k 9  k ]  do [ k  def k 12  k ]`,
			"fn do$body: residual read of `k` precedes its rebind in the same body (Stage 4b)"},
		// A multi-run body reading a name it rebinds: the read is
		// iteration-varying, its residual re-push would trail the resident
		// install, so the closure declines. Since S1a (2026-09-19, each
		// declares CompileDynBody) the decline no longer lands on the word's
		// Stage-2 refusal but on the dyn-body seat's twin-regime gate: the
		// body's bind transition has no stream placement in a multi-run body.
		// Measured before: `[9 9]` for `[5 9]`, `[6 7]` for `[5 6]`.
		{`def k 5  [1 2] each [ k  def k 9 ]`, "twin regime: a bind transition has no stream placement"},
		{`def k 5  [1 2] each [ k  def k (k add 1) ]`, "twin regime: a bind transition has no stream placement"},
		// A resident def whose value is a live read of the same body's
		// rebound name has no re-pushable source for the resident install.
		{`def k 5  [1 2] each [ def t k  def k 9  t ]`, "arm-resident def `t` of unknown provenance"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — this shape has graduated; move it to the parity rows with its interpreter answer", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, compiled, errC, _, _ := runBothEngines(t, c.src)
		if compiled {
			t.Errorf("%q: compiled — this shape has graduated; move it to the parity rows", c.src)
			continue
		}
		// No fallback re-runs it, so there is no compiled answer to compare:
		// the failure is booked as the defect it is (compile_defect_test.go).
		requireCompileDefect(t, c.src, gotC, errC)
	}
}
