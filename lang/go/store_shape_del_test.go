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
		// The widened read still feeds downstream dispatch cleanly. Its sum
		// is dynamic(Number): the widened key may hold a Float at run time,
		// so Integer (what the numeric tower's old blanket default said)
		// was unsound (check/go carrier.go ReturnsNumericBinary, 2026-09-27).
		{`def f (flex {}) set k 1 f end del k f end (f.k) add 1`,
			"FlexMap FlexMap dynamic(Number)", "widened key composes without a false error"},
		// ...while the del-free twin's narrowed Integer key still sums to an
		// Integer: the Number result is the widening's, not add's.
		{`def f (flex {}) set k 1 f end (f.k) add 1`,
			"FlexMap dynamic(Integer)", "the narrowed key's sum stays Integer"},
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
