package modules

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestRandWithSeedShapeIsAModel pins NUR331's mark at its source: the
// check-mode shape of `Rand.with-seed N` is a MODEL (a generator seeded 0),
// so its methods are homed in a registry marked ShapeModel and never bake as
// constants; a real seeded instance — what the run builds — is unmarked and
// bakes as data like any module method.
func TestRandWithSeedShapeIsAModel(t *testing.T) {
	out := randWithSeedReturns(nil, nil)
	if len(out) != 1 {
		t.Fatalf("want one shape value, got %d", len(out))
	}
	shape, err := native.AsMap(out[0])
	if err != nil || shape == nil {
		t.Fatalf("the shape is a concrete map: %v", err)
	}
	inst, err := BuildSeededRandInstance(3)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"int", "bool", "float", "string", "one-of", "list-of", "map-from"} {
		m, _ := shape.Get(name)
		fd, ok := m.Data.(native.FnDefInfo)
		if !ok || !fd.ShapeModelHomed() || core.IsInertConst(m) {
			t.Errorf("%s: the model's method must be ShapeModelHomed and no constant", name)
		}
		rv, _ := inst.Get(name)
		rfd, ok := rv.Data.(native.FnDefInfo)
		if !ok || rfd.ShapeModelHomed() || !core.IsInertConst(rv) {
			t.Errorf("%s: a real instance's method is unmarked and bakes as data", name)
		}
	}
}
