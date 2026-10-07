package lang

import (
	"fmt"
	"testing"
)

// TestTokenBodyInModuleFnReadsModuleScope pins the registry a run-time-
// stamped TOKEN body dispatches on. A module fn's compiled unit runs on the
// module's registry (CompiledFn.Reg), so a lambda param a quotation reads —
// `ev` below — is dyn-installed THERE. The quotation reaches `filter` raw
// (the collection is gradual, so the call is a poly dispatch and the body is
// a plain list at run time), is stamped as a token body and hosted on the VM;
// hosted on the RUNNING registry, its read of `ev` missed the module install
// and raised `undefined word: ev` where the interpreter answered
// (kg/validate.boru check-code-units, 2026-10-07). The body's home is the
// CALLING registry — the one RunResolved steps it on — and the seam hosts it
// there (eng/go vm_token_body.go). Both lanes agree.
func TestTokenBodyInModuleFnReadsModuleScope(t *testing.T) {
	src := `import module [
  def ids fn [[c:Any] [Any] [c]]
  def hits fn [[xs:List ks:List] [List] [
    each ([ev] => [ ((filter [eq ev.id] (ids ks)) size) gt 0 ]) xs
  ]]
  export "M" {hits: hits/v}
] end (M.hits [{id:"a"} {id:"zz"}] ["a" "b"])`
	gotC, errC := mustNew(t).RunCompiledStrict(src)
	if errC != nil {
		t.Fatalf("strict compiled run: %v", errC)
	}
	gotI, errI := mustNew(t).RunInterp(src)
	if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[[true false]]" {
		t.Errorf("compiled %v, interp %v / %v, want [[true false]]", gotC, gotI, errI)
	}
}
