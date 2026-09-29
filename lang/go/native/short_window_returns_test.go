package native

import "testing"

// TestNativeReturnsFnsGuardShortWindow pins NUR332's guard on the language
// layer's ReturnsFns the failed-dispatch recovery reached over a window
// shorter than their signature (`fold`, `outer`, `inner`, `as` and `behave`
// stranded with too few operands panicked the check pass). The check pass no
// longer hands a short window to a ReturnsFn; each also refuses to index one:
// the dynamic Any (behave and the Service `add` check half: nothing — the
// latter's guard predates NUR332 and was reached only through the recovery's
// short window, which declaredReturnCarriers now refuses at the source).
func TestNativeReturnsFnsGuardShortWindow(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		fn   func([]Value, *Registry) []Value
		full int
		want int
	}{
		{"fold (seeded)", foldWithInitReturnsFn, 3, 1},
		{"fold", foldNoInitReturnsFn, 2, 1},
		{"outer", outerReturnsFn, 3, 1},
		{"inner", innerReturnsFn, 4, 1},
		{"as", asReturns, 2, 1},
		{"behave", behaveReturns, 2, 0},
		{"service add", serviceAddCheck, 2, 0},
	} {
		for n := 0; n < c.full; n++ {
			args := make([]Value, n)
			for i := range args {
				args[i] = NewInteger(1)
			}
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("%s over %d operand(s) panicked: %v", c.name, n, rec)
					}
				}()
				out := c.fn(args, r)
				if len(out) != c.want || (c.want == 1 && (!out[0].Dynamic || !out[0].Parent.Equal(TAny))) {
					t.Errorf("%s over %d operand(s) = %#v, want %d dynamic Any", c.name, n, out, c.want)
				}
			}()
		}
	}
}
