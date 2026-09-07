package core

import (
	"math/big"

	"github.com/cockroachdb/apd/v3"
)

// Payload is the static type of Value.Data — the kernel-known shape
// of the data a Value carries. During the type-decoupling migration
// (Step 5 of TYPE-DECOUPLING.10.md), Payload is a `= any` alias so
// constructors and type assertions can migrate one batch at a time
// without breaking the build. The final commit of Step 5 will
// rename this to a sealed interface (an unexported marker method
// on each variant), at which point the compiler catches any
// remaining non-variant assignment to Value.Data.
//
// Two strategies for satisfying Payload:
//
//  1. eng-defined struct or pointer types act as payloads directly,
//     with an unexported payloadMarker() method (added to the type's
//     definition file). No wrapper needed — the type's name already
//     conveys its role (WordInfo, ForwardInfo, RecordTypeInfo, …).
//
//  2. Primitives (int64, string, bool, float64), Go-built-in slice
//     types ([]Value, []InterpPart), and external types (time.Time,
//     time.Duration, *time.Location), and types defined in other
//     packages (lang/go/engine's QueryBuilder) cannot carry the
//     unexported marker; they are wrapped in named variant structs
//     declared below (IntPayload, ListPayload, TimePayload, …) or
//     in the catch-all ExtensionPayload{Body: any}.
//
// As of Step 5g, Payload is now a sealed interface. Any code path
// that tries to assign a non-marker-bearing value to Value.Data will
// fail to compile — `Value{Data: "hello"}`, `Value{Data: int64(5)}`,
// `Value{Data: qb}` (where qb is from another package), and similar
// mismatched-shape constructions are rejected at the type-check
// level. The seal is the kernel guarantee that fulfils the
// "make illegal values unrepresentable" goal stated in
// design/TYPE-DECOUPLING.10.md.
type Payload interface {
	payloadMarker()
	// IsTypeContent reports whether this payload is a TYPE's content —
	// the structural body of a type declaration — as opposed to an
	// ordinary value's data. It is the sealed-payload half of the ONE
	// type-recognition seam (design/TYPE-REPRESENTATION.1.md §N4):
	// IsTypeBody asks the payload instead of enumerating shapes, so a
	// new kind declares itself by answering here rather than by
	// growing an arm at every consumer. Most payloads answer with a
	// constant; the two variants shared between types and ordinary
	// values answer from value state — MapPayload from its OrderedMap's
	// Implicit flag (a record shape vs a concrete map), ExtensionPayload
	// from its nested hostTypeBody marker (a host type body vs a host
	// instance) — and the two function payloads from the owner's
	// dispatch identity (owner carries the Value whose Data this is;
	// implementations that don't need it ignore it).
	IsTypeContent(owner *Value) bool
}

// =================================================================
// Wrapper variants (Step 5b–5c) for types that can't carry methods.
// =================================================================

// IntPayload carries the int64 payload for a Scalar/Number/Integer
// value. Constructed by NewInteger.
type IntPayload struct{ N int64 }

// FloatPayload carries the float64 payload for a Scalar/Number/Float
// value. Constructed by NewFloat.
type FloatPayload struct{ F float64 }

// BigIntPayload carries an arbitrary-precision integer for a
// Scalar/Number/BigInteger value. Constructed by NewBigInteger. The
// *big.Int is treated as immutable after construction — arithmetic
// always allocates a fresh result — so the shallow Value copy and
// sameContainer identity stay sound (see eng/go/CLAUDE.md Sealed Payload).
type BigIntPayload struct{ N *big.Int }

// DecimalPayload carries an arbitrary-precision base-10 decimal for a
// Scalar/Number/BigDecimal value. Constructed by NewBigDecimal. The
// *apd.Decimal is likewise treated as immutable after construction.
type DecimalPayload struct{ D *apd.Decimal }

// StrPayload carries the string payload for a Scalar/String value.
// Constructed by NewString.
type StrPayload struct{ S string }

