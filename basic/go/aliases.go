// Package basic is the boru base language layer: the fundamental
// words (stack, definition, control, type-generics) and the predefined
// global content types (the Scalar/Time family, Resource/Entity),
// registered against the kernel.
//
// It depends on CORE and (for tests) parser — and on nothing else
// (ADR-013 rule 1, as amended 2026-08-08). The checker is no longer
// among them: defining language types and words is expressible in the
// interpreter's own primitives, so where a word here has an analysis
// half it is written in core's vocabulary (carrier_join.go,
// carrier_body.go, guard_narrow.go) and where it genuinely needs the
// analysis PASS — the fn / loop body models — it goes through a
// core-owned seam slot (core.RunFnBodyAnalysis, core.RunLoopBodyAnalysis).
//
// This file is the core shim: it re-exports core's types and functions
// so the word files in this package can stay module-agnostic, mirroring
// lang/go/native/aliases.go.
package basic

import (
	core "github.com/boru-lang/boru/core/go"
)

// *Type aliases — every exported type from core is re-exported here.
type (
	BranchRecord       = core.BranchRecord
	PendingResidue     = core.PendingResidue
	BranchJoin         = core.BranchJoin
	CodeEffectInfo     = core.CodeEffectInfo
	EmitRecorder       = core.EmitRecorder
	DeqIndex           = core.DeqIndex
	EmitFragmentRef    = core.EmitFragmentRef
	BoruError          = core.BoruError
	RenderOpts         = core.RenderOpts
	DiagSpan           = core.DiagSpan
	DiagSuggestion     = core.DiagSuggestion
	CalDurationData    = core.CalDurationData
	CheckDiagnostic    = core.CheckDiagnostic
	CheckFullStackFunc = core.CheckFullStackFunc
	CheckSeverity      = core.CheckSeverity
	CheckState         = core.CheckState
	ChildTypeInfo      = core.ChildTypeInfo
	ReachInfo          = core.ReachInfo
	ReachSeg           = core.ReachSeg
	ChildEntry         = core.ChildEntry
	// Sealed Payload variants (post Step 5).
	Payload              = core.Payload
	IntPayload           = core.IntPayload
	FloatPayload         = core.FloatPayload
	StrPayload           = core.StrPayload
	BoolPayload          = core.BoolPayload
	AtomPayload          = core.AtomPayload
	BigIntPayload        = core.BigIntPayload
	DecimalPayload       = core.DecimalPayload
	PathonPayload        = core.PathonPayload
	MicronPayload        = core.MicronPayload
	MicronTypeInfo       = core.MicronTypeInfo
	ListPayload          = core.ListPayload
	FlexListData         = core.FlexListData
	MapPayload           = core.MapPayload
	ExtensionPayload     = core.ExtensionPayload
	MaterializerPayload  = core.MaterializerPayload
	TimePayload          = core.TimePayload
	DurationPayload      = core.DurationPayload
	TimezonePayload      = core.TimezonePayload
	DefCleanupInfo       = core.DefCleanupInfo
	DepBound             = core.DepBound
	DepKind              = core.DepKind
	DepScalarInfo        = core.DepScalarInfo
	DisjunctInfo         = core.DisjunctInfo
	Engine               = core.Engine
	ErrorInfo            = core.ErrorInfo
	FlowCtrl             = core.FlowCtrl
	CapturedBinding      = core.CapturedBinding
	FnDefInfo            = core.FnDefInfo
	FnParam              = core.FnParam
	FnSig                = core.FnSig
	FnSigSpec            = core.FnSigSpec
	FnUndefInfo          = core.FnUndefInfo
	Loop                 = core.Loop
	ForwardInfo          = core.ForwardInfo
	Handler              = core.Handler
	IfCont               = core.IfCont
	InterpPart           = core.InterpPart
	IntervalInfo         = core.IntervalInfo
	MarkInfo             = core.MarkInfo
	MatchResult          = core.MatchResult
	Materializer         = core.Materializer
	TypeBehavior         = core.TypeBehavior
	UnifyError           = core.UnifyError
	ModuleDesc           = core.ModuleDesc
	MoveInfo             = core.MoveInfo
	NativeFunc           = core.NativeFunc
	SigImpl              = core.SigImpl  // sealed run-impl sum (GoImpl | BoruImpl)
	GoImpl               = core.GoImpl   // native / internal Go-handler implementation
	BoruImpl             = core.BoruImpl // boru body implementation (module ref / lambda / installed fn)
	GoOpt                = core.GoOpt    // optional dispatch knob for Go(...)
	CompileEffect        = core.CompileEffect
	CallableSpec         = core.CallableSpec
	StoredBodySpec       = core.StoredBodySpec
	ClassInstanceInfo    = core.ClassInstanceInfo
	ClassTypeInfo        = core.ClassTypeInfo
	ResourceInstanceInfo = core.ResourceInstanceInfo
	ResourceTypeInfo     = core.ResourceTypeInfo
	SurfaceInfo          = core.SurfaceInfo
	GenParam             = core.GenParam
	GenSpecInfo          = core.GenSpecInfo
	TypeSchemaInfo       = core.TypeSchemaInfo
	GenInstRef           = core.GenInstRef
	SchemaKind           = core.SchemaKind
	OptionsTypeInfo      = core.OptionsTypeInfo
	OrderedMap           = core.OrderedMap
	PathonInfo           = core.PathonInfo
	ReadList             = core.ReadList
	ReadMap              = core.ReadMap
	RecordTypeInfo       = core.RecordTypeInfo
	Registry             = core.Registry
	TapeConfig           = core.TapeConfig
	DefTable             = core.DefTable
	ReturnCheckInfo      = core.ReturnCheckInfo
	ReturnsFunc          = core.ReturnsFunc
	Signature            = core.Signature
	SrcPos               = core.SrcPos
	StoreInstanceInfo    = core.StoreInstanceInfo
	TableData            = core.TableData
	TableTypeInfo        = core.TableTypeInfo
	TimeoutInfo          = core.TimeoutInfo
	TraceCallback        = core.TraceCallback
	Type                 = core.Type
	Value                = core.Value
	WordInfo             = core.WordInfo
)

