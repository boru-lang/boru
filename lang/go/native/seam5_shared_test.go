package native

import (
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Shared helpers for the *_seam5_test.go coverage suites
// (design/TEST-SEAMS.10.md). Every seam5 test file drives handlers
// either directly (crafted args) or through these three entry points.

// seam5Reg returns a fresh default registry with the parser installed.
func seam5Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	return r
}

// seam5Run parses and runs src on r, returning the residual stack.
func seam5Run(r *Registry, src string) ([]Value, error) {
	toks, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	r.Source = src
	e := NewTop(r)
	e.SetSource(src)
	return e.Run(toks)
}

// seam5Check runs src in plain check mode (static analysis pass, no
// emission recorder), mirroring lang.(*Boru).Check's setup.
func seam5Check(r *Registry, src string) ([]Value, error) {
	toks, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	r.Source = src
	defer r.Check.Begin()()
	ResetModuleExportGrowth(r)
	ResetCheckFnCarrierBinds(r)
	e := NewTop(r)
	e.SetSource(src)
	return e.Run(toks)
}