// BoolPayload carries the bool payload for a Scalar/Boolean value.
// Constructed by NewBoolean.
type BoolPayload struct{ B bool }

// AtomPayload carries the unquoted-name payload for a Scalar/Atom value.
// Constructed by NewAtom.
//
// Referent, when non-nil, is a snapshot of the value the atom's name was
// bound to at the moment it was quoted (by the `quote` word) or when the
// program was loaded (the run-start resolution pass) — it records what a
// quoted name referred to. It is nil when the name was unbound at capture
// time. Referent is METADATA ONLY: atom identity — equality, ordering,
// canonical form — is by Name alone and must ignore Referent (see
// valuesEqualDefault's TAtom case and CanonValue), so two atoms with the
// same name stay equal regardless of what each referred to.
type AtomPayload struct {
	Name     string
	Referent *Value
}

// PathonPayload wraps a PathonInfo for a Scalar/Micron/Pathon value.
type PathonPayload struct{ Info PathonInfo }

// ListPayload carries the standard []Value payload for a Node/List
// value. Constructed by NewList. Wrapping []Value rather than
// adding a marker to the slice type itself is necessary because Go
// does not allow methods on built-in slice types.
type ListPayload struct{ Elems []Value }

// MapPayload carries the *OrderedMap payload for a Node/Map value.
// Constructed by NewMap. Same wrapping motivation as ListPayload.
type MapPayload struct{ M *OrderedMap }

// FlexListData is the mutable element store for a Node/List/FlexList
// value. It is pointer-backed (stored as *FlexListData) so in-place
// growth — append/push — is visible
// through every Value copy sharing the payload. FlexMap needs no
// counterpart: MapPayload's *OrderedMap is already pointer-backed.
// Constructed by NewFlexList.
type FlexListData struct{ Elems []Value }

// XmlElementPayload is the immutable backing for a Node/Xml element
// value (an embedded `<tag>…</tag>` literal, or the future remapped
// `parse xml` output). Tag is the element name; Attr is the
// insertion-ordered attribute map (name → String value); Cren ("child
// nodes") holds every child in document order — nested Node/Xml
// elements and Scalar/String text nodes interleaved. The element-only
// view (DOM `children`) and concatenated text (DOM `textContent`) are
// computed from Cren by words, not stored. See design/XML-LITERAL.0.md.
type XmlElementPayload struct {
	Tag  string
	Attr *OrderedMap
	Cren []Value
}

// XmlInterpPayload is the backing for an interpolated XML literal — the
// deferred analogue of XmlElementPayload. A literal that embeds any
// `${expr}` (in text, a child position, or an attribute value) cannot be
// built at parse time, so the matcher emits this skeleton (Word/__XI) and
// the engine evaluates it in place to a concrete Node/Xml at runtime,
// exactly as InterpStringPayload defers a `${}` template string. A
// literal with no interpolation stays a constant XmlElementPayload. See
// design/XML-LITERAL.0.md §4 and engine.go::EvalXmlInterp.
type XmlInterpPayload struct{ Tmpl XmlTmpl }

// XmlTmpl is one element in an XML interpolation skeleton. Tag is static
// (interpolated tag names are deferred — design §7). Each attribute value
// is a list of InterpParts (literal + `${expr}` segments concatenated to
// a String at eval time); each child is an XmlCren (literal text, a nested
// element, or a `${expr}` hole).
type XmlTmpl struct {
	Tag  string
	Attr []XmlAttrTmpl
	Cren []XmlCren
}

// XmlAttrTmpl is one attribute of an XmlTmpl: a name and the InterpParts
// its value evaluates from.
type XmlAttrTmpl struct {
	Name  string
	Parts []InterpPart
}

// XmlCrenKind discriminates the three child shapes in an XmlTmpl.
type XmlCrenKind int

const (
	XmlCrenLit   XmlCrenKind = iota // literal text node (Lit)
	XmlCrenChild                    // nested element (Child)
	XmlCrenExpr                     // ${expr} hole (Expr) — splice rule at eval
)