// Well-known *Type values — re-exported for convenience.
var (
	DefaultBehavior = core.DefaultBehavior
	TAny            = core.TAny
	TAtom           = core.TAtom
	TBoolean        = core.TBoolean
	// TCalendarDuration / TClockDuration / TDate / TDateTime / TDuration
	// moved to lang/go/engine/native_temporal.go (Step 8) — declared
	// directly in this package, no eng alias needed.
	TFloat      = core.TFloat
	TDefCleanup = core.TDefCleanup
	TDisjunct   = core.TDisjunct
	TEnum       = core.TEnum
	TError      = core.TError
	// TFetchFunction / TFetchRequest / TFetchResponse moved to
	// lang/go/native/fetch.go at Step 8. References use native.TFetch*
	// directly; this aliases block no longer re-exports them.
	TFnUndef  = core.TFnUndef
	TForward  = core.TForward
	TFunction = core.TFunction
	TInspect  = core.TInspect
	// TInstant moved to lang/go/engine/native_temporal.go (Step 8).
	TInteger      = core.TInteger
	TBigInteger   = core.TBigInteger
	TBigDecimal   = core.TBigDecimal
	TInternal     = core.TInternal
	TInterpString = core.TInterpString
	// TInterval moved to lang/go/engine/native_misc.go (Step 8).
	TList         = core.TList
	TListArgs     = core.TListArgs
	TFlexList     = core.TFlexList
	TWeakFlexList = core.TWeakFlexList
	TMap          = core.TMap
	TFlexMap      = core.TFlexMap
	TWeakFlexMap  = core.TWeakFlexMap
	TXml          = core.TXml
	TFlexXml      = core.TFlexXml
	TWeakFlexXml  = core.TWeakFlexXml
	TMark         = core.TMark
	// TMatrix moved to lang/go/internal/nativemod/matrix.go (Step 8).
	TMove           = core.TMove
	TNever          = core.TNever
	TNode           = core.TNode
	TNone           = core.TNone
	TNumber         = core.TNumber
	TClass          = core.TClass
	TSurface        = core.TSurface
	TSelf           = core.TSelf
	TTypeParam      = core.TTypeParam
	TGenSpec        = core.TGenSpec
	TGenParam       = core.TGenParam
	TIdeal          = core.TIdeal
	TOpenParen      = core.TOpenParen
	TOptions        = core.TOptions
	TParenExpr      = core.TParenExpr
	TReach          = core.TReach
	TPathon         = core.TPathon
	TMicron         = core.TMicron
	TEmailon        = core.TEmailon
	TUrlon          = core.TUrlon
	TIpon           = core.TIpon
	THoston         = core.THoston
	TSemveron       = core.TSemveron
	TCidron         = core.TCidron
	TMacon          = core.TMacon
	TColoron        = core.TColoron
	TMimon          = core.TMimon
	TQion           = core.TQion
	TPhonon         = core.TPhonon
	TRecord         = core.TRecord
	TResource       = core.TResource
	TResourceEntity = core.TResourceEntity
	TReturnCheck    = core.TReturnCheck
	TScalar         = core.TScalar
	TStore          = core.TStore
	TStoreSystem    = core.TStoreSystem
	TString         = core.TString
	TStringEmpty    = core.TStringEmpty
	TStringProper   = core.TStringProper
	TTable          = core.TTable
	// TTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// TTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// TTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	TType = core.TType
	TWord = core.TWord
)

