package check

// zz_cover_carrier_analysis_test.go — white-box coverage for the
// carrier-analysis surface in carrier.go: the concrete→carrier strip
// (toCarrier), the param/element carrier builders, the fn / loop body
// models (AnalyseFnBody, RunFnBodyOnce, AnalyseLoopBody) with their
// memoisation, recursion-bail and loop-capture protocols,
// narrowing-through-use, the fresh-instance constructor model, and the
// helpers around them (carrierTypeName, BodyFreeForFallback,
// stripZeroOutResiduals, resolveTypeNameArgs,
// DeferredParamListResidual).
//
// Every test drives the DOCUMENTED behaviour from the function's doc
// comment; fixture words follow the corpus convention of a `q` suffix.
// Helpers are prefixed `zca` so this file shares no identifier with
// the other zz_cover_* files.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- fixtures ----------------------------------------------------------------

func zcaRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

// zcaEmit is an EmitRecorder stub over the inactive recorder: an
// activity switch plus probes for the loop-capture protocol
// (AnalyseLoopBody), the zero-out residual filter, and the
// uncompilable compile failure RunFnBodyOnce issues on a body error.
type zcaEmit struct {
	core.EmitRecorder
	active      bool
	loopArm     bool
	locals      []string
	beginLC     int
	endLC       int
	carried     map[string]bool
	checkpoints int
	rollbacks   int
	branchArms  int
	zeroOut     map[string]bool
	uncomp      []string
}

func zcaNewEmit() *zcaEmit {
	return &zcaEmit{
		EmitRecorder: core.TheInactiveEmit,
		carried:      map[string]bool{},
		zeroOut:      map[string]bool{},
	}
}

func (s *zcaEmit) Active() bool { return s.active }
func (s *zcaEmit) ConsumeLoopArm() bool {
	armed := s.loopArm
	s.loopArm = false
	return armed
}
func (s *zcaEmit) RegisterLocal(id string) int {
	s.locals = append(s.locals, id)
	return len(s.locals) - 1
}
func (s *zcaEmit) BeginLoopCarried() { s.beginLC++ }
func (s *zcaEmit) EndLoopCarried()   { s.endLC++ }
func (s *zcaEmit) NoteLoopCarried(name string, _, _ core.Value) {
	s.carried[name] = true
}
func (s *zcaEmit) Checkpoint() core.EmitCheckpoint {
	s.checkpoints++
	return nil
}
func (s *zcaEmit) Rollback(core.EmitCheckpoint)   { s.rollbacks++ }
func (s *zcaEmit) ArmBranchCapture()              { s.branchArms++ }
func (s *zcaEmit) ZeroOutProduced(id string) bool { return s.zeroOut[id] }
func (s *zcaEmit) MarkUncompilable(reason string) { s.uncomp = append(s.uncomp, reason) }

// zcaGoImpl adapts a plain function to the native-func impl shape.
func zcaGoImpl(f func(args []core.Value) []core.Value) *core.GoImpl {
	return core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return f(args), nil
	})
}

// zcaRegisterPushq installs `pushq name value`, a RunInCheck fixture
// that pushes a def during check-mode analysis — the minimal stand-in
// for `def` that lets a body produce net def additions (the loop
// model's join input) and consume a value.
func zcaRegisterPushq(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "pushq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAtom, core.TAny},
			QuoteArgs:  map[int]bool{0: true},
			Returns:    []*core.Type{},
			BarrierPos: -1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, err
				}
				reg.Defs.Push(name, args[1])
				return nil, nil
			}, core.RunInCheck()),
		}},
	})
}

// zcaRegisterBoomq installs `boomq`, a RunInCheck fixture whose handler
// always errors — the check-mode body-error trigger for RunFnBodyOnce.
func zcaRegisterBoomq(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "boomq",
		Signatures: []core.Signature{{
			Returns:    []*core.Type{},
			BarrierPos: -1,
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, &core.BoruError{Code: "boomq_error", Detail: "boomq: always fails"}
			}, core.RunInCheck()),
		}},
	})
}

