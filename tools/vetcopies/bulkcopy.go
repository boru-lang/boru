package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// corePath is the import path of the package that declares Value.
const corePath = "github.com/boru-lang/boru/core/go"

// BulkCopy reports the bulk copies of Values that copylocks does not see:
// the built-in copy into a []core.Value, and an append onto one whose source
// is spread with "...". Each copies every element's header, which is what
// the policy's bulk-copy count measures (scripts/vet-copies.sh).
var BulkCopy = &analysis.Analyzer{
	Name:     "bulkcopy",
	Doc:      "report copy() and variadic append() over []core.Value",
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
			if isValueSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
				pass.Reportf(call.Pos(), "bulk copy of []core.Value: copy")
			}
		case "append":
			if call.Ellipsis.IsValid() && isValueSlice(pass.TypesInfo.TypeOf(call.Args[0])) {
				pass.Reportf(call.Pos(), "bulk copy of []core.Value: variadic append")
			}
		}
	})
	return nil, nil
}

// isValueSlice reports whether t is a slice of core.Value. Type is an alias
// of Value, so a []Type counts too, and so does a named slice type whose
// underlying type is []Value.
func isValueSlice(t types.Type) bool {
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	if !ok {
		return false
	}
	return isValue(s.Elem())
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
