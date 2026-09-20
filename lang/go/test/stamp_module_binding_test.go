package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// stampEventsFor loads src as an inline module body (module load is the
// detached-stamp trigger site) with runtime stamping armed, then returns
// the stamp events recorded for its fns.
func stampEventsFor(t *testing.T, src string) []core.StampEvent {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	reg.EnableRuntimeStamping()
	engine := native.NewTop(reg)
	vals, pErr := parser.Parse(src)
	if pErr != nil {
		t.Fatal(pErr)
	}
	if _, rErr := engine.Run(vals); rErr != nil {
		t.Fatal(rErr)
	}
	return reg.StampEvents()
}

// runModule loads modSrc (the detached-stamp trigger), then runs prog in the
// same engine and returns its result stack. With stamping=true it arms runtime
// stamping (the compiled path); with stamping=false the fns run on the
// interpreter — the two together are the end-to-end parity harness.
func runModule(t *testing.T, modSrc, prog string, stamping bool) []native.Value {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	if stamping {
		reg.EnableRuntimeStamping()
	}
	engine := native.NewTop(reg)
	for _, step := range []string{modSrc, prog} {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			t.Fatal(pErr)
		}
		out, rErr := engine.Run(vals)
		if rErr != nil {
			t.Fatal(rErr)
		}
		if step == prog {
			return out
		}
	}
	return nil // unreachable: the loop returns on the prog step by construction
}

// runStampedModule loads modSrc (the detached-stamp trigger) with runtime
// stamping armed, then runs prog in the same engine and returns its result
// stack — the end-to-end parity harness for newly stamped shapes.
func runStampedModule(t *testing.T, modSrc, prog string) []native.Value {
	t.Helper()
	return runModule(t, modSrc, prog, true)
}

func asStringVal(v native.Value) (string, error) { return core.AsString(v) }

func stampOutcome(evs []core.StampEvent, name string) (string, bool) {
	for _, ev := range evs {
		if ev.Name == name {
			if ev.Stamped {
				return "", true
			}
			return ev.Reason, false
		}
	}
	return "no stamp event", false
}

// TestStampModuleBindingValueRead pins that a stored-fn unit whose body
// passes a CONCRETE module-scope value binding as a dispatch operand
// (mini-s3's `Net.recv-until conn s3-crlf {…}` shape — the delimiter is a
// module-level `def s3-crlf (convert Bytes "\r\n")`) STAMPS, with the
// binding baked as a const guarded by the ref's dep snapshot: a later
// rebind of the name degrades the ref to the interpreter rather than
// serving the stale const (the depSnap/depsFresh machinery, pinned in
// eng/go/stamp_runtime_test.go and lang/go/stamp_runtime_vm_test.go —
// storedFnDeps collects every Defs-bound body word, including value
// bindings like crlf, so the existing freshness gates cover the newly
// baked read).
func TestStampModuleBindingValueRead(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def crlf (convert Bytes "\r\n")
  def read-modbind fn [[b:Bytes] [Any] [ size (b add crlf) ]]
  export "SockT" { read-modbind: read-modbind/v }
] import
`)
	reason, ok := stampOutcome(evs, "read-modbind")
	if !ok {
		t.Fatalf("read-modbind did not stamp: %s", reason)
	}
}

// TestStampLocalDelimStillStamps pins the sibling shapes that already
// stamped before this work (body-local and param-supplied values) so the
// module-binding extension cannot regress them.
func TestStampLocalDelimStillStamps(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def read-local fn [[b:Bytes] [Any] [ def d (convert Bytes "\r\n") size (b add d) ]]
  def read-param fn [[b:Bytes d:Bytes] [Any] [ size (b add d) ]]
  export "SockT" { read-local: read-local/v read-param: read-param/v }
] import
`)
	for _, name := range []string{"read-local", "read-param"} {
		if reason, ok := stampOutcome(evs, name); !ok {
			t.Fatalf("%s did not stamp: %s", name, reason)
		}
	}
}

// TestStampModuleBindingNotMaterialisable pins the negative: a module
// binding holding a value that cannot bake as a const (a mutable flex
// map — instance identity is shared across calls) must still decline
// rather than bake a snapshot that would diverge from the interpreter's
// live instance. The read now compiles NOT by baking (which would diverge)
// but by lowering to a runtime dyn-scope lookup (OpLookupDynScope) that
// re-resolves the ONE shared instance per call — see the parity test below.
func TestStampModuleFlexReadStampsViaDynScope(t *testing.T) {
	evs := stampEventsFor(t, `
module [
  def shared (flex {n: 1})
  def read-flex fn [[k:String] [Any] [ shared get k ]]
  export "SockT" { read-flex: read-flex/v }
] import
`)
	// Stage-2 (detached-stamp flex delivery): a module-scope mutable-reference
	// read used to DECLINE because the only compile option was baking a diverging
	// snapshot. It now STAMPS — the read lowers to a live dyn-scope lookup, so no
	// snapshot is baked and the shared instance stays shared.
	if reason, ok := stampOutcome(evs, "read-flex"); !ok {
		t.Fatalf("read-flex did not stamp: %s", reason)
	}
}

