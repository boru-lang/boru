package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// corePath is the import path of the package that declares Value and Datum.
const corePath = "github.com/boru-lang/boru/core/go"

// BulkCopy reports the bulk copies of Datums that copylocks does not see:
// the built-in copy into a []core.Datum, and an append onto one whose source
// is spread with "...". Each copies every element's datum, which is what
// the policy's bulk-copy count measures (scripts/vet-copies.sh). A
// []core.Value is an array of handles — references and their holders'
// occurrence state — so copying one copies no datum (the exceptions list's
// C1 and R4) and is not reported.
var BulkCopy = &analysis.Analyzer{
	Name:     "bulkcopy",
	Doc:      "report copy() and variadic append() over []core.Datum",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runBulkCopy,
}

func runBulkCopy(pass *analysis.Pass) (any, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	ins.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		id, ok := ast.Unparen(call.Fun).(*ast.Ident)
		if !ok {
			return
		}
		b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
		if !ok {
			return
		}
		// A type-checked copy or append always has its destination.
		switch b.Name() {
		case "copy":
			if isDatumSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
				pass.Reportf(call.Pos(), "bulk copy of []core.Datum: copy")
			}
		case "append":
			if call.Ellipsis.IsValid() && isDatumSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
				pass.Reportf(call.Pos(), "bulk copy of []core.Datum: variadic append")
			}
		}
	})
	return nil, nil
}

// isDatumSlice reports whether t is a slice of core.Datum, or a named slice
// type whose underlying type is []Datum.
func isDatumSlice(t types.Type) bool {
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	return isDatum(s.Elem())
}

// isDatum reports whether t is core.Datum.
func isDatum(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Datum" && obj.Pkg() != nil && obj.Pkg().Path() == corePath
}

// isValue reports whether t is core.Value (Type is an alias of it).
func isValue(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Value" && obj.Pkg() != nil && obj.Pkg().Path() == corePath
}