func zcaHasDiag(r *core.Registry, code string) bool {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

// --- carrierTypeName ---------------------------------------------------------

func TestZcaCarrierTypeNameRootNode(t *testing.T) {
	// A normal carrier names its Parent.
	if got := carrierTypeName(core.NewCarrier(core.TInteger)); got != "Integer" {
		t.Errorf("carrierTypeName(Integer carrier) = %q, want Integer", got)
	}
	// A root-node value (None here) has a nil Parent — it IS its own
	// lattice node — so the name must come from the value itself, not a
	// Parent deref.
	lit := core.NewTypeLiteral(core.TNone)
	if lit.Parent != nil {
		t.Fatal("fixture: the None literal must have a nil Parent")
	}
	if got := carrierTypeName(lit); got != "None" {
		t.Errorf("carrierTypeName(None literal) = %q, want None", got)
	}
	// The doc's panic guard: a none fold-seed promoted to a dynamic
	// carrier reaches this path with a nil Parent and must not panic.
	seed := core.NewDynamicCarrierValue(core.NewTypeLiteral(core.TNone))
	if got := carrierTypeName(seed); got == "" {
		t.Error("a promoted none seed must still name itself")
	}
}

// --- toCarrier ---------------------------------------------------------------

func TestZcaToCarrierPreservations(t *testing.T) {
	// FnDef / Function payloads stay concrete: stripping would lose the
	// body, params, and Captured list.
	fnv := core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Returns: []*core.Type{core.TInteger},
		Impl:    core.Boru([]core.Value{core.NewInteger(1)}),
	}}})
	if got := toCarrier(fnv); !core.IsConcrete(got) {
		t.Error("a Function value must survive carrier-stripping concrete")
	} else if _, ok := got.Data.(core.FnDefInfo); !ok {
		t.Errorf("Function payload = %T after strip, want core.FnDefInfo", got.Data)
	}

	// DepScalar constraints stay concrete: the DepScalarInfo bounds ARE
	// the type definition.
	dep := core.NewDepScalar(core.DepGT, core.NewInteger(10))
	if _, ok := dep.Data.(core.DepScalarInfo); !ok {
		t.Fatalf("fixture: NewDepScalar payload = %T", dep.Data)
	}
	if got := toCarrier(dep); !core.IsConcrete(got) {
		t.Error("a DepScalar constraint must survive carrier-stripping concrete")
	} else if _, ok := got.Data.(core.DepScalarInfo); !ok {
		t.Errorf("DepScalar payload = %T after strip, want core.DepScalarInfo", got.Data)
	}

	// Generic SCHEMA values stay concrete: the *TypeSchemaInfo IS the
	// type definition `of` instantiates.
	fields := core.NewOrderedMap()
	fields.Set("value", core.NewTypeLiteral(core.TInteger))
	schema := core.NewTypeSchema(core.TType, &core.TypeSchemaInfo{
		Name: "ZcaStripBox",
		Body: core.NewRecordType(fields),
		Kind: core.SchemaRecord,
	})
	if got := toCarrier(schema); !core.IsConcrete(got) {
		t.Error("a schema value must survive carrier-stripping concrete")
	} else if _, ok := got.Data.(*core.TypeSchemaInfo); !ok {
		t.Errorf("schema payload = %T after strip, want *core.TypeSchemaInfo", got.Data)
	}

	// Micron values stay concrete: immutable structured scalars whose
	// payload IS the value, preserved so the `make` model's surfaced
	// construction reaches the concrete-operand mirrors (PR #346).
	mfields := core.NewOrderedMap()
	mfields.Set("address", core.NewString("a@b.c"))
	micron := core.NewValueRaw(core.TEmailon, core.MicronPayload{Fields: mfields})
	if got := toCarrier(micron); !core.IsConcrete(got) {
		t.Error("a Micron value must survive carrier-stripping concrete")
	} else if _, ok := got.Data.(core.MicronPayload); !ok {
		t.Errorf("Micron payload = %T after strip, want core.MicronPayload", got.Data)
	}
	pathon := core.NewValueRaw(core.TPathon, core.PathonPayload{Info: core.PathonInfo{Parts: []string{"a"}}})
	if got := toCarrier(pathon); !core.IsConcrete(got) {
		t.Error("a Pathon value must survive carrier-stripping concrete")
	} else if _, ok := got.Data.(core.PathonPayload); !ok {
		t.Errorf("Pathon payload = %T after strip, want core.PathonPayload", got.Data)
	}
	// The negative pair: a micron-typed CARRIER (no payload) is already
	// in carrier form and passes through unchanged — preservation is
	// payload-gated, not type-gated.
	if got := toCarrier(core.NewCarrier(core.TEmailon)); core.IsConcrete(got) || !got.Carrier {
		t.Error("a micron-typed carrier must stay a carrier")
	}

	// MODULE values stay concrete so the get-resolution elision can
	// still follow the descriptor.
	mod := core.NewModuleInstance(core.ModuleDesc{ID: "zca:mod"})
	if !core.IsModuleFamilyValue(mod) {
		t.Fatal("fixture: NewModuleInstance must be a module-family value")
	}
	if got := toCarrier(mod); !core.IsConcrete(got) {
		t.Error("a module value must survive carrier-stripping concrete")
	}

	// Reach values stay concrete: stripping would null the ReachInfo and
	// the chain would never expand in check mode.
	reach := core.NewReachFromKeys(core.NewInteger(1), []core.Value{core.NewString("a")})
	if !core.IsReach(reach) {
		t.Fatal("fixture: NewReachFromKeys must build a Reach")
	}
	if got := toCarrier(reach); !core.IsReach(got) || !core.IsConcrete(got) {
		t.Error("a Reach value must survive carrier-stripping concrete")
	}

	// A concrete-PAYLOAD carrier (Carrier=true, Data set — the check-mode
	// literal shape) is already a carrier and returns unchanged.
	lit := core.NewInteger(5)
	lit.Carrier = true
	got := toCarrier(lit)
	if !got.Carrier || got.Data == nil {
		t.Error("a concrete-payload carrier must return unchanged")
	}
	if n, err := core.AsInteger(got); err != nil || n != 5 {
		t.Errorf("payload after passthrough = (%v, %v), want 5", n, err)
	}

	// Everything else strips: Carrier set, payload dropped. `none` is a
	// concrete value whose payload is not preserved by any guard.
	none := core.NewNone()
	stripped := toCarrier(none)
	if !stripped.Carrier || stripped.Data != nil {
		t.Errorf("toCarrier(none) = Carrier=%v Data=%T, want a payload-less carrier",
			stripped.Carrier, stripped.Data)
	}
	if !stripped.Parent.Equal(core.TNone) {
		t.Error("the stripped carrier must keep its None type")
	}
}

// --- ParamInputCarrier -------------------------------------------------------

