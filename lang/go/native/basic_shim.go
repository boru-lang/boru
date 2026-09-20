// The basic shim: re-exports the fundamental-word and predefined-type
// layer that moved to the basic/go module (ADR-013), mirroring what
// aliases.go does for eng. The word files that stay in this package —
// and the sibling packages that import native — keep their spellings;
// the implementations live one layer down in basic, which depends on
// eng only.
package native

import (
	basic "github.com/boru-lang/boru/basic/go"
)

// Registration groups consumed by Register (register.go) at the
// historical registration points — the interleaving with the rest of
// the word library is order-sensitive (overload appends).
var (
	stackNatives      = basic.StackNatives
	definitionNatives = basic.DefinitionNatives
	genNatives        = basic.GenNatives
	controlNatives    = basic.ControlNatives
	constNative       = basic.ConstNative

	registerDefKeywordForms = basic.RegisterDefKeywordForms
	installResourceTypes    = basic.InstallResourceTypes
)

// Helpers still referenced by the word files that stay in this package.
var (
	defName                      = basic.DefName
	doEvalList                   = basic.DoEvalList
	checkFnCarrierBind           = basic.CheckFnCarrierBind
	installAndRecordDef          = basic.InstallAndRecordDef
	lookupResourceTypeByName     = basic.LookupResourceTypeByName
	genWrapSchema                = basic.GenWrapSchema
	installMicronIdeals          = basic.InstallMicronIdeals
	basicCheckMicronConstruction = basic.CheckMicronConstruction
	basicMicronProperty          = basic.MicronProperty
	basicMicronSchemaFor         = basic.MicronSchemaFor
	genUnsupported               = basic.GenUnsupported
	recordTypeInitErr            = basic.RecordTypeInitError
)

// Exported passthroughs used by sibling packages (lang/go/modules, the
// public lang API, cmd/go).
var (
	MatchFnSig               = basic.MatchFnSig
	ResetCheckFnCarrierBinds = basic.ResetCheckFnCarrierBinds
	RecordTypeInitError      = basic.RecordTypeInitError
	SwapTypeInitErrs         = basic.SwapTypeInitErrs
	TypeInitErrs             = basic.TypeInitErrs
	TypeInitError            = basic.TypeInitError
)

// Handler / analysis internals that this package's test suites (and a
// few staying word files) still reference under their historical
// spellings. Call-only — none is seam-swapped (the one swappable seam,
// the type-init accumulator, goes through SwapTypeInitErrs above).
var (
	afnHandler                       = basic.AfnHandler
	argsHandler                      = basic.ArgsHandler
	asInt64Or                        = basic.AsInt64Or
	caseBranchJoin                   = basic.CaseBranchJoin
	caseHandler                      = basic.CaseHandler
	caseMatchHandler                 = basic.CaseMatchHandler
	caseNumericValue                 = basic.CaseNumericValue
	caseReturnsFn                    = basic.CaseReturnsFn
	constHandler                     = basic.ConstHandler
	defHandler                       = basic.DefHandler
	defStackOnly                     = basic.DefStackOnly
	defTypedHandler                  = basic.DefTypedHandler
	defWordExtension                 = basic.DefWordExtension
	defaultBareHandler               = basic.DefaultBareHandler
	doListHandler                    = basic.DoListHandler
	doListReturnsFn                  = basic.DoListReturnsFn
	doMapHandler                     = basic.DoMapHandler
	durationFromValue                = basic.DurationFromValue
	errorHandler                     = basic.ErrorHandler
	errorReturnsFn                   = basic.ErrorReturnsFn
	extendsHandler                   = basic.ExtendsHandler
	fnHandler                        = basic.FnHandler
	fnSigArgList                     = basic.FnSigArgList
	fnTripleHandler                  = basic.FnTripleHandler
	fnsigHandler                     = basic.FnsigHandler
	forCountHandler                  = basic.ForCountHandler
	forRangeHandler                  = basic.ForRangeHandler
	genHandler                       = basic.GenHandler
	hasStructuralSlots               = basic.HasStructuralSlots
	ifListHandler                    = basic.IfListHandler
	ifListReturnsFn                  = basic.IfListReturnsFn
	isCaseOpenCallHead               = basic.IsCaseOpenCallHead
	loopIterations                   = basic.LoopIterations
	markFnPredicateBindUncompilable  = basic.MarkFnPredicateBindUncompilable
	noteCheckFnCarrierBind           = basic.NoteCheckFnCarrierBind
	ofHandler                        = basic.OfHandler
	parseRange                       = basic.ParseRange
	pickCheckFullStack               = basic.PickCheckFullStack
	popArgsHandler                   = basic.PopArgsHandler
	recordTypedBindOrDeclineConcrete = basic.RecordTypedBindOrDeclineConcrete
	registerTemporalType             = basic.RegisterTemporalType
	resolveResourceTypeInfo          = basic.ResolveResourceTypeInfo
	resourceDefKey                   = basic.ResourceDefKey
	rollCheckFullStack               = basic.RollCheckFullStack
	runForLoop                       = basic.RunForLoop
	shiftPosFlags                    = basic.ShiftPosFlags
	undefFnHandler                   = basic.UndefFnHandler
	varHandler                       = basic.VarHandler
)

