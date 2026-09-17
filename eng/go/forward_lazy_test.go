package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestResolveForwardArgs_LazySkipOrdering pins the structure-first, lazy
// forward-argument behaviour at the kernel level (design/legacy/LAZY-ARG-RESOLUTION.10.ignore).
//
// `imp` has a String-headed arity-1 overload and a higher-arity overload, so
// the former eager scan (preEvalParens up to MaxForwardArgs == 2) would
// pre-evaluate the trailing `(probe)` group BEFORE `imp` dispatched. Under
// lazy resolution the leading String literal prunes the viable set to the
// arity-1 overload, which does not consume position 1, so the group is left
// raw — `imp` runs first and the group runs afterwards as an ordinary
// expression.
//
// Pure-value output cannot distinguish the two models (matchSignature selects
// the arity-1 overload either way); only the EXECUTION ORDER of the word vs
// the group's contents differs. We observe that order directly:
//   - lazy  → ["imp", "probe"]   (word dispatches, then the trailing group)
//   - eager → ["probe", "imp"]   (group pre-evaluated, then the word)
func TestResolveForwardArgs_LazySkipOrdering(t *testing.T) {
	var order []string

	setup := func(r *core.Registry) {
		// imp: [String] (arity 1) records "imp"; [Integer Integer] (arity 2)
		// exists only to raise MaxForwardArgs to 2 so the eager model would
		// have pre-evaluated the trailing group.
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "imp",
			Signatures: []core.Signature{
				{
					Args: []*core.Type{core.TString},
					Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
						order = append(order, "imp")
						s, _ := core.AsString(args[0])
						return []core.Value{core.NewString("file:" + s)}, nil
					}),
					Returns: []*core.Type{core.TString}, BarrierPos: -1,
				},
				{
					Args: []*core.Type{core.TInteger, core.TInteger},
					Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
						return []core.Value{core.NewString("two")}, nil
					}),
					Returns: []*core.Type{core.TString}, BarrierPos: -1,
				},
			},
		})
		// probe: 0-arg word that records "probe" and yields a value.
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "probe",
			Signatures: []core.Signature{{
				Args: []*core.Type{},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					order = append(order, "probe")
					return []core.Value{core.NewInteger(1)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: 0,
			}},
		})
	}

	// imp "x" (probe)
	input := []core.Value{
		core.NewWord("imp"),
		core.NewString("x"),
		core.NewOpenParen(),
		core.NewWord("probe"),
		core.NewCloseParen(),
	}

	out := runWith(t, setup, input)

	if len(order) != 2 || order[0] != "imp" || order[1] != "probe" {
		t.Fatalf("execution order = %v, want [imp probe] (lazy: word before its trailing group)", order)
	}
	// imp selected [String] and consumed only "x"; the group ran separately.
	if len(out) != 2 {
		t.Fatalf("stack = %v (len %d), want 2 values [file:x 1]", out, len(out))
	}
	if s, _ := core.AsString(out[0]); s != "file:x" {
		t.Fatalf("out[0] = %v, want file:x", out[0])
	}
	if n, _ := core.AsInteger(out[1]); n != 1 {
		t.Fatalf("out[1] = %v, want 1", out[1])
	}
}

// TestResolveForwardArgs_ClaimedGroupStillEvaluated guards the other
// direction: when an overload genuinely consumes a forward group, lazy
// resolution still evaluates it (the group is a claimed argument), so a
// word like `addq2 1 (probe)` runs the group and binds its result.
func TestResolveForwardArgs_ClaimedGroupStillEvaluated(t *testing.T) {
	var order []string
	setup := func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "addq2",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger, core.TInteger},
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					order = append(order, "addq2")
					a, _ := core.AsInteger(args[0])
					b, _ := core.AsInteger(args[1])
					return []core.Value{core.NewInteger(a + b)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "probe",
			Signatures: []core.Signature{{
				Args: []*core.Type{},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					order = append(order, "probe")
					return []core.Value{core.NewInteger(40)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: 0,
			}},
		})
	}

	// addq2 2 (probe)  →  the paren IS a claimed forward arg, so it must be
	// evaluated (probe runs first, before addq2 binds 2 + 40).
	input := []core.Value{
		core.NewWord("addq2"),
		core.NewInteger(2),
		core.NewOpenParen(),
		core.NewWord("probe"),
		core.NewCloseParen(),
	}
	out := runWith(t, setup, input)
	if len(order) != 2 || order[0] != "probe" || order[1] != "addq2" {
		t.Fatalf("execution order = %v, want [probe addq2] (claimed group evaluated first)", order)
	}
	if len(out) != 1 {
		t.Fatalf("stack = %v, want 1 value [42]", out)
	}
	if n, _ := core.AsInteger(out[0]); n != 42 {
		t.Fatalf("out[0] = %v, want 42", out[0])
	}
}
