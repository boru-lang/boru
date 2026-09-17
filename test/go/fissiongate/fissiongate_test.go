// fissiongate_test.go gates the type-handling code against re-fission.
//
// The type-node fusion (issue #392, design/legacy/TYPE-REPRESENTATION.1.ignore)
// established one rule: a named type IS its minted lattice node, and every
// consumer routes membership and constraint handling through the node's
// Behavior — Match/Unify keyed by core.HasConstraintUnify — never through
// per-kind probes. The spec corpus pins the resulting BEHAVIOUR per kind,
// but behaviour tests cannot stop a new special-case branch that arrives
// with its own passing test. This gate pins the ARCHITECTURE the same way
// the compiled-census ceilings pin refusals: the node-kind predicates
// (IsPredicateTypeNode, IsDisjunctTypeNode) may be CALLED from exactly the
// sites pinned below, each of which is a deliberate, design-cited
// divergence. A call anywhere else fails this test.
//
// If you are here because the test failed on a site you added: the fix is
// almost always to route through the seam instead — the node Behavior's
// Match/Unify (gated by core.HasConstraintUnify), or core.TypeContentOf
// for structure — so a named type and an inline body stay one case. Only
// if the new site is a §6-class pinned divergence (like the `is` word's
// strict-disjunct routing) does it belong in the table, with a comment
// citing the design section that sanctions it.
//
// If it failed because a pinned site DISAPPEARED: that is the ratchet
// tightening — a special case was retired. Update the table downward.
package fissiongate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kindPredicates are the node-kind probes whose call sites are ratcheted.
// core.HasConstraintUnify is deliberately NOT listed — it is the sanctioned
// router.
//
// The predicates' own defining files (core/go/unify_predicate.go,
// core/go/unify_disjunct.go) are NOT excluded: a func DECLARATION is not a
// CallExpr, so the walk below never counts one, and excluding those files
// wholesale would blind the ratchet to a consumer call added inside them —
// precisely the files most likely to grow one. They hold zero calls today,
// so they carry no pin; a call appearing there fails the gate as an
// unpinned site, exactly as it would anywhere else.
var kindPredicates = map[string]bool{
	"IsPredicateTypeNode": true,
	"IsDisjunctTypeNode":  true,
}

// pinnedKindRoutingSites is the exhaustive allowlist of consumer call
// sites, repo-relative file → number of calls (both predicates combined).
// Every entry is a deliberate divergence with a design citation.
var pinnedKindRoutingSites = map[string]int{
	// The `is` word's strict-disjunct routing: dispatch through a union
	// param stays loose on the newtype-alternative swap while `is` stays
	// strict — the DIVERGENCE PIN of design/legacy/TYPE-REPRESENTATION.1.ignore §6.
	"lang/go/native/native_type.go": 2,
	// (compiler/go/emit.go was pinned at 1 — the recorder refusing a
	// predicate-type node at a fn-invoking word "exactly as the fn value it
	// replaced". RETIRED 2026-08-28: a predicate node is not a fn value in
	// disguise, it is a bare type literal riding as data whose body runs
	// through the callback seam, so the recorder had no kind to route on once
	// that seam stopped interpreting. design/FULL-COMPILATION.0.md §6.3. This
	// is the ratchet doing its job: the pin's own failure message asked for
	// the table to be tightened.)
	// The typed-def gate splits a Function-parented constraint from a
	// predicate-type node so `def x:T v` runs the predicate rather than
	// binding the fn shape — design/legacy/TYPE-REPRESENTATION.1.ignore §6.
	"basic/go/native_definition.go": 1,
}

func TestKindPredicateCallSitesArePinned(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		t.Fatalf("repo root %s has no go.work — test moved without updating the root hop?", root)
	}

	found := map[string]int{}
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			// A file the Go toolchain itself would reject is someone
			// else's failure; the gate only counts what compiles.
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if kindPredicates[fun.Name] {
					found[rel]++
				}
			case *ast.SelectorExpr:
				if kindPredicates[fun.Sel.Name] {
					found[rel]++
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking repo: %v", walkErr)
	}

	for rel, n := range found {
		pinned, ok := pinnedKindRoutingSites[rel]
		if !ok {
			t.Errorf("kind-predicate call in UNPINNED file %s (%d call(s)): route through the node Behavior seam (Match/Unify via core.HasConstraintUnify, structure via core.TypeContentOf) instead of probing the kind — or, if this is a deliberate §6-class divergence, pin it here with a design citation", rel, n)
			continue
		}
		if n != pinned {
			t.Errorf("%s has %d kind-predicate call(s), pinned %d: a new per-kind special case re-splits what the type-node fusion unified (issue #392) — prefer the Behavior seam; update the pin only for a design-cited divergence", rel, n, pinned)
		}
	}
	for rel, pinned := range pinnedKindRoutingSites {
		if _, ok := found[rel]; !ok {
			t.Errorf("pinned site %s (%d call(s)) no longer calls a kind predicate — if the special case was retired, tighten the table (that is the ratchet working)", rel, pinned)
		}
	}
}