func TestZcaParamInputCarrierShapes(t *testing.T) {
	// Untyped / explicitly-Any: a DYNAMIC carrier (gradual dispatch).
	for _, tt := range []*core.Type{nil, core.TAny} {
		c := ParamInputCarrier(tt)
		if !c.Dynamic || !c.Carrier {
			t.Errorf("ParamInputCarrier(%v) must be a dynamic carrier", tt)
		}
	}
	// A concrete declared type stays a strict carrier.
	c := ParamInputCarrier(core.TInteger)
	if c.Dynamic || !c.Carrier || !c.Parent.Equal(core.TInteger) {
		t.Error("a concrete declared type must bind a strict typed carrier")
	}
	// A NAMED-UNION param binds the DISTRIBUTING disjunct carrier with
	// DisjunctInfo.Declared set: the annotation claims every alternative
	// is a valid input, so a per-alternative dispatch failure is an ERROR.
	r := zcaRegistry(t)
	union := core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TInteger),
		core.NewTypeLiteral(core.TString),
	})
	if err := core.InstallType(r, "ZcaUq", union); err != nil {
		t.Fatalf("InstallType(ZcaUq): %v", err)
	}
	def := r.LookupTypeName("ZcaUq")
	if def == nil {
		t.Fatal("LookupTypeName(ZcaUq) = nil after install")
	}
	dv := ParamInputCarrier(def)
	di, ok := dv.Data.(core.DisjunctInfo)
	if !ok {
		t.Fatalf("named-union param carrier Data = %T, want core.DisjunctInfo", dv.Data)
	}
	if !di.Declared {
		t.Error("the union param carrier must be marked Declared")
	}
}

// --- joinedElementCarrier ----------------------------------------------------

func TestZcaJoinedElementCarrierDeclines(t *testing.T) {
	// Not a plain list/map payload: decline.
	if _, ok := joinedElementCarrier(core.NewInteger(5)); ok {
		t.Error("a scalar payload has no element join")
	}
	// Fewer than two elements: the plain-type path is already precise.
	single := core.NewList([]core.Value{core.NewInteger(1)})
	if _, ok := joinedElementCarrier(single); ok {
		t.Error("a single-element list must decline the join")
	}
	// A leading nil-Parent element (a bare root literal) declines too.
	rooty := core.NewList([]core.Value{
		core.NewTypeLiteral(core.TNone),
		core.NewInteger(1),
	})
	if _, ok := joinedElementCarrier(rooty); ok {
		t.Error("a nil-Parent lead element must decline the join")
	}
}

// --- DataListElemTypeFromValue -----------------------------------------------

func TestZcaDataListElemTypeShapes(t *testing.T) {
	// An empty concrete map: no value type to read — Any.
	if got := DataListElemTypeFromValue(core.NewMap(core.NewOrderedMap())); !got.Equal(core.TAny) {
		t.Errorf("empty map elem type = %v, want Any", got)
	}
	// A map whose only value is a bare root literal (nil Parent): the
	// join never lands on a type — Any.
	om := core.NewOrderedMap()
	om.Set("k", core.NewTypeLiteral(core.TNone))
	if got := DataListElemTypeFromValue(core.NewMap(om)); !got.Equal(core.TAny) {
		t.Errorf("nil-Parent map value elem type = %v, want Any", got)
	}
	// A non-list, non-map payload: Any.
	if got := DataListElemTypeFromValue(core.NewInteger(3)); !got.Equal(core.TAny) {
		t.Errorf("scalar data elem type = %v, want Any", got)
	}
	// A list whose elements share no ancestor below Any: the join
	// short-circuits at the Any root.
	mixed := core.NewList([]core.Value{
		core.NewInteger(1),
		core.NewList(nil),
		core.NewInteger(2),
	})
	if got := DataListElemTypeFromValue(mixed); !got.Equal(core.TAny) {
		t.Errorf("Integer+List elem type = %v, want Any", got)
	}
}

// --- ReturnsPreserveListAt ---------------------------------------------------

func TestZcaReturnsPreserveListAtElemConstraint(t *testing.T) {
	// The source's element constraint is copied onto the residual so the
	// check-mode write mirror fires ((reverse xs) set 0 "bad" for
	// xs:[:Integer] is diagnosed at check).
	src := core.NewCarrierTypedList(core.TInteger)
	src.SetElemConstraint(core.NewCarrier(core.TInteger))
	out := ReturnsPreserveListAt(0)([]core.Value{src}, nil)
	if len(out) != 1 {
		t.Fatalf("ReturnsPreserveListAt produced %d values, want 1", len(out))
	}
	ec, ok := out[0].ElemConstraint()
	if !ok {
		t.Fatal("the residual must carry the source's element constraint")
	}
	if !ec.Parent.Equal(core.TInteger) {
		t.Errorf("element constraint = %v, want Integer", ec.Parent)
	}
	// Index out of range: a plain List carrier.
	fallback := ReturnsPreserveListAt(3)([]core.Value{src}, nil)
	if len(fallback) != 1 || !fallback[0].Parent.Equal(core.TList) {
		t.Error("an out-of-range index must produce a bare List carrier")
	}
}

// --- BodyFreeForFallback -----------------------------------------------------

func TestZcaBodyFreeForFallback(t *testing.T) {
	r := zcaRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zcanoopq",
		Signatures: []core.Signature{{
			Returns:    []*core.Type{core.TInteger},
			BarrierPos: -1,
			Impl:       zcaGoImpl(func(_ []core.Value) []core.Value { return []core.Value{core.NewInteger(0)} }),
		}},
	})

	// Registered natives and known bare literals resolve: free.
	free := core.NewList([]core.Value{
		core.NewWord("zcanoopq"), core.NewWord("true"), core.NewWord("Integer"),
	})
	if !BodyFreeForFallback(r, free) {
		t.Error("a body of registered / literal words must be island-free")
	}
	// A flow-control sentinel declines: the VM cannot propagate it across
	// the island boundary.
	for _, w := range []string{"break", "continue", "return"} {
		if BodyFreeForFallback(r, core.NewList([]core.Value{core.NewWord(w)})) {
			t.Errorf("a body holding %q must decline the island", w)
		}
	}
	// Context-dependent words (args / __pa) read the interpreter's
	// per-call args stack the VM does not maintain: decline.
	for _, w := range []string{"args", "__pa"} {
		if BodyFreeForFallback(r, core.NewList([]core.Value{core.NewWord(w)})) {
			t.Errorf("a body holding %q must decline the island", w)
		}
	}
	// After one unresolvable word flips the verdict, later words take the
	// short-circuit exit and cannot flip it back.
	poisoned := core.NewList([]core.Value{
		core.NewWord("zca-unbound-q"), core.NewWord("zcanoopq"),
	})
	if BodyFreeForFallback(r, poisoned) {
		t.Error("an unresolvable word must poison the whole body")
	}
}