// Severity constants for diagnostic classification.
const (
	SeverityError   = core.SeverityError
	SeverityWarning = core.SeverityWarning
	SeverityInfo    = core.SeverityInfo
)

// Generic-schema kind constants (core.SchemaKind).
const (
	SchemaRecord = core.SchemaRecord
	SchemaClass  = core.SchemaClass
	SchemaFnSig  = core.SchemaFnSig
	SchemaFn     = core.SchemaFn
)

// Engine-level constants.
const (
	MaxArgs                = core.MaxArgs
	DefaultCheckStepBudget = core.DefaultCheckStepBudget

	DepGT  = core.DepGT
	DepGTE = core.DepGTE
	DepLT  = core.DepLT
	DepLTE = core.DepLTE

	// Compile-effect classifications (core.CompileEffect) for the bytecode
	// recorder — declared on a Signature instead of a name-keyed eng table.
	CompileDefault            = core.CompileDefault
	CompileReadsFn            = core.CompileReadsFn
	CompileStoresFn           = core.CompileStoresFn
	CompileModuleFold         = core.CompileModuleFold
	CompileIslandPure         = core.CompileIslandPure
	CompileScalarFold         = core.CompileScalarFold
	CompileFallbackBody       = core.CompileFallbackBody
	CompileQuoteInert         = core.CompileQuoteInert
	CompileQuoteKey           = core.CompileQuoteKey
	CompileDiverges           = core.CompileDiverges
	CompileValueDiverges      = core.CompileValueDiverges
	CompileStoresBody         = core.CompileStoresBody
	CompileStoresBodyList     = core.CompileStoresBodyList
	CompileFnHandlerStrict    = core.CompileFnHandlerStrict
	CompileResteps            = core.CompileResteps
	CompileOwnLowering        = core.CompileOwnLowering
	CompileRunsBodyIsolated   = core.CompileRunsBodyIsolated
	CompileRunsBodyOnRegistry = core.CompileRunsBodyOnRegistry
	CompileDynBody            = core.CompileDynBody

	// CallableSpec.BodyOut's whole-residual sentinel (core.BodyOutResidual):
	// the driving handler returns the body's entire residual (`do`).
	BodyOutResidual = core.BodyOutResidual
)

// Flow-control signal values exposed by the engine. These travel
// through Registry.FlowCtrl, not the error channel.
const (
	FlowNone     = core.FlowNone
	FlowBreak    = core.FlowBreak
	FlowContinue = core.FlowContinue
)

