// Native Returns/ReturnsFn coverage gate (G7 —
// design/legacy/CHECKER-BYTECODE-COMPLETION-PLAN.0.ignore Phase 4.3).
//
// Every registered native signature — in the default registry and in
// every `boru:` module's sub-registry — must tell the checker what it
// produces: a declared `Returns` (an empty non-nil slice is a valid
// "produces nothing"), a check-mode `ReturnsFn`, a full-stack
// `CheckFullStack` shape function, or a boru body (whose returns the
// analyser derives itself). Signature.DeclaresCheckReturns is the single
// definition of that condition.
//
// A signature declaring none of these hits the `missing_returns`
// fallback at EVERY call site: a per-site warning plus a dynamic(Any)
// result that feeds the Any frontier (TestCheckAnyFrontier). This gate
// is what holds "missing_returns on the spec corpus → 0" as the
// registry grows: a new native cannot land unannotated.
//
// Annotate truthfully — declare ONLY what the handler actually
// guarantees (wrong Returns annotations were a historical soundness
// source; see the pinnedTypeSoundnessViolations history). Where the
// result genuinely depends on the input, either write a ReturnsFn that
// inspects the statically-known operands or declare `[]*Type{TAny}`
// (which rides as dynamic(Any) — same checker behaviour as the
// fallback, minus the noise). Only when neither is possible — a sig
// shape the annotation surface cannot express at all — add the word to
// nativeReturnsOptOut below WITH a one-line justification.
package langspec

import (
	"fmt"
	"sort"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/parser/go"
)

// nativeReturnsOptOut is the explicit opt-out list: "word/arity" (or
// "module:word/arity" for a module sub-registry native) → a one-line
// justification for why the signature cannot declare its check-mode
// returns. Keep it EMPTY unless a sig genuinely cannot be annotated:
// prefer Returns: []*Type{TAny} (honest unknown) or a ReturnsFn over an
// entry here. Every entry must name a real registered sig — a stale
// entry fails the gate so the list cannot rot.
var nativeReturnsOptOut = map[string]string{
	// (empty — every registered native signature is annotated today)
}

func TestNativeReturnsCoverage(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)

	seen := map[string]bool{} // opt-out keys actually encountered
	var missing []string

	scanReg := func(prefix string, r *core.Registry) {
		for _, name := range r.RegisteredWordNames() {
			if !r.IsBuiltinWord(name) {
				continue // def-bound values, not registered natives
			}
			fd := r.Lookup(name)
			if fd == nil {
				continue
			}
			for i := range fd.Signatures {
				sig := &fd.Signatures[i]
				key := fmt.Sprintf("%s%s/%d", prefix, name, sig.TotalArgs())
				if sig.DeclaresCheckReturns() {
					continue
				}
				if _, ok := nativeReturnsOptOut[key]; ok {
					seen[key] = true
					continue
				}
				missing = append(missing, key)
			}
		}
	}

	scanReg("", reg)
	modNames := modules.Names()
	sort.Strings(modNames)
	for _, mod := range modNames {
		desc, err := modules.Resolve(mod, reg)
		if err != nil {
			t.Fatalf("resolve boru:%s: %v", mod, err)
		}
		if desc.Src != nil {
			scanReg(mod+":", desc.Src)
		}
	}

	sort.Strings(missing)
	// De-duplicate: the same core native appears in every module
	// sub-registry (each is a DefaultRegistry), so one unannotated core
	// sig would otherwise report once per module.
	deduped := missing[:0]
	for i, k := range missing {
		if i == 0 || missing[i-1] != k {
			deduped = append(deduped, k)
		}
	}
	for _, k := range deduped {
		t.Errorf("native sig %s declares no check-mode returns — add Returns "+
			"(use []*Type{TAny} for an honest unknown), a ReturnsFn, or a "+
			"CheckFullStack shape fn; see the gate header before opting out", k)
	}

	// A stale opt-out (the sig was annotated or removed) must be deleted,
	// so the allowlist cannot accumulate dead justifications.
	for key := range nativeReturnsOptOut {
		if !seen[key] {
			t.Errorf("nativeReturnsOptOut entry %q matched no registered "+
				"unannotated signature — remove the stale entry", key)
		}
	}
}
