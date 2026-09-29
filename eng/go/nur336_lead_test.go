package eng

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The NUR336 remainders' VM pieces: a paren apply's lead that no signature
// of which takes the paren's values is the island's before it runs
// (parenMissesWindow), a placing apply runs it only where its run is the
// paren's (placesAlone), a def-bound word's lead is its name's dispatch
// (namedRuns, appliedLead, namedMissRaise), and the island writes the lead
// as the paren steps it (substIsland's Reach and Named arms) over a prefix
// that may seat a type node (RestartType).

func n336Fn(name string, types ...*core.Type) core.Value {
	params := make([]core.FnParam, len(types))
	for i, ty := range types {
		params[i] = core.FnParam{Type: ty}
	}
	sig := core.Signature{Params: params, BarrierPos: len(types), Impl: core.Go(scConst(9))}
	core.NormalizeSig(&sig)
	return core.NewFunction(core.FnDefInfo{Name: name, Anonymous: true, Signatures: []core.Signature{sig}, MaxForwardArgs: len(types)})
}

func n336Closure(unit int) core.Value {
	return core.NewValueRaw(core.TFunction, core.ClosurePayload{Unit: unit})
}

func TestParenMissesWindow(t *testing.T) {
	vc := &vmContext{p: &compiler.Program{Fns: []compiler.CompiledFn{{NParams: 2, NCaptures: 1}}}, r: seam7Reg(t)}
	seven := core.NewInteger(7)
	one := []core.Value{seven}
	for _, c := range []struct {
		name string
		v    core.Value
		args []core.Value
		want bool
	}{
		{"a signature takes the value", n336Fn("f", core.TAny), one, false},
		{"none takes two", n336Fn("f", core.TAny), []core.Value{seven, seven}, true},
		{"none admits an Integer", n336Fn("f", core.TString), one, true},
		{"a lambda of none", n336Fn("f"), one, true},
		{"no signature to tell by", core.NewFunction(core.FnDefInfo{Name: "f"}), one, false},
		{"a closure of one", n336Closure(0), one, false},
		{"a closure of one over two", n336Closure(0), []core.Value{seven, seven}, true},
		{"a closure of no known unit", n336Closure(5), []core.Value{seven, seven}, false},
		{"data", seven, one, false},
	} {
		if got := vc.parenMissesWindow(c.v, c.args); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPlacesAlone(t *testing.T) {
	vc := &vmContext{p: &compiler.Program{Fns: []compiler.CompiledFn{{NParams: 0}}}, r: seam7Reg(t)}
	seven := core.NewInteger(7)
	two := []core.Value{seven, seven}
	twoSigs := n336Fn("f", core.TAny)
	fd := twoSigs.Data.(core.FnDefInfo)
	fd.Signatures = append(fd.Signatures, fd.Signatures[0])
	twoSigs.Data = fd
	for _, c := range []struct {
		name  string
		v     core.Value
		alone bool
		want  bool
	}{
		{"nothing beneath, nothing after", n336Fn("f", core.TString), true, true},
		{"a closure of none", n336Closure(0), false, true},
		{"a closure of no known unit", n336Closure(5), false, false},
		{"a fn of none", n336Fn("f"), false, true},
		{"one signature of fewer, taking the first", n336Fn("f", core.TInteger), false, true},
		{"one signature of fewer, missing the first", n336Fn("f", core.TString), false, false},
		{"two signatures", twoSigs, false, false},
		{"data", seven, false, false},
	} {
		if got := vc.placesAlone(c.v, two, c.alone); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if vc.placesAlone(n336Fn("f", core.TAny, core.TAny), two, false) {
		t.Error("a signature of the window's count is no fewer")
	}
}

func TestNamedRunsAndAppliedLead(t *testing.T) {
	zero := n336Fn("g")
	if !namedRuns(zero) || namedRuns(n336Fn("g", core.TAny)) || namedRuns(core.NewInteger(1)) {
		t.Error("a fn of no argument only")
	}
	if fd := appliedLead(zero).Data.(core.FnDefInfo); !fd.Applied {
		t.Error("marked to run where it is stepped")
	}
	if got := appliedLead(core.NewInteger(1)); fmt.Sprint(got) != "1" {
		t.Errorf("data as it is: %v", got)
	}
}

// A def-bound word's lead none of whose signatures admits the paren's values
// raises the name's no-match on a fork holding the name as the interpreter's
// def binds it; any other miss, or a fork run that does not raise, defers.
func TestNamedMissRaise(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	spec := &compiler.DynMethodSpec{Word: "(paren apply)", Paren: true, LeadName: "g", LeadPos: core.SrcPos{Row: 1, Col: 2}}
	seven := []core.Value{core.NewInteger(7)}
	err := vc.namedMissRaise(spec, n336Fn("g", core.TString), seven, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "cannot call `g`") {
		t.Errorf("the name's no-match: %v", err)
	}
	if _, bound := r.Defs.Top("g"); bound {
		t.Error("the fork's install stays on the fork")
	}
	err = vc.namedMissRaise(spec, n336Fn("g", core.TAny, core.TAny), seven, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "def-bound word `g`") {
		t.Errorf("a signature of another count defers: %v", err)
	}
	err = vc.namedMissRaise(spec, n336Fn("g", core.TAny), seven, seam7Dbg, 0)
	if err == nil || !strings.Contains(err.Error(), "def-bound word `g`") {
		t.Errorf("a fork run that does not raise defers: %v", err)
	}
}

// A placing apply whose lead is a def-bound word of no argument runs it
// before the paren's values, as the name's dispatch does, and places what
// that leaves.
func TestNamedLeadPlaces(t *testing.T) {
	r := seam7Reg(t)
	vc := &vmContext{p: &compiler.Program{}, r: r, ceiling: 1 << 20, stepLimit: 1 << 20}
	spec := &compiler.DynMethodSpec{Word: "(paren apply)", NArgs: 1, NOut: 1, Paren: true, Place: true, LeadName: "g"}
	res, _, err := vc.callDynMethod(r, spec, 0, []core.Value{core.NewInteger(7), n336Fn("g")}, seam7Dbg, 0)
	if err != nil || fmt.Sprint(res) != "[9 7]" {
		t.Errorf("the name's call, then its values: %v %v", res, err)
	}
	raising := n336Fn("g")
	fd := raising.Data.(core.FnDefInfo)
	fd.Signatures[0].Impl = core.Go(func([]core.Value, map[string]core.Value, []core.Value, *core.Registry) ([]core.Value, error) {
		return nil, fmt.Errorf("boom")
	})
	raising.Data = fd
	if _, _, err := vc.callDynMethod(r, spec, 0, []core.Value{core.NewInteger(7), raising}, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("the name's call raising: %v", err)
	}
}

// The island writes a paren apply's lead as the reach group its member read
// lowers to, and a def-bound word's fn of no argument marked to run.
func TestSubstIslandLeadArms(t *testing.T) {
	vc := seam7VC(seam7Reg(t))
	lead := n336Fn("f", core.TAny)
	island := []core.Value{core.NewParenExpr([]core.Value{core.NewWord("m"), core.NewInteger(7)})}
	guard := compiler.RestartSrc{Kind: compiler.RestartGuard}
	got, err := vc.substIsland(island, []compiler.RestartSubst{{Path: []int{0, 0}, Span: 1, Src: guard, Placed: true, Reach: true}}, []core.Value{lead}, 0, nil, seam7Dbg, 0)
	if err != nil {
		t.Fatal(err)
	}
	toks, _ := core.AsParenExpr(got[0])
	if ri, err := core.AsReach(toks[0]); err != nil || !ri.Eval || len(ri.Receiver) != 1 {
		t.Errorf("the reach group of the value: %v %v", toks, err)
	}
	zero := n336Fn("g")
	got, err = vc.substIsland(island, []compiler.RestartSubst{{Path: []int{0, 0}, Span: 1, Src: guard, Placed: true, Named: true}}, []core.Value{zero}, 0, nil, seam7Dbg, 0)
	if err != nil {
		t.Fatal(err)
	}
	toks, _ = core.AsParenExpr(got[0])
	if fd, ok := toks[0].Data.(core.FnDefInfo); !ok || !fd.Applied {
		t.Errorf("the name's call of a fn of none: %v", toks)
	}
	if _, err := vc.substIsland(island, []compiler.RestartSubst{{Path: []int{0, 0}, Span: 1, Src: guard, Placed: true, Named: true}}, []core.Value{lead}, 0, nil, seam7Dbg, 0); err == nil {
		t.Error("a named lead that takes a value is the parked-fn defer")
	}
}

// A told stack's type node is seated as the canonical node its ID names.
func TestStatementRestartSeatsTypes(t *testing.T) {
	r := seam7Reg(t)
	vc := seam7VC(r)
	srcs := []compiler.RestartSrc{{Kind: compiler.RestartType, Val: core.NewTypeLiteral(core.TInteger)}}
	stack, _, err := vc.statementRestart(r, srcs, []core.Value{core.NewInteger(5)}, 0, 1, true, 0, nil, seam7Dbg, 0)
	if err != nil || fmt.Sprint(stack) != "[Integer 5]" {
		t.Errorf("the type beneath the statement: %v %v", stack, err)
	}
	gone := core.NewTypeLiteral(core.TInteger)
	gone.ID = "no-such-type"
	if _, _, err := vc.statementRestart(r, []compiler.RestartSrc{{Kind: compiler.RestartType, Val: gone}}, []core.Value{core.NewInteger(5)}, 0, 1, true, 0, nil, seam7Dbg, 0); err == nil || !strings.Contains(err.Error(), "unresolvable statement-island type") {
		t.Errorf("a type no table holds: %v", err)
	}
}
