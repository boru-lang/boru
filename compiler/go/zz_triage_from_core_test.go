package compiler

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestRecordDynApplyPendingConsume(t *testing.T) {
	es := NewEmitState()
	fn := core.NewDynamicCarrier(core.TFunction)
	seedProduced(es, fn, 1)
	es.units[len(es.units)-1].pendingApply = []pendingApply{{id: fn.ID}}
	out := core.NewInteger(0)
	if _, ok := es.RecordDynApply([]core.Value{core.NewInteger(10)}, fn, out, core.SrcPos{}); !ok {
		t.Fatal("resolvable apply with pending entry should record")
	}
	if len(es.units[len(es.units)-1].pendingApply) != 0 {
		t.Fatal("pending apply entry should be consumed")
	}
}

func TestRecordPolyCallGuard(t *testing.T) {
	// A multi-result poly (flex `pop` → [remaining, popped]) now RECORDS: each
	// result seats under its own index, the event is marked generic, and the
	// VM enforces the recorded result-count claim (PolyRef.NOut).
	es := NewEmitState()
	o1 := core.NewDynamicCarrier(core.TFlexList)
	o2 := core.NewDynamicCarrier(core.TAny)
	if !es.RecordPolyCall("pop", nil, []core.Value{o1, o2}, core.SrcPos{}, nil, nil) {
		t.Fatal("multi-out poly call should record")
	}
	pr1, ok1 := es.producedBy[o1.ID]
	pr2, ok2 := es.producedBy[o2.ID]
	if !ok1 || !ok2 {
		t.Fatal("both results should register in producedBy")
	}
	if pr1.idx != 0 || pr2.idx != 1 {
		t.Errorf("result indices = %d,%d, want 0,1", pr1.idx, pr2.idx)
	}
	if pr1.seq != pr2.seq {
		t.Errorf("results should share one event seq, got %d and %d", pr1.seq, pr2.seq)
	}
	if !es.eventInfo[pr1.seq].generic {
		t.Error("a multi-result poly event should be marked generic")
	}
	// inactive recorder → declines.
	if inactiveEmitState().RecordPolyCall("pop", nil, []core.Value{o1, o2}, core.SrcPos{}, nil, nil) {
		t.Fatal("inactive multi-out poly call should decline")
	}

	// De-collision: a later result whose ID collides with an EARLIER event's
	// output (different seq) is re-minted so provenance lookups stay distinct —
	// UNLESS that ID is an input passthrough (a receiver flowing straight
	// through), which keeps its identity.
	es2 := NewEmitState()
	a := core.NewDynamicCarrier(core.TFlexList)
	if !es2.RecordPolyCall("pop", nil, []core.Value{a, core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, nil, nil) {
		t.Fatal("seed poly should record")
	}
	// A second poly whose first result reuses a's ID, with a NOT among the
	// args → the colliding result is re-minted. (RecordPolyCall mutates the
	// outs SLICE in place, so observe the element, not the local copy.)
	c := core.NewDynamicCarrier(core.TFlexList)
	c.ID = a.ID
	outs2 := []core.Value{c, core.NewDynamicCarrier(core.TAny)}
	if !es2.RecordPolyCall("shift", nil, outs2, core.SrcPos{}, nil, nil) {
		t.Fatal("collision poly should record")
	}
	if outs2[0].ID == a.ID {
		t.Error("a colliding non-passthrough result ID should be re-minted")
	}
	// A third poly whose result reuses a's ID WHILE a is passed as an arg →
	// the passthrough exemption keeps the ID.
	d := core.NewDynamicCarrier(core.TAny)
	d.ID = a.ID
	outs3 := []core.Value{d, core.NewDynamicCarrier(core.TAny)}
	if !es2.RecordPolyCall("shift", []core.Value{a}, outs3, core.SrcPos{}, nil, nil) {
		t.Fatal("passthrough poly should record")
	}
	if outs3[0].ID != a.ID {
		t.Error("a passthrough result ID should be preserved")
	}
}

func TestFnCompileRefusesIdentitylessCapture(t *testing.T) {
	// A closure whose capture snapshot is a runtime mint (no ID) cannot
	// be compiled: capture slots are positional, so skipping one would
	// misalign every later capture. The unit must refuse (conservative
	// interpreter fallback), never collapse slots.
	c := &core.CheckState{}
	defer c.BeginCompilePass()()
	es, ok := c.Emit.(*EmitState)
	if !ok {
		t.Fatal("BeginCompilePass did not install an EmitState")
	}
	blankCap := core.Value{Parent: core.TInteger, Data: core.IntPayload{N: 5}} // hand-built: no ID
	_, fin, started := es.StartFnCompile("k", "f", nil, nil, nil, nil,
		[]core.CapturedBinding{{Name: "x", Value: blankCap}}, false, core.SrcPos{})
	if started && fin != nil {
		fin(nil)
	}
	if es.Compilable {
		t.Error("identity-less capture must mark the program uncompilable")
	}
}

// compileTokens runs the check pass with the emit recorder armed and
// finalizes the recording into a Program — the eng-level equivalent of
// lang's CompileCheck. Returns (nil, reason) when the program is
// uncompilable or check found errors.
func compileTokens(t *testing.T, r *core.Registry, tokens []core.Value) (*Program, string) {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	engine := core.NewTop(r)
	residual, runErr := engine.Run(tokens)
	var prog *Program
	reason := ""
	func() {
		defer done()
		if runErr != nil {
			prog, reason = nil, "check error: "+runErr.Error()
			return
		}
		for _, d := range r.Check.Diagnostics {
			if d.Severity == core.SeverityError {
				prog, reason = nil, "check diagnostics: "+d.Detail
				return
			}
		}
		if r.Check.SuppressedRuntimeError {
			prog, reason = nil, "suppressed runtime error"
			return
		}
		if r.Check.AmbiguousGradualSplit {
			prog, reason = nil, "ambiguous gradual split"
			return
		}
		p, why, ok := r.Check.Recorder().(*EmitState).Finalize(residual)
		if !ok {
			prog, reason = nil, "finalize: "+why
			return
		}
		prog = p
	}()
	return prog, reason
}

func TestDisassembleOpcodeArms(t *testing.T) {
	p := &Program{
		Consts: []core.Value{core.NewInteger(1)},
		Types:  []TypeRef{{Name: "Integer"}},
		Sigs:   []SigRef{{Word: "w"}},
		Fns: []CompiledFn{{
			Name: "f", NParams: 1, NLocals: 2,
			LocalNames: []string{"x", ""},
			Code:       []Instr{{Op: OpRet}},
		}},
		PolyRefs:    []PolyRef{{}},
		UserPolys:   []UserPolyRef{{}},
		MakeMaps:    []MakeMapSpec{{Keys: []string{"a"}}},
		Traps:       []TrapSpec{{Code: "trap_code"}},
		Interps:     []InterpSpec{{NHoles: 1}},
		TypedBinds:  []core.TypedBindSpec{{Name: "x", Describe: "Integer"}},
		GlobalBinds: []GlobalBindSpec{{Name: "g", Depth: 1}},
		BindTwins:   []core.BindTransition{{Kind: core.BindDef, Name: "tw", Depth: 1}},
		DynMethods:  []DynMethodSpec{{Word: "m", NArgs: 1, NOut: 1}},
	}
	ops := []Opcode{
		OpPushConst, OpJmp, OpJmpIfFalse, OpForNext, OpPushLocal,
		OpForSetup, OpStoreLocal, OpPushType, OpCallUser, OpTailCallUser,
		OpPushClosure, OpCallNativePoly, OpCallDynamic,
		OpCallDynamicTrailing, OpCallDynFrame, OpMakeList, OpMakeMap,
		OpTrap, OpReverse, OpInterp, OpBindTyped, OpBindGlobal,
		OpBindTwin, OpCallDynMethod,
	}
	for _, op := range ops {
		p.Code = append(p.Code, Instr{Op: op})
	}
	out := p.Disassemble()
	if !strings.Contains(out, "fn f0") || !strings.Contains(out, "[x _]") {
		t.Errorf("Disassemble missing fn header/local names:\n%s", out)
	}
	// Since the flip there is ONE bind regime: every twin replays, every
	// global bind pushes, and the disassembler says so — no inert tag, no
	// push tag, because there is no other mode for either to contrast with.
	if !strings.Contains(out, "bind twin def tw @depth 1 (replay)") ||
		strings.Contains(out, "(inert)") || strings.Contains(out, "(push)") {
		t.Errorf("disassembly bind tags wrong:\n%s", out)
	}
	if !strings.Contains(out, "polyrefs=1") || !strings.Contains(out, "userpolys=1") {
		t.Errorf("Disassemble missing poly summaries:\n%s", out)
	}
	if p.StoredRefCount() != 0 || p.StoredRefStampedCount() != 0 {
		t.Error("stored-ref counters must be zero on a synthetic program")
	}
	if ClosureWantsKeyVal(core.NewInteger(1)) {
		t.Error("plain integer is not a keyval-hungry closure")
	}
	if IsCompiledClosure(core.NewInteger(1)) {
		t.Error("plain integer is not a compiled closure")
	}
}
