package native

import "testing"

// reifyReturns over a `tor` UNION target: the check residual is the
// dynamic disjunct ($class selects the member at run time), not a plain
// dynamic(Any) — the Disjunct arm. Driven directly: the anonymous
// disjunct of class bodies is what `(A tor B)` evaluates to.
func TestReifyReturnsUnionTarget(t *testing.T) {
	r := seam5Reg(t)
	out, err := seam5Run(r, `def A class {x:1}  def B class {y:2}  (A tor B)`)
	if err != nil || len(out) == 0 {
		t.Fatalf("build union target: %v / %v", out, err)
	}
	target := out[len(out)-1]
	if !IsDisjunct(&target) {
		t.Fatalf("(A tor B) must evaluate to a disjunct, got %v", target)
	}
	res := reifyReturns([]Value{target, NewMap(NewOrderedMap())}, r)
	if len(res) != 1 || !res[0].Dynamic || !IsDisjunct(&res[0]) {
		t.Fatalf("a union target's residual is the dynamic disjunct, got %v", res)
	}
}
