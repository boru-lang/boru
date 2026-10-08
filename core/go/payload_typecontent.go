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
//   - FnDefInfo: a predicate-type candidate iff the owner is parented
//     at TFunction — a reparented fn value (`def f:Mapper …`) is not.
//   - FnUndefInfo / DisjunctInfo / NegationInfo / ChildTypeInfo /
//     RecordTypeInfo / OptionsTypeInfo / TableTypeInfo / TableData /
//     MaterializerPayload: keyed on the owner's dispatch identity,
//     mirroring their Is* predicates.

// --- type content ----------------------------------------------------------

func (DisjunctInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.ConformsTo(TDisjunct)
}

func (NegationInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.ConformsTo(TNegation)
}

// ChildTypeInfo carries a typed list ([:T]), a typed map ({:T}), or a
// bounded Type (`Type of [B]`), told apart by the owner's parent.
func (ChildTypeInfo) IsTypeContent(owner *Value) bool {
	if owner == nil || owner.Parent == nil {
		return false
	}
	return owner.Parent.Equal(TList) || owner.Parent.Equal(TMap) || owner.Parent.Equal(TType)
}

func (RecordTypeInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TMap)
}

func (OptionsTypeInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TMap)
}

func (TableTypeInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TList)
}

// A concrete table and a materializer count as table TYPES under a
// List parent (IsTableType's admission), so they answer the same way.
func (TableData) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TList)
}

func (MaterializerPayload) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TList)
}

func (ClassTypeInfo) IsTypeContent(*Value) bool   { return true }
func (*SurfaceInfo) IsTypeContent(*Value) bool    { return true }
func (*TypeSchemaInfo) IsTypeContent(*Value) bool { return true }
func (DepScalarInfo) IsTypeContent(*Value) bool   { return true }
func (MicronTypeInfo) IsTypeContent(*Value) bool  { return true }

// A fn-shape (fnsig) value is type content at its own dispatch
// identity; a predicate candidate only while it still carries the
// generic Function identity.
func (FnUndefInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TFnUndef)
}

func (FnDefInfo) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TFunction)
}

// A record shape iff the OrderedMap is Implicit-flagged under a Map
// parent (IsImplicitMap); a concrete map is a value.
func (mp MapPayload) IsTypeContent(owner *Value) bool {
	return owner != nil && owner.Parent.Equal(TMap) && mp.M != nil && mp.M.Implicit
}

// A host type body iff the nested Body carries the hostTypeBody
// marker; a host instance is a value (the payload seal holds — only
// the one bit is read).
func (ep ExtensionPayload) IsTypeContent(*Value) bool {
	_, ok := ep.Body.(interface{ hostTypeBody() })
	return ok
}

// --- ordinary values -------------------------------------------------------

func (ReachInfo) IsTypeContent(*Value) bool            { return false }
func (IntPayload) IsTypeContent(*Value) bool           { return false }
func (FloatPayload) IsTypeContent(*Value) bool         { return false }
func (BigIntPayload) IsTypeContent(*Value) bool        { return false }
func (DecimalPayload) IsTypeContent(*Value) bool       { return false }
func (StrPayload) IsTypeContent(*Value) bool           { return false }
func (BoolPayload) IsTypeContent(*Value) bool          { return false }
func (AtomPayload) IsTypeContent(*Value) bool          { return false }
func (PathonPayload) IsTypeContent(*Value) bool        { return false }
func (MicronPayload) IsTypeContent(*Value) bool        { return false }
func (ListPayload) IsTypeContent(*Value) bool          { return false }
func (ParenExprPayload) IsTypeContent(*Value) bool     { return false }
func (InterpStringPayload) IsTypeContent(*Value) bool  { return false }
func (TimePayload) IsTypeContent(*Value) bool          { return false }
func (DurationPayload) IsTypeContent(*Value) bool      { return false }
func (TimezonePayload) IsTypeContent(*Value) bool      { return false }
func (NonePayload) IsTypeContent(*Value) bool          { return false }
func (WordInfo) IsTypeContent(*Value) bool             { return false }
func (ForwardInfo) IsTypeContent(*Value) bool          { return false }
func (MarkInfo) IsTypeContent(*Value) bool             { return false }
func (MoveInfo) IsTypeContent(*Value) bool             { return false }
func (SpliceInfo) IsTypeContent(*Value) bool           { return false }
func (SugarInfo) IsTypeContent(*Value) bool            { return false }
func (ReturnCheckInfo) IsTypeContent(*Value) bool      { return false }
func (DefCleanupInfo) IsTypeContent(*Value) bool       { return false }
func (FrameOpenInfo) IsTypeContent(*Value) bool        { return false }
func (ModuleDesc) IsTypeContent(*Value) bool           { return false }
func (ClassInstanceInfo) IsTypeContent(*Value) bool    { return false }
func (DispatchModInfo) IsTypeContent(*Value) bool      { return false }
func (ErrorInfo) IsTypeContent(*Value) bool            { return false }
func (GenInstRef) IsTypeContent(*Value) bool           { return false }
func (GenParam) IsTypeContent(*Value) bool             { return false }
func (PathonInfo) IsTypeContent(*Value) bool           { return false }
func (PayloadBase) IsTypeContent(*Value) bool          { return false }
func (ResourceInstanceInfo) IsTypeContent(*Value) bool { return false }
func (ResourceTypeInfo) IsTypeContent(*Value) bool     { return false }
func (XmlElementPayload) IsTypeContent(*Value) bool    { return false }
func (XmlInterpPayload) IsTypeContent(*Value) bool     { return false }
func (CalDurationData) IsTypeContent(*Value) bool      { return false }
func (noneSentinel) IsTypeContent(*Value) bool         { return false }
func (*FlexListData) IsTypeContent(*Value) bool        { return false }
func (*FlexXmlData) IsTypeContent(*Value) bool         { return false }
func (*GenSpecInfo) IsTypeContent(*Value) bool         { return false }
func (*IntervalInfo) IsTypeContent(*Value) bool        { return false }
func (*StoreInstanceInfo) IsTypeContent(*Value) bool   { return false }
func (*TimeoutInfo) IsTypeContent(*Value) bool         { return false }
func (*WeakFlexListData) IsTypeContent(*Value) bool    { return false }
func (*WeakFlexMapData) IsTypeContent(*Value) bool     { return false }
func (*WeakFlexXmlData) IsTypeContent(*Value) bool     { return false }
