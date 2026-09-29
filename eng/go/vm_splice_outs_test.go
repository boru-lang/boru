package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// spliceOutsSeat seats a `do`'s run without the splices the pass fired only
// when the run is the pass's: as many results, each tape-coupled one a
// splice rendering as the pass's at its position (NUR348). Anything else is
// the screen's loud defer.
func TestSpliceOutsSeat(t *testing.T) {
	splice := func(vals ...core.Value) core.Value { return core.NewSplice(core.NewList(vals)) }
	one, two, three := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	want := []core.Value{splice(one, two), three}
	kept, ok := spliceOutsSeat(want, []core.Value{splice(one, two), three})
	if !ok || len(kept) != 1 || !core.DeepEqual(kept[0], three) {
		t.Errorf("the pass's run is seated without its splice: %v %v", kept, ok)
	}
	for _, c := range []struct {
		name string
		want []core.Value
		got  []core.Value
	}{
		{"no record", nil, []core.Value{splice(one, two)}},
		{"another count", want, []core.Value{splice(one, two)}},
		{"a plain value where the pass stepped a splice", want, []core.Value{one, three}},
		{"a splice over other tokens", want, []core.Value{splice(one, three), three}},
		{"a splice where the pass held a value", []core.Value{three, three}, []core.Value{splice(one, two), three}},
		{"another tape-coupled token", want, []core.Value{core.NewWord("add"), three}},
	} {
		if _, ok := spliceOutsSeat(c.want, c.got); ok {
			t.Errorf("%s: seated", c.name)
		}
	}
}