// Function re-exports — every exported borueng function.
var (
	AnalyseLoopBody          = core.RunLoopBodyAnalysis
	AsAtom                   = core.AsAtom
	AsChildType              = core.AsChildType
	AsDisjunct               = core.AsDisjunct
	AsError                  = core.AsError
	AsInterpString           = core.AsInterpString
	AsList                   = core.AsList
	AsMap                    = core.AsMap
	AsMutableList            = core.AsMutableList
	AsMutableMap             = core.AsMutableMap
	AsFlexList               = core.AsFlexList
	AsFlexXml                = core.AsFlexXml
	AsWeakFlexMap            = core.AsWeakFlexMap
	AsWeakFlexList           = core.AsWeakFlexList
	AsWeakFlexXml            = core.AsWeakFlexXml
	IsWeakFlexNode           = core.IsWeakFlexNode
	WeakRefusalError         = core.WeakRefusalError
	IsXmlValue               = core.IsXmlValue
	XmlParts                 = core.XmlParts
	AsClassInstance          = core.AsClassInstance
	AsClassType              = core.AsClassType
	AsResourceInstance       = core.AsResourceInstance
	AsResourceType           = core.AsResourceType
	AsOptionsType            = core.AsOptionsType
	AsRecordType             = core.AsRecordType
	AsStore                  = core.AsStore
	AsTableType              = core.AsTableType
	IsAtom                   = core.IsAtom
	IsError                  = core.IsError
	IsImplicitMap            = core.IsImplicitMap
	Canon                    = core.Canon
	IsNoneShape              = core.IsNoneShape
	IsClassInstance          = core.IsClassInstance
	IsResourceInstance       = core.IsResourceInstance
	IsResourceType           = core.IsResourceType
	IsFlatInstance           = core.IsFlatInstance
	IsOptionsType            = core.IsOptionsType
	PendingLiteralSteps      = core.PendingLiteralSteps
	NewReach                 = core.NewReach
	NewReachFromKeys         = core.NewReachFromKeys
	ApplyReach               = core.ApplyReach
	IsPathon                 = core.IsPathon
	IsRecordType             = core.IsRecordType
	IsStore                  = core.IsStore
	IsTableType              = core.IsTableType
	IsTypeValue              = core.IsTypeValue
	IsFlexMap                = core.IsFlexMap
	IsFlexList               = core.IsFlexList
	IsFlexNode               = core.IsFlexNode
	IsValueOfType            = core.IsValueOfType
	AsBoolean                = core.AsBoolean
	AsFloat                  = core.AsFloat
	AsInteger                = core.AsInteger
	AsNumber                 = core.AsNumber
	AsFloatApprox            = core.AsFloatApprox
	AsBigInteger             = core.AsBigInteger
	AsBigDecimal             = core.AsBigDecimal
	AsPathon                 = core.AsPathon
	AsString                 = core.AsString
	BaseValueForConstraint   = core.BaseValueForConstraint
	BoundToKind              = core.BoundToKind
	CoerceBoolean            = core.CoerceBoolean
	CompareValues            = core.CompareValues
	CowSet                   = core.CowSet
	CowDel                   = core.CowDel
	ExpandOptionalSigs       = core.ExpandOptionalSigs
	parseFnParams            = core.ParseFnParams
	resolveTypeName          = core.ResolveTypeName
	TandValues               = core.TandValues
	parseFnDef               = core.ParseFnDef
	parseFnUndefSpec         = core.ParseFnUndefSpec
	ValidateWordName         = core.ValidateWordName
	TypeOf                   = core.TypeOf
	TypeNameOf               = core.TypeNameOf
	TypePathOf               = core.TypePathOf
	ValueType                = core.ValueType
	NewNone                  = core.NewNone
	NewNegation              = core.NewNegation
	IsNegation               = core.IsNegation
	AsNegation               = core.AsNegation
	FormatFloat              = core.FormatFloat
	NewTypedListWithElements = core.NewTypedListWithElements
	NewTypedMapWithEntries   = core.NewTypedMapWithEntries
	FlattenDisjunctAlts      = core.FlattenDisjunctAlts
	FnDefHasSig              = core.FnDefHasSig
	NewGenSpec               = core.NewGenSpec
	IsGenSpec                = core.IsGenSpec
	AsGenSpec                = core.AsGenSpec
	NewGenParamValue         = core.NewGenParamValue
	IsGenParamValue          = core.IsGenParamValue
	AsGenParamValue          = core.AsGenParamValue
	IsTypeSchema             = core.IsTypeSchema
	AsTypeSchema             = core.AsTypeSchema
	NewGenInstRef            = core.NewGenInstRef
	IsGenInstRef             = core.IsGenInstRef
	AsGenInstRef             = core.AsGenInstRef
	MintTypeParam            = core.MintTypeParam
	PushGenBindings          = core.PushGenBindings
	PopGenBindings           = core.PopGenBindings
	InstantiateSchema        = core.InstantiateSchema
	AsSurfaceType            = core.AsSurfaceType
	IsSurfaceType            = core.IsSurfaceType
	FnDefsOverlap            = core.FnDefsOverlap
	FnSigMatchesSpec         = core.FnSigMatchesSpec
	FnSigSatisfiesSpec       = core.FnSigSatisfiesSpec
	FnUndefMatchesFnDef      = core.FnUndefMatchesFnDef
	BuiltinIDForPath         = core.BuiltinIDForPath
	MintTestType             = core.MintTestType
	NewSugar                 = core.NewSugar
	AsSugar                  = core.AsSugar
	SugarExpansion           = core.SugarExpansion
	GenerateID               = core.GenerateID
	GenerateObjectTypeID     = core.GenerateObjectTypeID
	BeginIDMintScope         = core.BeginIDMintScope
	ResetModuleExportGrowth  = core.ResetModuleExportGrowth
	MakeClassFieldValue      = core.MakeClassFieldValue
	MakeClassInstance        = core.MakeClassInstance
	MakeResource             = core.MakeResource
	ClassFields              = core.ClassFields
	FlatInstanceFields       = core.FlatInstanceFields
	InstallDef               = core.InstallDef
	InstallFnDef             = core.InstallFnDef
	InstallWordExtension     = core.InstallWordExtension
	TransplantExtension      = core.TransplantExtension
	IsWordExtension          = core.IsWordExtension
	IsSealedWord             = core.IsSealedWord
	HasLockedSigs            = core.HasLockedSigs
	NewWordExtension         = core.NewWordExtension
	// Owner sentinels for the ownership-anchored signature rules
	// (design/OPEN-WORDS.1.md §4).
	OwnerKernel              = core.OwnerKernel
	OwnerLang                = core.OwnerLang
	OwnerProgram             = core.OwnerProgram
	NewPathonFromString      = core.NewPathonFromString
	IsCapitalisedName        = core.IsCapitalisedName
	IsModuleFamilyValue      = core.IsModuleFamilyValue
	IsSteplessWindow         = core.IsSteplessWindow
	IsFnValueResidual        = core.IsFnValueResidual
	IsGenMemoName            = core.IsGenMemoName
	IsRecordShape            = core.IsRecordShape
	JoinCarrierStacks        = core.JoinCarrierStacks
	JoinCarriers             = core.JoinCarriers
	FoldVariadicArms         = core.FoldVariadicArms
	MakeBoruError            = core.MakeBoruError
	ExitCode                 = core.ExitCode
	IsInternalError          = core.IsInternalError
	IsVMDefer                = core.IsVMDefer
	NewExitError             = core.NewExitError
	RenderCheckDiagnostic    = core.RenderCheckDiagnostic
	MapFieldBoolean          = core.MapFieldBoolean
	MapFieldFloat            = core.MapFieldFloat
	MapFieldInteger          = core.MapFieldInteger
	MapFieldString           = core.MapFieldString
	MatchSignature           = core.MatchSignature
	NewSplice                = core.NewSplice
	LiteralCondValue         = core.LiteralCondValue
	BoolWord                 = core.BoolWord
	ApplyGuardNarrowing      = core.ApplyGuardNarrowing
	ApplyComplementNarrowing = core.ApplyComplementNarrowing
	RunCarrierBodyWithDefs   = core.RunCarrierBodyWithDefs
	RunCarrierArmBody        = core.RunCarrierArmBody
	RunCarrierCondBody       = core.RunCarrierCondBody
	InstallJoinedDefs        = core.InstallJoinedDefs
	InstallTakenArmDefs      = core.InstallTakenArmDefs
	InstallDecidedJoinedDefs = core.InstallDecidedJoinedDefs
	New                      = core.New
	RunPooled                = core.RunPooled
	RunPooledTop             = core.RunPooledTop
	RunResolved              = core.RunResolved
	InvokeBody               = core.InvokeBody
	CallRegion               = core.CallRegion
	BodyTokens               = core.BodyTokens
	IsLoopRegion             = core.IsLoopRegion
	ConvertIdealToMap        = core.ConvertIdealToMap
	ConvertIdealToList       = core.ConvertIdealToList
	CloneValue               = core.CloneValue
	NewSyncWriter            = core.NewSyncWriter
	NewReadList              = core.NewReadList
	ContextStoreLookup       = core.ContextStoreLookup
	ExactEqual               = core.ExactEqual
	TraceWrap                = core.TraceWrap
	FlexibleMatch            = core.FlexibleMatch
	TraceVisibleLen          = core.TraceVisibleLen
	TraceColorize            = core.TraceColorize
	RunTrace                 = core.RunTrace
	WalkBodyWords            = core.WalkBodyWords
	PadRight                 = core.PadRight
	DeepEqual                = core.DeepEqual
	FormatForPrint           = core.FormatForPrint
	FormatValueJSON          = core.FormatValueJSON
	NewAtom                  = core.NewAtom
	AtomReferent             = core.AtomReferent
	NewBoolean               = core.NewBoolean
	// NewCalendarDuration moved to lang/go/engine/native_temporal.go (Step 8).
	StampModuleCallGates = core.StampModuleCallGates
	// The fn-carrier side table moved down to core (check_fncarrier.go)
	// so the engine's compile-pass undefined-word branches can consult it.
	NoteCheckFnCarrierBind   = core.NoteCheckFnCarrierBind
	CheckFnCarrierBind       = core.CheckFnCarrierBind
	CheckFnCarrierBindDepth  = core.CheckFnCarrierBindDepth
	CheckFnCarrierBoundName  = core.CheckFnCarrierBoundName
	DropCheckFnCarrierBind   = core.DropCheckFnCarrierBind
	ResetCheckFnCarrierBinds = core.ResetCheckFnCarrierBinds
	NewCarrierTypedListValue = core.NewCarrierTypedListValue
	CarrierTypedListOf       = core.CarrierTypedListOf
	NewDynamicCarrierValue   = core.NewDynamicCarrierValue
	// NewClockDuration moved to lang/go/engine/native_temporal.go (Step 8).
	// NewDate / NewDateTime moved to lang/go/engine/native_temporal.go (Step 8).
	NewFloat      = core.NewFloat
	NewDefCleanup = core.NewDefCleanup
	NewDepScalar  = core.NewDepScalar
	NewDisjunct   = core.NewDisjunct
	NewEnum       = core.NewEnum
	NewError      = core.NewError
	NewEvalList   = core.NewEvalList
	NewEvalMap    = core.NewEvalMap
	NewFnUndef    = core.NewFnUndef
	NewForward    = core.NewForward
	NewFunction   = core.NewFunction
	// MarkPredicateFn stamps `fnpred`'s declaration onto a fn value —
	// the explicit route into the predicate-type branch (NUR099).
	MarkPredicateFn = core.MarkPredicateFn
	NewImplicitMap  = core.NewImplicitMap
	ModuleNSOf      = core.ModuleNSOf
	// NewInstant moved to lang/go/engine/native_temporal.go (Step 8).
	NewInteger               = core.NewInteger
	IsMicronValue            = core.IsMicronValue
	IsMicronType             = core.IsMicronType
	InstallType              = core.InstallType
	AsMicronType             = core.AsMicronType
	MakePathon               = core.MakePathon
	CanonValue               = core.CanonValue
	ErrNoComparer            = core.ErrNoComparer
	CheckAddUniqueDiagnostic = core.CheckAddUniqueDiagnostic
	CheckAddUnique           = core.CheckAddUnique
	AsMicronFields           = core.AsMicronFields
	PathonContentEqual       = core.PathonContentEqual
	NewBigInteger            = core.NewBigInteger
	NewBigDecimal            = core.NewBigDecimal
	FormatBigInteger         = core.FormatBigInteger
	FormatBigDecimal         = core.FormatBigDecimal
	NewInterpString          = core.NewInterpString
	// NewInterval moved to lang/go/engine/native_misc.go (Step 8).
	NewList        = core.NewList
	NewFlexList    = core.NewFlexList
	NewMap         = core.NewMap
	NewFlexMap     = core.NewFlexMap
	NewMark        = core.NewMark
	NewMove        = core.NewMove
	NewMoveCont    = core.NewMoveCont
	NewMoveIf      = core.NewMoveIf
	NewOpenParen   = core.NewOpenParen
	NewCloseParen  = core.NewCloseParen
	NewEnd         = core.NewEnd
	NewOptionsType = core.NewOptionsType
	NewOrderedMap  = core.NewOrderedMap
	NewParenExpr   = core.NewParenExpr
	NewPathon      = core.NewPathon
	NewPathonVol   = core.NewPathonVol
	NewRecordType  = core.NewRecordType
	NewRegistry    = core.NewRegistry
	NewReturnCheck = core.NewReturnCheck
	NewString      = core.NewString
	NewXmlElement  = core.NewXmlElement
	NewTableType   = core.NewTableType
	// NewTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// NewTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// NewTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	NewTop                 = core.NewTop
	NewType                = core.NewType
	NewVariadicCarrier     = core.NewVariadicCarrier
	NewTypedList           = core.NewTypedList
	NewTypedMap            = core.NewTypedMap
	NewWord                = core.NewWord
	NewWordModified        = core.NewWordModified
	Go                     = core.Go
	Boru                   = core.Boru
	RunInCheck             = core.RunInCheck
	Park                   = core.Park
	FullStack              = core.FullStack
	CheckFullStack         = core.CheckFullStack
	NextMarkID             = core.NextMarkID
	MatchFnSig             = core.MatchFnSig
	OpenUnifyMap           = core.OpenUnifyMap
	RankSignatures         = core.RankSignatures
	UnaryNumOpNative       = core.UnaryNumOpNative
	BinaryNumOpNative      = core.BinaryNumOpNative
	BinaryIntOpNative      = core.BinaryIntOpNative
	RequireConcreteList    = core.RequireConcreteList
	RequireConcreteMap     = core.RequireConcreteMap
	ResolveTypeLiteralDef  = core.ResolveTypeLiteralDef
	ResolveTypePath        = core.ResolveTypePath
	ResolveWordValue       = core.ResolveWordValue
	ResolveWordsDeep       = core.ResolveWordsDeep
	ReturnsIdentity        = core.ReturnsIdentity
	RunCarrierBody         = core.RunCarrierBody
	RunCarrierBodyKeepDefs = core.RunCarrierBodyKeepDefs
	SetIDSeed              = core.SetIDSeed
	SeverityFor            = core.SeverityFor
	CompareSignatures      = core.CompareSignatures
	SimplifyDisjunctAlts   = core.SimplifyDisjunctAlts
	SizeOf                 = core.SizeOf
	SortSignatures         = core.SortSignatures
	StoreKey               = core.StoreKey
	TypeNameTable          = core.TypeNameTable
	Unify                  = core.Unify
	UnifyR                 = core.UnifyR
	UnifyExplain           = core.UnifyExplain
	UnifyExplainR          = core.UnifyExplainR
	UninstallDef           = core.UninstallDef
	UninstallFnSigs        = core.UninstallFnSigs
	ValToString            = core.ValToString
	ValidateTypeNameParts  = core.ValidateTypeNameParts
	ValuesEqual            = core.ValuesEqual
	WithPos                = core.WithPos
	WithPosAt              = core.WithPosAt
	DynStackShuffleWords   = core.DynStackShuffleWords
	// `make` helpers, ported alongside the make word in eng/go/core_make.go.
	ResolveFieldType = core.ResolveFieldType

	// `get`/`set` helper, ported with those words to eng/go/core_storage.go.

	// The kept `if` condition run (NUR212's follow-up).
	RunCarrierCondBodyKeepDefs = core.RunCarrierCondBodyKeepDefs
	GeneraliseSpecUndef        = core.GeneraliseSpecUndef
)

