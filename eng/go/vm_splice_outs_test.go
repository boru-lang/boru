package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// spliceOutsSeat seats a `do`'s run without the splices the pass fired only
// when the run is the pass's: as many results, each tape-coupled one a
// splice provably the same token as the pass's at its position (NUR348).
// Anything else is the screen's loud defer.
func TestSpliceOutsSeat(t *testing.T) {
	splice := func(vals ...core.Value) core.Value { return core.NewSplice(core.NewList(vals)) }
	quoted := func(v core.Value) core.Value { v.Quoted = true; return v }
	one, two, three := core.NewInteger(1), core.NewInteger(2), core.NewInteger(3)
	want := []core.Value{splice(one, two), three}
	kept, ok := spliceOutsSeat(want, []core.Value{splice(one, two), three})
	if !ok || len(kept) != 1 || !core.DeepEqual(kept[0], three) {
		t.Errorf("the pass's run is seated without its splice: %v %v", kept, ok)
	}
	// The provable tokens: words, nested lists, Integer / string / Boolean /
	// Atom leaves, and a splice inside the payload.
	rich := func() core.Value {
		return core.NewSplice(core.NewList([]core.Value{core.NewWord("add"), core.NewList([]core.Value{one}), core.NewString("s"),
			core.NewBoolean(true), core.NewAtom("a"), core.NewSplice(core.NewList([]core.Value{two}))}))
	}
	if _, ok := spliceOutsSeat([]core.Value{rich()}, []core.Value{rich()}); !ok {
		t.Error("a payload of provable tokens is the same token for token")
	}
	// Two closures of one name and signature over different captures render
	// alike (`fn f(Integer)`) but are different tokens: the review of #522's
	// silent wrong answer, where the run's marker was dropped and the
	// expansion recorded for the pass's capture ran instead.
	closure := func(n int64) core.Value {
		return core.NewFunction(core.FnDefInfo{Name: "f", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, BarrierPos: 1, Impl: core.Boru([]core.Value{core.NewWord("n")}),
		}}, Captured: []core.CapturedBinding{{Name: "n", Value: core.NewInteger(n)}}})
	}
	if core.CanonValue(closure(1)) != core.CanonValue(closure(2)) {
		t.Fatalf("the witness needs two closures rendering alike: %s / %s", core.CanonValue(closure(1)), core.CanonValue(closure(2)))
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
		{"a closure over another capture", []core.Value{splice(closure(1))}, []core.Value{splice(closure(2))}},
		{"a fn value, even over the same capture", []core.Value{core.NewSplice(closure(1))}, []core.Value{core.NewSplice(closure(1))}},
		{"a Float leaf", []core.Value{splice(core.NewFloat(1.5))}, []core.Value{splice(core.NewFloat(1.5))}},
		{"a list of another length", []core.Value{splice(one, two)}, []core.Value{splice(one, two, three)}},
		{"a list where the pass held a word", []core.Value{splice(core.NewWord("add"))}, []core.Value{splice(core.NewList([]core.Value{one}))}},
		{"a word where the pass held a list", []core.Value{splice(core.NewList([]core.Value{one}))}, []core.Value{splice(core.NewWord("add"))}},
		{"a word of another name", []core.Value{splice(core.NewWord("add"))}, []core.Value{splice(core.NewWord("sub"))}},
		{"a quoted token where the pass held a plain one", []core.Value{splice(one)}, []core.Value{splice(quoted(one))}},
		{"a carrier leaf", []core.Value{splice(core.NewCarrier(core.TInteger))}, []core.Value{splice(core.NewCarrier(core.TInteger))}},
		{"a payload of another type", []core.Value{core.NewSplice(one)}, []core.Value{core.NewSplice(core.NewString("1"))}},
		{"a splice where the pass held a list", []core.Value{splice(core.NewList([]core.Value{one}))}, []core.Value{splice(core.NewSplice(one))}},
		{"an untyped token", []core.Value{core.NewSplice(core.Value{})}, []core.Value{core.NewSplice(core.Value{})}},
	} {
		if _, ok := spliceOutsSeat(c.want, c.got); ok {
			t.Errorf("%s: seated", c.name)
		}
	}
}
