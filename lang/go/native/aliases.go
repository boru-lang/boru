// Package engine is a thin shim over the eng module: it re-exports
// eng's types and functions so the surrounding lang codebase can import
// "github.com/boru-lang/boru/lang/go/native" while the actual engine
// machinery lives in the standalone eng module.
//
// Word-defining files (native_*.go, format.go, query.go, sqlite.go,
// fileio.go, conditional.go, forloop.go) continue to live here. Anything
// that's truly engine machinery (Registry, Value, *Type, signatures,
// matching, unification, …) is now in eng.
package native

import (
	check "github.com/boru-lang/boru/check/go"
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// *Type aliases — every exported type from borueng is re-exported here.
type (
	BranchRecord       = core.BranchRecord
	DeqIndex           = core.DeqIndex
	EmitFragment       = core.EmitFragmentRef
	BoruError          = core.BoruError
	RenderOpts         = core.RenderOpts
	DiagSpan           = core.DiagSpan
	DiagSuggestion     = core.DiagSuggestion
	CalDurationData    = core.CalDurationData
	CheckDiagnostic    = core.CheckDiagnostic
	CheckFullStackFunc = core.CheckFullStackFunc
	CheckSeverity      = core.CheckSeverity
	CheckState         = core.CheckState
	EmitState          = compiler.EmitState
	ChildTypeInfo      = core.ChildTypeInfo
	CodeEffectInfo     = core.CodeEffectInfo
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
	PathonPayload        = core.PathonPayload
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
	LoopDriver           = core.LoopDriver
	LoopOpenInfo         = core.LoopOpenInfo
	LoopInvoke           = core.LoopInvoke
	ForwardInfo          = core.ForwardInfo
	GuardClause          = core.GuardClause
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
	TKeyVal   = core.TKeyVal
	// TModuleInst keeps the lang spelling for Ideal/Module, kernel-declared
	// as core.TModule since the ADR-012 stage-2 move (moduletype.go).
	TModuleInst = core.TModule
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
	CarrierDisjunctCap     = core.CarrierDisjunctCap
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
	CompileSideEffect         = core.CompileSideEffect

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
	AnalyseFnBody             = core.RunFnBodyAnalysis
	RunPendingFnBodyChecks    = core.RunPendingFnBodyChecks
	AnalyseLoopBody           = core.RunLoopBodyAnalysis
	AsAtom                    = core.AsAtom
	AsChildType               = core.AsChildType
	AsDisjunct                = core.AsDisjunct
	AsError                   = core.AsError
	AsInterpString            = core.AsInterpString
	AsList                    = core.AsList
	AsMap                     = core.AsMap
	AsMutableList             = core.AsMutableList
	AsMutableMap              = core.AsMutableMap
	AsFlexList                = core.AsFlexList
	AsFlexXml                 = core.AsFlexXml
	AsWeakFlexMap             = core.AsWeakFlexMap
	AsWeakFlexList            = core.AsWeakFlexList
	AsWeakFlexXml             = core.AsWeakFlexXml
	IsWeakFlexNode            = core.IsWeakFlexNode
	WeakRefusalError          = core.WeakRefusalError
	IsXmlValue                = core.IsXmlValue
	XmlParts                  = core.XmlParts
	AsClassInstance           = core.AsClassInstance
	AsClassType               = core.AsClassType
	AsResourceInstance        = core.AsResourceInstance
	AsResourceType            = core.AsResourceType
	AsOptionsType             = core.AsOptionsType
	AsRecordType              = core.AsRecordType
	AsStore                   = core.AsStore
	AsTableType               = core.AsTableType
	IsAtom                    = core.IsAtom
	IsError                   = core.IsError
	IsImplicitMap             = core.IsImplicitMap
	Canon                     = core.Canon
	IsNoneShape               = core.IsNoneShape
	IsClassInstance           = core.IsClassInstance
	IsResourceInstance        = core.IsResourceInstance
	IsResourceType            = core.IsResourceType
	IsFlatInstance            = core.IsFlatInstance
	IsOptionsType             = core.IsOptionsType
	NewReach                  = core.NewReach
	NewReachFromKeys          = core.NewReachFromKeys
	ApplyReach                = core.ApplyReach
	IsSteplessWindow          = core.IsSteplessWindow
	IsPathon                  = core.IsPathon
	IsRecordType              = core.IsRecordType
	IsStore                   = core.IsStore
	IsTableType               = core.IsTableType
	IsTypeValue               = core.IsTypeValue
	IsFlexMap                 = core.IsFlexMap
	IsFlexList                = core.IsFlexList
	IsFlexNode                = core.IsFlexNode
	IsValueOfType             = core.IsValueOfType
	AsBoolean                 = core.AsBoolean
	AsFloat                   = core.AsFloat
	AsInteger                 = core.AsInteger
	AsNumber                  = core.AsNumber
	AsFloatApprox             = core.AsFloatApprox
	AsBigInteger              = core.AsBigInteger
	AsBigDecimal              = core.AsBigDecimal
	AsPathon                  = core.AsPathon
	AsString                  = core.AsString
	BaseValueForConstraint    = core.BaseValueForConstraint
	BoundToKind               = core.BoundToKind
	CoerceBoolean             = core.CoerceBoolean
	CompareValues             = core.CompareValues
	CowSet                    = core.CowSet
	CowDel                    = core.CowDel
	DataListElemTypeFromValue = check.DataListElemTypeFromValue
	StaticListLen             = check.StaticListLen
	ExpandOptionalSigs        = core.ExpandOptionalSigs
	parseFnParams             = core.ParseFnParams
	parseFnReturns            = core.ParseFnReturns
	resolveSigType            = core.ResolveSigType
	resolveTypeName           = core.ResolveTypeName
	TandValues                = core.TandValues
	ValidateWordName          = core.ValidateWordName
	TypeOf                    = core.TypeOf
	TypeNameOf                = core.TypeNameOf
	TypePathOf                = core.TypePathOf
	ValueType                 = core.ValueType
	NewNone                   = core.NewNone
	NewNegation               = core.NewNegation
	IsNegation                = core.IsNegation
	AsNegation                = core.AsNegation
	FormatFloat               = core.FormatFloat
	NewTypedListWithElements  = core.NewTypedListWithElements
	NewTypedMapWithEntries    = core.NewTypedMapWithEntries
	FlattenDisjunctAlts       = core.FlattenDisjunctAlts
	FnDefHasSig               = core.FnDefHasSig
	NewGenSpec                = core.NewGenSpec
	IsGenSpec                 = core.IsGenSpec
	AsGenSpec                 = core.AsGenSpec
	NewGenParamValue          = core.NewGenParamValue
	IsGenParamValue           = core.IsGenParamValue
	AsGenParamValue           = core.AsGenParamValue
	IsTypeSchema              = core.IsTypeSchema
	AsTypeSchema              = core.AsTypeSchema
	NewGenInstRef             = core.NewGenInstRef
	IsGenInstRef              = core.IsGenInstRef
	AsGenInstRef              = core.AsGenInstRef
	MintTypeParam             = core.MintTypeParam
	PushGenBindings           = core.PushGenBindings
	PopGenBindings            = core.PopGenBindings
	InstantiateSchema         = core.InstantiateSchema
	AsSurfaceType             = core.AsSurfaceType
	IsSurfaceType             = core.IsSurfaceType
	FnDefsOverlap             = core.FnDefsOverlap
	FnSigMatchesSpec          = core.FnSigMatchesSpec
	FnSigSatisfiesSpec        = core.FnSigSatisfiesSpec
	FnUndefMatchesFnDef       = core.FnUndefMatchesFnDef
	BuiltinIDForPath          = core.BuiltinIDForPath
	MintTestType              = core.MintTestType
	NewSugar                  = core.NewSugar
	AsSugar                   = core.AsSugar
	SugarExpansion            = core.SugarExpansion
	GenerateID                = core.GenerateID
	GenerateObjectTypeID      = core.GenerateObjectTypeID
	BeginIDMintScope          = core.BeginIDMintScope
	ResetModuleExportGrowth   = core.ResetModuleExportGrowth
	MakeClassFieldValue       = core.MakeClassFieldValue
	MakeClassInstance         = core.MakeClassInstance
	MakeResource              = core.MakeResource
	ClassFields               = core.ClassFields
	FlatInstanceFields        = core.FlatInstanceFields
	InstallDef                = core.InstallDef
	InstallFnDef              = core.InstallFnDef
	InstallWordExtension      = core.InstallWordExtension
	TransplantExtension       = core.TransplantExtension
	IsWordExtension           = core.IsWordExtension
	IsSealedWord              = core.IsSealedWord
	HasLockedSigs             = core.HasLockedSigs
	NewWordExtension          = core.NewWordExtension
	// Owner sentinels for the ownership-anchored signature rules
	// (design/OPEN-WORDS.1.md §4).
	OwnerKernel              = core.OwnerKernel
	OwnerLang                = core.OwnerLang
	OwnerProgram             = core.OwnerProgram
	NewPathonFromString      = core.NewPathonFromString
	TypeContentOf            = core.TypeContentOf
	IsCapitalisedName        = core.IsCapitalisedName
	DeepConcrete             = core.DeepConcrete
	DeepKnown                = core.DeepKnown
	IsRecordShape            = core.IsRecordShape
	JoinCarrierStacks        = core.JoinCarrierStacks
	JoinCarriers             = core.JoinCarriers
	FoldVariadicArms         = core.FoldVariadicArms
	MakeBoruError            = core.MakeBoruError
	ExitCode                 = core.ExitCode
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
	RunCarrierCondBody       = core.RunCarrierCondBody
	InstallJoinedDefs        = core.InstallJoinedDefs
	New                      = core.New
	RunPooled                = core.RunPooled
	RunPooledTop             = core.RunPooledTop
	RunResolved              = core.RunResolved
	InvokeBody               = core.InvokeBody
	BodyEscaped              = core.BodyEscaped
	InvokeCallbackBody       = core.InvokeCallbackBody
	InvokeCallbackFn         = core.InvokeCallbackFn
	FnValueOnlyZeroArgSigs   = core.FnValueOnlyZeroArgSigs
	FnHome                   = core.FnHome
	FnHomeLookup             = core.FnHomeLookup
	HomeExportedFn           = core.HomeExportedFn
	ConvertIdealToMap        = core.ConvertIdealToMap
	ConvertIdealToList       = core.ConvertIdealToList
	IsCompiledClosure        = compiler.IsCompiledClosure
	ClosureWantsKeyVal       = compiler.ClosureWantsKeyVal
	ClosureIsFnValue         = compiler.ClosureIsFnValue
	ClosureAsFnDef           = core.ClosureAsFnDef
	ClosureSigMatched        = core.ClosureSigMatched
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
	SetAtomReferent          = core.SetAtomReferent
	NewBoolean               = core.NewBoolean
	// NewCalendarDuration moved to lang/go/engine/native_temporal.go (Step 8).
	StampFnValueInPlace      = compiler.StampFnValueInPlace
	StampModuleCallGates     = core.StampModuleCallGates
	ElementCarrierFromValue  = check.ElementCarrierFromValue
	ElementCarrierOf         = check.ElementCarrierOf
	NewCarrierTypedListValue = core.NewCarrierTypedListValue
	CarrierTypedListOf       = core.CarrierTypedListOf
	ValueCarrier             = core.ValueCarrier
	NewDynamicCarrierValue   = core.NewDynamicCarrierValue
	// NewClockDuration moved to lang/go/engine/native_temporal.go (Step 8).
	// NewDate / NewDateTime moved to lang/go/engine/native_temporal.go (Step 8).
	NewFloat       = core.NewFloat
	NewDefCleanup  = core.NewDefCleanup
	NewDepScalar   = core.NewDepScalar
	NewDisjunct    = core.NewDisjunct
	NewEnum        = core.NewEnum
	NewError       = core.NewError
	NewEvalList    = core.NewEvalList
	NewEvalMap     = core.NewEvalMap
	NewFnUndef     = core.NewFnUndef
	NewForward     = core.NewForward
	NewFunction    = core.NewFunction
	NewImplicitMap = core.NewImplicitMap
	ModuleNSOf     = core.ModuleNSOf
	// NewInstant moved to lang/go/engine/native_temporal.go (Step 8).
	NewInteger        = core.NewInteger
	NewKeyVal         = core.NewKeyVal
	NewModuleInstance = core.NewModuleInstance
	AsModuleDesc      = core.AsModuleDesc
	NewBigInteger     = core.NewBigInteger
	NewBigDecimal     = core.NewBigDecimal
	FormatBigInteger  = core.FormatBigInteger
	FormatBigDecimal  = core.FormatBigDecimal
	NewInterpString   = core.NewInterpString
	// NewInterval moved to lang/go/engine/native_misc.go (Step 8).
	NewList             = core.NewList
	NewFlexList         = core.NewFlexList
	NewMap              = core.NewMap
	MarkShapeModel      = core.MarkShapeModel
	NewFlexMap          = core.NewFlexMap
	NewMark             = core.NewMark
	NewMove             = core.NewMove
	NewMoveCont         = core.NewMoveCont
	StartLoop           = core.StartLoop
	IsLoopRegion        = core.IsLoopRegion
	DriveLoop           = core.DriveLoop
	NewMoveIf           = core.NewMoveIf
	NewOpenParen        = core.NewOpenParen
	NewCloseParen       = core.NewCloseParen
	NewEnd              = core.NewEnd
	NewOptionsType      = core.NewOptionsType
	NewOrderedMap       = core.NewOrderedMap
	NoteFnBodyPendingIn = core.NoteFnBodyPendingIn
	NewParenExpr        = core.NewParenExpr
	NewPathon           = core.NewPathon
	NewPathonVol        = core.NewPathonVol
	NewRecordType       = core.NewRecordType
	NewRegistry         = core.NewRegistry
	NewReturnCheck      = core.NewReturnCheck
	NewString           = core.NewString
	NewXmlElement       = core.NewXmlElement
	NewTableType        = core.NewTableType
	// NewTimeOfDay moved to lang/go/engine/native_temporal.go (Step 8).
	// NewTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// NewTimezone moved to lang/go/engine/native_temporal.go (Step 8).
	NewTop                 = core.NewTop
	NewEmitState           = compiler.NewEmitState
	NewType                = core.NewType
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
	ReturnsFreshInstance   = check.ReturnsFreshInstance
	ReturnsIdentity        = core.ReturnsIdentity
	ReturnsListElemAt      = check.ReturnsListElemAt
	ReturnsNumericBinary   = check.ReturnsNumericBinary
	ReturnsAddConcat       = check.ReturnsAddConcat
	ReturnsPreserveListAt  = check.ReturnsPreserveListAt
	ReturnsStatic          = check.ReturnsStatic
	ReturnsDynUnion        = check.ReturnsDynUnion
	DryPassReturns         = check.DryPassReturns
	DryPassWrap            = check.DryPassWrap
	DeepConcreteOptions    = check.DeepConcreteOptions
	DeepConcreteOptionsAt  = check.DeepConcreteOptionsAt
	MirrorReturns          = check.MirrorReturns
	CheckListIndex         = check.CheckListIndex
	CheckAtIndices         = check.CheckAtIndices
	RunCarrierBody         = core.RunCarrierBody
	RunCarrierBodyKeepDefs = core.RunCarrierBodyKeepDefs
	AnalyseCodeEffect      = compiler.AnalyseCodeEffectCarrier
	SetIDSeed              = core.SetIDSeed
	SeverityFor            = core.SeverityFor
	CompareSignatures      = core.CompareSignatures
	SimplifyDisjunctAlts   = core.SimplifyDisjunctAlts
	SizeOf                 = core.SizeOf
	SortSignatures         = core.SortSignatures
	StoreKey               = core.StoreKey
	StripToCarriers        = check.StripToCarriers
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
	// `make` helpers, ported alongside the make word in eng/go/core_make.go.
	makeFieldValue   = core.MakeFieldValue
	ResolveFieldType = core.ResolveFieldType

	// `get`/`set` helper, ported with those words to eng/go/core_storage.go.
	getKey = core.GetKey
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
func AsDefCleanup(v *core.Value) (core.DefCleanupInfo, error)   { return core.AsDefCleanup(v) }
func AsMark(v *core.Value) (core.MarkInfo, error)               { return core.AsMark(v) }
func AsMove(v *core.Value) (core.MoveInfo, error)               { return core.AsMove(v) }
func AsParenExpr(v *core.Value) ([]core.Value, error)           { return core.AsParenExpr(v) }
func AsReturnCheck(v *core.Value) (core.ReturnCheckInfo, error) { return core.AsReturnCheck(v) }
func IsBoolean(v *core.Value) bool                              { return core.IsBoolean(v) }
func IsCloseParen(v *core.Value) bool                           { return core.IsCloseParen(v) }
func IsDefCleanup(v *core.Value) bool                           { return core.IsDefCleanup(v) }
func IsDisjunct(v *core.Value) bool                             { return core.IsDisjunct(v) }
func IsEnd(v *core.Value) bool                                  { return core.IsEnd(v) }
func IsForward(v *core.Value) bool                              { return core.IsForward(v) }
func IsFrameOpen(v *core.Value) bool                            { return core.IsFrameOpen(v) }
func AsFrameOpen(v *core.Value) (core.FrameOpenInfo, error)     { return core.AsFrameOpen(v) }
func IsInterpString(v *core.Value) bool                         { return core.IsInterpString(v) }
func IsMark(v *core.Value) bool                                 { return core.IsMark(v) }
func IsSplice(v *core.Value) bool                               { return core.IsSplice(v) }
func AsSplice(v *core.Value) (core.SpliceInfo, error)           { return core.AsSplice(v) }
func IsMove(v *core.Value) bool                                 { return core.IsMove(v) }
func IsNone(v *core.Value) bool                                 { return core.IsNone(v) }
func IsClassType(v *core.Value) bool                            { return core.IsClassType(v) }
func IsOpenParen(v *core.Value) bool                            { return core.IsOpenParen(v) }
func IsParenExpr(v *core.Value) bool                            { return core.IsParenExpr(v) }
func IsReach(v *core.Value) bool                                { return core.IsReach(v) }
func AsReach(v *core.Value) (core.ReachInfo, error)             { return core.AsReach(v) }
func IsReturnCheck(v *core.Value) bool                          { return core.IsReturnCheck(v) }
func IsTypedList(v *core.Value) bool                            { return core.IsTypedList(v) }
func IsTypedMap(v *core.Value) bool                             { return core.IsTypedMap(v) }
func IsWord(v *core.Value) bool                                 { return core.IsWord(v) }
func AsForward(v *core.Value) (core.ForwardInfo, error)         { return core.AsForward(v) }
func AsWord(v *core.Value) (core.WordInfo, error)               { return core.AsWord(v) }
func BaseValue(t *core.Type) (core.Value, error)                { return core.BaseValue(t) }
func CommonAncestorType(a *core.Type, b *core.Type) *core.Type  { return core.CommonAncestorType(a, b) }
func SubstituteSelf(spec core.FnSigSpec, node *core.Type) core.FnSigSpec {
	return core.SubstituteSelf(spec, node)
}
func NewTypeSchema(t *core.Type, info *core.TypeSchemaInfo) core.Value {
	return core.NewTypeSchema(t, info)
}
func AttachGenBound(r *core.Registry, node *core.Type, p core.GenParam) {
	core.AttachGenBound(r, node, p)
}
func IsTypeParamNode(t *core.Type) bool { return core.IsTypeParamNode(t) }
func TypeParamName(t *core.Type) string { return core.TypeParamName(t) }
func SubstituteTypeParams(r *core.Registry, v core.Value, bindings map[string]core.Value, selfNode *core.Type) (core.Value, error) {
	return core.SubstituteTypeParams(r, v, bindings, selfNode)
}
func SchemaInfoOf(t *core.Type) (*core.TypeSchemaInfo, bool) { return core.SchemaInfoOf(t) }
func NewSurfaceType(t *core.Type, info *core.SurfaceInfo) core.Value {
	return core.NewSurfaceType(t, info)
}
func IsSugar(v *core.Value) bool                              { return core.IsSugar(v) }
func IDPrefixForType(t *core.Type) string                     { return core.IDPrefixForType(t) }
func CanonicalType(r *core.Registry, t *core.Type) *core.Type { return core.CanonicalType(r, t) }
func ReparentValue(v core.Value, def *core.Type) core.Value   { return core.ReparentValue(v, def) }
func IsBareTypeNode(v *core.Value) bool                       { return core.IsBareTypeNode(v) }
func IsConcrete(v *core.Value) bool                           { return core.IsConcrete(v) }
func IsTypeBody(v *core.Value) bool                           { return core.IsTypeBody(v) }
func IsTypeLiteral(v *core.Value) bool                        { return core.IsTypeLiteral(v) }
func NewValueRaw(t *core.Type, data core.Payload) core.Value  { return core.NewValueRaw(t, data) }
func NewCarrier(t *core.Type) core.Value                      { return core.NewCarrier(t) }
func NewElementCarrier(t *core.Type) core.Value               { return check.NewElementCarrier(t) }
func NewCarrierTypedList(elem *core.Type) core.Value          { return core.NewCarrierTypedList(elem) }
func NewDynamicCarrier(t *core.Type) core.Value               { return core.NewDynamicCarrier(t) }
func NewClassInstance(t *core.Type, info core.ClassInstanceInfo) core.Value {
	return core.NewClassInstance(t, info)
}
func NewClassType(t *core.Type, info core.ClassTypeInfo) core.Value {
	return core.NewClassType(t, info)
}
func NewResourceInstance(t *core.Type, info core.ResourceInstanceInfo) core.Value {
	return core.NewResourceInstance(t, info)
}
func NewResourceType(t *core.Type, info core.ResourceTypeInfo) core.Value {
	return core.NewResourceType(t, info)
}
func NewStore(t *core.Type) core.Value { return core.NewStore(t) }
func NewStoreValue(t *core.Type, si *core.StoreInstanceInfo) core.Value {
	return core.NewStoreValue(t, si)
}
func NewStoreWithPrototype(t *core.Type, prototype *core.StoreInstanceInfo) core.Value {
	return core.NewStoreWithPrototype(t, prototype)
}
func NewTypeLiteral(t *core.Type) core.Value     { return core.NewTypeLiteral(t) }
func NewBoundedType(bound *core.Type) core.Value { return core.NewBoundedType(bound) }
func NewBoundedTypeBody(node *core.Type, body core.Value) core.Value {
	return core.NewBoundedTypeBody(node, body)
}
func NewCarrierTypedListLen(elem *core.Type, n int) core.Value {
	return check.NewCarrierTypedListLen(elem, n)
}
func makeConvert(src core.Value, targetType *core.Type) (core.Value, error) {
	return core.MakeConvert(src, targetType)
}