// --- resolveTypeNameArgs -----------------------------------------------------

func TestZcaResolveTypeNameArgs(t *testing.T) {
	wordInt := core.NewWord("Integer")
	wordCarrier := core.NewCarrier(core.TWord) // IsWord, but no WordInfo payload
	wordUnknown := core.NewWord("zca-not-a-type-q")
	scalar := core.NewInteger(3)

	args := []core.Value{wordInt, wordCarrier, wordUnknown, scalar}
	out := resolveTypeNameArgs(args)

	// The builtin type name resolves to the canonical literal.
	if node := core.TypeNodeOf(out[0]); node == nil || !node.Equal(core.TInteger) {
		t.Errorf("Integer word resolved to %v, want the Integer literal", out[0])
	}
	// Everything else passes through untouched.
	if !core.IsWord(out[1]) || out[1].Data != nil {
		t.Error("a payload-less Word-typed carrier must pass through")
	}
	if w, err := core.AsWord(out[2]); err != nil || w.Name != "zca-not-a-type-q" {
		t.Error("a non-type word must pass through as a word")
	}
	if n, err := core.AsInteger(out[3]); err != nil || n != 3 {
		t.Error("a non-word arg must pass through")
	}
	// The copy is lazy AND non-destructive: the caller's slice keeps the
	// raw word.
	if !core.IsWord(args[0]) || args[0].Data == nil {
		t.Error("resolveTypeNameArgs must not mutate the caller's slice")
	}
	// The all-resolved common case allocates nothing: same values back.
	plain := []core.Value{core.NewInteger(1), core.NewString("s")}
	same := resolveTypeNameArgs(plain)
	if len(same) != 2 {
		t.Fatal("no-word args must come back unchanged")
	}
}

// --- stripZeroOutResiduals ---------------------------------------------------

func TestZcaStripZeroOutResiduals(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	phantom := core.NewCarrier(core.TNone)
	real := core.NewCarrier(core.TInteger)

	// Recording inactive: the stack passes through untouched (the
	// zeroOut flag is only set during the compile pass).
	got := stripZeroOutResiduals(r, []core.Value{phantom, real})
	if len(got) != 2 {
		t.Fatalf("inactive recorder: %d residuals, want 2 (untouched)", len(got))
	}

	// Recording active: the 0-output statement-guard phantom is dropped,
	// the real residual kept.
	es := zcaNewEmit()
	es.active = true
	es.zeroOut[phantom.ID] = true
	r.Check.Emit = es
	got = stripZeroOutResiduals(r, []core.Value{phantom, real})
	if len(got) != 1 {
		t.Fatalf("active recorder: %d residuals, want 1", len(got))
	}
	if got[0].ID != real.ID {
		t.Error("the surviving residual must be the non-phantom value")
	}
	// An empty stack short-circuits.
	if out := stripZeroOutResiduals(r, nil); len(out) != 0 {
		t.Error("an empty residual stack must stay empty")
	}
}

// --- DeferredParamListResidual -----------------------------------------------

func TestZcaDeferredParamListResidual(t *testing.T) {
	deferred := core.NewList([]core.Value{core.NewWord("c1")})
	deferred.Eval = true

	// The def-node-binding shape: a lone unquoted evaluated list body
	// naming a param returns the list raw.
	if v, ok := DeferredParamListResidual([]core.Value{deferred}, []string{"c1"}); !ok {
		t.Error("a deferred list body naming a param must qualify")
	} else if !core.ValuesEqual(v, deferred) {
		t.Error("the raw list itself must be returned")
	}
	// No named params at all: nothing to defer over.
	if _, ok := DeferredParamListResidual([]core.Value{deferred}, []string{"", ""}); ok {
		t.Error("a body with no named params must decline")
	}
	// A body that references no param folds under the normal path.
	other := core.NewList([]core.Value{core.NewWord("zca-other-q")})
	other.Eval = true
	if _, ok := DeferredParamListResidual([]core.Value{other}, []string{"c1"}); ok {
		t.Error("a body that references no param must decline")
	}
	// Multi-statement and non-list bodies keep their existing analysis.
	if _, ok := DeferredParamListResidual([]core.Value{deferred, deferred}, []string{"c1"}); ok {
		t.Error("a multi-statement body must decline")
	}
	quoted := deferred
	quoted.Quoted = true
	if _, ok := DeferredParamListResidual([]core.Value{quoted}, []string{"c1"}); ok {
		t.Error("a quoted list body must decline")
	}
}

// --- narrowDynamicUses -------------------------------------------------------

func TestZcaNarrowDynamicUsesTightens(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	// The doc's example: a dynamic-Any binding (`def nodes (tree get
	// "nodes")`) consumed by a List slot tightens to dynamic(List) for
	// downstream uses.
	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("zcadynq")
	r.Defs.Push("zcadynq", v)

	sig := &core.Signature{Args: []*core.Type{core.TList}, BarrierPos: 1}
	narrowDynamicUses(r, "", sig, []core.Value{v})

	got, ok := r.Defs.Top("zcadynq")
	if !ok {
		t.Fatal("the binding must survive the narrowing")
	}
	if !got.Dynamic {
		t.Error("the narrowed binding stays a dynamic carrier")
	}
	if got.Parent == nil || !got.Parent.Equal(core.TList) {
		t.Errorf("bound after use = %v, want dynamic(Any ∩ List) = List", got.Parent)
	}
	// Identity-preserving rebind: the narrowing refines the SAME runtime
	// value, so the producing event's ID is kept.
	if got.ID != v.ID {
		t.Error("the narrowed binding must keep the original binding's ID")
	}
}