// XmlCren is one child slot of an XmlTmpl. Exactly one of Lit / Child /
// Expr is meaningful, per Kind. An Expr hole evaluates at runtime: a List
// result splices each element as a child, a Node/Xml result is one child
// element, any other value becomes a text node.
type XmlCren struct {
	Kind  XmlCrenKind
	Lit   string
	Child *XmlTmpl
	Expr  []Value
}

// FlexXmlData is the mutable backing for a Node/Xml/FlexXml element —
// the build-in-place counterpart of the immutable XmlElementPayload. It
// is pointer-backed (stored as *FlexXmlData, like *FlexListData) so
// in-place mutation — append a child, set an attribute — is visible
// through every Value copy sharing the payload. Attr is a fresh
// *OrderedMap (never aliased with an immutable source). Constructed by
// NewFlexXml. See design/XML-LITERAL.0.md §5 and core_flex.go.
type FlexXmlData struct {
	Tag  string
	Attr *OrderedMap
	Cren []Value
}

// ParenExprPayload carries the unevaluated tokens of a paren-expression
// awaiting inline evaluation. Wrapping []Value.
type ParenExprPayload struct{ Toks []Value }

// ReachInfo is the payload of an Ideal/Reach value — a first-class dot-access
// node (m.a.b). Receiver is the token sequence of the base expression (empty
// for a receiverless reach); Segments are the .key / !.key steps. See
// design/REACH.10.md.
type ReachInfo struct {
	Receiver []Value
	Segments []ReachSeg
	Eval     bool // evaluate-by-default (like list Eval); quote/codequote suppress
	// unit caches this lens's compiled one-param unit (reach_unit.go). A
	// POINTER, so every copy of the Reach value shares one cache — the same
	// trick *BoruImpl plays for a signature's compiled ref. Unexported and
	// never rendered: canon builds a Reach's text from Segments alone, so this
	// field takes no part in equality, canon, or serialisation.
	unit *lensUnit
}

// ReachSeg is one step of a Reach: get (lenient) or getr (strict), with a
// literal key (KeyLit) or a computed-key expression (KeyExpr when Computed).
type ReachSeg struct {
	Getr     bool
	Computed bool
	KeyLit   Value
	KeyExpr  []Value
}

// InterpStringPayload carries the parts of a template-string
// interpolation. Wrapping []InterpPart.
type InterpStringPayload struct{ Parts []InterpPart }

// =================================================================
// External / domain payloads (Step 5f).
// =================================================================

// TimePayload carries a time.Time for Date / DateTime / Instant —
// the Parent discriminates which kind it is. Body is interface{}-typed
// here so the eng package doesn't pull the `time` import; the
// dedicated NewDate / NewDateTime / NewInstant constructors handle
// the typed wrapping.
type TimePayload struct {
	T any /* time.Time */
}

// DurationPayload carries a time.Duration for TimeOfDay /
// ClockDuration; same Parent-discriminator pattern as TimePayload.
type DurationPayload struct {
	D any /* time.Duration */
}

// TimezonePayload carries a *time.Location for Scalar/Time/Timezone.
type TimezonePayload struct {
	Loc any /* *time.Location */
}

// MaterializerPayload wraps an external Materializer (e.g. the
// production engine's QueryBuilder) into a payload-satisfying
// variant. The Materializer interface itself can't be required to
// satisfy Payload because its implementors live in other packages
// (lang/go/engine), so we wrap instead. AsList unwraps and calls
// .M.Materialize() to surface rows.
type MaterializerPayload struct{ M Materializer }

// =================================================================
// Type-literal / Carrier sentinels.
// =================================================================

// NonePayload is the sentinel for the VALUE `none` (the unique
// inhabitant of None). Replaces the old `noneSentinel` struct so
// it satisfies Payload. The TYPE LITERAL `None` carries Data == nil.
type NonePayload struct{}