// Sugar roles (eng/go/sugar.go — ADR-012 rule 3, 2026-08-04
// amendment): the keys bindSugarWords binds boru's word names to.
const (
	SugarUsurp       = core.SugarUsurp
	SugarStackArgs   = core.SugarStackArgs
	SugarForwardArgs = core.SugarForwardArgs
	SugarForceArity  = core.SugarForceArity
	SugarMini        = core.SugarMini
	SugarLambda      = core.SugarLambda
	SugarAngle       = core.SugarAngle
	SugarTypeBound   = core.SugarTypeBound
	SugarGenHead     = core.SugarGenHead
	SugarGenApply    = core.SugarGenApply
	SugarGenDefault  = core.SugarGenDefault
)

// Re-exports of functions that read a Value through a pointer. They are
// wrapper functions rather than variables, so a call is direct: it can
// inline, and escape analysis sees that the pointer does not escape (a
// call through a function variable would move every &v to the heap).
func AsMove(v *core.Value) (core.MoveInfo, error)              { return core.AsMove(v) }
func AsParenExpr(v *core.Value) ([]core.Value, error)          { return core.AsParenExpr(v) }
func IsDisjunct(v *core.Value) bool                            { return core.IsDisjunct(v) }
func IsSplice(v *core.Value) bool                              { return core.IsSplice(v) }
func IsNone(v *core.Value) bool                                { return core.IsNone(v) }
func IsClassType(v *core.Value) bool                           { return core.IsClassType(v) }
func IsParenExpr(v *core.Value) bool                           { return core.IsParenExpr(v) }
func IsReach(v *core.Value) bool                               { return core.IsReach(v) }
func IsTypedList(v *core.Value) bool                           { return core.IsTypedList(v) }
func IsTypedMap(v *core.Value) bool                            { return core.IsTypedMap(v) }
func IsWord(v *core.Value) bool                                { return core.IsWord(v) }
func AsWord(v *core.Value) (core.WordInfo, error)              { return core.AsWord(v) }
func CommonAncestorType(a *core.Type, b *core.Type) *core.Type { return core.CommonAncestorType(a, b) }
func NewTypeSchema(t *core.Type, info *core.TypeSchemaInfo) core.Value {
	return core.NewTypeSchema(t, info)
}
func AttachGenBound(r *core.Registry, node *core.Type, p core.GenParam) {
	core.AttachGenBound(r, node, p)
}
func TypeParamName(t *core.Type) string                        { return core.TypeParamName(t) }
func SchemaInfoOf(t *core.Type) (*core.TypeSchemaInfo, bool)   { return core.SchemaInfoOf(t) }
func NodeOf(t *core.Type) *core.Type                           { return core.NodeOf(t) }
func MintRetagged(v *core.Value, def *core.Type) core.Value    { return core.MintRetagged(v, def) }
func ThreadPos(dst, src *core.Value)                           { core.ThreadPos(dst, src) }
func IsBareTypeNode(v *core.Value) bool                        { return core.IsBareTypeNode(v) }
func IsConcrete(v *core.Value) bool                            { return core.IsConcrete(v) }
func IsTypeBody(v *core.Value) bool                            { return core.IsTypeBody(v) }
func IsTypeLiteral(v *core.Value) bool                         { return core.IsTypeLiteral(v) }
func NewValueRaw(t *core.Type, data core.Payload) core.Value   { return core.NewValueRaw(t, data) }
func DataOf(v *core.Value) core.Payload                        { return core.DataOf(v) }
func ParentOf(v *core.Value) *core.Type                        { return core.ParentOf(v) }
func IsCarrier(v *core.Value) bool                             { return core.IsCarrier(v) }
func OriginOf(v *core.Value) core.OriginKind                   { return core.OriginOf(v) }
func IsInternal(v *core.Value) bool                            { return core.IsInternal(v) }
func SetData(v *core.Value, d core.Payload)                    { core.SetData(v, d) }
func SetParent(v *core.Value, t *core.Type)                    { core.SetParent(v, t) }
func SetCarrier(v *core.Value, b bool)                         { core.SetCarrier(v, b) }
func SetOrigin(v *core.Value, o core.OriginKind)               { core.SetOrigin(v, o) }
func SetInternal(v *core.Value, b bool)                        { core.SetInternal(v, b) }
func MakeValue(t *core.Type, d core.Payload) core.Value        { return core.MakeValue(t, d) }
func MakeCarrierValue(t *core.Type, d core.Payload) core.Value { return core.MakeCarrierValue(t, d) }
func NewCarrier(t *core.Type) core.Value                       { return core.NewCarrier(t) }
func NewDynamicCarrier(t *core.Type) core.Value                { return core.NewDynamicCarrier(t) }
func NewResourceType(t *core.Type, info core.ResourceTypeInfo) core.Value {
	return core.NewResourceType(t, info)
}
func NewTypeLiteral(t *core.Type) core.Value     { return core.NewTypeLiteral(t) }
func NewBoundedType(bound *core.Type) core.Value { return core.NewBoundedType(bound) }
func NewBoundedTypeBody(node *core.Type, body core.Value) core.Value {
	return core.NewBoundedTypeBody(node, body)
}

// The occurrence state (core/go/occurrence.go): the identity and the flags,
// read and written through the holder, never as fields of the value.
func IDOf(v *core.Value) string           { return core.IDOf(v) }
func IsQuoted(v *core.Value) bool         { return core.IsQuoted(v) }
func IsFailedDispatch(v *core.Value) bool { return core.IsFailedDispatch(v) }
func IsDynamic(v *core.Value) bool        { return core.IsDynamic(v) }
