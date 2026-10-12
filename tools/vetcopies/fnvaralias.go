package main

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// FnVarAlias reports a package-level variable that aliases a function taking
// a *core.Value (`var IsWord = core.IsWord`). A call through a function
// variable is indirect: it cannot inline, and escape analysis cannot see the
// callee, so every &v handed to it moves v to the heap. Re-export such a
// function as a wrapper declaration instead (lang/go/native/aliases.go).
var FnVarAlias = &analysis.Analyzer{
	Name: "fnvaralias",
	Doc:  "report function variables that alias a function taking a *core.Value",
	Run:  runFnVarAlias,
}

func runFnVarAlias(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for _, val := range vs.Values {
					var id *ast.Ident
					switch x := ast.Unparen(val).(type) {
					case *ast.Ident:
						id = x
					case *ast.SelectorExpr:
						id = x.Sel
					default:
						continue
					}
					fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
					if !ok || !takesValuePtr(fn.Type().(*types.Signature)) {
						continue
					}
					pass.Reportf(val.Pos(), "function variable aliases %s, which takes a *core.Value: declare a wrapper func", fn.Name())
				}
			}
		}
	}
	return nil, nil
}

// takesValuePtr reports whether a parameter of sig is spelled *core.Value.
// A *core.Type parameter is the same type under its alias, but it carries a
// lattice node — a canonical pointer that already lives on the heap — so a
// call through a variable moves nothing there.
func takesValuePtr(sig *types.Signature) bool {
	for i := 0; i < sig.Params().Len(); i++ {
		ptr, ok := sig.Params().At(i).Type().(*types.Pointer)
		if !ok || !isValue(ptr.Elem()) {
			continue
		}
		if a, ok := ptr.Elem().(*types.Alias); ok && a.Obj().Name() == "Type" {
			continue
		}
		return true
	}
	return false
}
