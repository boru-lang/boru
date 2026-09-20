package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// closureMode answers "how much of this package comes with these symbols?"
//
// It is the measurement that decides whether a proposed bottom module is a
// real cut or a rename. Seeds are named on the command line; the closure
// follows types.Info.Uses and enforces the two rules that make a cut legal
// Go — a named type drags its whole method set (a method must live in its
// receiver's package), and a bottom module may not import upward, so every
// object reachable from a moved declaration moves with it.
//
// A file is ROOT when every top-level declaration in it is in the closure,
// CORE when none is, and SPLIT otherwise. SPLIT files are the work list:
// each one needs a head/tail regroup before the physical move.
//
// -cut names declarations whose uses are NOT followed. That models a seam:
// "this type switch becomes a capability-interface call, so its body stops
// naming the concrete arms". Comparing a run with and without a -cut set
// measures what the seam would actually buy, before anyone writes it.
type closureDecl struct {
	obj  types.Object
	file string
	kind string // func | method | type | var | const
	recv types.Object
	uses map[types.Object]bool
	loc  int
}

func closureMode(dir string, seedNames, cutNames []string) error {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles,
		Dir:   dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return fmt.Errorf("load %s: %w", dir, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fmt.Errorf("load %s: package has type errors", dir)
	}
	pkg := pkgs[0]
	fset := pkg.Fset
	fileOf := func(pos token.Pos) string { return filepath.Base(fset.Position(pos).Filename) }

	cut := map[string]bool{}
	for _, n := range cutNames {
		if n != "" {
			cut[n] = true
		}
	}

	decls, byFile := indexDecls(pkg, fileOf)
	methodsOf := map[types.Object][]*closureDecl{}
	for _, d := range decls {
		if d.recv != nil {
			methodsOf[d.recv] = append(methodsOf[d.recv], d)
		}
	}

	seeds, err := resolveSeeds(seedNames, decls)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}

	in := closureOf(seeds, decls, methodsOf, cut)

	type fileSplit struct {
		name          string
		inN, outN     int
		inLoc, outLoc int
		piece         string
		foreign       []string
	}
	var files []*fileSplit
	var inLoc, outLoc int
	for name, ds := range byFile {
		fs := &fileSplit{name: name}
		for _, d := range ds {
			if in[d.obj] {
				fs.inN++
				fs.inLoc += d.loc
			} else {
				fs.outN++
				fs.outLoc += d.loc
			}
			// A method whose receiver type is declared in another file
			// cannot move without it: Go keeps a method set in one package.
			if d.recv != nil {
				if rd := decls[d.recv]; rd != nil && rd.file != d.file {
					fs.foreign = append(fs.foreign,
						fmt.Sprintf("%s.%s", d.recv.Name(), d.obj.Name()))
				}
			}
		}
		switch {
		case fs.outN == 0:
			fs.piece = "in"
		case fs.inN == 0:
			fs.piece = "out"
		default:
			fs.piece = "SPLIT"
		}
		inLoc += fs.inLoc
		outLoc += fs.outLoc
		files = append(files, fs)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].inLoc > files[j].inLoc })

	var nIn, nOut, nSplit int
	for _, f := range files {
		switch f.piece {
		case "in":
			nIn++
		case "out":
			nOut++
		default:
			nSplit++
		}
	}
	total := inLoc + outLoc
	share := 0.0
	if total > 0 {
		share = 100 * float64(inLoc) / float64(total)
	}

	fmt.Printf("seeds: %s\n", strings.Join(seedNames, ", "))
	if len(cutNames) > 0 {
		fmt.Printf("cut:   %s\n", strings.Join(cutNames, ", "))
	}
	fmt.Printf("declarations: %d in closure / %d total\n", len(in), len(decls))
	fmt.Printf("files: %d in / %d out / %d SPLIT\n", nIn, nOut, nSplit)
	fmt.Printf("decl-LOC: %d in / %d out (closure holds %.1f%%)\n\n", inLoc, outLoc, share)

	fmt.Println("SPLIT files — each needs a regroup before the cut:")
	for _, f := range files {
		if f.piece != "SPLIT" {
			continue
		}
		fmt.Printf("  %-34s in %3d decls/%5d loc | out %3d decls/%5d loc\n",
			f.name, f.inN, f.inLoc, f.outN, f.outLoc)
	}

	fmt.Println("\nfiles with a method on a type declared elsewhere (cannot move alone):")
	for _, f := range files {
		if len(f.foreign) == 0 {
			continue
		}
		show := f.foreign
		if len(show) > 3 {
			show = show[:3]
		}
		fmt.Printf("  %-34s %s\n", f.name, strings.Join(show, ", "))
	}
	return nil
}

