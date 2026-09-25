package lang

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// requireSameVerdict runs src on both lanes and requires the same value, or
// the same error by code AND detail — the positions a return-contract error
// carries differ by lane (NUR118: the compiled lane blames the unit, the
// interpreter the call site), so the rendered text is not compared.
func requireSameVerdict(t *testing.T, src string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if noteCompileDefect(t, src, gotC, errC) {
		t.Errorf("%q: this shape is meant to compile and run", src)
		return
	}
	if !compiled {
		t.Errorf("%q: expected the compiled path to run", src)
	}
	if (errC == nil) != (errI == nil) {
		t.Errorf("%q: verdict divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		return
	}
	if errC == nil {
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: value divergence: compiled=%v interp=%v", src, gotC, gotI)
		}
		return
	}
	var aeC, aeI *core.BoruError
	if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
		t.Errorf("%q: expected BoruError from both engines, got c=%v i=%v", src, errC, errI)
		return
	}
	if aeC.Code != aeI.Code || aeC.Detail != aeI.Detail {
		t.Errorf("%q: error divergence\n compiled=[%s] %s\n interp=[%s] %s", src, aeC.Code, aeC.Detail, aeI.Code, aeI.Detail)
	}
}

// TestModuleFnReturnContractIsTheFrames pins NUR191's close. A boru fn
// defined inside a module runs its body through the CallBoru seam in the
// module's registry, and that seam enforced only the declared TYPES over
// the aligned tail of the residual (enforceCallBoruReturns) — never the
// COUNT the spliced frame's ReturnCheck raises — so `def d1 fn
// [[x:Integer][Integer][x 3]]` answered `[10 3]` as a module fn where the
// same fn on the main registry, and the compiled module fn, raise
// `expected 1 return value(s), got 2`; and a factory's residual `[(mk x)
// 3]` handed its PARKED closure and the 3 back to the caller's tape, where
// the closure re-stepped over the 3 (13 for the count error). A named call
// through the seam enforces the frame's count now, before the types
// (CallBoruStrict, NamedFnReturnCount), and a user fn's single returned
// closure delivered as a handler result is parked where it lands, as the
// frame's return is (`3 M.d1 10` is `[3 fn (Integer)]` on both lanes).
func TestModuleFnReturnContractIsTheFrames(t *testing.T) {
	const mk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] `
	for _, src := range []string{
		// NUR191's witnesses: the count, and the parked closure's re-step.
		`import module [` + mk + `def d1 (fn [[x:Integer][Integer][(mk x) 3]]) export "M" {d1: d1/v}] end M.d1 10`,
		`import module [` + mk + `def d1 fn [[x:Integer][Any][(mk x) 3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end 3 M.d1 10`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end (M.d1 10) 3`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end ((M.d1 10) 3)`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end 3 (M.d1 10) apply`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end each (M.d1 10) [1 2]`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end M.d1 10 apply`,
		`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end (M.d1 10)/v apply`,
		`import module [` + mk + `def d1 fn [[x:Integer][][(mk x) 3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [` + mk + `def d1 fn [[x:Integer][Integer][((mk x) 3)]] export "M" {d1: d1/v}] end M.d1 10`,
		// The count contract on its own: too many, too few, the wrong type
		// beside the wrong count (the frame raises the count first), a
		// two-return contract met, a `do` body's residual, an unnamed param's
		// leftover, and the contracts a module fn keeps meeting.
		`import module [def d1 fn [[x:Integer][Integer][x 3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[x:Integer][Any][x 3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[x:Integer][Integer][]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[x:Integer][Integer][x 'a']] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[x:Integer][Integer Integer][x 3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[x:Integer][Integer][do [x 3]]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def d1 fn [[Integer][Integer][3]] export "M" {d1: d1/v}] end M.d1 10`,
		`import module [def inc fn [[n:Integer][Integer][n add 1]] export "M" {inc: inc/v}] end 5 M.inc`,
		`import module [def inc fn [[n:Integer][Integer][n add 1]] export "M" {inc: inc/v}] end M.inc 5 add 1`,
		`import module [def one fn [[][Integer][1]] export "M" {one: one/v}] end M.one`,
		`import module [def two fn [[][Integer Integer][1 2]] export "M" {two: two/v}] end M.two`,
		`import module [def one fn [[][Function][([] => [42])]] export "M" {one: one/v}] end (M.one) apply`,
		`import module [def one fn [[][Function][([] => [42])]] export "M" {one: one/v}] end M.one apply`,
		`import module [def ff fn [[][Function][inc/v]] def inc fn [[n:Integer][Integer][n add 1]] export "M" {ff: ff/v}] end 5 (M.ff)`,
		`import module [def ff fn [[x:Integer][Function][inc/v]] def inc fn [[n:Integer][Integer][n add 1]] export "M" {ff: ff/v}] end 5 M.ff 1`,
		// The main-registry twins, unchanged: the frame path's own rule.
		mk + `end def d1 fn [[x:Integer][Integer][(mk x) 3]] end d1 10`,
		mk + `end def d1 fn [[x:Integer][Function][(mk x)]] end 3 d1 10`,
		mk + `end def d1 fn [[x:Integer][][(mk x) 3]] end d1 10`,
		`def d1 fn [[x:Integer][Integer][x 3]] end d1 10`,
		`def d1 fn [[x:Integer][Integer][x 'a']] end d1 10`,
	} {
		requireSameVerdict(t, src)
	}
	// The interpreter's own answers.
	for _, c := range []struct{ src, want string }{
		{`import module [` + mk + `def d1 fn [[x:Integer][Integer][(mk x) 3]] export "M" {d1: d1/v}] end M.d1 10`, "ERROR:d1: expected 1 return value(s), got 2 — [fn (Integer) 3]"},
		{`import module [` + mk + `def d1 fn [[x:Integer][Function][(mk x)]] export "M" {d1: d1/v}] end 3 M.d1 10`, "[3 fn (Integer)]"},
		{`import module [def d1 fn [[x:Integer][Integer][x 3]] export "M" {d1: d1/v}] end M.d1 10`, "ERROR:d1: expected 1 return value(s), got 2 — [10 3]"},
		{`import module [def d1 fn [[x:Integer][Integer][x 'a']] export "M" {d1: d1/v}] end M.d1 10`, "ERROR:d1: expected 1 return value(s), got 2 — [10 'a']"},
		{`import module [def d1 fn [[x:Integer][Integer][]] export "M" {d1: d1/v}] end M.d1 10`, "ERROR:d1: expected 1 return value(s), got 0"},
		{`import module [def d1 fn [[x:Integer][Integer][x 3]] export "M" {d1: d1/v}] end do [M.d1 10] error [dot message]`, "[d1: expected 1 return value(s), got 2 — [10 3]]"},
	} {
		d := mustNew(t)
		got, err := d.RunInterp(c.src)
		if strings.HasPrefix(c.want, "ERROR:") {
			var ae *core.BoruError
			if !errors.As(err, &ae) || ae.Detail != strings.TrimPrefix(c.want, "ERROR:") {
				t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
			}
			continue
		}
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%q: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
	}
}

// TestModuleFnNamedValueThroughReachPending pins NUR204 as it stands: a
// module fn returning a NAMED fn value, called through its reach group with
// a value beneath — `5 M.ff` over `def ff fn [[][Function][inc/v]]` — is 6
// on the interpreter (the reach group `( M dot ff )` never parks: its
// collapse re-steps the lone survivor, a named fn at the pointer, which
// dispatches over the 5 — ADR-011, a name always calls) and `[5 fn
// inc(Integer)]` on the compiled lane, which seats the returned value as
// data; `M.ff 5` the same, 6 for `[fn inc(Integer) 5]`. The main-registry
// twin `5 ff` parks on both lanes (the frame path), and so does the
// parenthesised `5 (M.ff)`. Closing it must update this pin.
func TestModuleFnNamedValueThroughReachPending(t *testing.T) {
	const mod = `import module [def ff fn [[][Function][inc/v]] def inc fn [[n:Integer][Integer][n add 1]] export "M" {ff: ff/v}] end `
	for _, c := range []struct{ src, wantI, wantC string }{
		{mod + `5 M.ff`, "[6]", "[5 fn inc(Integer)]"},
		{mod + `M.ff 5`, "[6]", "[fn inc(Integer) 5]"},
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.wantI {
			t.Errorf("%q: the interpreter re-steps the reach group's named survivor: %v / %v", c.src, gotI, errI)
		}
		if errC != nil || !compiled || fmt.Sprint(gotC) != c.wantC {
			t.Errorf("%q: NUR204's compiled value %v / %v (compiled=%v), pinned as %s — closing the divergence must update this pin", c.src, gotC, errC, compiled, c.wantC)
		}
	}
}
