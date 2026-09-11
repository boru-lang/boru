package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// OpBindResident's TYPE arm, driven directly (the same direct-drive
// precedent the install/undef arms use): a type binding has no runtime
// value, so the op re-installs the captured BODY through the interpreter's
// own type installer — MINTING A FRESH NODE per element, which is what
// makes two executions stack two INDEPENDENTLY RETIRABLE levels the way the
// interpreter's per-element re-run does. The arm consumes no stack, which
// the trailing literal proves: it is still the program's residual.
func TestVMBindResidentTypeArm(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()

	// The captured entry's BODY is what the arm re-installs; its TypeDef is
	// the check pass's node, which the arm deliberately does NOT reuse.
	node := r.Types.MintType("Tq", core.TInteger)
	body := core.NewInteger(5) // a singleton type body: `def Tq 5` shape
	pos := core.SrcPos{Row: 1, Col: 1}
	p := &compiler.Program{
		Code: []compiler.Instr{
			{Op: compiler.OpPushConst, Arg: 0},
			{Op: compiler.OpBindResident, Arg: 0}, // element 1: install Tq
			{Op: compiler.OpBindResident, Arg: 0}, // element 2: stacks a second Tq
		},
		Debug:  []core.SrcPos{pos, pos, pos},
		Consts: []core.Value{core.NewInteger(7)},
		BindTwins: []core.BindTransition{
			{Kind: core.BindTypeInstall, Name: "Tq", Pos: pos},
		},
		BindTwinEntries: []core.DefEntry{
			{TypeDef: node, Body: body, Minted: true},
		},
		ResidentBinds: []compiler.ResidentBindSpec{
			{Name: "Tq", Twin: 0, TypeInstall: true},
		},
	}
	out, err := RunProgram(p, r)
	if err != nil {
		t.Fatalf("resident type program: %v", err)
	}
	if len(out) != 1 || out[0].String() != "7" {
		t.Fatalf("residual = %v, want the untouched [7] (the type arm has no stack effect)", out)
	}
	if d := r.Defs.Depth("Tq"); d != 2 {
		t.Fatalf("Tq depth after two elements = %d, want 2 (the interpreter's per-element leak)", d)
	}
	top, ok := r.Defs.TopEntry("Tq")
	if !ok || top.TypeDef == nil {
		t.Fatalf("Tq top entry = %+v, want a minted type binding", top)
	}
	if top.TypeDef == node {
		t.Fatal("the arm re-used the twin's captured node; each element must mint its own " +
			"(a shared node makes the first undef retire the level below it)")
	}
	// Independent retirement is the whole point: popping the second element's
	// binding must leave the first element's node resolvable.
	core.ApplyResidentBind(r, "Tq", true, core.Value{})
	rest, ok := r.Defs.TopEntry("Tq")
	if !ok || rest.TypeDef == nil || r.Types.LookupByID(rest.TypeDef.ID) == nil {
		t.Fatalf("after one undef the remaining Tq node is unresolvable (%+v) — the arm shared one node", rest)
	}
}

// The TYPE arm RAISES when the installer refuses, rather than installing
// nothing and running on. The check pass ran the same body under the same
// name, so a refusal here means a malformed program — driven directly with
// a body whose lattice node is gone, the one refusal a Program can be built
// to carry.
func TestVMBindResidentTypeArmInstallerRefusal(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()

	prefab := r.Types.MintRefinePrefab(core.TInteger)
	body := core.NewTypeLiteral(prefab)
	r.Types.Retire(prefab) // the node the installer would rename is no longer there

	pos := core.SrcPos{Row: 1, Col: 1}
	p := &compiler.Program{
		Code:            []compiler.Instr{{Op: compiler.OpBindResident, Arg: 0}},
		Debug:           []core.SrcPos{pos},
		BindTwins:       []core.BindTransition{{Kind: core.BindTypeInstall, Name: "Tlost", Pos: pos}},
		BindTwinEntries: []core.DefEntry{{Body: body}},
		ResidentBinds:   []compiler.ResidentBindSpec{{Name: "Tlost", Twin: 0, TypeInstall: true}},
	}
	out, err := RunProgram(p, r)
	if err == nil {
		t.Fatalf("a refused type install must raise, got out=%v", out)
	}
	if !strings.Contains(err.Error(), "BIND_RESIDENT type install") {
		t.Fatalf("error = %v, want the type-install arm's own wording", err)
	}
}