// TestStampModuleFlexReadDynScopeParity is the load-bearing anti-divergence
// proof for the stamp above: the COMPILED (stamped) reader over a module-scope
// mutable flex must produce byte-identical results to the INTERPRETER over the
// same shared-instance mutation sequence. The read lowers to a live dyn-scope
// lookup, so both paths resolve the ONE shared cell per call; a baked snapshot
// (the old compile failure's only alternative) would freeze a stale value and diverge.
func TestStampModuleFlexReadDynScopeParity(t *testing.T) {
	modSrc := `
module [
  def shared (flex [])
  def push-flex fn [[s:String] [Integer] [ def _ (shared push {v: s})  shared size ]]
  def read-flex fn [[i:Integer] [Any] [ shared get i ]]
  export "SockT" { push-flex: push-flex/v read-flex: read-flex/v }
] import
`
	prog := `[ (SockT.push-flex "a") (SockT.push-flex "b") (SockT.read-flex 0) (SockT.read-flex 1) ]`
	compiled := runModule(t, modSrc, prog, true)
	interp := runModule(t, modSrc, prog, false)
	if len(compiled) != 1 || len(interp) != 1 {
		t.Fatalf("residual compiled=%v interp=%v, want one list each", compiled, interp)
	}
	if compiled[0].String() != interp[0].String() {
		t.Fatalf("stamped/interpreter divergence over a shared module flex:\n compiled=%s\n interp  =%s",
			compiled[0].String(), interp[0].String())
	}
}

// lexModule is a module-exported custom-parser LEXER — the exact
// voxgig-boru/template shape: a runtime-constructed parser (`def p
// (Parse.parser g)`) whose matcher accumulates tokens onto a module-scope
// `flex`, dispatched from a fn body over the fn's `src:String` param. Loading
// it as a module (with stamping armed) detached-stamps the lexer.
const lexModule = `
module [
  import "boru:parse"
  import "boru:parselang"
  def acc (flex [])
  def g Parse.grammar
  Parse.matcher g lex 5 ([s:String] => [ def _ (acc push {v:s})  {src: s tin:'#TX' val:'t'} ]) end
  Parse.rule g val { open:[{s:'#TX' p:'val'}{}] close:[{s:'#ZZ'}{}] } end
  def p (Parse.parser g) end
  def lex-it fn [ [src:String] [List] [
    def st (acc size)
    def _ (parse p src)
    slice st (acc size) acc
  ] ]
  export "Lex" { lex: lex-it/v }
] import
`

// TestStampModuleParserLexerStamps pins Stage-2 parser delivery into a
// detached stamp: a module-exported lexer that dispatches `parse p src` over a
// RUNTIME-constructed parser `p` (concrete under the stamp) and a param-carrier
// source STAMPS. The concrete parser's direct dispatch is opaque, so
// parseFnExpand reroutes the non-concrete-source form through the compilable
// parselang-fn-dispatch CALL_NATIVE (the same seam the non-concrete-parser
// form uses).
func TestStampModuleParserLexerStamps(t *testing.T) {
	evs := stampEventsFor(t, lexModule)
	if reason, ok := stampOutcome(evs, "lex-it"); !ok {
		t.Fatalf("lex-it did not stamp: %s", reason)
	}
}

// TestStampModuleParserLexerParity is the anti-divergence proof for the lexer
// stamp: the compiled (stamped) lexer must produce byte-identical tokens to the
// interpreter over the same input — the parselang-fn-dispatch handler replays
// the identical `<parser> <src> {} end` tail, and the module `acc` flex is read
// live per call.
func TestStampModuleParserLexerParity(t *testing.T) {
	prog := `(Lex.lex "hello")`
	compiled := runModule(t, lexModule, prog, true)
	interp := runModule(t, lexModule, prog, false)
	if len(compiled) != 1 || len(interp) != 1 {
		t.Fatalf("residual compiled=%v interp=%v, want one token list each", compiled, interp)
	}
	if compiled[0].String() != interp[0].String() {
		t.Fatalf("stamped/interpreter lexer divergence:\n compiled=%s\n interp  =%s",
			compiled[0].String(), interp[0].String())
	}
}
