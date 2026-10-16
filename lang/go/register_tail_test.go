package lang

import (
	"testing"
)

// NUR128: an exported module fn's body gets the declaration-shaped analysis
// at EXPORT time, on the importing pass, in the module registry it was
// written in — so its dead branch warns whether or not anyone calls it,
// as a top-level fn's does.
func TestModuleExportedFnBodyAnalysedAtExport(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		`import module [def mf fn [[n:Integer][Integer][if true [n] [0]]] export "M" {mf: mf/v}]  1`,
		`import module [def mf fn [[n:Integer][Integer][if true [n] [0]]] export "M" {mf: mf/v}]  M.mf 4`,
		`def f fn [[n:Integer][Integer][if true [n] [0]]]  1`,
	} {
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		found := false
		for _, d := range res.Diagnostics {
			if d.Code == "unreachable_branch" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: no unreachable_branch among %+v", src, res.Diagnostics)
		}
	}
}