func TestZcaNarrowDynamicUsesNoOps(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	// (a) Same-type use: the bound did not actually tighten — no push.
	a := core.NewDynamicCarrier(core.TAny)
	a.SetDynFrom("zcasameq")
	r.Defs.Push("zcasameq", a)
	depth := r.Defs.Depth("zcasameq")
	narrowDynamicUses(r, "", &core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: 1}, []core.Value{a})
	if r.Defs.Depth("zcasameq") != depth {
		t.Error("a no-op narrowing must not grow the def stack")
	}

	// (b) Root-collapsed intersection (the None builder-pattern shape):
	// the (None|Integer) bound against a None slot collapses to the bare
	// None root, which must NOT be re-promoted (a nil bound would read as
	// "matches nothing" downstream) — the binding keeps its broader bound.
	b := core.NewDynamicCarrierValue(core.NewDisjunct([]core.Value{
		core.NewTypeLiteral(core.TNone),
		core.NewTypeLiteral(core.TInteger),
	}))
	b.SetDynFrom("zcarootq")
	r.Defs.Push("zcarootq", b)
	depth = r.Defs.Depth("zcarootq")
	narrowDynamicUses(r, "", &core.Signature{Args: []*core.Type{core.TNone}, BarrierPos: 1}, []core.Value{b})
	if r.Defs.Depth("zcarootq") != depth {
		t.Error("a root-collapsed narrowing must leave the binding untouched")
	}
	if got, _ := r.Defs.Top("zcarootq"); !got.Parent.Equal(core.TDisjunct) {
		t.Error("the binding must keep its prior, broader dynamic bound")
	}

	// (c) Disjoint slot (a since-invalid narrowing): Never intersections
	// are skipped rather than pushed.
	c := core.NewDynamicCarrier(core.TInteger)
	c.SetDynFrom("zcadisjq")
	r.Defs.Push("zcadisjq", c)
	depth = r.Defs.Depth("zcadisjq")
	narrowDynamicUses(r, "", &core.Signature{Args: []*core.Type{core.TString}, BarrierPos: 1}, []core.Value{c})
	if r.Defs.Depth("zcadisjq") != depth {
		t.Error("a disjoint intersection must not be pushed")
	}
}

func TestZcaNarrowDynamicUsesPolymorphicSlot(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	// The slice shape: the data slot is String in the matched overload
	// and List in an equally-reachable sibling — narrowing to the single
	// matched slot would be unsound, so the binding is left alone.
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zcaslcq",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TString, core.TInteger}, Returns: []*core.Type{core.TString}, BarrierPos: 2, Impl: zcaGoImpl(func(a []core.Value) []core.Value { return a[:1] })},
			{Args: []*core.Type{core.TList, core.TInteger}, Returns: []*core.Type{core.TList}, BarrierPos: 2, Impl: zcaGoImpl(func(a []core.Value) []core.Value { return a[:1] })},
		},
	})
	v := core.NewDynamicCarrier(core.TAny)
	v.SetDynFrom("zcapolyq")
	r.Defs.Push("zcapolyq", v)
	depth := r.Defs.Depth("zcapolyq")

	matched := &core.Signature{Args: []*core.Type{core.TString, core.TInteger}, BarrierPos: 2}
	narrowDynamicUses(r, "zcaslcq", matched, []core.Value{v, core.NewCarrier(core.TInteger)})

	if r.Defs.Depth("zcapolyq") != depth {
		t.Error("a polymorphic slot must not narrow the binding")
	}
	if got, _ := r.Defs.Top("zcapolyq"); !got.Parent.Equal(core.TAny) {
		t.Error("the binding must keep its dynamic Any bound at a polymorphic slot")
	}
}

// --- ReturnsFreshInstance ----------------------------------------------------

func zcaRecordSchema(t *testing.T) core.Value {
	t.Helper()
	fields := core.NewOrderedMap()
	fields.Set("value", core.NewTypeLiteral(core.TInteger))
	return core.NewTypeSchema(core.TType, &core.TypeSchemaInfo{
		Name: "ZcaRule",
		Body: core.NewRecordType(fields),
		Kind: core.SchemaRecord,
	})
}

func TestZcaReturnsFreshInstanceSchemaConcrete(t *testing.T) {
	r := zcaRegistry(t)
	schema := zcaRecordSchema(t)

	// CONCRETE construction data that validates: the carrier takes the
	// instantiated body's Map-branch type and rides the field schema
	// (dynamic + RecordTypeInfo) so downstream field reads narrow.
	om := core.NewOrderedMap()
	om.Set("value", core.NewInteger(42))
	good := core.NewMap(om)
	out := ReturnsFreshInstance(0)([]core.Value{schema, good}, r)
	if len(out) != 1 {
		t.Fatalf("fresh-instance returns %d values, want 1", len(out))
	}
	if !out[0].Carrier || !out[0].Dynamic {
		t.Error("a validated schema construction must be a dynamic instance carrier")
	}
	if rt, ok := out[0].Data.(core.RecordTypeInfo); !ok || rt.Fields == nil {
		t.Errorf("instance carrier Data = %T, want the instantiated RecordTypeInfo", out[0].Data)
	}
	if !out[0].Parent.ConformsTo(core.TMap) {
		t.Errorf("instance carrier type = %v, want the runtime Map tag", out[0].Parent)
	}

	// CONCRETE data the runtime constructor REJECTS (unknown field): the
	// pre-exception schema-node carrier flags the enclosing return.
	om2 := core.NewOrderedMap()
	om2.Set("bogus", core.NewInteger(1))
	bad := core.NewMap(om2)
	out = ReturnsFreshInstance(0)([]core.Value{schema, bad}, r)
	if len(out) != 1 {
		t.Fatalf("failing construction returns %d values, want 1", len(out))
	}
	if out[0].Dynamic {
		t.Error("a failing construction must NOT ride the optimistic dynamic carrier")
	}
	if !out[0].Parent.Equal(core.ValueType(schema)) {
		t.Errorf("failing construction carrier = %v, want the schema node", out[0].Parent)
	}
}

