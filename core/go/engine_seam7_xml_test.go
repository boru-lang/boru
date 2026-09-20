package core

// Wave-7 coverage, part 7: BuildXmlFromTmpl / EvalXmlInterp arms, reached
// by constructing XmlTmpl skeletons directly. See design/TEST-SEAMS.10.md.

import (
	"testing"
)

// xmlReg builds a cov registry plus a carrier-bound name (dynv) and a
// flow-control-raising word (cbreak) for driving the dynamic / flow arms.
func xmlReg(t *testing.T) *Registry {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cfailx",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, &BoruError{Code: "runtime_error", Detail: "xboom"}
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(NativeFunc{
			Name: "cbreak",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
					reg.FlowCtrl = FlowBreak
					return []Value{NewInteger(0)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
	r.Defs.Push("dynv", NewCarrier(TAny))
	return r
}

func TestS7BuildXmlEmptyLiteralChild(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenLit, Lit: ""}}}
	_, _, _, _, err := e.BuildXmlFromTmpl(tmpl)
	if err != nil {
		t.Fatalf("empty literal child: %v", err)
	}
}

func TestS7BuildXmlNestedChildError(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	child := &XmlTmpl{Tag: "span", Cren: []XmlCren{{
		Kind: XmlCrenExpr, Expr: []Value{NewWord("cfailx"), NewInteger(1)},
	}}}
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: child}}}
	if _, _, _, _, err := e.BuildXmlFromTmpl(tmpl); err == nil {
		t.Fatal("a nested child template that errors must propagate")
	}
}

func TestS7BuildXmlNilChildSkipped(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{Kind: XmlCrenChild, Child: nil}}}
	if _, _, _, _, err := e.BuildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("nil child should be skipped, got %v", err)
	}
}

func TestS7BuildXmlAttrFlowCtrl(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Attr: []XmlAttrTmpl{{
		Name: "x", Parts: []InterpPart{{Expr: []Value{NewWord("cbreak")}}},
	}}}
	if _, _, _, _, err := e.BuildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("attr flow-ctrl: %v", err)
	}
	if r.FlowCtrl != FlowBreak {
		t.Error("cbreak should have raised a flow-control signal")
	}
}

func TestS7BuildXmlChildExprFlowCtrl(t *testing.T) {
	r := xmlReg(t)
	e := NewTop(r)
	tmpl := XmlTmpl{Tag: "p", Cren: []XmlCren{{
		Kind: XmlCrenExpr, Expr: []Value{NewWord("cbreak")},
	}}}
	if _, _, _, _, err := e.BuildXmlFromTmpl(tmpl); err != nil {
		t.Fatalf("child expr flow-ctrl: %v", err)
	}
	if r.FlowCtrl != FlowBreak {
		t.Error("cbreak should have raised a flow-control signal")
	}
}

func TestS7EvalXmlInterpNonTemplate(t *testing.T) {
	// A non-XmlInterp value fails AsXmlInterp → error arm.
	e := NewTop(xmlReg(t))
	if _, err := e.EvalXmlInterp(NewInteger(5)); err == nil {
		t.Fatal("EvalXmlInterp on a non-template value should error")
	}
}

func TestS7EvalInterpStringNonTemplate(t *testing.T) {
	// A non-InterpString value → AsInterpString errors → empty string.
	e := NewTop(xmlReg(t))
	out, err := e.evalInterpString(NewInteger(5))
	if err != nil {
		t.Fatalf("evalInterpString: %v", err)
	}
	if s, _ := AsString(out); s != "" {
		t.Errorf("non-template → %q, want empty string", s)
	}
}

// --- EvalXmlInterp check-mode compile failure --------------------------------------
