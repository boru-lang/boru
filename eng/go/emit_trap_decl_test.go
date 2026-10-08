package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestSetUnitDeclGuard covers the out-of-range no-op in SetUnitDecl, the
// mirror of TestSetUnitParamTypesGuard: a unit index outside es.fnRecs is
// silently ignored (the named-fn / user-poly compile paths only ever pass a
// unit returned by StartFnCompile).
func TestSetUnitDeclGuard(t *testing.T) {
	compiler.NewEmitState().SetUnitDecl(-1, core.DeclSite{}) // out-of-range → no-op
}

// TestSetUnitReturnPatternsGuard is the RET-side twin of the two guards
// above: SetUnitReturnPatterns carries FnSig.ReturnPatterns onto the compiled
// unit so the VM's RET enforces a declared union / structural return, and it
// takes the same out-of-range no-op as its param-side and decl-site
// neighbours. Both real callers pass a unit StartFnCompile returned and have
// already screened `unit < 0`, so the guard is reachable only here.
func TestSetUnitReturnPatternsGuard(t *testing.T) {
	compiler.NewEmitState().SetUnitReturnPatterns(-1, nil) // out-of-range → no-op
}

// TestRecordTrapErrNil covers RecordTrapErr's nil-error guard: a nil BoruError
// records nothing and reports "not owned here". Callers always pass a
// freshly-built interpreter error, so this is the defensive floor.
func TestRecordTrapErrNil(t *testing.T) {
	if compiler.NewEmitState().RecordTrapErr(nil, core.SrcPos{}) {
		t.Fatal("RecordTrapErr(nil) should decline")
	}
}
