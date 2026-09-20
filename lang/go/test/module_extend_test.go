package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// File-module battery for open words (design/OPEN-WORDS.0.md): the
// export-transplant behaviours that need REAL module files — stable
// module refs for diamond/idempotence and conflict provenance — rather
// than the inline modules lang/spec/open-words.tsv uses (each inline
// module is a distinct instance, so it can never share a ref). Runs on
// the in-memory FS via runMemFSModuleSteps (module_memfs_test.go).

// flagExtModule is the canonical extending module: a minted user type
// (the module-scope core-word rule requires one), a merged `add`
// overload whose body uses a module-PRIVATE helper, a constructor, and
// the exports that carry all of it to the importer.
const flagExtModule = `def Flag (refine Boolean)
def flip fn [[b:Boolean] [Boolean] [b not]]
def add fn [[a:Flag b:Flag] [Boolean] [flip ((flip a) and (flip b))]]
def mk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "FlagExt" {add: add/v mk: mk/v Flag: Flag}`

// TestModuleExtendFileTransplant pins the positive transplant through a
// real file: the importer's bare `add` gains the [Flag Flag] overload,
// the module-private helper (`flip`) resolves when the body runs at the
// importer (module-closure execution), and the locked native signatures
// are untouched.
func TestModuleExtendFileTransplant(t *testing.T) {
	files := map[string]string{"ext.boru": flagExtModule}
	// flip(flip a AND flip b) == a OR b.
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.boru"`,
		`add (FlagExt.mk true) (FlagExt.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")

	result, err = runMemFSModuleSteps(t, files, []string{
		`import "./ext.boru"`,
		`add 40 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")
}

// TestModuleExtendFileDiamond pins diamond-import behaviour: importing
// the same module file twice neither errors nor breaks dispatch. A
// re-run of the module body re-mints its user types (there is no
// module cache), so the second import's tuple is anchored to a fresh
// mint and coexists with the first; the rebound namespace's
// constructor produces values that dispatch the second instance's
// signature.
func TestModuleExtendFileDiamond(t *testing.T) {
	files := map[string]string{"ext.boru": flagExtModule}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.boru"`,
		`import "./ext.boru"`,
		`add (FlagExt.mk true) (FlagExt.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendTransplantConflictDirect pins the §4.4 collision
// machinery at the API level: the same unlocked tuple transplanted
// from two DIFFERENT module refs raises [boru/extend_conflict], while
// the same ref re-arriving is idempotent and quiet. Unreachable from
// boru source today — a user-typed tuple is anchored to a per-import
// mint, so two source modules can never build the identical tuple —
// but the guard must hold for a future module cache (shared mints
// across importers) and for host-constructed clones.
func TestModuleExtendTransplantConflictDirect(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)

	// Build a real word-extension clone by running a top-level merge
	// (unrestricted scope), then pop it so the registry is back to the
	// pristine base — the clone value is the transplant payload.
	src := `def Money (refine Integer)  def add fn [[a:Money b:Money] [Boolean] [true]]`
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.New(reg).Run(vals); err != nil {
		t.Fatal(err)
	}
	top, ok := reg.Defs.Top("add")
	if !ok {
		t.Fatal("add unbound after merge")
	}
	clone, ok := native.IsWordExtension(top)
	if !ok {
		t.Fatal("top of add is not a word-extension clone")
	}
	native.UninstallDef(reg, "add")

	if err := native.TransplantExtension(reg, clone, "./mod-a.boru", native.OwnerProgram); err != nil {
		t.Fatalf("first transplant: %v", err)
	}
	if err := native.TransplantExtension(reg, clone, "./mod-a.boru", native.OwnerProgram); err != nil {
		t.Fatalf("diamond re-transplant (same ref) must be idempotent, got %v", err)
	}
	err = native.TransplantExtension(reg, clone, "./mod-b.boru", native.OwnerProgram)
	if err == nil {
		t.Fatal("expected [boru/extend_conflict] for the same tuple from a different module, got no error")
	}
	if !strings.Contains(err.Error(), "extend_conflict") {
		t.Fatalf("expected extend_conflict, got %v", err)
	}
}

// TestModuleExtendFileOneLevel pins the one-level rule through files: a
// middle module imports the extender (receiving the transplant in ITS
// registry) but re-exports only a constructor — the program's `add`
// must NOT gain the extension, even though Flag-conforming values reach
// it (this is the firewall idiom as a file layout).
func TestModuleExtendFileOneLevel(t *testing.T) {
	files := map[string]string{
		"ext.boru": flagExtModule,
		"mid.boru": `import "./ext.boru"
def Flag FlagExt.Flag
def omk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "Mid" {mk: omk/v}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./mid.boru"`,
		`add (Mid.mk true) (Mid.mk true)`,
	})
	if err == nil {
		t.Fatal("expected an error (extension must not cross two levels), got no error")
	}
	// The [Flag Flag] extension did NOT cross, so `add flag flag` does not
	// dispatch it. Flag is a `refine Boolean`, so the call falls to the
	// core within-type default for Boolean — arithmetic is a defined error —
	// which still proves the extension is absent (it would otherwise return
	// a Flag, not raise).
	if !strings.Contains(err.Error(), "arithmetic is not defined on Boolean") {
		t.Fatalf("expected the Boolean within-type default error, got %v", err)
	}
}

