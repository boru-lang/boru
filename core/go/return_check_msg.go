package core

import "fmt"

// Shared return-check diagnostic text — the single source of truth for the
// strings both engines raise when a fn body violates its declared return
// contract. The interpreter (Engine.returnTypeError / returnCountError,
// engine.go) and the bytecode VM (vmReturnTypeErr / vmReturnCountErr, vm.go)
// MUST produce byte-identical detail/hint for the same mismatch, so
// error-scraping tooling can never tell which engine ran. They used to hold
// two hand-kept copies of the format strings; the copies are gone — both build
// the text here and wrap it in their own error-construction plumbing (the
// interpreter stamps a SrcPos + effectiveSource; the VM routes through
// r.BoruError* / stampAt).

// returnTypeErrorText is the detail + hint for a fn return-type mismatch:
// funcName's index-th (1-based) return value was expected to be `expected` but
// the body produced `got`.
func ReturnTypeErrorText(funcName string, index int, expected *Type, got Value) (detail, hint string) {
	gotType := got.Parent
	if gotType == nil {
		// A lattice root has no Parent: the None literal a missing member
		// reads as named itself `<nil>` here (NUR324).
		gotType = ValueType(got)
	}
	detail = fmt.Sprintf("%s: return value %d: expected %s, got %s", funcName, index, expected, gotType)
	hint = "value: " + diagValue(got)
	return detail, hint
}

// returnCountErrorText is the detail for funcName leaving the wrong number of
// return values (expected vs got), followed by the values themselves.
//
// A diagnostic names the values it is about
// (design/DIAGNOSTIC-VALUES.0.md). "expected 1 return
// value(s), got 2" states a disagreement over a COUNT and then withholds
// the one thing that identifies which values are involved — leaving the
// reader to work out what the second value even was. `got` and the
// rendered list always describe the same values: callers pass exactly
// the slice the count was taken over.
//
// values may be empty — a body that returned nothing has nothing to
// show, and the count alone is the whole story there.
func ReturnCountErrorText(funcName string, expected, got int, values []Value) string {
	base := fmt.Sprintf("%s: expected %d return value(s), got %d", funcName, expected, got)
	if len(values) == 0 {
		return base
	}
	return base + " — " + diagValueList(values)
}

// buildReturnTypeError assembles the COMPLETE return-type diagnostic — the
// shared detail/hint text PLUS the two secondary spans (the produced value,
// and the return-contract declaration) — from data alone, so the interpreter
// (Engine.returnTypeError) and the compiled VM (vmReturnTypeErr) raise a
// byte-identical report. primaryPos is the call site (interpreter) or the
// shared fn unit (VM); the two legitimately differ (only the compiled program
// can point inside its shared unit), which is why the produced-value span
// attaches on the value's OWN position whenever it has one — an
// engine-independent decision, so the span labels match across engines.
func BuildReturnTypeError(source, funcName string, index int, expected *Type, got Value, primaryPos SrcPos, decl DeclSite) *BoruError {
	detail, hint := ReturnTypeErrorText(funcName, index, expected, got)
	ae := makeBoruErrorAt("type_error", detail, funcName, source, hint, primaryPos)
	if gp := got.Pos(); gp.Row > 0 {
		ae.Spans = append(ae.Spans, DiagSpan{
			Pos:   gp,
			Label: "the returned value was produced here",
		})
	}
	attachDeclSpan(ae, decl, "the declaration says `"+funcName+"` returns "+expected.String())
	return ae
}

// buildReturnCountError assembles the COMPLETE return-count diagnostic — the
// shared detail text plus the declaration span — so both engines match.
func BuildReturnCountError(source, funcName string, expected, got int, values []Value, primaryPos SrcPos, decl DeclSite) *BoruError {
	ae := makeBoruErrorAt("type_error", ReturnCountErrorText(funcName, expected, got, values), funcName, source, "", primaryPos)
	attachDeclSpan(ae, decl,
		fmt.Sprintf("the declaration of `%s` expects %d return value(s)", funcName, expected))
	return ae
}