// =================================================================
// Extension payload (Step 8 — for plugin types).
// =================================================================

// ExtensionPayload is the one explicit escape hatch from the closed
// Payload type space. Plugin types — Color, Date as an external,
// any host-supplied domain type — flow through this variant. The
// Body is opaque to the kernel; only the owning type's
// TypeBehavior dereferences it. Use `eng.NewExtension(t, body)` to
// construct.
//
// Inside the owning module, the Body assertion is unsafe in
// principle but lives in exactly one place (the module's Behavior
// implementation) and is tested there.
type ExtensionPayload struct{ Body any }

// NewExtension constructs a Value carrying an ExtensionPayload.
// Plugin modules use this when introducing a new type that flows
// through ExtensionPayload rather than dedicated kernel variants.
func NewExtension(t *Type, body any) Value {
	return Value{
		ID:     GenerateID(IDPrefixForType(t)),
		Parent: t,
		Data:   ExtensionPayload{Body: body},
	}
}

// HostTypeBody is an embeddable marker. A host module that introduces
// a type-kind through an Ideal (see eng/go/ideal.go) embeds this in
// the struct it stores, via ExtensionPayload, for a *constructed type*
// — as opposed to an instance. The kernel's type machinery
// (IsTypeBody, TypeOf, isTypeLike, InstallType) then recognises the
// value as a type without inspecting its concrete shape, which it
// cannot — the payload Body is opaque to the kernel. See
// design/IDEAL.10.md §6.
type HostTypeBody struct{}

func (HostTypeBody) hostTypeBody() {}

// =================================================================
// Marker methods.
//
// When Step 5g seals Payload (renames the alias to an interface
// with an unexported marker), every payload-bearing type needs a
// `payloadMarker()` method. The wrapper variants above get one
// each; eng-defined struct payloads (WordInfo, ForwardInfo, …)
// get theirs in the file where they're defined. The collected list
// below documents what's intended to satisfy Payload after sealing
// — it's a checklist, not active code yet.
//
// Wrapper variants:
//   IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
//   PathonPayload, ListPayload, MapPayload, ParenExprPayload,
//   InterpStringPayload, TimePayload, DurationPayload,
//   TimezonePayload, NonePayload, ExtensionPayload.
//
// Direct (eng-defined struct or pointer types):
//   WordInfo, ForwardInfo, MarkInfo, MoveInfo, ReturnCheckInfo,
//   DefCleanupInfo, ModuleDesc, FnDefInfo, FnUndefInfo,
//   DisjunctInfo, ChildTypeInfo, CodeEffectInfo, RecordTypeInfo, OptionsTypeInfo,
//   TableTypeInfo, TableData, ClassTypeInfo, ClassInstanceInfo,
//   *StoreInstanceInfo, *StoreShapeInfo, *TimeoutInfo,
//   *IntervalInfo, ErrorInfo, CalDurationData,
//   DepScalarInfo, Materializer (interface), noneSentinel
//   (legacy — to be removed in Step 5f).
// =================================================================

// payloadMarker is the unexported marker method that will close the
// Payload type space once Payload is renamed from an alias to a
// sealed interface (Step 5g). Defined as a no-op on every type that
// gets stored in Value.Data. Defining it here in payload.go keeps
// the catalogue centralised; the methods are dispatch-free.

// Wrapper-variant markers.
func (ReachInfo) payloadMarker()           {}
func (IntPayload) payloadMarker()          {}
func (FloatPayload) payloadMarker()        {}
func (BigIntPayload) payloadMarker()       {}
func (DecimalPayload) payloadMarker()      {}
func (StrPayload) payloadMarker()          {}
func (BoolPayload) payloadMarker()         {}
func (AtomPayload) payloadMarker()         {}
func (PathonPayload) payloadMarker()       {}
func (MicronPayload) payloadMarker()       {}
func (MicronTypeInfo) payloadMarker()      {}
func (ListPayload) payloadMarker()         {}
func (MapPayload) payloadMarker()          {}
func (ParenExprPayload) payloadMarker()    {}
func (InterpStringPayload) payloadMarker() {}
func (TimePayload) payloadMarker()         {}
func (DurationPayload) payloadMarker()     {}
func (TimezonePayload) payloadMarker()     {}
func (MaterializerPayload) payloadMarker() {}
func (NonePayload) payloadMarker()         {}
func (ExtensionPayload) payloadMarker()    {}

