package check

import (
	core "github.com/boru-lang/boru/core/go"
)

// CompileFnSigUnit compiles one own signature of fnDef to a fn unit exactly
// as a dispatch of it would (the ReturnsFn closure BuildFnBodyReturnsFn
// builds), without recording a call: the body is analysed in the fn's OWN
// registry with the dispatching pass's check state shared into it
// (shareCheckStateFrom — name resolution stays the module's, the
// recording lands on this pass), and the unit is stamped with the
// signature's declared param types and patterns, its returns and return
// patterns and its declaration site, so the VM enforces at CALL_USER and
// RET what the interpreter's dispatch and ReturnCheck enforce. The inputs
// are the params' own carriers (ParamInputCarrier). Returns the unit, or
// -1 for a Go-implemented or fallback signature, a recorder that declines,
// or a nil registry.
//
// The compiler's speculative fn def uses it for the OUTER overload a
// conditional-body redefinition replaces (review of #466): the model holds
// the shadow from the clobber on, so no dispatch would compile the outer,
// and the routed op needs the live signature's own unit.
func CompileFnSigUnit(caller *core.Registry, fnDef core.FnDefInfo, sigIdx int) int {
	if caller == nil || caller.Check == nil || sigIdx < 0 || sigIdx >= len(fnDef.Signatures) {
		return -1
	}
	s := &fnDef.Signatures[sigIdx]
	if _, isBoru := s.Impl.(*core.BoruImpl); !isBoru || s.Fallback {
		return -1
	}
	r := caller
	if fnDef.Registry != nil {
		r = fnDef.Registry
	}
	restore := shareCheckStateFrom(r, caller)
	defer restore()
	es := r.Check.Recorder()
	paramNames := make([]string, len(s.Params))
	paramTypes := make([]*core.Type, len(s.Params))
	paramPatterns := make([]*core.Value, len(s.Params))
	inputs := make([]core.Value, len(s.Params))
	for i, p := range s.Params {
		paramNames[i], paramTypes[i], paramPatterns[i] = p.Name, p.Type, p.Pattern
		t := p.Type
		if t == nil {
			t = core.TAny
		}
		inputs[i] = ParamInputCarrier(t)
	}
	declaredReturns := append([]*core.Type(nil), s.Returns...)
	declaredReturnPatterns := append([]*core.Value(nil), s.ReturnPatterns...)
	compileReturns := declaredReturns
	if fnDef.Anonymous {
		compileReturns = LambdaCountContract(len(s.Returns))
		declaredReturns = nil
		declaredReturnPatterns = nil
	}
	body := append([]core.Value(nil), s.Body()...)
	var fnPos core.SrcPos
	if len(body) > 0 {
		fnPos = body[0].Pos()
	}
	key := FnAnalysisKey(r.AnalysisScopeID(), fnDef.Name, inputs, fnDef.Captured, body)
	unit, finish, ok := es.StartFnCompile(key, fnDef.Name, r, inputs, compileReturns, paramNames, fnDef.Captured, fnDef.Gen != nil, fnPos)
	if !ok {
		return -1
	}
	es.SetUnitParamTypes(unit, paramTypes, paramPatterns)
	es.SetUnitBody(unit, body)
	es.SetUnitReturnPatterns(unit, declaredReturnPatterns)
	es.SetUnitDecl(unit, s.Decl)
	if finish != nil {
		delete(r.Check.FnSummaries, key)
		finish(AnalyseFnBody(r, fnDef.Name, paramNames, body, inputs, fnDef.Captured, declaredReturns, fnDef.Anonymous))
	}
	return unit
}
