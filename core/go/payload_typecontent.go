package core

// IsTypeContent — the sealed-payload half of the one type-recognition
// seam (design/legacy/TYPE-REPRESENTATION.1.ignore §N4). Every Payload variant
// answers "am I a TYPE's structural content?" here, replacing the
// 18-arm shape enumeration IsTypeBody used to carry. The answers below
// mirror that enumeration exactly (the per-shape predicates' payload
// AND owner guards), pinned by TestIsTypeContentMirrorsLegacy.
//
// Most payloads answer with a constant. The exceptions answer from
// value state, because their payload variant is SHARED between types
// and ordinary values:
//   - MapPayload: a record shape iff its OrderedMap carries the
//     Implicit flag under a Map parent (IsImplicitMap) — a concrete
//     map is a value.
//   - ExtensionPayload: a host type body iff the nested Body carries
//     the hostTypeBody marker (design/IDEAL.10.md §13) — a host
//     instance is a value.
//   - FnDefInfo: a predicate-type candidate iff the owning value is
//     parented at TFunction — a reparented fn value (`def f:Mapper …`)
//     is not.
//   - FnUndefInfo / DisjunctInfo / NegationInfo / ChildTypeInfo /
//     RecordTypeInfo / OptionsTypeInfo / TableTypeInfo / TableData /
//     MaterializerPayload: keyed on the owning value's dispatch identity
//     (its Parent, the one fact the seam is handed), mirroring their Is*
//     predicates.

// --- type content ----------------------------------------------------------

func (DisjunctInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.ConformsTo(TDisjunct)
}

func (NegationInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.ConformsTo(TNegation)
}

// ChildTypeInfo carries a typed list ([:T]), a typed map ({:T}), or a
// bounded Type (`Type of [B]`), told apart by the owner's parent.
func (ChildTypeInfo) IsTypeContent(parent *Type) bool {
	if parent == nil {
		return false
	}
	return parent.Equal(TList) || parent.Equal(TMap) || parent.Equal(TType)
}

func (RecordTypeInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TMap)
}

func (OptionsTypeInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TMap)
}

func (TableTypeInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TList)
}

// A concrete table and a materializer count as table TYPES under a
// List parent (IsTableType's admission), so they answer the same way.
func (TableData) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TList)
}

func (MaterializerPayload) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TList)
}

func (ClassTypeInfo) IsTypeContent(*Type) bool   { return true }
func (*SurfaceInfo) IsTypeContent(*Type) bool    { return true }
func (*TypeSchemaInfo) IsTypeContent(*Type) bool { return true }
func (DepScalarInfo) IsTypeContent(*Type) bool   { return true }
func (MicronTypeInfo) IsTypeContent(*Type) bool  { return true }

// A fn-shape (fnsig) value is type content at its own dispatch
// identity; a predicate candidate only while it still carries the
// generic Function identity.
func (FnUndefInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TFnUndef)
}

func (FnDefInfo) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TFunction)
}

// A record shape iff the OrderedMap is Implicit-flagged under a Map
// parent (IsImplicitMap); a concrete map is a value.
func (mp MapPayload) IsTypeContent(parent *Type) bool {
	return parent != nil && parent.Equal(TMap) && mp.M != nil && mp.M.Implicit
}

// A host type body iff the nested Body carries the hostTypeBody
// marker; a host instance is a value (the payload seal holds — only
// the one bit is read).
func (ep ExtensionPayload) IsTypeContent(*Type) bool {
	_, ok := ep.Body.(interface{ hostTypeBody() })
	return ok
}

// --- ordinary values -------------------------------------------------------

func (ReachInfo) IsTypeContent(*Type) bool            { return false }
func (IntPayload) IsTypeContent(*Type) bool           { return false }
func (FloatPayload) IsTypeContent(*Type) bool         { return false }
func (BigIntPayload) IsTypeContent(*Type) bool        { return false }
func (DecimalPayload) IsTypeContent(*Type) bool       { return false }
func (StrPayload) IsTypeContent(*Type) bool           { return false }
func (BoolPayload) IsTypeContent(*Type) bool          { return false }
func (AtomPayload) IsTypeContent(*Type) bool          { return false }
func (PathonPayload) IsTypeContent(*Type) bool        { return false }
func (MicronPayload) IsTypeContent(*Type) bool        { return false }
func (ListPayload) IsTypeContent(*Type) bool          { return false }
func (ParenExprPayload) IsTypeContent(*Type) bool     { return false }
func (InterpStringPayload) IsTypeContent(*Type) bool  { return false }
func (TimePayload) IsTypeContent(*Type) bool          { return false }
func (DurationPayload) IsTypeContent(*Type) bool      { return false }
func (TimezonePayload) IsTypeContent(*Type) bool      { return false }
func (NonePayload) IsTypeContent(*Type) bool          { return false }
func (WordInfo) IsTypeContent(*Type) bool             { return false }
func (ForwardInfo) IsTypeContent(*Type) bool          { return false }
func (MarkInfo) IsTypeContent(*Type) bool             { return false }
func (MoveInfo) IsTypeContent(*Type) bool             { return false }
func (SpliceInfo) IsTypeContent(*Type) bool           { return false }
func (SugarInfo) IsTypeContent(*Type) bool            { return false }
func (ReturnCheckInfo) IsTypeContent(*Type) bool      { return false }
func (DefCleanupInfo) IsTypeContent(*Type) bool       { return false }
func (FrameOpenInfo) IsTypeContent(*Type) bool        { return false }
func (LoopOpenInfo) IsTypeContent(*Type) bool         { return false }
func (ModuleDesc) IsTypeContent(*Type) bool           { return false }
func (ClassInstanceInfo) IsTypeContent(*Type) bool    { return false }
func (DispatchModInfo) IsTypeContent(*Type) bool      { return false }
func (ErrorInfo) IsTypeContent(*Type) bool            { return false }
func (GenInstRef) IsTypeContent(*Type) bool           { return false }
func (GenParam) IsTypeContent(*Type) bool             { return false }
func (PathonInfo) IsTypeContent(*Type) bool           { return false }
func (PayloadBase) IsTypeContent(*Type) bool          { return false }
func (ResourceInstanceInfo) IsTypeContent(*Type) bool { return false }
func (ResourceTypeInfo) IsTypeContent(*Type) bool     { return false }
func (XmlElementPayload) IsTypeContent(*Type) bool    { return false }
func (XmlInterpPayload) IsTypeContent(*Type) bool     { return false }
func (CalDurationData) IsTypeContent(*Type) bool      { return false }
func (noneSentinel) IsTypeContent(*Type) bool         { return false }
func (*FlexListData) IsTypeContent(*Type) bool        { return false }
func (*FlexXmlData) IsTypeContent(*Type) bool         { return false }
func (*GenSpecInfo) IsTypeContent(*Type) bool         { return false }
func (*IntervalInfo) IsTypeContent(*Type) bool        { return false }
func (*StoreInstanceInfo) IsTypeContent(*Type) bool   { return false }
func (*TimeoutInfo) IsTypeContent(*Type) bool         { return false }
func (*WeakFlexListData) IsTypeContent(*Type) bool    { return false }
func (*WeakFlexMapData) IsTypeContent(*Type) bool     { return false }
func (*WeakFlexXmlData) IsTypeContent(*Type) bool     { return false }
