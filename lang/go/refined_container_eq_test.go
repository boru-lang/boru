package lang

import (
	"fmt"
	"testing"
)

// TestRefinedContainerIsEqToItself pins NUR142: `eq` is identity for the
// container family and a refinement of a container type is a member of
// the family, so a refined flex map, flex list or map is eq to itself and
// `deq` reaches the container arm through the same fold (containerFamily,
// core/go/equal.go). Ordering keeps the exact fold — a refinement's own
// Comparer is still consulted — and two stores stay distinct under eq.
func TestRefinedContainerIsEqToItself(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"def S (refine FlexMap)  def w:S (flex {a:1})  w eq w", "[true]"},
		{"def L (refine FlexList)  def v:L (flex [1 2])  v eq v", "[true]"},
		{"def M (refine Map)  def m:M {a:1}  m eq m", "[true]"},
		{"def S (refine FlexMap)  def w:S (flex {a:1})  [w] deq [w]", "[true]"},
		{"def S (refine FlexMap)  def w:S (flex {a:1})  def u (flex {a:1})  w eq u", "[false]"},
	} {
		got, err := mustNew(t).RunInterp(c.src)
		if err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: interpreter = %v / %v, want %s", c.src, got, err, c.want)
		}
		requireEngineParity(t, c.src, true)
	}
}
