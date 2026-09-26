package native

import (
	"testing"
)

// w9CompileMode puts r into the bytecode-emission recorder mode
// (mirroring lang.(*Boru).CompileCheck) and returns the check-state
// cleanup, so recorder-decline arms can be driven by direct handler calls.
func w9CompileMode(t *testing.T, r *Registry) func() {
	t.Helper()
	cleanup := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	return cleanup
}

// W9_nativeB — coverage for native_definition.go def / undef / var / fn /
// typed-def arms. Direct handler calls drive the pure arg guards; crafted
// boru programs (run + check) drive the paths that need real dispatch.
// See design/TEST-SEAMS.10.md.

// --- defHandler guards ---

func TestW9DefHandlerInvalidWordName(t *testing.T) {
	r := seam5Reg(t)
	// "1x" is lowercase (not a type) but not a valid word name.
	if _, err := defHandler([]Value{NewString("1x"), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def with invalid word name must error")
	}
}

func TestW9DefHandlerNameClashWithType(t *testing.T) {
	r := seam5Reg(t)
	// Bind a lowercase name as a TYPE, then def the same lowercase name.
	r.Defs.PushType("foo", TInteger, NewTypeLiteral(TInteger))
	if _, err := defHandler([]Value{NewWord("foo"), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def of a name already bound as a type must error")
	}
}

// --- defTypedHandler guards ---

func TestW9DefTypedHandlerEmptyNameMap(t *testing.T) {
	r := seam5Reg(t)
	if _, err := defTypedHandler([]Value{NewMap(NewOrderedMap()), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def typed: empty name map must error")
	}
}

func TestW9DefTypedHandlerInvalidWordName(t *testing.T) {
	r := seam5Reg(t)
	om := NewOrderedMap()
	om.Set("1bad", NewTypeLiteral(TInteger))
	if _, err := defTypedHandler([]Value{NewMap(om), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def typed: invalid word name must error")
	}
}

func TestW9DefTypedHandlerBuiltinName(t *testing.T) {
	r := seam5Reg(t)
	om := NewOrderedMap()
	om.Set("add", NewTypeLiteral(TInteger))
	if _, err := defTypedHandler([]Value{NewMap(om), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def typed: builtin name must error")
	}
}

func TestW9DefTypedHandlerNameClashWithType(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.PushType("foo", TInteger, NewTypeLiteral(TInteger))
	om := NewOrderedMap()
	om.Set("foo", NewTypeLiteral(TInteger))
	if _, err := defTypedHandler([]Value{NewMap(om), NewInteger(1)}, nil, nil, r); err == nil {
		t.Fatal("def typed: name already a type must error")
	}
}

func TestW9DefTypedParenAnnotationError(t *testing.T) {
	// The parenthesised annotation runs and errors on an undefined word.
	if _, err := seam5Run(seam5Reg(t), `def x:(zzznope) 5`); err == nil {
		t.Fatal("def x:(undefined) must error in the annotation")
	}
}

func TestW9DefTypedParenAnnotationMultiValue(t *testing.T) {
	// The annotation paren must produce exactly one type value.
	if _, err := seam5Run(seam5Reg(t), `def x:(1 2) 5`); err == nil {
		t.Fatal("def x:(1 2) must error (annotation produced two values)")
	}
}

func TestW9DefTypedSchemaInferenceError(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def Box gen [T] refine Record [value:T]`); err != nil {
		t.Fatalf("def Box: %v", err)
	}
	// An empty body can't infer T → InferAndInstantiateSchema errors.
	if _, err := seam5Run(r, `def b:Box {}`); err == nil {
		t.Fatal("def b:Box {} must error (uninferable type parameter)")
	}
}

func TestW9DefTypedResourceInstancePassThrough(t *testing.T) {
	r := seam5Reg(t)
	// A pre-made resource instance whose nominal type matches the
	// annotation is accepted (the IsResourceInstance branch).
	src := `def e (make Entity {spec:"s" entity:"e" kind:"k"})  def e2:Entity e  e2`
	out, err := seam5Run(r, src)
	if err != nil {
		t.Fatalf("resource instance pass-through: %v", err)
	}
	if len(out) != 1 || !IsResourceInstance(out[0]) {
		t.Fatalf("expected a resource instance, got %v", Canon(out))
	}
}

// --- resolveResourceTypeInfo / lookupResourceTypeByName ---

func TestW9ResolveResourceTypeInfoDirect(t *testing.T) {
	r := seam5Reg(t)
	entity, ok := r.Defs.Top(resourceDefKey("Entity"))
	if !ok {
		t.Fatal("Entity schema not bound under the hidden key")
	}
	// A ResourceType value resolves directly (the IsResourceType arm).
	if _, isRes := resolveResourceTypeInfo(r, entity); !isRes {
		t.Fatal("Entity value must resolve as a ResourceType")
	}
	// Negative pair: a plain Integer does not.
	if _, isRes := resolveResourceTypeInfo(r, NewInteger(1)); isRes {
		t.Fatal("Integer must not resolve as a ResourceType")
	}
}

func TestW9LookupResourceTypeByNameEmpty(t *testing.T) {
	r := seam5Reg(t)
	if _, ok := lookupResourceTypeByName(r, ""); ok {
		t.Fatal("empty name must not resolve a ResourceType")
	}
}

func TestW9DefTypedResourceNonMutableMapBody(t *testing.T) {
	r := seam5Reg(t)
	// A bare Entity type literal is what `def e:Entity …` parses to; it
	// resolves as a Resource via lookupResourceTypeByName. A map-typed but
	// non-OrderedMap body (a carrier) then fails AsMutableMap.
	om := NewOrderedMap()
	om.Set("e", NewTypeLiteral(TResourceEntity))
	if _, err := defTypedHandler([]Value{NewMap(om), NewCarrier(TMap)}, nil, nil, r); err == nil {
		t.Fatal("def e:Entity <carrier map> must error (not a concrete map)")
	}
}

// --- compile-recorder decline arms: dynamic refine / DepScalar typed-defs ---

func TestW9DefTypedRefineDynamicUncompilable(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def Pos refine Integer`); err != nil {
		t.Fatalf("def Pos: %v", err)
	}
	cleanup := w9CompileMode(t, r)
	defer cleanup()
	if !r.Check.Emit.Active() {
		t.Fatal("recorder should start active")
	}
	// A DYNAMIC (carrier) body with no resolvable provenance: RecordTypedBind
	// declines, so the refine-bare branch marks the program uncompilable.
	om := NewOrderedMap()
	om.Set("x", NewWord("Pos"))
	if _, err := defTypedHandler([]Value{NewMap(om), NewCarrier(TInteger)}, nil, nil, r); err != nil {
		t.Fatalf("def x:Pos <carrier>: %v", err)
	}
	if r.Check.Emit.Active() {
		t.Fatal("a dynamic refine typed-def with no provenance must mark uncompilable")
	}
}

func TestW9DefTypedDepScalarDynamicUncompilable(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `def Big (Integer gt 10)`); err != nil {
		t.Fatalf("def Big: %v", err)
	}
	cleanup := w9CompileMode(t, r)
	defer cleanup()
	if !r.Check.Emit.Active() {
		t.Fatal("recorder should start active")
	}
	// A DepScalar constraint over an abstract (carrier) body conforming to the
	// base: RecordTypedBind declines, so the DepScalar branch marks uncompilable.
	om := NewOrderedMap()
	om.Set("x", NewWord("Big"))
	if _, err := defTypedHandler([]Value{NewMap(om), NewCarrier(TInteger)}, nil, nil, r); err != nil {
		t.Fatalf("def x:Big <carrier>: %v", err)
	}
	if r.Check.Emit.Active() {
		t.Fatal("a dynamic DepScalar typed-def with no provenance must mark uncompilable")
	}
}

// --- undefFnHandler / varHandler / fnHandler direct guards ---

func TestW9UndefFnHandlerNonSpec(t *testing.T) {
	r := seam5Reg(t)
	if _, err := undefFnHandler([]Value{NewAtom("x"), NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("undef fn: non-spec second arg must error")
	}
}

func TestW9VarHandlerNotAList(t *testing.T) {
	r := seam5Reg(t)
	if _, err := varHandler([]Value{NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("var: non-list arg must error")
	}
}

func TestW9VarHandlerNonConcreteList(t *testing.T) {
	r := seam5Reg(t)
	// A List CARRIER has Parent==TList (passes the is-a-list check) but is
	// not concrete → the concrete-list guard fires.
	if _, err := varHandler([]Value{NewCarrier(TList)}, nil, nil, r); err == nil {
		t.Fatal("var: non-concrete list must error")
	}
}

func TestW9VarHandlerStringNameDecl(t *testing.T) {
	r := seam5Reg(t)
	// decl = ["v" 5] — the name comes from a string.
	decl := NewList([]Value{NewString("v"), NewInteger(5)})
	decls := NewList([]Value{decl})
	outer := NewList([]Value{decls, NewWord("v")})
	if _, err := varHandler([]Value{outer}, nil, nil, r); err != nil {
		t.Fatalf("var with string-named decl: %v", err)
	}
}

func TestW9VarHandlerBadDeclName(t *testing.T) {
	r := seam5Reg(t)
	// decl = [1 2] — the name element is neither a word nor a string.
	decl := NewList([]Value{NewInteger(1), NewInteger(2)})
	decls := NewList([]Value{decl})
	outer := NewList([]Value{decls, NewWord("x")})
	if _, err := varHandler([]Value{outer}, nil, nil, r); err == nil {
		t.Fatal("var: a non-word/non-string decl name must error")
	}
}

func TestW9FnHandlerNotAList(t *testing.T) {
	r := seam5Reg(t)
	if _, err := fnHandler([]Value{NewInteger(5)}, nil, nil, r); err == nil {
		t.Fatal("fn: non-list arg must error")
	}
}

func TestW9FnHandlerNonConcreteList(t *testing.T) {
	r := seam5Reg(t)
	// A List carrier is a list but not concrete → the concrete guard fires.
	if _, err := fnHandler([]Value{NewCarrier(TList)}, nil, nil, r); err == nil {
		t.Fatal("fn: non-concrete list must error")
	}
}

func TestW9FnHandlerGenSpecFailurePopsBindings(t *testing.T) {
	// A pending gen spec + an invalid fn list triggers the failGen path.
	if _, err := seam5Run(seam5Reg(t), `def Id gen [T] fn [x:T]`); err == nil {
		t.Fatal("gen [T] fn [x:T] (length not a multiple of 3) must error")
	}
}

// --- afnHandler / fnsigHandler / args / __pa ---

func TestW9AfnHandlerBadInputSig(t *testing.T) {
	r := seam5Reg(t)
	// afn input is args[1]; a sig list with an unknown type name is rejected
	// by parseFnParams.
	body := NewList([]Value{NewWord("x")})
	inputSig := NewList([]Value{NewWord("ZZUnknownType")})
	if _, err := afnHandler([]Value{body, inputSig}, nil, nil, r); err == nil {
		t.Fatal("afn: an unresolvable input sig must error")
	}
}

func TestW9FnsigHandlerNonConcreteList(t *testing.T) {
	r := seam5Reg(t)
	if _, err := fnsigHandler([]Value{NewCarrier(TList)}, nil, nil, r); err == nil {
		t.Fatal("fnsig: non-concrete list must error")
	}
}

func TestW9FnsigHandlerBadLength(t *testing.T) {
	r := seam5Reg(t)
	// Odd length (must be a non-zero multiple of 2).
	if _, err := fnsigHandler([]Value{NewList([]Value{NewTypeLiteral(TInteger)})}, nil, nil, r); err == nil {
		t.Fatal("fnsig: odd-length list must error")
	}
}

func TestW9FnsigHandlerGenSpecErrorPops(t *testing.T) {
	// A pending gen spec + an fnsig spec that parseFnUndefSpec rejects
	// (unknown output type) triggers the gen-pop error arm.
	if _, err := seam5Run(seam5Reg(t), `def M gen [T] fnsig [[T] [zznope]]`); err == nil {
		t.Fatal("fnsig with an unknown output type must error")
	}
}

func TestW9ArgsHandlerNilStack(t *testing.T) {
	r := seam5Reg(t)
	// A nil args stack surfaces the defensive error (not a panic).
	r.Args = nil
	if _, err := argsHandler(nil, nil, nil, r); err == nil {
		t.Fatal("args: a nil args stack must error")
	}
}

func TestW9PopArgsHandlerNilStack(t *testing.T) {
	r := seam5Reg(t)
	// PopFrameArgs over a nil args stack surfaces the error.
	r.Args = nil
	if _, err := popArgsHandler(nil, nil, nil, r); err == nil {
		t.Fatal("__pa: a nil args stack must error")
	}
}

// --- fnSigArgList: a nil arg type renders as "Any" ---

func TestW9FnSigArgListNilArgType(t *testing.T) {
	sig := Signature{Args: []*Type{nil, TInteger}}
	got := fnSigArgList(sig)
	if got != "[Any Integer]" {
		t.Fatalf("fnSigArgList with a nil arg type: want [Any Integer], got %q", got)
	}
}

// --- check-mode paths: installAndRecordDef / defWordExtension DefsUsed init ---

func TestW9InstallAndRecordDefLazyDefsUsed(t *testing.T) {
	// In check mode with DefsUsed not yet allocated, installAndRecordDef
	// lazily creates the map (the DefsUsed==nil arm) and then clears the
	// def's own spurious self-use.
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.DefsUsed = nil
	if _, err := installAndRecordDef(r, "myvar", NewInteger(1), SrcPos{}); err != nil {
		t.Fatalf("installAndRecordDef: %v", err)
	}
	if r.Check.DefsUsed == nil {
		t.Fatal("installAndRecordDef must lazily allocate DefsUsed in check mode")
	}
}

func TestW9DefTypedChildTypeExprError(t *testing.T) {
	// A typed-list annotation whose child is a paren expression that errors
	// surfaces through ResolveChildTypeExpr.
	if _, err := seam5Run(seam5Reg(t), `def xs:[:(zzznope)] [1]`); err == nil {
		t.Fatal("def xs:[:(undefined)] must error in the child-type expression")
	}
}

func TestW9CheckModeWordExtensionLazyDefsUsed(t *testing.T) {
	// defWordExtension's DefsUsed==nil arm: extend a builtin (`add`) in
	// check mode with DefsUsed not yet allocated. The fn body references
	// only a param, so no use is recorded before the arm runs.
	r := seam5Reg(t)
	// An EMPTY fn body means InstallWordExtension's construction pass records
	// no use, so DefsUsed is still nil when the lazy-allocation arm runs.
	// The sig anchors on a program-minted type (rev-2 ownership admission);
	// the fn is built in the SAME registry so the anchor is owned there.
	fnVal, err := seam5Run(r, `def Anc (refine Map)  fn [[a:Anc b:Any] [Any] []]`)
	if err != nil || len(fnVal) != 1 {
		t.Fatalf("build fn: %v %v", err, Canon(fnVal))
	}
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.DefsUsed = nil
	handled, xerr := defWordExtension(r, "add", fnVal[0], SrcPos{})
	if !handled || xerr != nil {
		t.Fatalf("defWordExtension add: handled=%v err=%v", handled, xerr)
	}
	if r.Check.DefsUsed == nil {
		t.Fatal("defWordExtension must lazily allocate DefsUsed in check mode")
	}
}

func TestW9CheckModeWordExtension(t *testing.T) {
	// Extending a builtin word (`add`) with a new overload in check mode
	// runs defWordExtension, including its DefsUsed use-snapshot.
	src := `def Flag (refine Boolean)  def add fn [[a:Flag b:Flag] [Flag] [a]]`
	if _, err := seam5Check(seam5Reg(t), src); err != nil {
		t.Fatalf("check-mode word extension: %v", err)
	}
}

// --- check-mode generic fn body analysis: empty body + untyped param ---

func TestW9CheckModeGenericFnBodyEmpty(t *testing.T) {
	// Empty body → the len(Body)==0 continue in the construction-time
	// generic analysis.
	if _, err := seam5Check(seam5Reg(t), `def id gen [T] fn [[x:T] [T] []]`); err != nil {
		t.Fatalf("check-mode generic fn empty body: %v", err)
	}
}
