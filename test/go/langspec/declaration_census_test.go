// The declaration census — Stage 6's worklist, measured.
//
// design/FULL-COMPILATION.0.md section 6.8 says the handler-contract
// triple {tapeBound, needs, env} "produces the worklist" of handlers that
// must be migrated, and section 10 stages that migration. The worklist is
// a number nobody has counted, so Stage 6 has no scorecard and no way to
// tell progress from motion. This counts it.
//
// A signature is DECLARATION-RELEVANT when the recorder has to know
// something about its handler that the type surface does not say: it takes
// an unevaluated code body (NoEvalArgs), it quotes an operand (QuoteArgs),
// it declares a callable body convention (Callable), or it can receive a
// FUNCTION-typed operand. Those are exactly the cases where "what does the
// Go handler do with this operand" is irreducible — the lesson the
// fn-util defect taught three times.
//
// A relevant signature is DECLARED when it carries any of the compile
// facts that answer the question today: a CompileEffect flag, a
// CallableSpec, or per-slot inert-fn markings. An UNDECLARED relevant
// signature is one where the recorder is running on the zero value's
// assumption — which is the substantive claim the triple's C1 exists to
// stop being silent about, since a tri-state tapeBound that is unset must
// refuse rather than default permissive.
package langspec

import (
	"os"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	native "github.com/boru-lang/boru/lang/go/native"
)

// undeclaredHandlerCeiling is the number of declaration-relevant
// signatures carrying no compile declaration at all. Monotone DOWN only:
// Stage 6 migrates handlers to the triple, and every migration removes
// one. Never raise it — a new undeclared relevant handler is a new
// fn-util defect waiting to be found by someone measuring.
//
// Scope: the DEFAULT registry — core words with no modules imported. A
// program that imports modules registers more, so this is a floor on the
// worklist, not its total; it is the part that is always present and can
// therefore be ratcheted deterministically.
const undeclaredHandlerCeiling = 114 // 114 (2026-08-25, Stage-1 baseline) -> 0 (Stage 6)

// relevant reports whether the recorder needs a handler declaration for
// this signature, and why.
func relevant(sig *core.Signature) (bool, string) {
	switch {
	case len(sig.NoEvalArgs) > 0:
		return true, "code-body"
	case sig.Callable != nil:
		return true, "callable"
	case len(sig.QuoteArgs) > 0:
		return true, "quoted"
	}
	for _, t := range sig.ArgTypes() {
		if t != nil && t.ConformsTo(core.TFunction) {
			return true, "fn-operand"
		}
	}
	return false, ""
}

// declared reports whether the signature carries any compile fact.
func declared(sig *core.Signature) bool {
	if sig.CompileEffect != 0 || sig.Callable != nil {
		return true
	}
	for _, inert := range sig.FnInertArgs {
		if inert {
			return true
		}
	}
	return false
}

func TestDeclarationCensus(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	var total, rel, undeclared int
	byReason := map[string]int{}
	undeclaredBy := map[string]int{}
	for _, name := range reg.RegisteredWordNames() {
		fd := reg.Lookup(name)
		if fd == nil {
			continue
		}
		for i := range fd.Signatures {
			sig := &fd.Signatures[i]
			total++
			ok, why := relevant(sig)
			if !ok {
				continue
			}
			rel++
			byReason[why]++
			if !declared(sig) {
				undeclared++
				undeclaredBy[why]++
				// BORU_LOG_UNDECLARED=1 names every undeclared signature —
				// the Stage-6 handler-migration worklist (`make
				// handler-worklist`; design/HANDLER-MIGRATION-LINE.0.md).
				if os.Getenv("BORU_LOG_UNDECLARED") != "" {
					t.Logf("UNDECLARED\t%s\t%s\t%s", name, why, sigShape(sig))
				}
			}
		}
	}

	render := func(m map[string]int) string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if m[keys[i]] != m[keys[j]] {
				return m[keys[i]] > m[keys[j]]
			}
			return keys[i] < keys[j]
		})
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + "×" + itoa(m[k])
		}
		if len(parts) == 0 {
			return "none"
		}
		return strings.Join(parts, ", ")
	}

	t.Logf("declaration census: %d signatures, %d declaration-relevant (%s), %d undeclared (%s)",
		total, rel, render(byReason), undeclared, render(undeclaredBy))

	if undeclared > undeclaredHandlerCeiling {
		t.Errorf("undeclared declaration-relevant signatures %d exceed ceiling %d — the recorder is assuming a handler contract nobody wrote down: %s",
			undeclared, undeclaredHandlerCeiling, render(undeclaredBy))
	}
}

// sigShape renders a signature's parameter types for the worklist line,
// `(Integer List)` — the shape a declaration will be written against.
func sigShape(sig *core.Signature) string {
	parts := make([]string, len(sig.Args))
	for i, a := range sig.Args {
		if a == nil {
			parts[i] = "?"
			continue
		}
		parts[i] = a.Name()
	}
	return "(" + strings.Join(parts, " ") + ")"
}
