package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Phase A of design/REACH.10.md: the Ideal/Reach type, payload, and accessors.
// The parser does not emit Reach yet (Phase B), so these construct it directly.

func TestReachConstructAndAccess(t *testing.T) {
	info := core.ReachInfo{
		Receiver: []core.Value{core.NewWord("m")},
		Segments: []core.ReachSeg{
			{KeyLit: core.NewAtom("a")},
			{Getr: true, KeyLit: core.NewAtom("b")},
			{Computed: true, KeyExpr: []core.Value{core.NewWord("k")}},
		},
		Eval: true,
	}
	v := core.NewReach(info)

	if !core.IsReach(&v) {
		t.Fatalf("IsReach = false for a Reach value (parent %s)", core.ParentOf(&v))
	}
	if !core.ParentOf(&v).Equal(core.TReach) {
		t.Fatalf("parent = %s, want Ideal/Reach", core.ParentOf(&v))
	}

	got, err := core.AsReach(&v)
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if len(got.Receiver) != 1 || len(got.Segments) != 3 || !got.Eval {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Segments[1].Getr != true || got.Segments[2].Computed != true {
		t.Fatalf("segment flags not preserved: %+v", got.Segments)
	}
}

// AsReach must error (not panic) on a non-Reach value.
func TestAsReachRejectsNonReach(t *testing.T) {
	if _, err := core.AsReach(addrOf(core.NewInteger(5))); err == nil {
		t.Fatal("AsReach(5) should error, got nil")
	}
}

// Phase C: canon renders a Reach back to its dotted surface (round-trip).
func TestReachCanonRoundTrip(t *testing.T) {
	mk := func(recv []core.Value, segs []core.ReachSeg) core.Value {
		return core.NewReach(core.ReachInfo{Receiver: recv, Segments: segs, Eval: true})
	}
	cases := []struct {
		name string
		v    core.Value
		want string
	}{
		{"bare", mk([]core.Value{core.NewWord("m")}, []core.ReachSeg{{KeyLit: core.NewWord("a")}, {KeyLit: core.NewWord("b")}}), "m.a.b"},
		{"getr", mk([]core.Value{core.NewWord("m")}, []core.ReachSeg{{Getr: true, KeyLit: core.NewWord("x")}}), "m!.x"},
		{"strkey", mk([]core.Value{core.NewWord("a")}, []core.ReachSeg{{KeyLit: core.NewString("x")}, {KeyLit: core.NewWord("c")}}), "a.'x'.c"},
		{"numkey", mk([]core.Value{core.NewWord("a")}, []core.ReachSeg{{KeyLit: core.NewInteger(0)}}), "a.0"},
		{"computed", mk([]core.Value{core.NewWord("m")}, []core.ReachSeg{{Computed: true, KeyExpr: []core.Value{core.NewWord("k")}}}), "m.(k)"},
		{"parenrecv", mk([]core.Value{core.NewParenExpr([]core.Value{core.NewWord("m"), core.NewWord("a")})}, []core.ReachSeg{{KeyLit: core.NewWord("b")}}), "(m a).b"},
		// Receiverless reach (a detached lens) renders with the reserved
		// `$` sentinel receiver, so `$.a.b` round-trips back to a lens.
		{"receiverless", mk(nil, []core.ReachSeg{{KeyLit: core.NewWord("a")}, {KeyLit: core.NewWord("b")}}), "$.a.b"},
		{"receiverless-getr", mk(nil, []core.ReachSeg{{Getr: true, KeyLit: core.NewWord("x")}}), "$!.x"},
	}
	for _, c := range cases {
		if got := core.CanonValue(c.v); got != c.want {
			t.Errorf("%s: canon = %q, want %q", c.name, got, c.want)
		}
	}
}

// A Quoted (codequote-captured) reach wraps in (codequote …) so it round-trips.
func TestReachCanonQuotedWraps(t *testing.T) {
	v := core.NewReach(core.ReachInfo{Receiver: []core.Value{core.NewWord("m")}, Segments: []core.ReachSeg{{KeyLit: core.NewWord("a")}}, Eval: true})
	core.SetQuoted(&v, true)
	if got := core.CanonValue(v); got != "(codequote m.a)" {
		t.Errorf("quoted reach canon = %q, want %q", got, "(codequote m.a)")
	}
}