// Direct eng-defined struct markers.
func (WordInfo) payloadMarker()             {}
func (ForwardInfo) payloadMarker()          {}
func (MarkInfo) payloadMarker()             {}
func (MoveInfo) payloadMarker()             {}
func (SpliceInfo) payloadMarker()           {}
func (SugarInfo) payloadMarker()            {}
func (ReturnCheckInfo) payloadMarker()      {}
func (DefCleanupInfo) payloadMarker()       {}
func (FrameOpenInfo) payloadMarker()        {}
func (ModuleDesc) payloadMarker()           {}
func (FnDefInfo) payloadMarker()            {}
func (FnUndefInfo) payloadMarker()          {}
func (DisjunctInfo) payloadMarker()         {}
func (NegationInfo) payloadMarker()         {}
func (ChildTypeInfo) payloadMarker()        {}
func (RecordTypeInfo) payloadMarker()       {}
func (OptionsTypeInfo) payloadMarker()      {}
func (TableTypeInfo) payloadMarker()        {}
func (TableData) payloadMarker()            {}
func (ClassTypeInfo) payloadMarker()        {}
func (ClassInstanceInfo) payloadMarker()    {}
func (*SurfaceInfo) payloadMarker()         {}
func (*GenSpecInfo) payloadMarker()         {}
func (GenParam) payloadMarker()             {}
func (*TypeSchemaInfo) payloadMarker()      {}
func (GenInstRef) payloadMarker()           {}
func (*FlexListData) payloadMarker()        {}
func (XmlElementPayload) payloadMarker()    {}
func (XmlInterpPayload) payloadMarker()     {}
func (*FlexXmlData) payloadMarker()         {}
func (*StoreInstanceInfo) payloadMarker()   {}
func (ResourceTypeInfo) payloadMarker()     {}
func (ResourceInstanceInfo) payloadMarker() {}
func (*TimeoutInfo) payloadMarker()         {}
func (*IntervalInfo) payloadMarker()        {}
func (ErrorInfo) payloadMarker()            {}
func (CalDurationData) payloadMarker()      {}
func (DepScalarInfo) payloadMarker()        {}
func (PathonInfo) payloadMarker()           {} // legacy; replaced by PathonPayload at Step 5b but may still flow through some paths

// noneSentinel is kept for backward compat with code that reads it
// directly. NewNone() now produces NonePayload below.
func (noneSentinel) payloadMarker() {}

// PayloadBase is the S6 seal extension (design/ENG-FOUR-PIECE.0.md): a
// payload variant declared OUTSIDE core embeds PayloadBase to satisfy the
// sealed Payload interface, since payloadMarker itself is only definable
// beside the seal. Kernel-declared variants keep their direct markers.
type PayloadBase struct{}

func (PayloadBase) payloadMarker() {}

// Cross-piece payload DATA shapes re-homed to core (Stage 4f, the
// state-record principle): the sealed-payload catalogue is core-owned;
// the machinery that fills these stays in its piece.

// GuardFactInfo is the payload a check-mode paren evaluation attaches
// to a single Boolean carrier result: the group's ORIGINAL tokens,
// preserved so guard narrowing can see the `x is T` structure that
// evaluation reduced to a bare Boolean
// (design/checker-accuracy-review.10.md A3 — without it, the canonical
// `if (x is T) …` paren form narrowed nothing while the list form
// `if [x is T] …` narrowed fine). Check-mode only; the runtime never
// produces carriers.
type GuardFactInfo struct {
	PayloadBase
	Toks []Value
	// Prev is the payload the group actually reduced to (a BoolPayload for a
	// statically-decided cond), preserved so LiteralCondValue can still read
	// the literal truth value through the wrapper. Without it, wrapping a
	// decided cond in a GuardFactInfo carrier hid its value from the
	// unreachable-branch analysis.
	Prev Payload
}

