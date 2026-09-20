// The sentinel gate — every meaning a nil or empty field carries is read in
// ONE named predicate.
//
// A fn value's Registry, a compiled unit's Reg, a registry's ModuleRef and
// home, and a fn's Name read against its Anonymous flag each encode a meaning
// beyond the field's own type: "Go-built, no home", "runs on the running
// registry", "not a module", "its own canonical registry", "a def-bound
// verbose fn rather than a closure literal". NUR152 is what one of those
// readings cost when it was spelled at the site instead of named — every
// reader had its own idea of what a nil Registry meant, and the ideas
// disagreed. This gate keeps each reading in the predicate that owns it
// (FnDefInfo.HasHome / FnHomeLookup / FnHomeForeign, Registry.IsModule /
// Home / SameHome, FnDefInfo.NamedDef, DefCleanupInfo.FrameOn, the VM's
// dispatchRegistry): production Go compares none of these fields to nil, to
// "", or to another registry anywhere else.
//
// A predicate's own defining line carries a `//sentinel:home <reason>` marker,
// and the marker count is pinned (sentinelHomes) so a new reading is a
// deliberate change to this file, never a drive-by. The Engine's Registry
// (`e.Registry`) is a seam default — nil is "no registry bound", a designed
// meaning the kernel's other seams share — and is out of scope, as are the
// idiomatic `err != nil` and the local defensive `reg == nil` guards.
package sentinelgate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sentinelHomes is the number of `//sentinel:home` markers in production Go:
// one per predicate that reads a sentinel. Pinned in BOTH directions — a new
// marker is a new reading and must be argued here; a lost one means a
// predicate's compare moved out of its home.
const sentinelHomes = 5 // HasHome, NamedDef, IsModule, Home, FrameOn — all in core; the VM's dispatchRegistry compares its ARGUMENT, not a field, so it needs none

// sentinelFields are the selector names whose nil / "" / pointer compares the
// gate forbids outside a marked home.
var sentinelFields = map[string]bool{
	"Registry":  true,
	"Reg":       true,
	"ModuleRef": true,
	"home":      true,
}

// finding is one forbidden compare: where, and the source line.
type finding struct {
	pos  string
	line string
}

// scanFile parses one production Go file and reports its forbidden compares
// and its marker count.
func scanFile(t *testing.T, root, path string) ([]finding, int) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	lines := strings.Split(string(src), "\n")
	homes := map[int]bool{}
	markers := 0
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "//sentinel:home ") {
				homes[fset.Position(c.Pos()).Line] = true
				markers++
			}
		}
	}
	rel, _ := filepath.Rel(root, path)
	var out []finding
	report := func(n ast.Node) {
		line := fset.Position(n.Pos()).Line
		if homes[line] {
			return
		}
		out = append(out, finding{pos: filepath.ToSlash(rel) + ":" + itoa(line), line: strings.TrimSpace(lines[line-1])})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch be.Op {
		case token.EQL, token.NEQ:
			if sentinelCompare(be.X, be.Y) || sentinelCompare(be.Y, be.X) {
				report(be)
			}
		case token.LAND, token.LOR:
			// Name and Anonymous read together in one condition: the compound
			// closure-literal reading NamedDef owns. Flatten the && / || tree
			// and look for both leaves.
			leaves := flatten(be)
			if hasAnonymousLeaf(leaves) && hasNameEmptyLeaf(leaves) {
				report(be)
				return false // the nested binary exprs are the same finding
			}
		}
		return true
	})
	return out, markers
}

// sentinelCompare reports whether x is a sentinel field selector compared
// against y, where y is nil, "", or another registry-valued expression. The
// Engine's own registry (`e.Registry`) is the seam default and is skipped.
func sentinelCompare(x, y ast.Expr) bool {
	sel, ok := x.(*ast.SelectorExpr)
	if !ok || !sentinelFields[sel.Sel.Name] {
		return false
	}
	if id, isIdent := sel.X.(*ast.Ident); isIdent && id.Name == "e" && sel.Sel.Name == "Registry" {
		return false
	}
	switch y := y.(type) {
	case *ast.Ident:
		return true // nil, or a registry variable
	case *ast.BasicLit:
		return y.Kind == token.STRING && y.Value == `""`
	case *ast.SelectorExpr:
		return true // another registry field
	}
	return false
}