// Behavior structs of the moved Scalar/Time family and timer handles —
// the test suites construct them directly.
type (
	timeCompareBehavior       = basic.TimeCompareBehavior
	dateFormatBehavior        = basic.DateFormatBehavior
	dateTimeFormatBehavior    = basic.DateTimeFormatBehavior
	instantFormatBehavior     = basic.InstantFormatBehavior
	timeOfDayFormatBehavior   = basic.TimeOfDayFormatBehavior
	calDurationFormatBehavior = basic.CalDurationFormatBehavior
	clkDurationFormatBehavior = basic.ClkDurationFormatBehavior
	timezoneFormatBehavior    = basic.TimezoneFormatBehavior
	timeoutFormatBehavior     = basic.TimeoutFormatBehavior
	intervalFormatBehavior    = basic.IntervalFormatBehavior
)

// Scalar/Bytes lives in basic (type + Behavior + bridge); the bytes
// words and the binary-frame machinery stay here.
var (
	TBytes        = basic.TBytes
	NewBytesValue = basic.NewBytesValue
	AsBytesValue  = basic.AsBytesValue
	newBytes      = basic.NewBytes
	asBytes       = basic.AsBytes
)

type bytesBehavior = basic.BytesBehavior

// The opaque-handle content types (Patrun / Pid / Service) live in
// basic; the matcher and service state stay here with the words and
// implement basic's delegation interfaces.
var (
	TPatrun  = basic.TPatrun
	TPid     = basic.TPid
	TService = basic.TService

	NewPid     = basic.NewPid
	PidProcess = basic.PidProcess
	asPid      = basic.AsPid

	registerPatrunType  = basic.RegisterPatrunType
	registerPidType     = basic.RegisterPidType
	registerServiceType = basic.RegisterServiceType
)

// Behavior structs of the moved handle types — the coverage suites
// construct them directly.
type (
	patrunBehavior  = basic.PatrunBehavior
	pidBehavior     = basic.PidBehavior
	serviceBehavior = basic.ServiceBehavior
)

// The Scalar/Time family — types, constructors, accessors and the
// module-mint surface — lives in basic (its owning component).
type TemporalModuleTypes = basic.TemporalModuleTypes

var (
	TTime     = basic.TTime
	TDate     = basic.TDate
	TDateTime = basic.TDateTime
	TInstant  = basic.TInstant

	MintTemporalModuleTypes = basic.MintTemporalModuleTypes
	NewDate                 = basic.NewDate
	NewDateTime             = basic.NewDateTime
	NewInstant              = basic.NewInstant
	AsDate                  = basic.AsDate
	AsDateTime              = basic.AsDateTime
	AsInstant               = basic.AsInstant
	AsTimeOfDay             = basic.AsTimeOfDay
	AsCalendarDuration      = basic.AsCalendarDuration
	AsClockDuration         = basic.AsClockDuration
	AsTimezone              = basic.AsTimezone
)