func TestZcaReturnsFreshInstanceRecordBodyAndMisc(t *testing.T) {
	r := zcaRegistry(t)

	// A plain RECORD BODY target rides the declared field schema on the
	// instance carrier (dynamic + RecordTypeInfo).
	fields := core.NewOrderedMap()
	fields.Set("bits", core.NewTypeLiteral(core.TInteger))
	recBody := core.NewRecordType(fields)
	out := ReturnsFreshInstance(0)([]core.Value{recBody, core.NewMap(core.NewOrderedMap())}, r)
	if len(out) != 1 {
		t.Fatalf("record-body target returns %d values, want 1", len(out))
	}
	if !out[0].Carrier || !out[0].Dynamic {
		t.Error("a record-body construction must be a dynamic instance carrier")
	}
	if rt, ok := out[0].Data.(core.RecordTypeInfo); !ok || rt.Fields == nil {
		t.Errorf("record-body carrier Data = %T, want the RecordTypeInfo", out[0].Data)
	}

	// Out-of-range mapping: Any carrier. Dynamic target: identity.
	out = ReturnsFreshInstance(5)([]core.Value{recBody}, r)
	if len(out) != 1 || !out[0].Parent.Equal(core.TAny) {
		t.Error("an out-of-range mapping must produce an Any carrier")
	}
	dyn := core.NewDynamicCarrier(core.TType)
	out = ReturnsFreshInstance(0)([]core.Value{dyn}, r)
	if len(out) != 1 || out[0].ID != dyn.ID {
		t.Error("a dynamic/computed target keeps the prior identity behaviour")
	}
}

// --- AnalyseLoopBody ---------------------------------------------------------

func TestZcaAnalyseLoopBodyJoinAndCapture(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	done := r.Check.Begin()
	defer done()

	es := zcaNewEmit()
	es.loopArm = true // the `for` lowering armed loop capture
	r.Check.Emit = es

	// Pre-loop binding the body REBINDS (loop-carried), plus a fresh
	// body-local binding.
	r.Defs.Push("zcaaccq", core.NewCarrier(core.TInteger))

	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("zcaaccq"), core.NewString("s"),
		core.NewWord("pushq"), core.NewWord("zcafreshq"), core.NewInteger(7),
	})
	iter := core.NewCarrier(core.TInteger)
	stk := AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{iter}, true)

	if len(stk) != 0 {
		t.Errorf("the body consumes every value: residual = %d values, want 0", len(stk))
	}
	// "The loop may run zero times": the post-loop binding is the JOIN of
	// the body's rebind with the pre-loop binding (String | Integer).
	acc, ok := r.Defs.Top("zcaaccq")
	if !ok {
		t.Fatal("the joined post-loop binding must be installed")
	}
	if !acc.Parent.Equal(core.TDisjunct) {
		t.Errorf("post-loop bound = %v, want the String|Integer join", acc.Parent)
	}
	// A body-only binding leaks post-loop as itself (no pre to join).
	if fresh, ok := r.Defs.Top("zcafreshq"); !ok {
		t.Error("a fresh body binding must be installed post-loop")
	} else if n, err := core.AsInteger(fresh); err != nil || n != 7 {
		t.Errorf("fresh binding = %v, want 7", fresh)
	}
	// The loop-local binds are popped.
	if _, ok := r.Defs.Top("iq"); ok {
		t.Error("the loop-local iterator bind must be popped")
	}

	// The armed capture protocol: locals registered for the loop binds,
	// the loop-carried bracket opened and closed, the rebind noted, one
	// checkpoint per round, and the non-final round rolled back.
	if len(es.locals) != 1 || es.locals[0] != iter.ID {
		t.Errorf("RegisterLocal calls = %v, want the iterator's ID", es.locals)
	}
	if es.beginLC != 1 || es.endLC != 1 {
		t.Errorf("loop-carried bracket = begin %d / end %d, want 1/1", es.beginLC, es.endLC)
	}
	if !es.carried["zcaaccq"] {
		t.Error("the pre-existing rebind must be noted loop-carried")
	}
	if es.carried["zcafreshq"] {
		t.Error("a fresh body-local binding is not loop-carried")
	}
	if es.checkpoints != 2 || es.branchArms != 2 {
		t.Errorf("rounds ran %d checkpoints / %d arms, want 2/2 (round 2 stabilises)", es.checkpoints, es.branchArms)
	}
	if es.rollbacks != 1 {
		t.Errorf("non-final rounds rolled back %d times, want 1", es.rollbacks)
	}
}

