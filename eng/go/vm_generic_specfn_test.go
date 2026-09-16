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
	t.Run("the live signature's own unit, by body", func(t *testing.T) {
		p, reg := genericWorld(t)
		p.SpecFnNames = map[string]bool{"w": true}
		// w's body token carries no position: no unit is located, and the
		// dispatch defers.
		var bails []string
		disarm := reg.ArmRuntimeBailHook(func(ev core.BailEvent) { bails = append(bails, ev.Site) })
		_, err := RunProgram(p, reg)
		disarm()
		wantInternal(t, err, "body has no unit")
		if len(bails) != 1 || bails[0] != "vm:generic-foreign-unit" {
			t.Fatalf("one designed defer: %v", bails)
		}
		// A positioned body whose unit the program holds: that unit runs,
		// whatever the committed one was.
		at := core.SrcPos{Row: 4, Col: 2}
		reg.Defs.Push("w", core.NewFunction(core.FnDefInfo{Name: "w", Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2, Impl: core.Boru([]core.Value{core.WithPosAt(core.NewWord("a"), at)}),
		}}}))
		p.Generics[0].Unit = 0 // the committed unit is wrong on purpose
		p.Fns = append(p.Fns, compiler.CompiledFn{Name: "w2", NParams: 2, NArgs: 2, NLocals: 2, BodyPos: at,
			Code: []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 1}, {Op: compiler.OpRet}}})
		out, err := RunProgram(p, reg)
		if err != nil || len(out) != 1 || !core.ValuesEqual(out[0], core.NewInteger(1)) {
			t.Fatalf("the live body's unit (w2, returning its second operand) runs: out=%v err=%v", out, err)
		}
	})
	t.Run("specFnUnit", func(t *testing.T) {
		p := &compiler.Program{Fns: []compiler.CompiledFn{{Name: "a", BodyPos: core.SrcPos{Row: 2, Col: 2}}}}
		empty := &core.Signature{Impl: core.Boru(nil)}
		if specFnUnit(p, empty) != -1 {
			t.Fatal("an empty body has no unit")
		}
		zero := &core.Signature{Impl: core.Boru([]core.Value{core.NewWord("x")})}
		if specFnUnit(p, zero) != -1 {
			t.Fatal("an unpositioned body locates nothing")
		}
		hit := &core.Signature{Impl: core.Boru([]core.Value{core.WithPosAt(core.NewWord("x"), core.SrcPos{Row: 2, Col: 2})})}
		if specFnUnit(p, hit) != 0 {
			t.Fatal("the body's unit is located by its first token")
		}
	})
}
