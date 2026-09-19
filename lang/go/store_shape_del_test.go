package lang

// The `del` check-mode twins (native_storage.go delMapTypedReturns /
// delFlexMapReturns): a deleted flex key must stop narrowing. The shape
// model is join-only monotone ("widen, never replace" —
// store_shape.go::RecordKey), so del expresses forgetting as widening
// the key's shape entry to dynamic Any; the Map form keeps set's
// typed-residual contract. Paired positive/negative per the repo's test
// discipline: the negative twin pins that WITHOUT del the narrow claim
// survives, so the widening is attributable to del alone.

import (
	"fmt"
	"strings"
	"testing"
)

// TestStoreShapeDelWidensKey — headline: after `del k f` a read of f.k
// is dynamic(Any), where the del-free twin stays dynamic(Integer).
func TestStoreShapeDelWidensKey(t *testing.T) {
	cases := []struct {
		src  string
		want string
		why  string
	}{
		// Negative twin (no del): the set-then-read narrow claim holds.
		{`def f (flex {}) set k 1 f end f.k`,
			"FlexMap dynamic(Integer)", "without del the key stays narrowed"},
		// Positive: del widens the key to dynamic Any (join-only model).
		{`def f (flex {}) set k 1 f end del k f end f.k`,
			"FlexMap FlexMap dynamic(Any)", "del widens the deleted key"},
		// The widened read still feeds downstream dispatch cleanly.
		{`def f (flex {}) set k 1 f end del k f end (f.k) add 1`,
			"FlexMap FlexMap dynamic(Integer)", "widened key composes without a false error"},
		// String-key spelling routes through the same twin.
		{`def f (flex {}) set 'k' 1 f end del 'k' f end f.k`,
			"FlexMap FlexMap dynamic(Any)", "string-key del widens too"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q (%s): checker flagged an error; want clean", c.src, c.why)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q (%s): residual %q, want %q", c.src, c.why, got, c.want)
		}
	}
}

// TestStoreShapeDelCompileDiscipline — every shape path is gated to
// plain (non-Compiling) passes: del programs still compile natively and
// the compiled/interpreted results agree (the compiler exemption added
// alongside the shape twins — del is set's inverse in the quoted-
// operand family). This is the positive direction of the exemption;
// the negative (other quoted-operand words must not poly/compile) stays
// pinned by eng's carrier_seam6a/emit_seam7 tests.
func TestStoreShapeDelCompileDiscipline(t *testing.T) {
	compiles := []struct {
		src string
	}{
		{`def f (flex {a:1 b:2}) del a f end f.b`},
		{`{a:1 b:2} del a`},
		{`def m ({a:1 b:2} del 'a') m`},
	}
	for _, c := range compiles {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: expected native compile; reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: compiled with a FALLBACK island; del must compile natively", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, errI := d.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled/interpreted parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				c.src, compiled, gotC, errC, gotI, errI)
		}
	}
}

// TestStoreShapeDelMapResidual — the Map (copy-returning) form's twin:
// del's residual is a Map carrier, and chaining reads/writes through it
// stays clean; behaviour is pinned IDENTICAL to set's residual so the
// two mutators cannot drift apart in check mode.
func TestStoreShapeDelMapResidual(t *testing.T) {
	pairs := []struct {
		del string
		set string
		why string
	}{
		{`({a:1 b:2} del a) dot b`, `({a:1 b:2} set c 3) dot b`, "plain map residual read"},
		{`(({: Integer a:1 b:2}) del a) dot b`, `(({: Integer a:1 b:2}) set c 3) dot b`,
			"typed map residual read"},
	}
	for _, p := range pairs {
		dstack, dflag := shapeCheckStack(t, p.del)
		sstack, sflag := shapeCheckStack(t, p.set)
		if dflag || sflag {
			t.Errorf("%s: checker flagged an error; want clean (del=%v set=%v)", p.why, dflag, sflag)
			continue
		}
		if got, want := strings.Join(dstack, " "), strings.Join(sstack, " "); got != want {
			t.Errorf("%s: del residual %q diverges from set residual %q", p.why, got, want)
		}
	}
}