func TestZcaAnalyseLoopBodyKleeneRounds(t *testing.T) {
	// The doc's `def acc (acc add 0.5)` shape: a rebinding whose join
	// keeps WIDENING needs the later rounds — round 2 compares a joined
	// binding of the same size but a different (wider) value against
	// round 1's, so the body is re-analysed with the joined bindings.
	// The grown alternatives are DISTANT cousins (String, List, Map): a
	// sibling pair would collapse to its shared parent and stabilise in
	// one round.
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	grow := []*core.Type{core.TString, core.TList, core.TMap}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zcastepq",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny},
			Returns:    []*core.Type{core.TAny},
			BarrierPos: 1,
			Impl:       zcaGoImpl(func(a []core.Value) []core.Value { return a }),
			ReturnsFn: func(args []core.Value, _ *core.Registry) []core.Value {
				// A DYNAMIC union result: a strict disjunct would take the
				// per-alternative partition path and re-derive per member.
				n := len(core.FlattenAlternatives(args[0]))
				j := core.JoinCarriers(args[0], core.NewCarrier(grow[(n-1)%len(grow)]))
				return []core.Value{core.NewDynamicCarrierValue(j)}
			},
		}},
	})
	done := r.Check.Begin()
	defer done()

	r.Defs.Push("zcagrowq", core.NewCarrier(core.TInteger))
	body := core.NewList([]core.Value{
		core.NewWord("pushq"), core.NewWord("zcagrowq"),
		core.NewOpenParen(), core.NewWord("zcastepq"), core.NewWord("zcagrowq"), core.NewCloseParen(),
	})
	AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, true)

	acc, ok := r.Defs.Top("zcagrowq")
	if !ok {
		t.Fatal("the joined post-loop binding must be installed")
	}
	if !acc.Parent.Equal(core.TDisjunct) {
		t.Fatalf("post-loop bound = %v, want a widened union", acc.Parent)
	}
	// Round 1 alone would leave a two-alternative join; the re-analysis
	// under the joined binding must have widened it further.
	if n := len(core.FlattenAlternatives(acc)); n < 3 {
		t.Errorf("post-loop union has %d alternatives, want the later rounds' widening (>= 3)", n)
	}
}

func TestZcaAnalyseLoopBodySentinelUnproven(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	// A body carrying a flow-control sentinel declines the proven-trips
	// stamp even when the caller proves a trip count: the split's
	// soundness argument breaks under break/continue.
	body := core.NewList([]core.Value{core.NewWord("break")})
	AnalyseLoopBody(r, body, []string{"iq"}, []core.Value{core.NewCarrier(core.TInteger)}, true)
	if r.Check.LoopBodyDepth != 0 {
		t.Errorf("LoopBodyDepth leaked to %d after a sentinel body", r.Check.LoopBodyDepth)
	}
	if _, ok := r.Defs.Top("iq"); ok {
		t.Error("the loop-local bind must be popped")
	}
}

// --- RunFnBodyOnce -----------------------------------------------------------

func TestZcaRunFnBodyOnceBodyError(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterBoomq(r)
	done := r.Check.Begin()
	defer done()

	es := zcaNewEmit()
	es.active = true // an ARMED recording (open fn unit)
	r.Check.Emit = es

	result := RunFnBodyOnce(r, "zcaboomq", nil, []core.Value{core.NewWord("boomq")}, nil, nil, false)
	if result != nil {
		t.Errorf("an erroring body must yield a nil result, got %v", result)
	}
	if !zcaHasDiag(r, "fn_body_error") {
		t.Error("the body error must surface as a fn_body_error diagnostic")
	}
	// Under an armed recording the unit would close EMPTY and silently
	// diverge — the program is declined instead.
	if len(es.uncomp) == 0 {
		t.Fatal("an armed erroring body must mark the program uncompilable")
	}
}

func TestZcaRunFnBodyOnceDefsUnwind(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterPushq(r)
	done := r.Check.Begin()
	defer done()

	// Params bind as defs for the body run and unwind afterwards;
	// captures install below params.
	body := []core.Value{core.NewWord("nq")}
	caps := []core.CapturedBinding{{Name: "zcacapq", Value: core.NewCarrier(core.TString)}}
	result := RunFnBodyOnce(r, "zcafnq", []string{"nq"}, body, []core.Value{core.NewCarrier(core.TInteger)}, caps, false)
	if len(result) != 1 || !result[0].Parent.Equal(core.TInteger) {
		t.Errorf("body residual = %v, want the param's Integer carrier", result)
	}
	if _, ok := r.Defs.Top("nq"); ok {
		t.Error("the param binding must unwind after the run")
	}
	if _, ok := r.Defs.Top("zcacapq"); ok {
		t.Error("the capture binding must unwind after the run")
	}
}

// --- AnalyseFnBody -----------------------------------------------------------

func TestZcaAnalyseFnBodyUndefinedArgGradualized(t *testing.T) {
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()

	// A forward-referenced fn name leaks in as a concrete Undefined Atom;
	// the analysis boundary gradualizes it to a dynamic Any carrier —
	// copy-on-write, so the caller's slice is untouched.
	undef := core.NewAtom("zca-forward-q")
	undef.Undefined = true
	body := []core.Value{core.NewInteger(1)}
	args := []core.Value{undef}

	result := AnalyseFnBody(r, "zcaundefq", []string{"xq"}, body, args, nil, nil, false)
	if len(result) != 1 {
		t.Fatalf("analysis returned %d values, want 1", len(result))
	}
	if n, err := core.AsInteger(result[0]); err != nil || n != 1 {
		t.Errorf("body residual = %v, want 1", result[0])
	}
	if !args[0].Undefined {
		t.Error("the caller's arg slice must be untouched (copy-on-write)")
	}
	// The memo key was built from the SANITIZED dynamic-Any arg, not the
	// phantom Atom.
	key := FnAnalysisKey(r.AnalysisScopeID(), "zcaundefq",
		[]core.Value{core.NewDynamicCarrier(core.TAny)}, nil, body)
	if _, ok := r.Check.FnSummaries[key]; !ok {
		t.Error("the summary must be memoised under the gradualized arg shape")
	}
}

