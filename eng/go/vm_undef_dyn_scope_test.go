package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// OpUndefDynScope and the miss it makes, driven directly: the op pops the
// name's live binding in the current registry (consuming no stack), and a
// LOOKUP_DYN_SCOPE of a name in Program.SpecUndefNames that then misses
// raises the interpreter's undefined_word at the read's own position —
// never the defer, whose interpreter re-run an earlier effect would fence.
// The same miss on a name outside the set keeps the defer.
func TestVMUndefDynScope(t *testing.T) {
	newReg := func() *core.Registry {
		r, err := core.NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		r.InitRootContext()
		r.Defs.Push("k", core.NewInteger(5))
		return r
	}
	undefAt := core.SrcPos{Row: 1, Col: 4}
	readAt := core.SrcPos{Row: 2, Col: 9}
	prog := func(spec bool) *compiler.Program {
		p := &compiler.Program{
			Code: []compiler.Instr{
				{Op: compiler.OpUndefDynScope, Arg: 0},
				{Op: compiler.OpLookupDynScope, Arg: 0},
			},
			Debug:  []core.SrcPos{undefAt, readAt},
			Consts: []core.Value{core.NewString("k")},
		}
		if spec {
			p.SpecUndefNames = map[string]bool{"k": true}
		}
		return p
	}

	r := newReg()
	_, err := RunProgram(prog(true), r)
	var ae *core.BoruError
	if !errors.As(err, &ae) || ae.Code != "undefined_word" || ae.Row != readAt.Row || ae.Col != readAt.Col {
		t.Fatalf("a placed name's miss raises undefined_word at the read: %v", err)
	}
	if r.Defs.Depth("k") != 0 {
		t.Fatalf("the undef popped the live binding: depth %d", r.Defs.Depth("k"))
	}

	r = newReg()
	_, err = RunProgram(prog(false), r)
	if !errors.As(err, &ae) || ae.Code != "internal_error" {
		t.Fatalf("a miss outside the set keeps the defer: %v", err)
	}

	// A second undef of a popped name is the interpreter's no-op.
	r = newReg()
	p := prog(true)
	p.Code = []compiler.Instr{{Op: compiler.OpUndefDynScope, Arg: 0}, {Op: compiler.OpUndefDynScope, Arg: 0}}
	p.Debug = []core.SrcPos{undefAt, undefAt}
	if _, err := RunProgram(p, r); err != nil || r.Defs.Depth("k") != 0 {
		t.Fatalf("two undefs: err=%v depth=%d", err, r.Defs.Depth("k"))
	}

	// The debug table's position for a pc it does not cover is the zero
	// position, which stampAt leaves unstamped.
	if got := debugPosAt(nil, 0); got != (core.SrcPos{}) {
		t.Fatalf("debugPosAt out of range = %+v", got)
	}
	if got := debugPosAt([]core.SrcPos{readAt}, 0); got != readAt {
		t.Fatalf("debugPosAt in range = %+v", got)
	}
}