// TestModuleExtendFileReExport pins opt-in transitivity through files:
// the middle module ALSO re-exports the word, so the program's `add`
// does gain the extension — and the body still resolves the innermost
// module's private helper.
func TestModuleExtendFileReExport(t *testing.T) {
	files := map[string]string{
		"ext.boru": flagExtModule,
		"mid.boru": `import "./ext.boru"
def Flag FlagExt.Flag
def omk fn [[b:Boolean] [Flag] [def v:Flag b v]]
export "Mid" {mk: omk/v add: add/v}`,
	}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./mid.boru"`,
		`add (Mid.mk true) (Mid.mk false)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendFileUserTypeRule pins the ownership-anchor rule at
// the file boundary (rev 2, design/OPEN-WORDS.1.md): a module extending
// a core word with an unanchored (all-kernel) tuple is declined with
// [boru/extend_owner] — `add 1 {}` can never start working because of
// an import.
func TestModuleExtendFileUserTypeRule(t *testing.T) {
	files := map[string]string{
		"bad.boru": `def add fn [[a:Integer b:Map] [Integer] [1]]
export "Bad" {add: add/v}`,
	}
	_, err := runMemFSModuleSteps(t, files, []string{
		`import "./bad.boru"`,
	})
	if err == nil {
		t.Fatal("expected [boru/extend_owner], got no error")
	}
	if !strings.Contains(err.Error(), "extend_owner") {
		t.Fatalf("expected extend_owner, got %v", err)
	}
}

// TestModuleExtendPointClass pins the canonical user-module shape end
// to end: a boru file module that mints a CLASS type Point {x,y},
// merges a [Point Point] overload into core `add` (its own class is
// the user type the module-scope rule requires), and exports both.
// The importer constructs Points through the exported type with
// `make`, and `p0 add p1` dispatches the transplanted overload —
// whose body runs in MODULE scope (it references the module's own
// `Point` binding via `make Point {…}`).
func TestModuleExtendPointClass(t *testing.T) {
	files := map[string]string{
		"pointer.boru": `def Point class {x:Integer y:Integer}
def add fn [[a:Point b:Point] [Point] [make Point {x:(a.x add b.x) y:(a.y add b.y)}]]
export "Pointer" {Point: Point add: add/v}`,
	}
	steps := []string{
		`import "./pointer.boru"`,
		`def p0 (make Pointer.Point {x:1 y:2})`,
		`def p1 (make Pointer.Point {x:4 y:6})`,
		`def p2 (p0 add p1)`,
	}

	result, err := runMemFSModuleSteps(t, files, append(steps, `p2.x`))
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "5")

	result, err = runMemFSModuleSteps(t, files, append(steps, `p2.y`))
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "8")

	result, err = runMemFSModuleSteps(t, files, append(steps, `p2 is Pointer.Point`))
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")

	result, err = runMemFSModuleSteps(t, files, append(steps, `typeof p2`))
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "Point")

	// Paired negative: numeric add is untouched, and a Point plus a
	// number is still a signature error — the extension is anchored to
	// the module's class on BOTH operands.
	result, err = runMemFSModuleSteps(t, files, append(steps, `add 40 2`))
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "42")

	_, err = runMemFSModuleSteps(t, files, append(steps, `add p0 1`))
	if err == nil {
		t.Fatal("add Point Integer: expected signature_error, got none")
	}
	if !strings.Contains(err.Error(), "signature_error") {
		t.Fatalf("add Point Integer: expected signature_error, got %v", err)
	}
}

// TestModuleExtendFileUndefReimport pins retraction + re-import: undef
// pops the transplant (base word intact), and importing the module
// again re-installs a working extension.
func TestModuleExtendFileUndefReimport(t *testing.T) {
	files := map[string]string{"ext.boru": flagExtModule}
	result, err := runMemFSModuleSteps(t, files, []string{
		`import "./ext.boru"`,
		`undef add`,
		`add 40 2`,
		`import "./ext.boru"`,
		`add (FlagExt.mk false) (FlagExt.mk true)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertResult(t, result, "true")
}

// TestModuleExtendTransplantSealedWord pins that the transplant path
// enforces the SAME sealed-word rejection InstallWordExtension applies
// for source-level `def`: a host/native module can construct a clone
// directly (NewWordExtension) and import reaches TransplantExtension
// without passing InstallWordExtension, so without this guard a module
// could add overloads to names the engine special-cases by identity
// (def / make / word / …).
func TestModuleExtendTransplantSealedWord(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)

	for _, sealed := range []string{"def", "make", "word"} {
		ext := native.NewWordExtension(native.OwnerProgram, sealed, []native.Signature{{
			Args:       []*native.Type{native.TString},
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{args[0]}, nil
			}),
		}})
		err := native.TransplantExtension(reg, ext, "./evil.boru", "")
		if err == nil {
			t.Fatalf("transplant onto sealed word %q was not declined", sealed)
		}
		if !strings.Contains(err.Error(), "sealed") {
			t.Fatalf("transplant onto %q: expected a sealed-word compile failure, got %v", sealed, err)
		}
	}
}