func TestZcaAnalyseFnBodyInflightBails(t *testing.T) {
	// PLAIN check, no declared returns: the in-flight self-call bails
	// with a variadic spread (element Never) — the 0-or-more residual
	// that covers the runtime's depth-many values.
	r := zcaRegistry(t)
	done := r.Check.Begin()
	defer done()
	body := []core.Value{core.NewInteger(1)}
	key := FnAnalysisKey(r.AnalysisScopeID(), "zcainfq", nil, nil, body)
	r.Check.FnInflight = map[string]bool{key: true}

	out := AnalyseFnBody(r, "zcainfq", nil, body, nil, nil, nil, false)
	if len(out) != 1 {
		t.Fatalf("in-flight bail returned %d values, want 1", len(out))
	}
	elem, ok := core.IsVariadicSpread(out[0])
	if !ok {
		t.Fatal("the plain-check bail must be a variadic spread carrier")
	}
	if !core.IsNeverShape(elem) {
		t.Errorf("spread element = %v, want Never", elem)
	}
	if r.Check.InflightBails != 1 {
		t.Errorf("InflightBails = %d, want 1", r.Check.InflightBails)
	}

	// ARMED (compiled) path: the Stage A model — a plain Any carrier.
	r2 := zcaRegistry(t)
	done2 := r2.Check.Begin()
	defer done2()
	es := zcaNewEmit()
	es.active = true
	r2.Check.Emit = es
	key2 := FnAnalysisKey(r2.AnalysisScopeID(), "zcainfq", nil, nil, body)
	r2.Check.FnInflight = map[string]bool{key2: true}

	out = AnalyseFnBody(r2, "zcainfq", nil, body, nil, nil, nil, false)
	if len(out) != 1 {
		t.Fatalf("armed in-flight bail returned %d values, want 1", len(out))
	}
	if _, ok := core.IsVariadicSpread(out[0]); ok {
		t.Error("the armed bail must NOT be a variadic spread")
	}
	if !out[0].Parent.Equal(core.TAny) || out[0].Dynamic {
		t.Errorf("armed bail = %v, want a strict Any carrier", out[0])
	}
}

func TestZcaAnalyseFnBodyNameReentrySuppressesErrors(t *testing.T) {
	r := zcaRegistry(t)
	zcaRegisterBoomq(r)
	done := r.Check.Begin()
	defer done()

	body := []core.Value{core.NewWord("boomq")}

	// Control: the canonical (non-re-entrant) analysis of an erroring
	// body reports fn_body_error.
	AnalyseFnBody(r, "zcasup2q", nil, body, nil, nil, nil, false)
	if !zcaHasDiag(r, "fn_body_error") {
		t.Fatal("control: the canonical analysis must report fn_body_error")
	}

	// Recursive RE-ENTRY by name (a self-call under a different arg
	// shape): error-level body diagnostics are suppressed — the outer
	// canonical analysis already reported any real defect.
	r2 := zcaRegistry(t)
	zcaRegisterBoomq(r2)
	done2 := r2.Check.Begin()
	defer done2()
	r2.Check.FnNameInflight = map[string]int{"zcasupq": 1}

	AnalyseFnBody(r2, "zcasupq", nil, body, nil, nil, nil, false)
	if zcaHasDiag(r2, "fn_body_error") {
		t.Error("a by-name re-entry must suppress the body's error diagnostics")
	}
	if r2.Check.SuppressBodyErrors != 0 {
		t.Errorf("SuppressBodyErrors = %d after the run, want 0 (restored)", r2.Check.SuppressBodyErrors)
	}
	if r2.Check.FnNameInflight["zcasupq"] != 1 {
		t.Errorf("FnNameInflight = %d after the run, want the caller's 1 restored", r2.Check.FnNameInflight["zcasupq"])
	}
}

func TestZcaAnalyseFnBodyRecursionRefines(t *testing.T) {
	// A recursive fn with NO declared returns: the first run consumes an
	// Any in-flight bail, so the summary is recomputed under the seeded
	// hypothesis (refineRecursiveSummary) instead of cached as final.
	r := zcaRegistry(t)
	zcaRegisterPushq(r)

	body := []core.Value{
		core.NewWord("pushq"), core.NewWord("zcatmpq"),
		core.NewOpenParen(), core.NewWord("zcarecq"), core.NewWord("nq"), core.NewCloseParen(),
		core.NewInteger(7),
	}
	core.InstallFnDef(r, "zcarecq", core.FnDefInfo{Signatures: []core.Signature{{
		Params:     []core.FnParam{{Name: "nq", Type: core.TInteger}},
		Returns:    nil, // unchecked: the Any bail + refinement path
		Impl:       core.Boru(body),
		BarrierPos: core.BarrierAllForward,
	}}})

	done := r.Check.Begin()
	defer done()

	out := AnalyseFnBody(r, "zcarecq", []string{"nq"}, body,
		[]core.Value{core.NewCarrier(core.TInteger)}, nil, nil, false)

	if r.Check.InflightBails == 0 {
		t.Fatal("the recursive self-call must consume an in-flight bail")
	}
	if len(out) != 1 {
		t.Fatalf("refined summary = %d values, want 1", len(out))
	}
	if out[0].Parent == nil || !out[0].Parent.ConformsTo(core.TInteger) {
		t.Errorf("refined summary = %v, want the Integer residual", out[0])
	}
	key := FnAnalysisKey(r.AnalysisScopeID(), "zcarecq",
		[]core.Value{core.NewCarrier(core.TInteger)}, nil, body)
	if got, ok := r.Check.FnSummaries[key]; !ok || len(got) != 1 {
		t.Error("the refined summary must be memoised under the call-shape key")
	}
}
