package basic

import "testing"

// TestControlReturnsFnsGuardShortWindow pins NUR332's guard on the control
// words' ReturnsFns: each read its operands positionally, and the failed-
// dispatch recovery reached each of them over a window shorter than its
// signature (`if`, `case`, `error`, `while`, `for` and `__arm` stranded in a
// clause list or an arm panicked). The check pass no longer hands a short
// window to a ReturnsFn; each also answers one with the dynamic Any rather
// than index what it was not given.
func TestControlReturnsFnsGuardShortWindow(t *testing.T) {
	r := newTestRegistry(t)
	for name, fn := range map[string]func([]Value, *Registry) []Value{
		"do / __arm": DoListReturnsFn,
		"if":         if3ReturnsFn,
		"error":      ErrorReturnsFn,
		"case":       CaseReturnsFn,
		"while":      whileReturnsFn,
		"for":        forIntegerListReturnsFn,
	} {
		for _, args := range [][]Value{nil, {NewInteger(1)}} {
			if name == "do / __arm" && len(args) > 0 {
				continue // one operand is its full window
			}
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("%s over %d operand(s) panicked: %v", name, len(args), rec)
					}
				}()
				out := fn(args, r)
				if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TAny) {
					t.Errorf("%s over %d operand(s) = %#v, want one dynamic Any", name, len(args), out)
				}
			}()
		}
	}
}
