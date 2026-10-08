package eng

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// nur317Fns builds the fn values TestDoReStep re-steps: g takes nothing and
// answers 7, l takes an Integer and adds one, bad takes nothing and raises.
func nur317Fns() (g, l, bad core.Value) {
	g = core.NewFunction(core.FnDefInfo{Name: "g", Signatures: []core.Signature{{Returns: []*core.Type{core.TInteger}, Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return []core.Value{core.NewInteger(7)}, nil
	})}}})
	l = core.NewFunction(core.FnDefInfo{Name: "l", Signatures: []core.Signature{{Args: []*core.Type{core.TInteger}, Params: []core.FnParam{{Name: "x", Type: core.TInteger}}, BarrierPos: 1, Returns: []*core.Type{core.TInteger}, Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		n, _ := core.AsInteger(args[0])
		return []core.Value{core.NewInteger(n + 1)}, nil
	})}}})
	bad = core.NewFunction(core.FnDefInfo{Name: "bad", Signatures: []core.Signature{{Impl: core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, errors.New("boom")
	})}}})
	return g, l, bad
}

// TestDoReStep pins the VM half of NUR317's second half: a `do`'s results
// re-stepped where the `do` stood (SigRef.ReStep). No dispatching value: the
// results stand. A fn taking nothing: the island steps them wherever the call
// sits. A fn taking an argument: only where the results are isolated —
// nothing beneath them in the frame, and the unit ending after the call —
// else the designed defer. A re-step leaving another count than the program
// seats defers, as does a named fn left over in a fn unit, whose frame's tail
// the island does not have.
func TestDoReStep(t *testing.T) {
	r := newTestRegistry(t)
	vc := &vmContext{r: r}
	g, l, bad := nur317Fns()
	five := core.NewInteger(5)
	call := []compiler.Instr{{Op: compiler.OpCallNative}, {Op: compiler.OpPushConst}}
	last := []compiler.Instr{{Op: compiler.OpCallNative}}
	ret := []compiler.Instr{{Op: compiler.OpCallNative}, {Op: compiler.OpRet}}
	ref := func(out int) *compiler.SigRef { return &compiler.SigRef{Word: "do", ReStep: true, ReStepOut: out} }
	for _, c := range []struct {
		why     string
		s       *compiler.SigRef
		results []core.Value
		bare    bool
		code    []compiler.Instr
		unit    int
		want    string
	}{
		{"no fn value: the results stand", ref(2), []core.Value{core.NewInteger(0), five}, false, call, -1, "[0 5]"},
		{"a fn taking nothing fires anywhere", ref(2), []core.Value{g, five}, false, call, -1, "[7 5]"},
		{"a fn taking an argument, not isolated", ref(1), []core.Value{l, five}, true, call, -1, "ERROR:re-steps where the `do` stood"},
		{"a fn taking an argument over a value beneath", ref(1), []core.Value{l, five}, false, last, -1, "ERROR:re-steps where the `do` stood"},
		{"isolated at the program's end", ref(-1), []core.Value{l, five}, true, last, -1, "[6]"},
		{"isolated before its unit's RET", ref(1), []core.Value{l, five}, true, ret, 0, "[6]"},
		{"another count than the program seats", ref(1), []core.Value{g, five}, false, call, -1, "ERROR:2 value(s) where the program seats 1"},
		{"the island's raise", ref(-1), []core.Value{bad}, false, call, -1, "ERROR:boom"},
		{"a named fn left in a fn unit", ref(-1), []core.Value{l}, true, ret, 0, "ERROR:meets its fn frame's tail"},
	} {
		out, err := vc.doReStep(r, c.s, c.results, c.bare, c.code, c.unit, nil, 0)
		if sub, isErr := strings.CutPrefix(c.want, "ERROR:"); isErr {
			if err == nil || !strings.Contains(err.Error(), sub) {
				t.Errorf("%s: got %v / %v, want an error containing %q", c.why, out, err, sub)
			}
			continue
		}
		if err != nil || fmt.Sprint(out) != c.want {
			t.Errorf("%s: got %v / %v, want %s", c.why, out, err, c.want)
		}
	}
}

// TestTakesArgsAtPointer pins doReStep's exactness test: a value the step
// loop dispatches may take an argument unless it is a fn every signature of
// which takes none.
func TestTakesArgsAtPointer(t *testing.T) {
	g, l, _ := nur317Fns()
	closure := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{}}
	for _, c := range []struct {
		why  string
		v    core.Value
		want bool
	}{
		{"data", core.NewInteger(7), false},
		{"a fn taking nothing", g, false},
		{"a fn taking an argument", l, true},
		{"a compiled closure", closure, true},
		{"a fn with no signature", core.NewFunction(core.FnDefInfo{Name: "e"}), true},
	} {
		if got := takesArgsAtPointer(c.v); got != c.want {
			t.Errorf("%s: takesArgsAtPointer = %v, want %v", c.why, got, c.want)
		}
	}
}