// StoreShapeInfo is the abstract check-mode shape of ONE store-class
// container (a context Store layer, a FlexMap): the per-key JOIN of the
// value carriers written through it. A pointer payload deliberately —
// every Value copy of the carrier (a def binding, a stack duplicate, a
// nested-field read) aliases the SAME shape, mirroring the runtime
// aliasing of the mutable container it stands for. All mutation is
// join-only (JoinCarriers), so sandbox rollbacks that cannot un-mutate
// a shared pointee only ever WIDEN a claim — monotone, never unsound.
type StoreShapeInfo struct {
	PayloadBase
	// Scope is the context-stack depth at mint time for context-layer
	// shapes (stage-2 layering substrate); 0 for non-context containers
	// (flex maps, patrun instances).
	Scope int
	// KeyTypes records key → joined written-value carrier, with the
	// same JoinCarriers-on-rewrite semantics as the flat ContextTypes
	// map (so a single-store program reads back exactly what the flat
	// path recorded). nil until the first write.
	KeyTypes map[string]Value
	// Vals is the join of the UNKEYED value writes for containers whose
	// write key is not a string (a patrun's pattern-keyed `add`). The
	// zero Value (Parent == nil) means "nothing recorded".
	Vals Value
	// ValsPoisoned marks the unkeyed join unusable: a write the shape
	// cannot describe honestly (a dispatch-bearing value — a stored
	// lambda re-dispatched by the reader) poisons it, and readers keep
	// the pre-existing dynamic(Any) hatch.
	ValsPoisoned bool
	// DeclaredVal is the DECLARED element type of a typed container (a
	// `patrun T` table): nil for an inferred/untyped shape. When set, a
	// reader (`find`) surfaces `dynamic(DeclaredVal ∪ None)` directly,
	// bypassing the Vals join and its poisoning entirely — the type is a
	// declaration, not an inference.
	DeclaredVal *Type
}

