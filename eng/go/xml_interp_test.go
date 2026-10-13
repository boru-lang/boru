package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// White-box arms of rebuildXmlFromTmpl (OpInterpXml, §9.2c) that the
// source-level rows cannot reach comfortably: a nil child template (the
// defensive mirror of BuildXmlFromTmpl's guard), the empty-text coalesce
// return, and the attribute literal-part arm; plus RecordInterpXml's
// inactive-state decline.
func TestRebuildXmlFromTmplArms(t *testing.T) {
	tmpl := core.XmlTmpl{
		Tag: "p",
		Attr: []core.XmlAttrTmpl{{Name: "a", Parts: []core.InterpPart{
			{Lit: "v"},
			{Expr: []core.Value{core.NewWord("x")}},
		}}},
		Cren: []core.XmlCren{
			{Kind: core.XmlCrenChild, Child: nil},
			{Kind: core.XmlCrenExpr, Expr: []core.Value{core.NewWord("x")}},
		},
	}
	out, used := core.RebuildXmlFromTmpl(tmpl, []core.Value{core.NewString("7"), core.NewString("")})
	if used != 2 {
		t.Fatalf("used = %d, want 2", used)
	}
	if got := out.String(); got != `<p a="v7"/>` {
		t.Fatalf("rebuild = %s, want <p a=\"v7\"/> (literal+hole attr, empty text hole)", got)
	}

	if (&compiler.EmitState{}).RecordInterpXml(core.XmlTmpl{}, []core.Value{core.NewInteger(1)}, &core.Value{}, core.SrcPos{}) {
		t.Fatal("an inactive EmitState must decline RecordInterpXml")
	}
}
