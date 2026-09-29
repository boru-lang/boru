package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestNUR347FnPosStamps pins the fn-value position helpers: a native's
// positionless fn result takes the op's position (stampFnPos), a value that
// has one keeps it, a non-fn value and an op with no position change
// nothing, and a binding drops what the stamp gave (dropFnPos).
func TestNUR347FnPosStamps(t *testing.T) {
	debug := []core.SrcPos{{Row: 1, Col: 7, Src: "m"}, {}}
	fn := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{}}
	if got := stampFnPos(fn, debug, 0).Pos(); got.Row != 1 || got.Col != 7 {
		t.Fatalf("positionless fn: got %v, want 1:7", got)
	}
	if got := stampFnPos(fn, debug, 1).Pos(); got.Row != 0 {
		t.Fatalf("op without a position stamped %v", got)
	}
	if got := stampFnPos(fn, debug, 5).Pos(); got.Row != 0 {
		t.Fatalf("pc past the table stamped %v", got)
	}
	own := fn
	own.SetPos(core.SrcPos{Row: 2, Col: 3})
	if got := stampFnPos(own, debug, 0).Pos(); got.Row != 2 {
		t.Fatalf("a fn with its own position was restamped: %v", got)
	}
	if got := stampFnPos(core.NewInteger(1), debug, 0).Pos(); got.Row != 0 {
		t.Fatalf("a non-fn value was stamped: %v", got)
	}
	res := []core.Value{fn, core.NewInteger(1)}
	stampFnResultPos(res, debug, 0)
	if res[0].Pos().Row != 1 || res[1].Pos().Row != 0 {
		t.Fatalf("stampFnResultPos: %v %v", res[0].Pos(), res[1].Pos())
	}
	if got := dropFnPos(own).Pos(); got.Row != 0 {
		t.Fatalf("dropFnPos kept %v", got)
	}
	n := core.NewInteger(1)
	n.SetPos(core.SrcPos{Row: 4, Col: 1})
	if got := dropFnPos(n).Pos(); got.Row != 4 {
		t.Fatalf("dropFnPos dropped a non-fn value's position")
	}
	// A binding's rename drops it too, the closure's construction anchor
	// with it.
	cl := core.Value{Parent: core.TFunction, Data: core.ClosurePayload{RetPos: core.SrcPos{Row: 1, Col: 36}}}
	cl.SetPos(core.SrcPos{Row: 1, Col: 37})
	named := nameClosureValue(cl, "h")
	if named.Pos().Row != 0 || named.Data.(core.ClosurePayload).RetPos.Row != 0 || named.Data.(core.ClosurePayload).RetName != "h" {
		t.Fatalf("nameClosureValue: %v %+v", named.Pos(), named.Data)
	}
}

// TestNUR347ClosureFrameName pins the frame a closure's contract names: its
// binding's name, `<fn>` for a nameless verbose value (a returned closure's
// Named push, or a callback's verbose Source), and nothing for a lambda.
func TestNUR347ClosureFrameName(t *testing.T) {
	verbose := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{}}
	lambda := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Anonymous: true}}
	named := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	notFn := core.NewInteger(1)
	for _, c := range []struct {
		cl   core.ClosurePayload
		want string
	}{
		{core.ClosurePayload{RetName: "h"}, "h"},
		{core.ClosurePayload{Named: true}, core.FnValueFrameName},
		{core.ClosurePayload{Source: &verbose}, core.FnValueFrameName},
		{core.ClosurePayload{Source: &lambda}, ""},
		{core.ClosurePayload{Source: &named}, "g"},
		{core.ClosurePayload{Source: &notFn}, ""},
		{core.ClosurePayload{}, ""},
	} {
		if got := closureFrameName(c.cl); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.cl, got, c.want)
		}
	}
}

// TestNUR347AnchorFinal pins the frame-anchored error: a nameless closure
// with no position of its own marks its contract error AnchorFinal, which an
// apply op's stampAt leaves unstamped and a native call's stampHandlerAt
// stamps; a named closure, one with an anchor, a non-BoruError and a nil
// error are left as they are.
func TestNUR347AnchorFinal(t *testing.T) {
	debug := []core.SrcPos{{Row: 1, Col: 67, Src: "mk"}}
	mk := func() *core.BoruError { return &core.BoruError{Code: "type_error", Detail: "d"} }
	ae := mk()
	if err := anchorClosureErr(core.ClosurePayload{}, ae); err != ae || !ae.AnchorFinal {
		t.Fatalf("nameless positionless closure: %+v", ae)
	}
	if stampAt(ae, debug, 0, nil); ae.Row != 0 {
		t.Fatalf("an apply op stamped a frame-anchored error at %d:%d", ae.Row, ae.Col)
	}
	if stampHandlerAt(ae, debug, 0, nil); ae.Row != 1 || ae.Col != 67 || ae.AnchorFinal {
		t.Fatalf("a native boundary left %+v", ae)
	}
	for _, cl := range []core.ClosurePayload{{RetName: "h"}, {RetPos: core.SrcPos{Row: 1, Col: 37}}} {
		e := mk()
		if anchorClosureErr(cl, e); e.AnchorFinal {
			t.Fatalf("%+v marked its error", cl)
		}
		if stampAt(e, debug, 0, nil); e.Row != 1 {
			t.Fatalf("%+v: the op did not stamp", cl)
		}
	}
	if err := anchorClosureErr(core.ClosurePayload{}, nil); err != nil {
		t.Fatalf("nil error became %v", err)
	}
}