func flatten(e ast.Expr) []ast.Expr {
	if p, ok := e.(*ast.ParenExpr); ok {
		return flatten(p.X)
	}
	if be, ok := e.(*ast.BinaryExpr); ok && (be.Op == token.LAND || be.Op == token.LOR) {
		return append(flatten(be.X), flatten(be.Y)...)
	}
	return []ast.Expr{e}
}

func hasAnonymousLeaf(leaves []ast.Expr) bool {
	for _, l := range leaves {
		if u, ok := l.(*ast.UnaryExpr); ok && u.Op == token.NOT {
			l = u.X
		}
		if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "Anonymous" {
			return true
		}
	}
	return false
}

func hasNameEmptyLeaf(leaves []ast.Expr) bool {
	for _, l := range leaves {
		be, ok := l.(*ast.BinaryExpr)
		if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
			continue
		}
		for _, side := range [][2]ast.Expr{{be.X, be.Y}, {be.Y, be.X}} {
			sel, isSel := side[0].(*ast.SelectorExpr)
			lit, isLit := side[1].(*ast.BasicLit)
			if isSel && isLit && sel.Sel.Name == "Name" && lit.Value == `""` {
				return true
			}
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// productionGoFiles walks the repository's Go modules for non-test sources,
// skipping the same directories the compile failure-site census does (agent
// worktrees under .claude in particular, which would count every home twice).
func productionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "vendor", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestSentinelGate(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	var findings []finding
	markers := 0
	for _, path := range productionGoFiles(t, root) {
		fs, m := scanFile(t, root, path)
		findings = append(findings, fs...)
		markers += m
	}
	for _, f := range findings {
		t.Errorf("%s: sentinel compared at the site, not through its predicate: %s", f.pos, f.line)
	}
	if markers != sentinelHomes {
		t.Errorf("%d //sentinel:home markers in production Go, ceiling pins %d: a new reading is a new predicate and is argued in this file; a lost one moved a compare out of its home", markers, sentinelHomes)
	}
}

// The scanner itself, on fixtures: each forbidden shape is reported, each
// allowed shape is not, and a marked home is exempt.
func TestSentinelScanner(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bad := write("m/go/bad.go", `package m

func f(fd *FnDefInfo, u *Unit, r, o *Registry, ok bool) bool {
	a := fd.Registry == nil
	b := nil != fd.Registry
	c := u.Reg != r
	d := r.ModuleRef == ""
	e := r.home != nil
	g := fd.Registry == o.Registry
	h := ok || (fd.Anonymous || fd.Name == "")
	i := fd.Name != "" && !fd.Anonymous
	return a || b || c || d || e || g || h || i
}
`)
	findings, markers := scanFile(t, root, bad)
	if markers != 0 || len(findings) != 8 {
		t.Fatalf("every forbidden shape is one finding: %d markers, findings %+v", markers, findings)
	}
	for i, want := range []string{"fd.Registry == nil", "nil != fd.Registry", "u.Reg != r", `r.ModuleRef == ""`, "r.home != nil", "fd.Registry == o.Registry", `fd.Anonymous || fd.Name == ""`, `fd.Name != "" && !fd.Anonymous`} {
		if !strings.Contains(findings[i].line, want) {
			t.Errorf("finding %d = %q, want it to contain %q", i, findings[i].line, want)
		}
	}
	if !strings.HasPrefix(findings[0].pos, "m/go/bad.go:4") {
		t.Errorf("a finding is positioned at its line: %q", findings[0].pos)
	}
	good := write("m/go/good.go", `package m

type e struct{ Registry *Registry }

func g(en *Engine, fd *FnDefInfo, r *Registry, err error, name string) bool {
	if e.Registry == nil { // the Engine's seam default
		return false
	}
	a := fd.Registry != nil //sentinel:home the predicate that owns the reading
	b := err != nil
	c := r.ModuleRef == "x"
	d := fd.Name == "" || fd.Macro
	f := fd.Anonymous || name == ""
	h := fd.Registry.SameHome(r)
	return a || b || c || d || f || h
}
`)
	findings, markers = scanFile(t, root, good)
	if markers != 1 || len(findings) != 0 {
		t.Errorf("a marked home, an error compare, a non-sentinel compare and a lone Name read pass: %d markers, findings %+v", markers, findings)
	}
	if _, isTest := interface{}(itoa(120)).(string); !isTest || itoa(120) != "120" || itoa(0) != "0" {
		t.Errorf("itoa renders line numbers: %q %q", itoa(120), itoa(0))
	}
}