// ClosurePayload is a runtime fn VALUE backed by a compiled body unit:
// OpPushClosure pushes one, and a higher-order word's native handler invokes
// it through the VM's re-entrant runner (via the InvokeBody seam) — never the
// interpreter (plan P2). Unit indexes Program.Fns; Captures are the
// construction-time lexical captures bound into the body's trailing local
// slots at invocation (empty for a capture-free body). InShape carries the
// unit's input convention (copied from CompiledFn at OpPushClosure) so the
// driving handler shapes each input correctly.
type ClosurePayload struct {
	PayloadBase
	// Prog is the PROGRAM Unit indexes, held opaquely: core sits below
	// compiler, so it boxes a *compiler.Program the same way BoruImpl.Compiled
	// boxes a *CompiledFnRef. It is load-bearing rather than bookkeeping — a
	// closure value is only interpretable relative to the program that minted
	// it, since Unit is an index into THAT program's Fns table and names a
	// different body, or none, against any other. The VM compares it against
	// the running program before indexing and hosts a mismatch in a nested
	// context (eng/go/vm_foreign_unit.go).
	//
	// The comparison became reachable when detached units started running
	// nested inside a live run: before that a closure could only be invoked
	// under the one program that was executing when it was pushed, which is
	// why the payload carried no identity for as long as it did. Nil means no
	// identity was recorded — only hand-built values reach that, and they read
	// as the running program, exactly as they did before this field.
	Prog     any
	Unit     int
	Captures []Value
	InShape  ClosureInShape
	// Ident is the closure's identity, minted once at OpPushClosure (one per
	// construction, as the interpreter mints one per `fn`; a sequence, not
	// an allocation) and copied with the value: `eq` compares it, and the
	// FnDefInfo the VM bridges the closure to for the interpreter's
	// dispatch carries it (NewFunctionIdentified), so two copies of one
	// closure — bridged or not — are one function. Zero for a hand-built
	// value: no identity.
	Ident FnIdentity
	// Render is the interpreter's formatFnDef string for the source fn this
	// closure compiled from (CompiledFn.Render, copied at OpPushClosure): a
	// closure VALUE then renders byte-identically to the interpreter's fn
	// value. Empty keeps the default rendering.
	Render string
	// RetTypes / RetPatterns / RetDecl / RetName / RetPos are the CALLBACK fn
	// value's own declared return contract, carried on the VALUE rather than
	// the unit.
	//
	// It has to be here, and the reason is measured. A named fn handed to a
	// higher-order word (`[1 2] each cbad/v`) lowers its body to a shared
	// closure unit, and that unit's RET knew nothing of the fn's declaration —
	// so a body violating its own `[Boolean]` answered the violating value
	// compiled where the interpreter raises type_error. Carrying the contract
	// on the UNIT instead was built and reverted: it needs a per-fn memo key,
	// and a distinct key alone (no contract at all) makes a SHARED closure unit
	// recompile and refuse on operand provenance, islanding conforming
	// callbacks and tripping TestListFoldCallbackOrderPin. Two fns with
	// identical bodies and inputs SHOULD share a unit; they differ only in what
	// their results must satisfy, which is a property of the value.
	//
	// Empty RetTypes means no contract (a raw token body, or an anonymous
	// lambda whose Returns=[Any] is a conservative placeholder rather than a
	// declaration).
	RetTypes    []*Type
	RetPatterns []*Value
	RetDecl     DeclSite
	RetName     string
	// RetTrim marks a closure handed through the fn-VALUE seam
	// (InvokeCallbackBody): the handler would hand a FnDefInfo to
	// InvokeCallbackFn, whose CallBoru path checks the declared TYPES over
	// the aligned residual and never raises on the COUNT
	// (enforceCallBoruReturns) — walk's hooks, the map-iteration each/fold,
	// filter's Function form. Off, the closure came through the TOKEN seam
	// (InvokeBody: each over a list, apply, a paren call), where the fn
	// value is stepped and __RC enforces the count. Set on the VALUE at the
	// seam, never on the stored closure: the same closure can cross both.
	RetTrim bool
	// RetPos is where the callback REFERENCE was written (`cbad/v`), which is
	// the position the interpreter anchors a return-contract error on: its
	// ReturnCheckInfo.Pos, stamped onto the Function value by stampResultPos
	// as the reference produces it. A compiled unit's RET has no call site to
	// read, so the closure path carries it here — without it the compiled
	// diagnostic agrees in value and taxonomy but loses the position, which
	// the full-corpus parity gate compares.
	RetPos SrcPos
}

// NewStoreShapeCarrier mints an abstract store-shaped carrier: a
// carrier of t (TStore, TFlexMap, …) whose Data is a FRESH
// *StoreShapeInfo. One mint per creation site / live layer — sharing
// the returned Value shares the shape (that is the aliasing model).
func NewStoreShapeCarrier(t *Type, scope int) Value {
	v := NewCarrier(t)
	v.Data = &StoreShapeInfo{Scope: scope}
	return v
}

// ClosureInShape tags HOW a higher-order word must present each per-invocation
// input to a compiled closure — the divergence the per-word callback
// conventions create. A map-iteration handler (each/fold/scan over a map) runs
// BOTH a token-quotation body (sees the bare value) and a lambda body (sees a
// KeyVal {k v i n}); the closure value alone cannot say which, so the compile
// records the shape on the unit and the handler reads it back here. The
// unambiguous handlers (filter list/map) build their own fixed shape and ignore
// it.
type ClosureInShape uint8
