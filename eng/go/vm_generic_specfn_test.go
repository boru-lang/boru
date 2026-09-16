package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// A speculative fn family's dispatch (Program.SpecFnNames, the seventieth
// increment): the lead's miss is the interpreter's undefined_word at the
// word, raised; a bound lead runs the LIVE signature's own unit, located by
// its body's first token (CompiledFn.BodyPos) — the outer overload or the
// arm's shadow, never the committed unit by shape — and a body with no unit
// defers.
func TestDispatchGenericSpecFn(t *testing.T) {
	t.Run("the miss raises the word", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Regions[0].Word = "nope"
		p.SpecFnNames = map[string]bool{"nope": true}
		_, err := RunProgram(p, reg)
		var be *core.BoruError
		if !errors.As(err, &be) || be.Code != "undefined_word" || be.Row != 1 || be.Col != 1 {
			t.Fatalf("undefined_word at the word, not a defer: %v", err)
		}
	})
	t.Run("the live signature's own unit, by declaration site", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.SpecFnNames = map[string]bool{"w": true}
		// w's signature carries no declaration site: no unit is located,
		// and the dispatch defers.
		var bails []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { bails = append(bails, ev.Site) })
		_, err := RunProgram(p, reg)
		disarm()
		wantInternal(t, err, "has no unit")
		if len(bails) != 1 || bails[0] != "vm:generic-foreign-unit" {
			t.Fatalf("one designed defer: %v", bails)
		}
		// A declared signature whose unit the program holds: that unit
		// runs, whatever the committed one was.
		decl := core.DeclSite{Pos: core.SrcPos{Row: 4, Col: 2}, File: "w.boru"}
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}), Decl: decl,
		}}}))
		p.Generics[0].Unit = 0 // the committed unit is wrong on purpose
		p.Fns = append(p.Fns, compiler.CompiledFn{Name: "w2", NParams: 2, NArgs: 2, NLocals: 2, Decl: decl,
			Code: []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 1}, {Op: compiler.OpRet}}})
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(1)) {
			t.Fatalf("the live signature's unit (w2, returning its second operand) runs: out=%v err=%v", out, err)
		}
	})
	// A stored handler's live lead (Program.LiveLeadNames — the
	// seventy-first increment) takes the same path: the miss raises the
	// word, a bound lead runs the live signature's own unit by its
	// declaration site.
	t.Run("liveLead: the miss raises the word", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.Regions[0].Word = "nope"
		p.LiveLeadNames = map[string]bool{"nope": true}
		_, err := RunProgram(p, reg)
		var be *core.BoruError
		if !errors.As(err, &be) || be.Code != "undefined_word" || be.Row != 1 || be.Col != 1 {
			t.Fatalf("undefined_word at the word, not a defer: %v", err)
		}
	})
	t.Run("liveLead: the live signature's own unit", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.LiveLeadNames = map[string]bool{"w": true}
		decl := core.DeclSite{Pos: core.SrcPos{Row: 5, Col: 2}, File: "w.boru"}
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.NewWord("a")}), Decl: decl,
		}}}))
		p.Generics[0].Unit = 0
		p.Fns = append(p.Fns, compiler.CompiledFn{Name: "w2", NParams: 2, NArgs: 2, NLocals: 2, Decl: decl,
			Code: []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 1}, {Op: compiler.OpRet}}})
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(1)) {
			t.Fatalf("the live signature's unit runs for a live lead: out=%v err=%v", out, err)
		}
		if !liveLeadWord(p, "w") || liveLeadWord(p, "x") {
			t.Fatal("liveLeadWord reads the union")
		}
	})
	t.Run("specFnUnit", func(t *testing.T) {
		decl := core.DeclSite{Pos: core.SrcPos{Row: 2, Col: 2}, File: "a.boru"}
		p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "a", Decl: decl}}}
		if specFnUnit(p, &core.Signature{Impl: core.Boru(nil)}) != -1 {
			t.Fatal("a signature with no declaration site has no unit")
		}
		other := core.DeclSite{Pos: core.SrcPos{Row: 2, Col: 2}, File: "b.boru"}
		if specFnUnit(p, &core.Signature{Impl: core.Boru(nil), Decl: other}) != -1 {
			t.Fatal("the same position in another file locates nothing")
		}
		if specFnUnit(p, &core.Signature{Impl: core.Boru(nil), Decl: decl}) != 0 {
			t.Fatal("the signature's unit is located by its declaration site")
		}
	})
}