// indexDecls records every top-level declaration with its file, its length
// in lines, and the package-local objects its body references.
func indexDecls(pkg *packages.Package, fileOf func(token.Pos) string) (map[types.Object]*closureDecl, map[string][]*closureDecl) {
	decls := map[types.Object]*closureDecl{}
	byFile := map[string][]*closureDecl{}
	fset := pkg.Fset

	add := func(obj types.Object, kind string, recv types.Object, n ast.Node) {
		if obj == nil {
			return
		}
		start, end := fset.Position(n.Pos()), fset.Position(n.End())
		d := &closureDecl{
			obj:  obj,
			file: fileOf(n.Pos()),
			kind: kind,
			recv: recv,
			uses: map[types.Object]bool{},
			loc:  end.Line - start.Line + 1,
		}
		ast.Inspect(n, func(nd ast.Node) bool {
			id, ok := nd.(*ast.Ident)
			if !ok {
				return true
			}
			if o := pkg.TypesInfo.Uses[id]; o != nil && o.Pkg() == pkg.Types {
				d.uses[o] = true
			}
			return true
		})
		decls[obj] = d
		byFile[d.file] = append(byFile[d.file], d)
	}

	for _, f := range pkg.Syntax {
		for _, di := range f.Decls {
			switch g := di.(type) {
			case *ast.FuncDecl:
				kind, recv := "func", types.Object(nil)
				if g.Recv != nil && len(g.Recv.List) > 0 {
					kind, recv = "method", receiverType(pkg.TypesInfo, g.Recv.List[0].Type)
				}
				add(pkg.TypesInfo.Defs[g.Name], kind, recv, g)
			case *ast.GenDecl:
				for _, s := range g.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						add(pkg.TypesInfo.Defs[sp.Name], "type", nil, sp)
					case *ast.ValueSpec:
						kind := "var"
						if g.Tok == token.CONST {
							kind = "const"
						}
						for _, nm := range sp.Names {
							add(pkg.TypesInfo.Defs[nm], kind, nil, sp)
						}
					}
				}
			}
		}
	}
	return decls, byFile
}

// qualifiedName is the unambiguous spelling of a declaration: `Recv.Method`
// for a method, the bare name for everything else. It is the same spelling the
// SPLIT report already uses for a method stranded from its receiver's file.
func qualifiedName(d *closureDecl) string {
	if d.recv != nil {
		return d.recv.Name() + "." + d.obj.Name()
	}
	return d.obj.Name()
}

// resolveSeeds turns command-line seed names into the objects they denote.
//
// A bare name is NOT always unique in a package: a method name is scoped to
// its receiver, so `core` alone declares String on five types and Match on
// twenty-two. Resolving such a name by scanning the decl map and taking the
// first hit picks a different one run to run — Go randomises map iteration —
// so the same command would price a different cut each time it ran, silently.
//
// So an ambiguous seed is an ERROR naming its candidates, and `Recv.Method`
// is accepted as the disambiguated form. Measurement that quietly changes its
// mind is worse than measurement that declines.
func resolveSeeds(seedNames []string, decls map[types.Object]*closureDecl) ([]types.Object, error) {
	matches := map[string][]*closureDecl{}
	for _, d := range decls {
		matches[d.obj.Name()] = append(matches[d.obj.Name()], d)
		if q := qualifiedName(d); q != d.obj.Name() {
			matches[q] = append(matches[q], d)
		}
	}

	var seeds []types.Object
	var missing, ambiguous []string
	seen := map[types.Object]bool{}
	for _, n := range seedNames {
		if n == "" {
			continue
		}
		switch ds := matches[n]; {
		case len(ds) == 0:
			missing = append(missing, n)
		case len(ds) > 1:
			cands := make([]string, 0, len(ds))
			for _, d := range ds {
				cands = append(cands, qualifiedName(d)+" ("+d.file+")")
			}
			sort.Strings(cands)
			ambiguous = append(ambiguous,
				fmt.Sprintf("%s matches %d declarations: %s",
					n, len(ds), strings.Join(cands, ", ")))
		default:
			if !seen[ds[0].obj] {
				seen[ds[0].obj] = true
				seeds = append(seeds, ds[0].obj)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("seed(s) not declared: %s", strings.Join(missing, ", "))
	}
	if len(ambiguous) > 0 {
		sort.Strings(ambiguous)
		return nil, fmt.Errorf("ambiguous seed(s), qualify as Recv.Method:\n  %s",
			strings.Join(ambiguous, "\n  "))
	}
	// Map iteration gave no order; the closure is a fixpoint either way, but a
	// stable seed list keeps any future order-sensitive reporting honest.
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].Name() < seeds[j].Name() })
	return seeds, nil
}

// closureOf walks uses from the seeds, dragging each named type's whole
// method set and each method's receiver, and stops at any -cut declaration.
func closureOf(seeds []types.Object, decls map[types.Object]*closureDecl,
	methodsOf map[types.Object][]*closureDecl, cut map[string]bool) map[types.Object]bool {

	in := map[types.Object]bool{}
	var queue []types.Object
	for _, o := range seeds {
		if _, ok := decls[o]; ok && !in[o] {
			in[o] = true
			queue = append(queue, o)
		}
	}
	for len(queue) > 0 {
		o := queue[0]
		queue = queue[1:]
		d := decls[o]
		if d == nil {
			continue
		}
		push := func(n types.Object) {
			if n == nil || in[n] {
				return
			}
			if _, ok := decls[n]; !ok {
				return
			}
			in[n] = true
			queue = append(queue, n)
		}
		if !cut[d.obj.Name()] {
			for u := range d.uses {
				push(u)
			}
		}
		if d.kind == "type" {
			for _, m := range methodsOf[o] {
				push(m.obj)
			}
		}
		if d.recv != nil {
			push(d.recv)
		}
	}
	return in
}

// receiverType unwraps *T, T[P] and *T[P] to the named receiver object.
func receiverType(info *types.Info, e ast.Expr) types.Object {
	switch t := e.(type) {
	case *ast.StarExpr:
		return receiverType(info, t.X)
	case *ast.IndexExpr:
		return receiverType(info, t.X)
	case *ast.IndexListExpr:
		return receiverType(info, t.X)
	case *ast.Ident:
		return info.Uses[t]
	}
	return nil
}
