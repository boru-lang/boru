package lang

import (
	"strings"
	"testing"
)

// TestForEachFnValueCallbackCompiles pins the generated sweep's `for-each` ×
// factory / container / module-export cells (test/go/sweep/seeds.tsv): a
// callback that is a fn VALUE the closure path cannot compile — a factory's
// result, a container member, a module export — declined "function-valued
// operand at for-each (Stage 3)" (the container read: the ambiguous-overload
// decline) where `each`'s twins compile through the dyn-body path (S1a).
// `for-each` now declares CompileDynBody, and tryRecordDynBody admits its
// 0-result dispatch (BodyOut 0 is the word's own count): the site lowers to
// a CALL_NATIVE under DynEnv and the handler drives the runtime value
// exactly as the interpreter's dispatch does.
func TestForEachFnValueCallbackCompiles(t *testing.T) {
	for _, src := range []string{
		`def acc (flex []) end def mk fn [[][Function][([e:Integer] => [acc push e])]] end for-each (mk) [1 2 3] end size acc`,
		`def acc (flex []) end def m {f: ([e:Integer] => [acc push e])} end for-each m.f [1 2 3] end size acc`,
		`import module [def acc (flex []) def stp fn [[e:Integer][Any][acc push e]] export "M" {stp: stp/v acc: acc}] end for-each M.stp [1 2 3] end size M.acc`,
		// The literal and lambda cells, unchanged beside them.
		`def acc (flex []) end for-each [acc swap push] [1 2 3] end size acc`,
		`def acc (flex []) end for-each ([e:Integer] => [acc push e]) [1 2 3] end size acc`,
		// The value's effects are the interpreter's, in order.
		`def acc (flex []) end def mk fn [[][Function][([e:Integer] => [acc push (e mul 2)])]] end for-each (mk) [1 2 3] end acc`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def acc (flex []) end def mk fn [[][Function][([e:Integer] => [acc push e])]] end for-each (mk) [1 2 3] end size acc`)
	if !strings.Contains(dis, "for-each") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the fn-value callback must drive a native for-each dispatch:\n%s", dis)
	}
}

// TestWalkFnValueHookCompiles pins the sweep's `walk` × factory / container
// / module-export cells: a descend hook that is a fn VALUE declined
// "code-body word walk (Stage 2)" (the factory and the container member) or
// "function value reaches walk (Stage 3)" (the module export). `walk`
// declares CompileDynBody now, so the hook the closure path cannot compile
// rides as a runtime value into a CALL_NATIVE under DynEnv, and the handler
// classifies it as the interpreter's walk does; a concrete hook reaching
// the same path is a CODE body (the NoEvalArgs slot), never a fixed
// value-eval, and the optional ascend slot is held to the body's own
// sentinel and replay rules.
func TestWalkFnValueHookCompiles(t *testing.T) {
	for _, src := range []string{
		`def acc (flex []) end def mk fn [[][Function][(m:Any => [acc push m.path])]] end walk {mode: "depth"} {a:1 b:[2 3]} (mk) end size acc`,
		`def acc (flex []) end def hs {h: (m:Any => [acc push m.path])} end walk {mode: "depth"} {a:1 b:[2 3]} hs.h end size acc`,
		`import module [def acc (flex []) def h fn [[m:Any][Any][acc push m.path]] export "M" {h: h/v acc: acc}] end walk {mode: "depth"} {a:1 b:[2 3]} M.h end size M.acc`,
		// The lambda and quotation cells, unchanged beside them.
		`def acc (flex []) end walk {mode: "depth"} {a:1 b:[2 3]} (m:Any => [acc push m.path]) end size acc`,
		`def acc (flex []) end walk {mode: "depth"} {a:1 b:[2 3]} [dot path acc swap push] end size acc`,
		// The visited paths, in the interpreter's order.
		`def acc (flex []) end def mk fn [[][Function][(m:Any => [acc push m.path])]] end walk {mode: "depth"} {a:1 b:[2 3]} (mk) end acc`,
	} {
		requireEngineParity(t, src, true)
	}
	dis := compileDisasm(t, `def acc (flex []) end def mk fn [[][Function][(m:Any => [acc push m.path])]] end walk {mode: "depth"} {a:1 b:[2 3]} (mk) end size acc`)
	if !strings.Contains(dis, "walk") || strings.Contains(dis, "FALLBACK") {
		t.Errorf("the fn-value hook must drive a native walk dispatch:\n%s", dis)
	}
}

// TestWalkCapturingClosureHookMatchesItsSignature pins walk's fn-VALUE
// closure arm (S1b-2): a CAPTURING factory lambda, def-bound or held in a
// container, reaches walk at run time as a compiled closure, not the
// FnDefInfo the non-capturing cells above carry. walk bridges it to the
// signature the interpreter's lambda carries and matches every node's
// payload map against it before the unit runs — in both modes and in the
// ascend slot as in the descend one — so a hook whose declared parameter
// the payload does not fit raises walk's own no-match, as the interpreter's
// lambda arm does, rather than running the unit blind over the map.
func TestWalkCapturingClosureHookMatchesItsSignature(t *testing.T) {
	const mk = `def acc (flex []) end def mk fn [[k:Integer][Function][(m:Any => [acc push (m.depth add k)])]] end `
	const mkN = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end `
	for _, src := range []string{
		mk + `def h (mk 10) end walk {mode: "depth"} {a:{b:1}} h/v end acc`,
		mk + `def h (mk 10) end walk {mode: "depth"} {a:{b:1}} h/v h/v end acc`,
		mk + `def h (mk 10) end walk {mode: "breadth"} {a:{b:1} c:2} h/v end acc`,
		mk + `def m {h: (mk 10)} end walk {mode: "breadth"} {a:{b:1} c:2} m.h m.h end acc`,
		`def acc (flex []) end def mk fn [[k:String][Function][([m:Map] => [acc push (m.path add k)])]] end def h (mk '!') end walk {} {a:{b:1}} h/v end acc`,
		// The payload map fits no Integer parameter: walk's no-match.
		mkN + `def h (mk 5) end walk {} {a:1} h/v`,
		mkN + `def m {h: (mk 5)} end walk {} {a:1} m.h`,
	} {
		requireEngineParity(t, src, true)
	}
}

// TestQuotedReceiverReachBakes pins the sweep's `codequote` × container /
// module-export cells: `typeof (codequote m.f)` is the interpreter's Reach
// (the quoted dot-access is data), and the operand declined "operand of
// unknown provenance or not statically materialisable at typeof" because a
// reach WITH a receiver was never an inert const (only the receiverless
// lens was). A QUOTED receiver reach whose tokens are inert members now
// bakes by value: neither engine expands a quoted reach, so it pushes,
// compares, renders and types identically.
func TestQuotedReceiverReachBakes(t *testing.T) {
	for _, src := range []string{
		`def m {f: ([n:Integer] => [n add 1])} end typeof (codequote m.f)`,
		`import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end typeof (codequote M.inc)`,
		`def m {f: ([n:Integer] => [n add 1])} end codequote m.f`,
		`def m {f: ([n:Integer] => [n add 1])} end def q (codequote m.f) end typeof q`,
		`def m {a:{b:2}} end (quote m.a.b) typeof`,
		`def m {a:{b:2}} end def q (codequote m.a.b) end [q (typeof q)]`,
		`def m {a:{b:2}} end (codequote m.a.b) eq (codequote m.a.b)`,
		// The unquoted reach still evaluates.
		`def m {a:{b:2}} end m.a.b`,
	} {
		requireEngineParity(t, src, true)
	}
}
