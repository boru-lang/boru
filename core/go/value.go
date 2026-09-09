package core

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/apd/v3"
)

// ReadList is a read-only view of a list of Values.
// Node list values expose this via AsList(). To mutate, use AsMutableList().
type ReadList struct {
	elems []Value
}

// NewReadList wraps a slice of Values as a ReadList. External callers
// (outside the borueng package) use this constructor because the elems
// field is unexported.
func NewReadList(elems []Value) ReadList {
	return ReadList{elems: elems}
}

// Get returns the element at index i.
// Internal use only — caller must ensure 0 <= i < Len().
func (l ReadList) Get(i int) Value {
	return l.elems[i]
}

// GetOk returns the element at index i and true, or the zero Value and false
// if i is out of bounds. Safe for use at system boundaries.
func (l ReadList) GetOk(i int) (Value, bool) {
	if i < 0 || i >= len(l.elems) {
		return Value{}, false
	}
	return l.elems[i], true
}

// Len returns the number of elements.
func (l ReadList) Len() int {
	return len(l.elems)
}

// Slice returns a copy of the underlying slice.
func (l ReadList) Slice() []Value {
	out := make([]Value, len(l.elems))
	copy(out, l.elems)
	return out
}

// IsNil reports whether this ReadList has no backing data.
func (l ReadList) IsNil() bool {
	return l.elems == nil
}

// ReadMap is a read-only view of an ordered key-value map.
// Node values (Map, Options) expose this interface via AsMap().
// To mutate, use AsMutableMap() which is only valid for Object instances.
type ReadMap interface {
	Get(key string) (Value, bool)
	Keys() []string
	SortedKeys() []string
	Len() int
}

// OrderedMap is a map that preserves insertion order of keys.
type OrderedMap struct {
	keys     []string
	vals     map[string]Value
	Implicit bool           // true when created from implicit pair syntax (e.g., [x:Integer])
	Meta     map[string]any // optional metadata for parser/engine communication
}

// NewOrderedMap creates an empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{vals: make(map[string]Value)}
}

// Set adds or updates a key-value pair, preserving insertion order.
func (m *OrderedMap) Set(key string, val Value) {
	if _, exists := m.vals[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

// Get retrieves a value by key.
func (m *OrderedMap) Get(key string) (Value, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Keys returns the keys in insertion order (defensive copy).
func (m *OrderedMap) Keys() []string {
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

// SortedKeys returns the keys in sorted order (for deterministic comparison).
func (m *OrderedMap) SortedKeys() []string {
	sorted := make([]string, len(m.keys))
	copy(sorted, m.keys)
	sort.Strings(sorted)
	return sorted
}

// Len returns the number of entries.
func (m *OrderedMap) Len() int {
	return len(m.keys)
}

// Delete removes a key-value pair. Returns true if the key existed.
func (m *OrderedMap) Delete(key string) bool {
	if _, exists := m.vals[key]; !exists {
		return false
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
	return true
}

// PathonInfo holds the data for a Scalar/Micron/Pathon value.
// A Path represents a filesystem path as a sequence of parts.
// Absolute paths start from the root (Abs = true).
type PathonInfo struct {
	Parts []string // path segments (e.g. ["usr", "local", "bin"])
	Abs   bool     // true for absolute/rooted paths (e.g. /usr/local/bin, C:\Windows)
	// Volume is a Windows drive prefix ("C:"), or empty for a POSIX /
	// driveless path. When set, the path renders in canonical Windows
	// (backslash) form.
	Volume string
}

// String returns the canonical path string. A driveless path renders
// POSIX-style with "/"; a drive path renders with "\" — "C:\a\b", or
// "C:a\b" when drive-relative (no separator after the colon).
func (p PathonInfo) String() string {
	if p.Volume != "" {
		body := strings.Join(p.Parts, `\`)
		if p.Abs {
			if body == "" {
				return p.Volume + `\`
			}
			return p.Volume + `\` + body
		}
		if body == "" {
			return p.Volume
		}
		return p.Volume + body // drive-relative: C:a\b
	}
	joined := strings.Join(p.Parts, "/")
	if p.Abs {
		return "/" + joined
	}
	return joined
}

// ChildTypeInfo holds the child-type constraint for a typed list or
// typed map.
//
// For example, [:String] constrains all list elements to be strings,
// and {:String} constrains all map values to be strings. Elements is
// non-nil when the source carried both literal elements AND a child
// constraint (e.g. `[{x:1} :{x:Integer} {x:2}]`); each element is
// validated against Child by `is` and similar predicates.
type ChildTypeInfo struct {
	Child    Value
	Elements []Value // optional: concrete elements alongside the child constraint
	Entries  []ChildEntry
	// Len is an optional statically-known length for a typed-list
	// carrier (nil = unknown). Set by length-producing words whose
	// result length can be computed exactly in check mode (e.g. iota),
	// it lets StaticListLen recover a bound for a computed list so the
	// index checker can flag a provably out-of-range access. It MUST be
	// an exact length or an upper bound — never an underestimate, which
	// would turn an in-bounds access into a false positive.
	Len *int
}

// ChildEntry is a (key, value) pair retained for typed maps that
// carry concrete entries alongside their child constraint
// (`{a:1 :{x:Integer} b:2}`).
type ChildEntry struct {
	Key   string
	Value Value
}

// RecordTypeInfo holds the field schema for a record type.
// Each field maps a name to a type-constraint Value (e.g. a type literal).
// A record type unifies with a concrete map if it has exactly the right keys
// and each value unifies with the corresponding field type.
type RecordTypeInfo struct {
	Fields *OrderedMap // field name → type-constraint Value
}

// OptionsTypeInfo holds the field schema for an options type.
// Each field maps a name to a default value or type constraint.
// Concrete values serve as defaults when the key is absent during unification;
// type literals require the caller to provide a value.
type OptionsTypeInfo struct {
	Fields *OrderedMap // field name → default value or type constraint
}

// TableTypeInfo holds the record schema for a table type.
// A table represents a list of record instances that all conform to the
// same record type.
type TableTypeInfo struct {
	Record RecordTypeInfo // the record type that each row must match
}

// FnParam describes one parameter in a function signature.
type FnParam struct {
	Name     string // empty for unnamed positional parameters
	Type     *Type
	Pattern  *Value // optional: map/list pattern for structural matching
	Optional bool   // true if this param was marked optional via ?
	// Quote marks a /q param (`name:Atom/q` in a boru input sig): the
	// slot captures an upcoming bare Word as its Atom NAME during
	// collection — the arg is presented as if quoted — with the same
	// binding-agnostic rule native QuoteArgs slots follow (`def`,
	// `inspect`, `quote`: the capture trumps any def binding of the
	// word). normalizeSig merges the flag into Signature.QuoteArgs,
	// which is the field every dispatch-side reader consults.
	Quote bool
}

// BarrierAllForward is the canonical sentinel for "no `|` boundary
// specified — default this sig to all-forward dispatch." Resolved
// at registration time (upsertFnDef) to len(Args). Stack-only sigs
// must set BarrierPos: 0 explicitly; this constant exists only for
// the all-forward default.
const BarrierAllForward = -1

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params  []FnParam
	Returns []*Type // declared return types (nil = unchecked)
	// ReturnPatterns is the symmetric twin of FnParam.Pattern, positional
	// against Returns (nil, or shorter, means "no extra constraint here").
	//
	// It exists because a declared return can carry a constraint that no
	// *Type can express. `def IS (Integer tor String)` used as an output
	// resolves through ResolveSigType to (TAny, &pattern): the union has no
	// minted lattice node to name, so the TYPE degrades to Any and the
	// domain lives entirely in the pattern. Without somewhere to keep it,
	// ParseFnReturns dropped the pattern and `Any` accepted everything —
	// the shorthand form silently accepted a Boolean body under a declared
	// union return while the bracket form rejected it.
	ReturnPatterns []*Value
	// Decl is where the return contract was declared — the output-sig
	// token of this triple, plus the declaring program's source/file —
	// threaded onto ReturnCheck markers so a return error can label the
	// declaration as a secondary span. Zero for Go-registered sigs.
	Decl DeclSite
	// BarrierPos is the forward/stack boundary expressed by `|` in
	// a boru fn parameter list. Three values carry distinct meaning:
	//
	//   -1 — unset (no `|` token in the source). Consumers
	//        (InstallFnDef, fnSigsToSignatures) default this to
	//        len(Params) so the fn behaves as all-forward, matching
	//        the convention `RegisterNativeFunc` already applies to
	//        natives that omit the field.
	//    0 — explicit all-stack. The user wrote `[| a b]` so every
	//        arg comes from the stack. Distinct from -1 because the
	//        default bump must NOT fire here.
	//   >0 — explicit barrier at position N. Args[0..N-1] are
	//        forward-eligible; args[N..] come from the stack.
	//
	// Go-side construction sites that don't go through ParseFnParams
	// (modules/math.go, modules/matrix.go, etc.) leave the field at
	// the Go zero value (0); the consumer treats that as "default"
	// for backwards compatibility — only the boru-source path emits
	// the -1 sentinel.
	BarrierPos int
	// NoEvalArgs lists per-sig-position the list-shaped args that
	// must NOT be auto-evaluated when consumed by this fn. Mirrors
	// Signature.NoEvalArgs so module FnDef wrappers can preserve
	// quoted code bodies passed at unnamed-param positions (e.g.
	// `rand.list-of [body] N` — body is code, not data).
	//
	// Honored by fnSigsToSignatures (forwards to Signature.NoEvalArgs)
	// and by execFnDefSig's auto-eval guard before CallBoru. Without
	// this, an Eval=true list passed in would be silently sub-Run'd,
	// breaking the quoted-body contract.
	NoEvalArgs map[int]bool
	// NoEvalMapArgs is the map-shaped counterpart to NoEvalArgs.
	// Used by sigs that take a Map at a code-body slot (e.g. a spec
	// schema where map values are quoted generators).
	NoEvalMapArgs map[int]bool
	// RawParens marks arg positions where a forward ParenExpr is captured
	// RAW (not pre-evaluated) so the handler receives the paren as code.
	// Opt-in; see Signature.RawParens and design/PAREN-REPRESENTATION.9.md.
	RawParens map[int]bool
	// FormArgs marks arg positions captured as a raw FORM — a generalization
	// of RawParens (don't pre-eval a paren) and QuoteArgs (capture a bare word
	// as data) to ANY operand: a word stays a Word, a paren/list/literal is
	// captured unevaluated, with no def resolution, no dispatch, and no
	// Word→Atom coercion. The macro definer sets it on every param so a macro
	// receives its operands as code. See design/MACROS-PHASE1.10.md §3.
	FormArgs map[int]bool

	// --- Run implementation + dispatch metadata. ---

	// Impl is the signature's run implementation as a sealed sum
	// (GoImpl | BoruImpl) — the SINGLE representation every reader consults
	// through the Signature accessors (dispatchHandler / body / fnFrame /
	// fullStack / checkFullStackFn / parkResult / runInCheckMode in
	// sigimpl.go). Native words and internal Go sites author `Go(handler,
	// opts...)`; module refs / un-installed lambdas author `boru(body)`;
	// InstallFnDef / compileFnDefLiteral build the installed-fn `BoruImpl`
	// (with a derived body-splicer + frame meta) directly. A Signature with
	// a nil Impl has no implementation (a check-mode shape synth, a raw
	// match-only test fixture) and dispatchHandler() returns nil.
	Impl SigImpl
	// Args / Patterns are the exported positional constructor-convenience
	// mirrors of Params (see signature.go; kernel reads Params).
	Args     []*Type
	Patterns map[int]Value
	// QuoteArgs / TypeArgs are per-position dispatch modifiers.
	QuoteArgs map[int]bool
	TypeArgs  map[int]bool
	// FnInertArgs marks per-position slots where a FN-VALUED operand is INERT
	// DATA for the bytecode recorder — read or compared, never invoked — on a
	// word that is NOT wholesale CompileReadsFn because ANOTHER slot does
	// invoke a fn. The one holder today is `is` (Stage M2d): its VALUE slot
	// only reads the operand's lattice tag (`(+re/…/) is (MiniLang.Re)`),
	// while a Function in its TYPE slot is a predicate the handler INVOKES
	// (`5 is Positive` — RunPredicate) and must keep refusing. Read by
	// recordCallOperands alongside CompileReadsFn/CompileStoresFn.
	FnInertArgs map[int]bool
	// FnDataArgs marks per-position slots where a fn-value operand resolved via
	// a dynamic-scope read (an enclosing `def op (Parse.parser g)`) must be READ
	// AS DATA — the lowering picks OpLookupDynScopeData, which PUSHES the
	// FnDefInfo binding instead of deferring like the plain OpLookupDynScope
	// (whose FnDefInfo defer is for a name-POSITION read the interpreter would
	// dispatch). The one holder is parselang-fn-dispatch's arg0 (the computed
	// parser fn), which the interpreter passes as data to parseFnDispatchHandler.
	FnDataArgs map[int]bool
	// Fallback marks the synthesized 0-arg catch-all sig.
	Fallback bool
	// ReturnsFn is the check-mode return computer (native-authored or
	// boru-derived); orthogonal to the run implementation in Impl.
	ReturnsFn ReturnsFunc
	// CompileEffect declares the word's compile-relevant semantics for the
	// bytecode recorder, so it can classify the word WITHOUT a name-keyed table
	// in eng (which couples the engine to specific, often module, word names).
	// The zero value is CompileDefault (an ordinary word). Set on the Signature
	// at registration; copied here by RegisterNativeFunc.
	CompileEffect CompileEffect
	// Callable, when non-nil, declares this word as a code-body higher-order
	// word whose body compiles to a closure unit (each / fold / do / a test
	// case body, …). The bytecode recorder reads the closure shape from here —
	// the body operand's position, its per-invocation output count, and the
	// input carriers it consumes — instead of a name-keyed eng table, so eng
	// names no specific (often module) word. Copied from NativeFunc.Callable
	// onto every signature at registration. nil = not closure-eligible.
	Callable *CallableSpec
	// StoredBodies declares the word's PARAM-CARRYING stored code-body
	// positions (Test.check-prop's gen/property): NoEvalArgs lists the
	// handler stores and later invokes per call with the declared params
	// bound (its own CallBoru frames). The recorder compiles each declared
	// position to a closure unit with those params and replaces the operand
	// with a synthetic fn-value carrier whose single sig mirrors the
	// handler's CallBoru sig — same Params, same raw Body — plus the
	// CompiledFnRef, so the handler upgrades to InvokeCallback and the VM
	// hosts the unit nested (same-program ref); every decline leaves the
	// raw list and the handler's interpreter path unchanged. Copied from
	// NativeFunc.StoredBodies onto every signature at registration.
	StoredBodies []StoredBodySpec

	// Locked marks a signature registered through the Go registration
	// layer (Registry.Register — every native / kernel word plus host
	// words). Locked signatures can never be replaced or removed by a
	// def-merge (design/OPEN-WORDS.0.md §2.3), and they sort FIRST in
	// match order (CompareSignatures), so an unlocked merged addition
	// can never pre-empt a locked match — no previously-valid call
	// changes its dispatch. Locking is a property of the Go layer, not
	// a boru language ability; InstallFnDef (user `def … fn`) never
	// sets it.
	Locked bool
	// Origin records which module contributed this signature via an
	// export transplant (the module ref, e.g. "./ext.boru" or
	// "boru:time-util"). Empty for native registrations and direct user
	// defs. Read by the transplant collision check: the same tuple
	// arriving from a DIFFERENT module raises [boru/extend_conflict],
	// while identical provenance (diamond re-import) is idempotent.
	Origin string
	// ModuleCall, when non-nil, names the module export this signature
	// dispatches — the {module, export} identity the per-export policy
	// gate (Check("modules","call")) verifies at every dispatch site on
	// BOTH engines (interpreter execMatch / ExecFnDefSigStackMatch /
	// CallBoru; VM CALL_NATIVE / CALL_USER / poly re-match /
	// tryNativeFnApply). Stamped ONCE at module resolution
	// (StampModuleCallGates) onto the export map's own sigs AND the
	// sub-registry's stored inner sigs, so every signature copy that
	// descends from them — dispatch aggregates, `def w Pkg.word`
	// rebindings (the fn-as-data laundering path), fn-value captures,
	// compiled SigRef/PolyRef re-matches — carries the identity. Nil
	// for every non-module signature: the gates cost one pointer test.
	ModuleCall *ModuleCallID
	// CoreDefault marks a core-provided UNLOCKED default overload that the
	// kernel appends to a builtin word (RegisterCoreDefault) — the Micron
	// field-wise arithmetic default is the sole user. Unlike a locked
	// native sig it dispatches AFTER (is beaten by) any more-specific
	// user/module overload, so a user's own `def add fn [[a:Kindon
	// b:Kindon] …]` wins by specificity. Unlike an ordinary unlocked def it
	// lives on the native FnDefInfo (so `undef` of the builtin still
	// refuses) and is SKIPPED by the export transplant (so it never rides
	// a module's word extension into an importer, where its builtin-only
	// tuple would trip the module-scope user-type rule).
	CoreDefault bool
}

// CompileEffect is a set of compile-relevant capability flags a word declares
// so the bytecode recorder can classify it from the Signature rather than from
// name-keyed tables — decoupling eng from specific (often module) word names.
// It is a BITFIELD: the flags are orthogonal and a word may carry several (e.g.
// `typeof` reads a fn value AND is a pure module reader AND re-dispatches as an
// island-pure word). The zero value, CompileDefault, is an ordinary word.
type CompileEffect uint16

const (
	// CompileReadsFn marks an INTROSPECTION word that READS a fn value's
	// immutable shape (typeof / inspect / arityof / type-algebra) and never
	// invokes it. The fn bakes as a const the handler inspects — and because only
	// the shape is read, even a CAPTURING fn is safe to bake (its captures are
	// irrelevant to its signature).
	CompileReadsFn CompileEffect = 1 << iota
	// CompileStoresFn marks a word that STORES a fn value for later
	// interpreter-side invocation (minilang / parselang register), never the VM
	// tape. A fn-valued operand rides as an inert const; but unlike
	// CompileReadsFn, only a PURE fn literal bakes (a capturing / sub-registry fn
	// declines at isInertConst), because the stored fn is invoked later and must
	// keep its real binding.
	CompileStoresFn
	// CompileModuleFold marks a PURE reader word (get / getr / convert / typeof /
	// is / size / has) whose result over an import-bound module value plus inert
	// const operands is deterministic — so the recorder const-folds it.
	CompileModuleFold
	// CompileIslandPure marks a PURE typed-dispatch word (get / getr / size /
	// make / is / typeof / type-algebra) with no side effects: where the checker
	// could not commit to one overload, the word may run as an interpreter island
	// that re-dispatches on the real runtime value (and a stack-form island when
	// it has no threaded body inputs).
	CompileIslandPure
	// CompileFallbackBody marks a code-body higher-order word (each / fold / scan
	// / filter / select / group / do / case / where / having / order / …) that
	// may compile its body as a Stage-5 interpreter island.
	CompileFallbackBody
	// CompileQuoteInert marks a word whose implicit-quote (QuoteArgs) operand is
	// INERT DATA the handler consumes verbatim — a quoted symbol (`quote name`,
	// `raise bad_input …`) or a quoted code body held as data (`timeout 1000
	// [body]`) — so the word bakes as a plain CALL_NATIVE once each quoted operand
	// resolves to an inert const: the VM runs the SAME handler over the SAME baked
	// value as the interpreter. It is the opt-in for the QuoteArgs refusal, the
	// declared analogue of the get/getr/set exemption. Do NOT set it on a
	// dispatch-manipulating meta word (usurp / force-arity / valof) whose quoted
	// operand drives a RE-STEPPING result the VM cannot reproduce by re-running
	// the handler.
	CompileQuoteInert
	// CompileDiverges marks a word whose handler ALWAYS raises (it never returns
	// normally) — `raise`, the user-error constructor. A call to it is recorded as
	// a CALL_NATIVE (the handler raises the byte-identical error at run time) but
	// the recorder treats it as a DIVERGENT terminal: control never reaches the
	// fragment's end, so a closure body ending in it (`do [raise …]`, `error`'s
	// handler tail) compiles with NO RET — the error propagates out of the VM and
	// the catching word (`do` / `error`) turns it into the Error value via
	// InvokeBody, exactly as the interpreter does. This is the bytecode side of
	// structured exception handling: a trapping closure body is catchable rather
	// than uncompilable. The divergence is sound in a branch/loop arm too (the
	// arm never produces a value, like break/continue).
	CompileDiverges
	// CompileRunsBodyIsolated marks a word whose NoEvalArgs body(ies) are NEITHER
	// spliced onto the tape NOR const-baked and re-run in the enclosing sub-engine.
	// Instead the handler executes each body via a fresh, ISOLATED CallBoru frame
	// against a registry the word CAPTURED at registration (not the passed-in `r`),
	// binding only that body's own parameters. Test.check-prop is the canonical
	// case: runCheckProp runs the generator body (param `r` = a seeded rand
	// instance) and the property body (one unnamed param) each through
	// parent.CallBoru, so name resolution inside a body is IDENTICAL under the
	// interpreter and the VM -- it never touches a compiled frame local, because the
	// CallBoru frame binds only the body's params and resolves everything else
	// against the captured parent (module / global scope), exactly as it does when
	// the interpreter drives the same handler. So a plain CALL_NATIVE bake is sound
	// even when the body is a DYNAMIC value (a map get, `p get "gen"`) whose tokens
	// are not statically inert -- the VM evaluates the operand to the same List value
	// and hands it to the same handler. The flag exempts ONLY the inert-scoped
	// disjunct of the code-body refusal; a word that declares a body-executing
	// CallableSpec still refuses.
	CompileRunsBodyIsolated

	// CompileScalarFold marks a PURE value-level word (the comparison family:
	// eq/neq/deq/cmp/tcmp/lt/lte/gt/gte) whose dispatch over ALL-inert-const
	// operands the check pass may CONST-FOLD by running the real handler
	// (tryFoldScalarConst, carrier.go): the concrete result replaces the
	// declared-type carrier, so a literal condition like `(n eq 0)` with a
	// const-bound n folds to a concrete boolean and downstream `if` analysis
	// sees a statically-determined branch instead of a Disjunct residual
	// (the forward-barrier.tsv:83 soundness pin). Folding is double-evaluated
	// with an agreement guard exactly like CompileModuleFold, so a
	// non-deterministic handler never freezes; an erroring dispatch (the
	// family-restricted `lt` on cross-family operands) declines the fold and
	// keeps today's diagnostic path.
	CompileScalarFold
	// CompileValueDiverges marks a word that diverges VALUE-DEPENDENTLY: it
	// returns its declared result for most operands but RAISES for a specific
	// statically-decidable operand shape — `div` / `mod` by a static-zero
	// integer divisor (`1 div 0`). Unlike CompileDiverges (ALWAYS raises), the
	// divergence is per-call, so the recorder infers it from the word's own
	// check-mode ReturnsFn: when the ReturnsFn produced NO result (the divergent
	// path — returnsDivMod returns nil for a static-zero divisor), the call
	// raises and is recorded as a divergent terminal (like raise), so a closure
	// body ending in it (`do [1 div 0]`) compiles with no RET and the catching
	// `do` turns the raised error into an Error value — instead of islanding.
	// The flag scopes the "0 results ⇒ diverges" inference to these words: a
	// genuinely void word (print/set, declared 0-result) is unaffected.
	CompileValueDiverges
	// CompileDynBody marks a body-running word (`do`) whose dispatch may lower
	// to a plain CALL_NATIVE even when the closure path declines — a COMPUTED
	// (carrier) body, or a concrete body carrying context-dependent words
	// (args) — because the handler's runtime execution (InvokeBody →
	// RunResolved, or a JIT-compiled unit) IS the interpreter's own semantics
	// PROVIDED the name environment matches. Recording such a site therefore
	// arms the program's DynEnv mode: every def and every named unit param
	// emits an OpBindDynScope twin (registry-visible, frame-unwound) and the
	// VM brackets each CALL_USER frame with an args-stack push, so the body's
	// sub-run resolves names and `args` exactly as it does under the
	// interpreter. The result is marked variadic (the runtime count is the
	// body's own), so only variadic-absorbing positions consume it.
	CompileDynBody
	// CompileStoresBody marks a word that STORES a NoEvalArgs CODE-BODY list to run
	// LATER on its own registry — `spawn`'s process body. Like CompileStoresFn but
	// for a code list rather than a fn value: the recorder compiles the body's
	// tokens to a 0-param unit and hands the word a synthetic fn-value carrier
	// (raw Body tokens + a CompiledFnRef), so the word runs the compiled unit via
	// RunUnit on its fork instead of a fresh interpreter sub-engine. A body that
	// refuses to compile rides as the plain inert const list and the word runs it
	// on the interpreter, unchanged.
	CompileStoresBody
	// CompileStoresBodyList marks a word that stores a NoEvalArgs LIST OF
	// code-body lists to run later on per-branch registry forks — `await`'s
	// parallels. The recorder compiles each list element to its own 0-param
	// unit (compileStoredBody) and rebuilds the list with synthetic fn-value
	// carriers in place of the elements that compiled, so the word runs each
	// branch via RunUnit on its fork. An element that refuses to compile
	// rides as its plain list and that branch interprets — per-element and
	// sound.
	CompileStoresBodyList
	// CompileFnHandlerStrict marks a store-fn word (a CompileStoresFn slot)
	// whose native VALIDATES and DISPATCHES its handler as an FnDefInfo value
	// — the `service`/`add` family (requireHandlerFn + the call seam's
	// MatchFnSig, which both need a real FnDefInfo). A CAPTURING handler at
	// such a slot cannot stamp (captures) and would otherwise fall through to
	// a bare OpPushClosure ClosurePayload the native rejects ("got Function")
	// — a divergence. The recorder refuses it so the interpreter owns the
	// shape; a non-strict store-fn word (a Patrun `add`, which invokes a
	// stored closure fine) is unaffected. Non-capturing handlers stamp
	// either way.
	CompileFnHandlerStrict
)

// CompileDefault is an ordinary word: no compile-relevant capability. A
// fn-valued operand reaching it means the handler invokes the fn on the tape,
// which the VM cannot honour, so the recorder refuses (Stage 3).
const CompileDefault CompileEffect = 0

// Has reports whether the effect set includes flag f.
func (e CompileEffect) Has(f CompileEffect) bool { return e&f != 0 }

// CallableSpec declares a code-body higher-order word's closure-compilation
// shape, so the bytecode recorder reads it from the resolved Signature
// (sig.Callable) rather than a name-keyed eng table — decoupling eng from the
// (often module) word names that own these words. A word declares one on its
// NativeFunc; RegisterNativeFunc copies the pointer onto every signature. nil
// on a Signature means the word is not closure-eligible.
// BodyOutResidual is CallableSpec.BodyOut's whole-residual sentinel: the
// driving handler returns the body's entire residual stack (do), so the
// body nets an arbitrary, per-body-exact count rather than a declared 0/1.
// Out-of-domain per the no-zero-value-overload rule — 0 remains the valid
// explicit "side-effect body" count.
const BodyOutResidual = -1

// StoredBodySpec is one Signature.StoredBodies entry: the sig position of a
// NoEvalArgs code-body list the word's handler stores and invokes per call,
// and the params the handler's own CallBoru frame binds for it (a named param
// binds the body's reads of that name; an unnamed param rides the stack).
type StoredBodySpec struct {
	Pos    int
	Params []FnParam
}

type CallableSpec struct {
	// BodyPos is the body operand's sig position (the code list / lambda).
	BodyPos int
	// BodyOut is how many values the body nets per invocation: 1 for a
	// map/transform body (each / fold), 0 for a SIDE-EFFECT body (a test
	// case whose assertions raise on failure and otherwise leave nothing). It
	// sets the compiled unit's declared return count. BodyOutResidual (the
	// out-of-domain sentinel; 0 stays the valid explicit side-effect count)
	// marks a whole-residual word (`do`): the driving handler returns the
	// body's ENTIRE residual, so the closure compiles count-AGNOSTIC and the
	// dispatch seats however many results the check run reported — the VM's
	// frameless RET already returns the full residual stack.
	BodyOut int
	// EmptyBodyErrors marks a word whose driving HANDLER itself raises a
	// runtime error when an invocation's body nets 0 values — each / fold /
	// scan raise `<word>_error "body produced no result"` from their InvokeBody
	// loop (invokeBodyTop / the list eachHandler return the empty residual and
	// the handler raises). For such a word a 0-net body is NOT a compile
	// refusal: the body compiles as a count-AGNOSTIC closure (declared nil
	// returns, like a side-effect body) so the handler raises the byte-identical
	// error at run time, instead of refusing the closure and islanding to let
	// the interpreter raise. The handler arbitrates the residual uniformly —
	// it takes the LAST value and errors on none — so a 1- or N-value body is
	// handled identically to before; only the unenforced count differs.
	EmptyBodyErrors bool
	// Inputs returns the per-invocation input carriers the body consumes, in the
	// order the driving handler supplies them via InvokeBody. The carriers are
	// GENERALISED types (not one call's concrete values) so the body compiles
	// once for every invocation. args is the call's full operand list, so a word
	// can derive the input type from its data operand (e.g. each's element type)
	// or its seed (fold's accumulator). Returns an empty slice for a 0-input body
	// (do / a test case). nil declines the compile (an unexpected operand shape).
	Inputs func(args []Value) []Value
	// BodyOnceKeepsDefs marks a word whose driving handler runs the body
	// EXACTLY ONCE per dispatch and KEEPS the bindings that run installs —
	// `do`, whose runtime defs leak to the enclosing scope and whose
	// check-mode body run is RunCarrierBodyKeepDefs (the keep=true
	// NestedBodyDepth class the bind ledger records). The twin regime's
	// compiler reads it as a replay license (AdoptBodyTwins): a bind twin
	// noted during this word's suspended body run may be placed as a twin
	// op after the dispatch's call event, because the one check-time run
	// IS the one runtime transition and the captured entry replays it
	// verbatim. Never set it on a multi-run body word (each/fold/scan):
	// the runtime re-runs such a body per element while the ledger noted
	// ONE generalized transition with carrier-valued captures — a single
	// replay would be wrong in count and in value, so their twins must
	// stay unplaced (refusing the regime program) until they are
	// arm-resident. Verified against the handler: DoListHandler drives
	// InvokeBody once and returns the whole residual.
	BodyOnceKeepsDefs bool
	// BodyMultiRunKeepsDefs marks a word whose driving handler runs the
	// body ONCE PER ELEMENT and keeps each run's installs — `each`, whose
	// runtime leaks one install per element per body def site, stacked in
	// element order with the per-element runtime value (measured:
	// `[10 20] each [var [[r] def x r x]]` leaves x = [20, 10] top-down;
	// the parity oracle's refused rows pin the full population). The twin
	// regime's compiler reads it as the ARM-RESIDENCY license: a bind
	// twin noted during this word's suspended body analysis may be
	// satisfied by a resident install op INSIDE the compiled
	// per-invocation unit — executing per element with the runtime value,
	// which the check pass's one generalized carrier-valued capture
	// cannot replay. Mutually exclusive with BodyOnceKeepsDefs (a body
	// runs once or per element, never both); never set it on a word
	// whose handler tears its body installs down. Verified against the
	// handler: eachHandler drives InvokeBody per element on the shared
	// registry with no def cleanup.
	BodyMultiRunKeepsDefs bool
	// BodyResultTop marks a driving handler that reads only the TOP of each
	// invocation's body residual (`res[len(res)-1]`) — each / fold / scan / filter.
	// The body may then leave UNCONSUMED values below that top (notably the
	// per-invocation INPUT a body that ignores its element leaves on the stack,
	// `each [add 1 0]` → `[input, 3]`), and the compiler may safely DROP them at
	// the RET: the handler never reads below the top. A handler that reads the
	// WHOLE residual (`do`, returning every value) leaves this false, so its
	// closure keeps the strict in-order reconciliation. Verified against the
	// handlers (native_array.go each/fold/scan, native/filter.go) — each takes
	// res[len(res)-1].
	BodyResultTop bool
	// CrossCollectionTokenShape marks a word whose TOKEN-quotation body is
	// SHAPE-GENERIC across the word's List-vs-Map overloads: both present the
	// closure the bare element/value (ClosureInValue), never a KeyVal. So a
	// gradual-Any (Dynamic) collection — statically ambiguous between the List and
	// Map overload — can still compile: the recorder commits the first reachable
	// (List) overload and lowers the token body to ONE closure, and the committed
	// handler is RUNTIME-ROBUST (it delegates to the sibling collection's iteration
	// when the value's concrete type is the other one), so the SAME closure drives
	// either shape == the interpreter, which dispatches the overload by the runtime
	// type. Without this flag a gradual collection refuses (the ambiguous-overload
	// MarkUncompilable below). Set ONLY on words whose every List/Map token overload
	// is ClosureInValue AND whose handlers cross-delegate (each/fold/scan). A LAMBDA
	// body never reaches this — it matches only the single TFunction overload, so
	// the ≥2-reachable ambiguity gate never fires for it.
	CrossCollectionTokenShape bool
	// StripsUnconsumedInput marks a word whose driving handler STRIPS its
	// pushed per-invocation input from the residual BOTTOM when the body
	// leaves it unconsumed — `error [handler]`: the caught Error is pushed
	// so the handler can bind it, and a handler that ignores it nets
	// [error, result]; the handler's identity probe strips the bottom so
	// the branch leaves exactly the result. The closure path then admits
	// two body shapes (both netting ONE runtime value): a 1-value residual,
	// and a 2-value residual whose bottom IS the input (param local 0).
	// The closure compiles count-agnostic (declared nil) and
	// stripResidualShapeOK screens everything else back to the refusal.
	StripsUnconsumedInput bool
	// LambdaSharesTokenShape marks a word that presents a LAMBDA callback the
	// SAME per-invocation inputs as a token-quotation body — Inputs(args) is the
	// single callback convention (walk's `{key value path parent depth}` payload
	// map, handed to a quotation on the stack and to a lambda as its one named
	// param). The recorder then compiles a lambda body against Inputs(args)
	// directly (ClosureInValue), instead of consulting the per-word
	// lambdaCallbackInputs table whose shapes (pair maps, KeyVals) only fit the
	// words that present DIFFERENT views to lambdas vs quotations.
	LambdaSharesTokenShape bool
}

// FnDefInfo holds the function specification for a def-defined function.
// Name is the function's registered name (set by InstallDef). If Registry is
// non-nil, the function was defined in a module and should execute in that
// registry's context (closure semantics).
//
// Signatures is the SINGLE per-function signature slice — one full-fidelity
// overload per entry. Each Signature carries the authored shape (Params with
// names, Returns, and the boru Body) AND, once compiled, the dispatch fields
// (a Go Handler, resolved BarrierPos, sorted order). Body vs Handler is the
// only Go-vs-Boru distinction: a Go builtin has a Handler and no Body, a boru
// fn carries Body tokens and (after install/compile) a body-splicing Handler.
//
// The slice on a DefStack entry or a constructed Function value holds only
// THAT definition's own overloads — it is NOT the cross-stack dispatch table.
// The accumulated, sorted, fallback-bearing dispatch table is built on demand
// at the registry boundary (Registry.Lookup → aggregateDispatch); every
// matcher / forward-planner reads the aggregate's Signatures, while the
// authored-shape readers (canon, inspect, predicate probes, trivial-delegation,
// targeted undef, overlap) read a single definition's own Signatures, skipping
// any synthetic Fallback. Whether the engine tries forward collection is
// determined per-signature via Signature.BarrierPos — derive the word-level
// summary via fn.HasForwardSigs.
type FnDefInfo struct {
	Name           string
	Signatures     []Signature // own full-fidelity overloads (see doc above)
	MaxForwardArgs int         // longest forward arg count across all sigs (respecting barriers)
	Registry       *Registry
	// Module/Export/Doc record a function's origin and one-line summary
	// when it is a native-module export (e.g. ArrayUtil.indices). They are
	// the provenance `describe` renders for a qualified name. Module is the
	// import id ("boru:array-util"), Export the namespace ("ArrayUtil"), Doc
	// a one-line summary. All three are empty for user/anonymous fns and
	// core words, which carry no module origin.
	Module string
	Export string
	Doc    string
	// Examples are hand-authored `describe` lines for a module export,
	// each a complete line shown verbatim. They exist because the
	// auto-generated positional permutations are actively misleading for
	// a capability word: `Net.fetch 'a' {a:1,b:2}` is well-formed and
	// teaches nothing, since the interesting part of `fetch` is the
	// shape of the options map, not the arity. Nil for core words (which
	// carry their examples on their static help Entry) and for exports
	// nobody has written examples for yet, both of which keep the
	// generated permutations.
	Examples []string
	// MiniKind names the mini-language kind whose expansion produced
	// this partially-applied Function ("" for everything else). The
	// per-kind member types the boru:minilang module exports
	// (MiniLang.Re, MiniLang.Gex, …) match on it, so a typed fn param
	// can require a specific kind's partial (e.g. a regexp matcher).
	MiniKind string
	// Anonymous is true iff the FnDef was produced by the `afn` word (i.e.
	// via the `=>` lambda sugar). The flag is read only in check mode: an
	// anonymous fn's static Returns is the conservative [Any], and the
	// check-mode dispatch path runs AnalyseFnBody to recover the real
	// return type for downstream type propagation. Named fns leave this
	// false and the check-mode path uses sig.Returns as authored.
	Anonymous bool
	// Macro is true iff the FnDef was produced by the `macro` definer. A
	// macro is an fn the expander runs on UNEVALUATED operand forms (every
	// param is FormArgs raw-capture; §3 of design/MACROS-PHASE1.10.md), whose
	// returned token list is spliced into the call site rather than left as a
	// value. Read at dispatch (stepWord / execFnDefLiteral) to branch to the
	// expander before normal forward collection. Unlike Anonymous (check-mode
	// only), Macro gates runtime dispatch.
	Macro bool
	// Predicate is true iff the FnDef was produced by the `fnpred` word —
	// the author DECLARED this function to be a membership test, so
	// InstallType routes a capitalised binding of it to the predicate-type
	// branch. It is the explicit half of a routing decision that is
	// otherwise made by counting parameters (isPredicateFnValue,
	// PredicateInputType), which ADR-016 forbids: arity must never decide
	// how a function behaves. A declared predicate says so; it is not
	// inferred from its shape. NUR099.
	//
	// It is a DECLARATION, not a one-shot signal like Applied: it rides the
	// value for its whole life, because "this function is a membership
	// test" is a property of what was written, not of one call site.
	Predicate bool
	// Applied is a ONE-SHOT signal that an application was explicitly
	// ASKED for at this value — set only by `apply` (native_valof.go) on a
	// fn whose only signatures are 0-arg, and consumed by the very next
	// re-step in execFnDefLiteral, which clears it alongside ReachGroup
	// under the same Quoted-transience discipline.
	//
	// It exists because the inert-lambda gate in execFnDefLiteral keys on
	// Anonymous, and that gate is load-bearing: it is what makes
	// `def f ([] => [42])` bind f to the FUNCTION rather than to 42. But
	// keying an APPLICATION on it made ORIGIN decide the outcome, which
	// ADR-016 forbids —
	//
	//	def z fn [[] [Integer] [42]]   z/v apply  ->  42
	//	def f ([] => [42])             f/v apply  ->  fn f   (was)
	//
	// The flag separates the two questions the gate had conflated: "is
	// this value data?" (origin) and "did someone ask to call it?"
	// (this). A lambda sitting on the stack still parks; the same lambda
	// behind an explicit `apply` dispatches through the ONE re-step path
	// every other arity already uses, so natives keep their Go handlers,
	// context mutations land in the caller's frame, and the check pass
	// models the result by re-stepping exactly as runtime does.
	//
	// Macros are deliberately NOT covered: applying a macro is never a
	// stack-value dispatch (design/MACROS-PHASE1.10.md §5, D4).
	Applied bool

	// ArgsReversed marks a value built by UsurpFunction (the `usurp` word and
	// the `/u` modifier): its signatures' Params are the wrapped word's in
	// REVERSE, and each carries a Go handler that re-dispatches the original.
	//
	// It exists because the reversal is otherwise UNDETECTABLE by inspection.
	// A wrapper keeps the wrapped word's Name, and comparing its params
	// against the registry's cannot see the swap whenever the sig is
	// homogeneous — `sub(Number, Number)` reversed is still
	// `sub(Number, Number)`. Measured the hard way: a param-type comparison
	// admitted `m.s/u 10 3` to a VM fast path that then answered -7 against
	// the interpreter's 7.
	//
	// Any future wrapper that changes the ARG-TO-PARAM mapping must set it
	// too. Wrappers that only re-base the barrier (`/s`, `/f`) must not —
	// they leave params alone, and the barrier is spent once the args are
	// collected.
	ArgsReversed bool
	// Wrap and Wraps expose what a MODIFIER WRAPPER re-dispatches, so a
	// consumer without a tape can do the re-dispatch itself.
	//
	// Every wrapper's handler returns TOKENS — a paren group
	// `( stack-part  orig  forward-part )` for the engine to step — which is
	// why a compiled dispatch of one has to island: a group needs a tape and
	// the VM has none. But the group is a pure RESHUFFLE, and working it
	// through CLAUDE.md's argument-order rule collapses it to a permutation
	// that does not depend on the barrier at all:
	//
	//	WrapReverse    sig[i] = args[n-1-i]   (usurp, `/u`)
	//	WrapRebarrier  sig[i] = args[i]       (forward-args, stack-args,
	//	                                       force-arity)
	//
	// So a consumer that can see through the wrapper can permute the args and
	// dispatch Wraps directly, with no tokens and no tape.
	//
	// ArgsReversed does NOT answer this. It is a one-way MARK — UsurpFunction
	// sets it true and the others propagate it — so `usurp (usurp f)` reports
	// reversed where the composed permutation is the identity. That is safe
	// for its own job (declining a fast path) and wrong for this one, which
	// is why the chain is walked rather than the flag read.
	Wrap  WrapKind
	Wraps *Value
	// Gen carries the generic-parameter spec for a generic fn
	// (`def identity gen [T] fn [[x:T] [T] [x]]`). Nil for ordinary
	// fns. Dispatch admission rides the placeholder nodes' Behaviors;
	// at each call the inferred bindings are installed as body-scoped
	// type bindings so `of [T]` / `make (Box of [T])` resolve. See
	// design/GENERICS.10.md Phase 4.
	Gen *GenSpecInfo
	// Captured holds enclosing-fn-local bindings snapshotted at fn-
	// construction time — the implementation of lexical closures.
	// Populated by computeCaptures during afn / fn handler execution
	// (fn_capture.go) when at least one body-Word resolves to a name
	// bound by an enclosing fn (Depth > TopFnBaseline). Installed as
	// defs in the per-call scope BEFORE body execution and torn down
	// by the same DefCleanup mechanism that pops named params; the
	// install/cleanup wiring lives in InstallFnDef (core_helpers.go),
	// execFnDefSig + spliceAnonCheckResult (engine.go), and CallBoru
	// (registry.go). Nil for top-level constructions and any inner
	// fn whose body references only params, module-global names, or
	// forward refs. See lang/go/CLAUDE.md "Closures and Capture".
	Captured []CapturedBinding
	// Extends marks this FnDefInfo as a WORD-EXTENSION CLONE: the result
	// of `def <word> fn […]` on a word that carries locked signatures
	// (design/OPEN-WORDS.0.md §2.1). The value is the base word's name.
	// A clone carries the base word's COMPLETE signature list plus the
	// merge, so Registry.Lookup stops aggregating at a clone — deeper
	// def-stack entries for the name are occluded, which is what makes
	// an unlocked-tuple REPLACEMENT effective and `undef` restore the
	// exact previous state. Detected via IsWordExtension (the named-
	// helper protocol — never probe the field inline); recognised at
	// module import for the export transplant. Empty for every ordinary
	// fn / native registration.
	Extends string
	// ExtOwner is the AUTHOR owner id of a HOST-authored word-extension
	// clone (design/OPEN-WORDS.1.md §4): the provenance the transplant
	// admission verifies the clone's anchor types against. Set only by
	// NewWordExtension from Go module builders — boru:time-util passes
	// its own id (matching the Date/duration types it registered),
	// boru:io passes OwnerKernel (its Pathon list/remove sigs anchor on
	// a kernel-owned type — the kernel-shipped host prerogative).
	// Empty for every source-authored clone, whose author is the
	// exporting module's TypeTable.MintOwner instead.
	ExtOwner string
	// ident is the function's IDENTITY token: the reference `eq`
	// compares for a fn value (NUR031). It is minted once, by
	// NewFunction, for a payload that arrives without one, and then
	// COPIED by every derivation that keeps the function the same
	// function — installation (InstallFnDef's compiled entry), the
	// cross-stack dispatch aggregate, the per-dispatch anon recompile,
	// a trivial-delegation rebind. Deriving a genuinely DIFFERENT
	// dispatch table (a word-extension clone, a usurp wrapper) builds a
	// fresh FnDefInfo literal instead, so it mints its own.
	//
	// A token is needed because the payload has no other stable
	// reference. The Signatures backing array looks like one and is
	// not: aggregateDispatch rebuilds the slice for every boru-bodied
	// word, per NAME, so `def a (f/v)` and `def b (f/v)` — one function
	// under two names — landed on two arrays and read as two functions.
	// The token is what survives the rebuild.
	//
	// Unexported so only this module can mint one: a FnDefInfo built as
	// a literal outside core (a compile-time carrier, a probe value)
	// carries nil and has no identity, which is the honest answer for a
	// synthesized payload with no fn behind it.
	ident *fnIdent
}

// fnIdent is a fn value's identity token — allocated for its ADDRESS
// alone, never read.
//
// The one byte is load-bearing. An empty struct is zero-sized, and Go
// gives every zero-sized allocation the same address (runtime.zerobase),
// so `&fnIdent{}` would hand every function in the program one shared
// token and make them all eq. A single byte forces distinct addresses.
type fnIdent struct {
	// closure is the identity SEQUENCE of the compiled closure this token
	// stands in for (NewFunctionIdentified): two tokens with the same
	// non-zero sequence are one function, however many bridges minted
	// them. Zero for an interpreter-minted fn, which identifies by ADDRESS.
	closure uint64
}

// FnIdentity is a fn value's identity token handed out OPAQUELY, so a value
// built outside core that stands in for one authored function can carry the
// token that function's copies share: a compiled closure (ClosurePayload.Ident,
// minted once at its push and copied with the value) and the FnDefInfo the VM
// bridges it to for the interpreter's dispatch. `eq` then answers what the
// interpreter answers for the source lambda — one function, however many
// copies the tape holds, bridged or not. A zero FnIdentity is "none": a
// hand-built payload has no identity and is eq to nothing, the honest answer
// for a value with no fn behind it.
//
// The identity is a process-wide SEQUENCE number, not a heap token: a closure
// is constructed on the VM's hot path (every `do body`, every element of an
// `each`), and the compiled lane's allocation ceilings (lang/go's alloc guard)
// hold one heap object per push against it — a counter costs an atomic add
// and nothing else. The token a bridge needs is minted at the bridge, where
// an allocation is already the price of the dispatch.
type FnIdentity struct{ seq uint64 }

var closureIdentSeq atomic.Uint64

// NewFnIdentity mints a fresh identity — one per fn CONSTRUCTION, as
// NewFunction mints one per authored function.
func NewFnIdentity() FnIdentity { return FnIdentity{seq: closureIdentSeq.Add(1)} }

// NewFunctionIdentified is NewFunction for a payload that stands in for a
// function whose identity already exists (the closure bridge): the token it
// mints carries id's sequence when id is one, else it is NewFunction's own.
func NewFunctionIdentified(info FnDefInfo, id FnIdentity) Value {
	if id.seq != 0 {
		info.ident = &fnIdent{closure: id.seq}
	}
	return NewFunction(info)
}

// CapturedBinding is one lexically-captured name in a closure. The
// list is sorted by Name for deterministic install order so that two
// captures with the same name (impossible today but cheap to keep
// reproducible) get a stable shadowing order.
type CapturedBinding struct {
	Name  string
	Value Value
}

// HasForwardSigs reports whether any compiled signature has a non-zero
// BarrierPos — i.e. at least one signature wants to collect args from
// tokens following the word. Used by the dispatcher to decide whether
// pre-evaluation of upcoming parens is worthwhile and whether a
// stack-only retry should be attempted on first-pass match failure.
// The result is derived from the sigs, not stored — Signature is the
// source of truth.
func (fn *FnDefInfo) HasForwardSigs() bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].BarrierPos > 0 {
			return true
		}
	}
	return false
}

// OwnSigs returns the function's authored overloads — its Signatures with
// any synthetic 0-arg Fallback sig filtered out. This is the view the
// authored-shape readers (canon, inspect, predicate probes, trivial-
// delegation, targeted undef, overlap detection) want: the signatures the
// user actually declared, never the dispatch-time fallback the aggregate
// injects.
func (fn *FnDefInfo) OwnSigs() []Signature {
	if fn == nil {
		return nil
	}
	hasFallback := false
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			hasFallback = true
			break
		}
	}
	if !hasFallback {
		return fn.Signatures
	}
	out := make([]Signature, 0, len(fn.Signatures))
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue
		}
		out = append(out, fn.Signatures[i])
	}
	return out
}

// FirstOwnSig returns a pointer to the first non-fallback signature and
// whether one exists. Used by the single-overload readers (predicate
// input-type gate, refine single-param probe).
func (fn *FnDefInfo) FirstOwnSig() (*Signature, bool) {
	if fn == nil {
		return nil, false
	}
	for i := range fn.Signatures {
		if !fn.Signatures[i].Fallback {
			return &fn.Signatures[i], true
		}
	}
	return nil, false
}

// FnSigSpec describes a signature specification without a body, used for
// targeted undef of specific function signatures.
type FnSigSpec struct {
	Params  []FnParam
	Returns []*Type
}

// FnUndefInfo holds signature specs for targeted undef of function signatures.
type FnUndefInfo struct {
	Sigs []FnSigSpec
}

// ReturnCheckInfo carries expected return types for fn-defined function validation.
type ReturnCheckInfo struct {
	FuncName string
	Returns  []*Type
	// ReturnPatterns mirrors FnSig.ReturnPatterns positionally against
	// Returns — the structural constraint for declared returns whose *Type
	// degraded to Any (a union output has no lattice node to name).
	ReturnPatterns []*Value
	UnnamedCount   int    // number of unnamed params pushed onto the stack before the body
	Pos            SrcPos // source position of the fn call site, for return errors
	// Decl is where the return contract was declared (the fn's output
	// signature), rendered as a secondary span on return errors. Zero
	// when unknown (Go-registered sigs).
	Decl DeclSite
}

// ReturnPattern returns the structural constraint declared for return
// position k, or nil. Tolerates a short or absent slice so a sig that
// declares patterns for only some positions needs no padding.
func (rc ReturnCheckInfo) ReturnPattern(k int) *Value {
	if k < 0 || k >= len(rc.ReturnPatterns) {
		return nil
	}
	return rc.ReturnPatterns[k]
}

// DisjunctInfo holds the alternatives for a disjunction (union) type.
// A disjunct unifies if any of its alternatives unifies with the target.
type DisjunctInfo struct {
	Alternatives []Value
	// Declared marks a check-mode carrier disjunct denoting a DECLARED
	// union domain — a named-union fn parameter binding
	// (ParamInputCarrier), where every alternative is an author-claimed
	// valid input. A body dispatch that fails for such an alternative is
	// an ERROR (the fn breaks its contract for a valid argument), where
	// the same partial failure on an ANALYSIS-join disjunct (an if/else
	// branch join, a heterogeneous element join) stays a warning — there
	// the runtime materialises only one alternative, so the failing path
	// may be dead. Never set on runtime disjunct values.
	Declared bool
}

// NegationInfo holds the inner type of a negation (complement) type. A
// value satisfies `tnot Inner` iff it does NOT satisfy Inner. The
// negation is the kernel's set-theoretic complement; together with
// DisjunctInfo (union) and TandValues (intersection) it closes the type
// algebra under Boolean operations — see
// design/elixir-types-in-boru-report.10.md.
type NegationInfo struct {
	Inner Value
}

// ClassTypeInfo holds the type definition for an object type.
// Object types form an inheritance hierarchy analogous to class inheritance.
// For example, Object/Foo has parent Object, Object/Foo/Bar has parent Foo.
// Fields are the type's own fields (not including inherited ones).
// Parent points to the parent object type (nil for direct children of Object root).
// ID is a unique internal identifier: "T_" followed by 12 lowercase hex characters.
// Name is the full type path (e.g. "Object/Foo/Bar"), set when the type is
// registered via def.
type ClassTypeInfo struct {
	Fields          *OrderedMap    // own fields (field name → type-constraint Value)
	Parent          *ClassTypeInfo // parent class type (nil if direct child of Ideal/Class)
	ID              string         // unique internal ID: "T_" + 12 hex chars
	Name            string         // full type path (e.g. "Class/Foo/Bar")
	Type            *Type          // canonical *Type identity; populated by MintType during installation
	BinaryLayout    Value          // for a binary-frame spec (a class carrying a Scalar/Bytes wire layout): the raw layout List<Map>; zero Value otherwise. Read by the Bytes codec (make/unpack/convert) and the Binary/BinarySpec membership predicates. See lang/go/native/native_bytes.go and design/go-modules/BYTES.10.md.
	cachedAllFields *OrderedMap    // lazily computed merged field map (immutable after first call)
}

// AllFields returns all fields including inherited ones. Parent fields come
// first, followed by the type's own fields. Own fields override inherited
// fields with the same name. The result is cached since ClassTypeInfo is
// immutable after registration.
func (o *ClassTypeInfo) AllFields() *OrderedMap {
	if o.cachedAllFields != nil {
		return o.cachedAllFields
	}
	result := NewOrderedMap()
	if o.Parent != nil {
		parentFields := o.Parent.AllFields()
		for _, k := range parentFields.Keys() {
			v, _ := parentFields.Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range o.Fields.Keys() {
		v, _ := o.Fields.Get(k)
		result.Set(k, v)
	}
	o.cachedAllFields = result
	return result
}

// ClassInstanceInfo holds a concrete instance of an object (class)
// type. TypeRef points back to the ClassTypeInfo that created this
// instance; Fields holds every resolved field. Instances are FLAT —
// class construction resolves the full field set (own + inherited)
// eagerly at make, so there is no prototype chain (open objects, which
// used one, were removed).
type ClassInstanceInfo struct {
	TypeRef *ClassTypeInfo // the object type this is an instance of
	Fields  *OrderedMap    // resolved field values (flat: own + inherited)
}

// GetField returns a field value (flat lookup — instances carry every
// field resolved at make).
func (oi ClassInstanceInfo) GetField(name string) (Value, bool) {
	return oi.Fields.Get(name)
}

// AllFields returns the instance's resolved fields (flat).
func (oi ClassInstanceInfo) AllFields() *OrderedMap {
	result := NewOrderedMap()
	for _, k := range oi.Fields.Keys() {
		v, _ := oi.Fields.Get(k)
		result.Set(k, v)
	}
	return result
}

// ResourceTypeInfo holds the type definition for the SDK Resource /
// Entity object-type hierarchy (Ideal/Resource, Ideal/Resource/Entity).
// It mirrors the shape of a class type — own fields, a parent chain, a
// minted lattice identity — but is a distinct payload so the shared
// class-instance representation stays class-only. Instances are flat
// (schema resolved eagerly at make; no prototype delegation).
type ResourceTypeInfo struct {
	Fields          *OrderedMap       // own fields (field name → type-constraint Value)
	Parent          *ResourceTypeInfo // parent resource type (nil for Resource root)
	ID              string            // unique internal ID: "T_" + 12 hex chars
	Name            string            // full type path (e.g. "Ideal/Resource/Entity")
	Type            *Type             // canonical *Type identity; populated by MintType during installation
	cachedAllFields *OrderedMap       // lazily computed merged field map
}

// AllFields returns all fields including inherited ones (parent first,
// own overriding), mirroring ClassTypeInfo.AllFields.
func (o *ResourceTypeInfo) AllFields() *OrderedMap {
	if o.cachedAllFields != nil {
		return o.cachedAllFields
	}
	result := NewOrderedMap()
	if o.Parent != nil {
		for _, k := range o.Parent.AllFields().Keys() {
			v, _ := o.Parent.AllFields().Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range o.Fields.Keys() {
		v, _ := o.Fields.Get(k)
		result.Set(k, v)
	}
	o.cachedAllFields = result
	return result
}

// ResourceInstanceInfo is a flat, schema-resolved instance of a
// ResourceType. Like a class instance it has no prototype chain — every
// field (own + inherited) is resolved at make into a single map.
type ResourceInstanceInfo struct {
	TypeRef *ResourceTypeInfo // the resource type this is an instance of
	Fields  *OrderedMap       // resolved field values (flat: own + inherited)
}

// GetField returns a field value (flat lookup — no prototype chain).
func (ri ResourceInstanceInfo) GetField(name string) (Value, bool) {
	return ri.Fields.Get(name)
}

// StoreInstanceInfo is a copy-on-write key-value store (Object/Store).
// Unlike regular Object instances which have typed fields, Store instances
// hold arbitrary key-value pairs. Key resolution walks the prototype chain,
// enabling scope-like lookup when contexts are nested.
//
// Copy-on-write: the boru `set` word creates a new Store layer (prototype =
// old Store) instead of mutating in place. If this Store is nested inside
// a parent Store, the parent is COW'd too, propagating up to the ctxStack.
type StoreInstanceInfo struct {
	TypeName  string             // full type path, e.g. "Object/Store" or "Object/Store/System"
	Data      map[string]Value   // own key-value pairs (COW layer)
	Deleted   map[string]bool    // tombstones: keys this layer hides from Prototype
	Prototype *StoreInstanceInfo // prototype chain for key lookup / COW base
	Parent    *StoreInstanceInfo // containing Store (for COW propagation), nil if root
	ParentKey string             // key in Parent that references this Store
}

// Get looks up a key in this store, walking the prototype chain if not
// found. A TOMBSTONE stops the walk: `del` on a Store cannot remove an
// inherited key from the layer that owns it (that layer is shared), so the
// deleting layer records the key as hidden and lookup reports it absent —
// the same shape as a JS prototype chain's own-property shadowing, except
// the shadow is "no value" rather than a value. Data wins over Deleted:
// a set after a del re-binds the key in the same layer.
func (si *StoreInstanceInfo) Get(key string) (Value, bool) {
	if v, ok := si.Data[key]; ok {
		return v, true
	}
	if si.Deleted[key] {
		return Value{}, false
	}
	if si.Prototype != nil {
		return si.Prototype.Get(key)
	}
	return Value{}, false
}

// VisibleEntries returns every key this store ANSWERS FOR with the value
// it answers WITH, walking the prototype chain in one pass with masking
// and tombstones — the same rule Get applies, so enumeration and lookup
// describe one keyset (NUR052).
//
// Enumeration used to read only the newest copy-on-write layer while Get
// walked the chain, so two sets through `context set` left two keys
// reachable by `get`/`has` and one visible to `size` / `convert Map`. A
// container whose enumeration disagrees with its lookup is answering
// about two different things.
//
// The walk mirrors Get's precedence exactly, which is what makes them
// agree by construction rather than by coincidence:
//
//   - a key in this layer's own Data wins, so a child's value SHADOWS the
//     parent's and the key appears ONCE, at the child's value;
//   - a key tombstoned here is invisible from here down, and the walk
//     does not consult the prototype for it;
//   - otherwise the prototype answers.
//
// It returns VALUES, not just keys, and that is a correctness property
// rather than a convenience: resolving each key with a separate Get from
// the head would re-walk the chain per key, which is quadratic in the
// layer count (a Store with one key per layer over 10k layers spends
// seconds in `convert Map` alone). Capturing the winning value at the
// moment the walk first sees the key keeps it linear.
//
// A tombstone masks only when its value is TRUE. `Deleted` is an exported
// map a host can write, and Get tests the boolean rather than the key's
// presence — so `Deleted[k] = false` leaves the inherited key live. The
// walk must test it the same way or enumeration would hide a key lookup
// still answers, which is this record's divergence in miniature.
//
// Sorted by key, because every caller renders or compares the result and
// map iteration order is not stable.
func (si *StoreInstanceInfo) VisibleEntries() []StoreEntry {
	seen := map[string]bool{}
	var out []StoreEntry
	for layer := si; layer != nil; layer = layer.Prototype {
		for k, v := range layer.Data {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, StoreEntry{Key: k, Value: v})
		}
		// A tombstone masks the key for every DEEPER layer, but only after
		// this layer's own Data has been consulted: CowSet-after-CowDel
		// writes the key back, and that revival must win.
		for k, dead := range layer.Deleted {
			if dead {
				seen[k] = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// StoreEntry is one visible key/value pair from VisibleEntries.
type StoreEntry struct {
	Key   string
	Value Value
}

// VisibleKeys is VisibleEntries projected to its keys, for callers that
// only need the keyset.
func (si *StoreInstanceInfo) VisibleKeys() []string {
	entries := si.VisibleEntries()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Key
	}
	return out
}

// Set stores a key-value pair directly (for internal/init use only).
// boru code should use the set word which does COW via CowSet.
func (si *StoreInstanceInfo) Set(key string, val Value) {
	// Allocate on demand: a layer pushed by ContextStack.Push carries a nil
	// Data map (writes go through CowSet, which replaces the layer rather
	// than mutating it), and this is the one path that writes in place.
	if si.Data == nil {
		si.Data = make(map[string]Value)
	}
	si.Data[key] = val
	// Track parent relationship for nested Stores.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = si
		childStore.ParentKey = key
	}
}

// "T_" followed by 12 lowercase hex characters (6 random bytes).
func GenerateObjectTypeID() string {
	return GenerateID("T_")
}

// markCounter is a global counter for generating unique mark IDs.
var markCounter atomic.Int64

// NextMarkID generates a unique mark ID.
func NextMarkID() string {
	n := markCounter.Add(1)
	return fmt.Sprintf("_m%d", n)
}

// MarkInfo identifies a mark on the stack. Marks are internal control-flow
// anchors placed by constructs like for-loops. Each mark has a unique ID
// so that a corresponding move can jump the pointer back to it.
// Body stores the original values between the mark and its paired move,
// enabling replay when the move fires.
type MarkInfo struct {
	ID   string  // unique identifier for this mark
	Body []Value // original content to replay (set by the move on first encounter)
}

// SpliceInfo carries an __SP (splice) marker's payload. When an __SP value
// reaches the engine pointer it is replaced, unevaluated, by its Data: a
// plain list contributes its top-level elements, any other value contributes
// itself. The spliced content is then re-stepped against the live stack, so a
// word-bearing list behaves as a Forth-style macro. Produced by the `word`
// native and by `def name word value`. See engine.go::stepLiteral.
type SpliceInfo struct {
	Data Value // the wrapped payload to splice in
}

// MoveInfo identifies a move on the stack. When the stack pointer reaches
// a move, it jumps back to the corresponding mark. The Reason field
// describes why the move exists (e.g. "for loop") and is used in error
// messages when the target mark cannot be found.
//
// Cont optionally carries for-loop continuation state. When set, stepMove
// uses it to drive multi-iteration loops: each firing advances the iterator,
// conditionally re-inserts mark+body+move for the next iteration, and
// accumulates results across iterations.
type MoveInfo struct {
	To     string   // ID of the target mark
	Reason string   // human-readable reason (for error messages)
	Cont   *ForCont // optional: for-loop iteration state
	IfCont *IfCont  // optional: if-statement continuation state
}

// ForCont holds the iteration state for a mark/move-driven for loop.
// It is carried by the MoveInfo and mutated across iterations.
type ForCont struct {
	Registry *Registry
	IterName string  // name of the iterator variable (e.g. "i")
	Current  int64   // current iteration value
	End      int64   // exclusive bound
	Step     int64   // increment per iteration
	Body     []Value // original body tokens (replayed each iteration)
	Results  []Value // accumulated results from completed iterations

	// While mode — a `while` loop rides the same continuation and
	// mark/move machinery as `for`, so break/continue's loop resolvers
	// find it with no second scan. WhileCond non-nil marks the mode:
	// IterName is empty and Current/End/Step are unused; the move
	// alternates between a CONDITION region (WhileInBody false, the
	// region's last value decides) and a BODY region (WhileInBody true,
	// the region's values accumulate into Results). Every region runs
	// through the ordinary Run loop, so the step budget meters the loop
	// and `while [true] []` trips evaluation_limit instead of hanging.
	WhileCond   []Value
	WhileInBody bool
}

// IfCont holds the continuation state for a mark/move-driven if statement.
// When the move fires, the condition result (between mark and move) is
// evaluated for truthiness to select the appropriate branch.
type IfCont struct {
	Then []Value // tokens to splice if condition is truthy
	Else []Value // tokens to splice if condition is falsy (nil for 2-arg if)
}

// ModuleDesc describes a module: its generated ID and named exports.
// Each export call adds a named entry mapping export name → export map.
type ModuleDesc struct {
	ID      string                 // generated internal identifier
	Exports map[string]*OrderedMap // export name → export map (name → value)
	// Descriptor metadata, populated by the loader (Resolve / loadFileModule
	// / RunModuleBody) and surfaced on the Ideal/Module instance at import.
	Ref    string // external module reference ("boru:math-util", "./lib.boru"); "" inline
	Kind   string // "native" | "file" | "inline"
	File   string // source file path ("" for native/inline)
	Folder string // source folder ("" for native/inline)
	// Src is the module's OWN registry — the sub-registry a native
	// builder registered its words/types/ideals into, or the body
	// registry an inline/file module ran in. Import uses it to
	// transport ESCAPED type machinery to the importer: each exported
	// bare type literal adopts its canonical node into the importer's
	// TypeTable (the compiled OpPushType path) and brings its matching
	// Ideal across, so a facade module re-exporting a constructible
	// type keeps `make` working. In-process only — never serialized.
	Src *Registry
}

// WrapKind names what a modifier wrapper does to the ARG-TO-PARAM mapping of
// the value it wraps. The zero is "not a wrapper", which is the honest
// reading of an absent Wraps.
type WrapKind uint8

const (
	WrapNone WrapKind = iota
	// WrapReverse reverses the arg-to-param mapping: `usurp` and `/u`.
	WrapReverse
	// WrapRebarrier leaves the mapping alone and only re-bases the barrier:
	// `forward-args` / `/f`, `stack-args` / `/s`, and `force-arity` / `/N`.
	// The barrier is spent once the args are collected, so by the time a
	// consumer holds them in sig order this wrapper is the identity.
	WrapRebarrier
)

// UnwrapModifierChain resolves a stack of modifier wrappers to the value they
// ultimately re-dispatch, and to the permutation the whole chain applies.
//
// It walks rather than reading ArgsReversed because that flag does not
// compose: it is set true by UsurpFunction and propagated by the others, so
// two usurps report reversed where they cancel. Parity is the property that
// matters here, and only the walk has it.
//
// ok is false when v is not a wrapper, in which case base is v unchanged.
func UnwrapModifierChain(v Value) (base Value, reverse bool, ok bool) {
	base = v
	for depth := 0; depth < 64; depth++ {
		fd, isFn := base.Data.(FnDefInfo)
		if !isFn || fd.Wraps == nil {
			return base, reverse, ok
		}
		if fd.Wrap == WrapReverse {
			reverse = !reverse
		}
		base = *fd.Wraps
		ok = true
	}
	// A cycle is impossible by construction (each wrapper captures an
	// ALREADY-BUILT value, so the chain is a finite tree path), but a bound
	// costs nothing and a runaway walk in the VM would be far worse than a
	// declined fast path.
	return v, false, false
}

// WordInfo carries the name and optional modifiers for a function reference.
type WordInfo struct {
	Name         string
	ArgCount     int  // -1 = unspecified
	ForceStack   bool // lower/s
	ForceForward bool // lower/f
	ForceVal     bool // lower/v — resolve to the bound value without invoking
	ForceUsurp   bool // lower/u — wrap the bound fn so its sig arg order is reversed
}

// ForwardInfo tracks forward argument collection for a deferred function call.
type ForwardInfo struct {
	FuncName      string
	ExpectedArgs  int
	CollectedArgs int
	StackArgs     int // number of sig args already consumed from the stack
	// FuncIndex records where the deferred function word sits in the stack.
	FuncIndex int
	Sig       *Signature // the matched signature, for direct execution on completion
	Pos       SrcPos     // source position of the forward-collecting word, for errors

	// Speculative records the planner's stop condition for the arrival
	// loop (design/FORWARD-COLLECTION-PHASES.10.md): matchSignature
	// filled at least one forward slot with a WORD bound to a
	// dispatching definition (an FnDefInfo binding accepted through the
	// Any-slot escape) — the plan treats that token as an operand, but
	// at runtime it dispatches as an operator. SpeculativeAt is the
	// sig-order index of the first such slot and is meaningful only
	// when Speculative is set (the bool guards the int so the struct's
	// zero value means "none" — No-Zero-Overload rule). The index is
	// plan-side bookkeeping: under zero/multi-value paren collapse the
	// arrival count can drift from plan positions, so consumers may use
	// it for tracing/diagnostics, never for slot arithmetic. The
	// commitBarrierForward scan must NOT be gated on this field — the
	// barrier commit doubles as zero-value-collapse recovery where the
	// plan had no speculative word at all.
	Speculative   bool
	SpeculativeAt int
}

// Value is the single node type of the boru kernel: it is at once a
// runtime value (an entry on the stack) and a node in the type
// lattice. Value and Type were historically separate structs; they
// are now one, with Type an alias for Value (see typetable.go). A
// leaf (5, 'hello', [1 2 3]) and a type node (Integer, List, Any)
// are the same kind of thing, differing only in which fields carry
// data.
//
// The lattice is encoded by Parent: a leaf's Parent is its type, a
// type node's Parent is its supertype.
//
// Every value carries a unique ID with a prefix indicating its
// category:
//   - "S_" for scalar values (String, Number, Boolean)
//   - "N_" for node values (List, Map, Table, Record)
//   - "W_" for word values (Word, Atom, Function, Internal/*)
//   - "T_" for type/object values (Object/*, type literals, Any, None)
//
// Each ID is the prefix followed by 12 lowercase hex characters.
//
// Field order is chosen to eliminate alignment padding: the three
// 16-byte fields (ID, DynFrom, Data) and the three pointers come first,
// then every one-byte field is clustered at the tail so the seven bools
// plus Origin pack into a single 8-byte-aligned run with no gaps. This is
// a pure layout choice — fields are still accessed by name — and it took
// the struct from 96 to 80 bytes on its own (design/
// INTERPRETER-SPEED-PLAN.10.md #1A, bool-packing follow-up).
type Value struct {
	ID string
	// Data is the kernel-known data payload; see payload.go for variants.
	Data Payload

	// Parent is the node directly above this one in the unified
	// lattice: for an ordinary value it is the value's type, for a
	// type node it is the supertype. nil only for lattice roots.
	Parent *Type
	// tmeta holds type-node metadata — the leaf Name, the five integer
	// lattice fields (FixedID, Rank, Depth, In, Out), and the Behavior —
	// behind ONE pointer instead of ~72 inline bytes on every Value. The
	// struct is copied by value on every stack push, arg, and tape cell,
	// so shrinking it cuts the interpreter's ~15% duffcopy cost (design/
	// INTERPRETER-SPEED-PLAN.10.md #1A). These fields are set once at type
	// registration, so a Value copy shares the SAME *typeMeta (an orphan
	// `&v`-derived *Type reads the identical metadata as its canonical
	// node — the property the old inline fields gave for free). Nil on
	// ordinary runtime values; the Name/FixedID/Rank/Depth/In/Out/Behavior
	// accessors return the zero value for nil. Writers go through
	// ensureTMeta so a mint site that forgets to allocate one cannot
	// nil-panic.
	tmeta *typeMeta
	// pos is the source position for error reporting, behind a pointer so
	// the ~24 inline bytes of SrcPos (Row/Col/Src) don't ride on every
	// Value copy — nil means "unknown" (design/INTERPRETER-SPEED-PLAN.10.md
	// #1A, Pos follow-up). A position is minted once at parse time; the
	// interpreter then THREADS it by copying the pointer (WithPos, the
	// internal `.pos = other.pos` assignments), so no per-value SrcPos is
	// allocated on the hot path and synthesized values (nil pos) carry
	// none. The Src text is never mutated after parse, so sharing one
	// *SrcPos across every copy of a value is sound. Read via Pos()
	// (nil-safe, returns the zero SrcPos); external packages set it via
	// SetPos since the field is unexported.
	pos *SrcPos
	// dynFrom is the binding name a dynamic carrier was resolved from
	// (check mode ONLY — never read at runtime), behind a pointer so its
	// 16-byte string doesn't ride on every runtime Value copy (nil for the
	// overwhelming majority that are not check-mode dynamic carriers). It
	// lets narrowing-through-use tighten that binding to dynamic(bound ∩
	// slot) at a typed use so a later provably-disjoint use of the same
	// name is caught. Read via DynFrom() (nil-safe), set via SetDynFrom.
	dynFrom *string
	// elem is the retained ELEMENT constraint of a concrete typed container
	// ({:T} map / [:T] list), behind a pointer so its Value doesn't ride on
	// every runtime copy (nil for the overwhelming majority — everything that
	// is not a concrete typed container). Construction (unifyTyped*WithConcrete)
	// is the sole origin; it lets a concrete {:T} value stay CONCRETE (Data is
	// still the MapPayload/ListPayload, AsMap/AsList/IsConcrete unchanged) while
	// carrying the element type that write-enforcement (set/setpath/merge) and
	// strict reads consult. The constraint is the child Value verbatim — a bare
	// type literal, a disjunct ({:(A tor B)}), or a nested typed-container
	// literal ({:[:Integer]}). Read via ElemConstraint() (nil-safe), set via
	// SetElemConstraint. See design/TYPED-CONTAINER-TAG-RETENTION.0.md.
	elem *Value
	// asc is the value's dispatch ASCRIPTION (`v as T` — design/
	// OPEN-WORDS.1.md §9): an ancestor type that signature MATCHING treats
	// as the value's tag while the value itself — payload, real Parent,
	// rendering, equality — is untouched. Upcast-only (the `as` word
	// refuses a non-ancestor), match-time-only: every arg-delivery boundary
	// (execMatch, fn-param binding, the VM call/bind/make ops) strips it,
	// so the ascription selects exactly one dispatch and never leaks into
	// handlers, bindings, containers, or results. Behind a pointer so the
	// overwhelming majority of values (no ascription) don't carry it. Read
	// via AscribedType() (nil-safe), set/clear via SetAscribed.
	asc *Type
	// modns marks a MAP value as a module NAMESPACE — the value `import`
	// binds for each `export "Name" {…}`. Behind a nil-by-default pointer
	// so the overwhelming majority of values carry no inline bytes for it
	// (the pos/dynFrom/elem/asc discipline). The map's own payload IS the
	// export set; the facet carries only the PROVENANCE the retired
	// Ideal/ModuleExport wrapper used to encode — the export name
	// (`$name`) and the owning Module descriptor (`$module`) — so an
	// exported fn stays a Function, an exported constant stays that
	// constant, and an exported type stays a plain type (NUR038:
	// provenance is a facet on a plain value, never a wrapper
	// type that masks the value). Read via ModuleNSOf (nil-safe free
	// function); set via WithModuleNS. The Sealed-Payload rule is
	// untouched: the facet holds a Value the kernel treats opaquely.
	modns *ModuleNSInfo

	// One-byte fields, clustered so they pack without padding.
	IsInternal bool       // Word/__XX runtime markers — not user-facing
	Origin     OriginKind // builtin / userdef
	Quoted     bool       // produced by the quote word; prevents auto-evaluation
	Eval       bool       // parser-created list that should auto-evaluate at end of Run
	Undefined  bool       // atom created from an undefined word (error if left on result stack)
	// FailedDispatch marks a named Function value that a dispatch attempt
	// left on the tape because no signature matched. CHECK MODE ONLY: at
	// runtime the failure raises at the dispatch site
	// (design/FN-VALUE-DISPATCH.0.md), so no such value survives. Analysis
	// continues past the finding, so the marker is how a later check-mode
	// consumer tells dispatch wreckage from a value the program meant to
	// produce (defWordExtension reads it).
	FailedDispatch bool
	// ReachGroup marks an OPEN-PAREN marker that the REACH LOWERING emitted
	// rather than the user writing — the synthetic group `a.b` expands to
	// (expandReach → lowerReach). A function value alone inside such a group
	// must not dispatch there; see execFnDefLiteral. Only the open marker
	// carries it, since that is the side the dispatch test looks at, and
	// IsOpenParen keys on Parent alone so every other marker consumer is
	// unaffected.
	ReachGroup bool
	Carrier    bool // static-typecheck carrier (type-only, Data stripped of concrete payload)
	// Dynamic marks a carrier as a bounded gradual value (Elixir-style
	// dynamic(T) — design/dynamic-modality-report.10.md). Implies Carrier.
	// Its Parent/Data is a BOUND, not a proven type: at a signature
	// boundary it matches the slot unless PROVABLY disjoint from it
	// (not-disjoint rule), rather than by strict ConformsTo. Set only on
	// carriers the checker cannot prove exactly (escape hatches); cleared
	// by a successful guard, which discharges the gradual obligation.
	Dynamic bool
}

// typeMeta carries the integer lattice fields of a type node, held behind
// Value.tmeta so ordinary values don't pay 40 inline bytes for metadata
// only type nodes populate. All fields are assigned once at registration
// (MintType / RegisterType / the builtin-decl loop /
// labelIntervals) and never mutated, so sharing one *typeMeta across
// every copy of a type Value is sound.
type typeMeta struct {
	// Name is the type-node leaf name (e.g. "ProperString").
	Name string
	// Behavior is the type node's pluggable dispatch (Match/Format/Equal +
	// optional capabilities); non-nil is the marker of a type node. A
	// `behave` word rewrites it through the canonical *Type — and because
	// every copy of a type Value shares this one *typeMeta, that rewrite is
	// now visible through every copy, not just the canonical pointer (the
	// orphan-*Type gap the CanonicalType discipline exists to paper over).
	Behavior TypeBehavior
	// FixedID is >0 for builtin type nodes; 0 otherwise. Baked into the
	// serialised Value ID (formatFixedID).
	FixedID int
	// Rank is the unified lattice rank — the total order CompareValues /
	// compareTypes use for every cross-type ordering.
	Rank int
	// Depth is the parent-chain length (root = 1); 0 = unset (ad-hoc
	// *Type). Cached for typeDepth / LCA.
	Depth int
	// In/Out are the DFS nested-set interval of a type node within the
	// STATIC builtin lattice: In is the pre-order entry number, Out the
	// largest entry number in the node's subtree. A descendant d of an
	// ancestor a satisfies a.In <= d.In <= a.Out, so IsAncestor is an O(1)
	// range test instead of a parent-chain walk. Assigned once by
	// labelIntervals at builtin-table construction; 0 = unlabelled
	// (minted, external, or ad-hoc types), which routes IsAncestor through
	// the walk.
	In  int
	Out int
	// Owner is the registration/mint provenance of the type node — the
	// authority the ownership-anchored signature rules check against
	// (design/OPEN-WORDS.1.md §4): OwnerKernel for kernel builtins and
	// kernel-shipped global types, a module id (e.g. "boru:matrix-util",
	// "module#N" for source modules) for module registrations and
	// module-body mints, OwnerProgram for top-level program mints.
	// Empty means UNOWNED (ad-hoc / test-fixture types) — an unowned
	// type never anchors an extension and is never redefinable-by-owner.
	Owner string
	// Body is the structural CONTENT the type was declared with — the
	// value InstallType bound alongside the minted node (the disjunct's
	// alternatives, the class schema, the record shape, the singleton
	// inhabitant, …), nil for kinds with no structure (builtins, bare
	// refine newtypes) and for adopted aliases (their content is the
	// adopted node's own). It makes the declaration's structure
	// recoverable FROM the node (design/TYPE-REPRESENTATION.1.md §N2),
	// generalizing the per-kind recoveries (SurfaceInfoOf, SchemaInfoOf,
	// UnionCarrierForType, ResolveTypeLiteralDef) into one accessor,
	// TypeBody. Stamped once at install; shared through the tmeta
	// pointer like every other field here.
	Body *Value
}

// ensureTMeta returns v's typeMeta, allocating it if absent. Writers of
// the lattice integer fields call this so a mint site that did not
// pre-allocate a typeMeta cannot nil-panic. v must be addressable (a
// *Type or an addressable Value).
func (v *Value) ensureTMeta() *typeMeta {
	if v.tmeta == nil {
		v.tmeta = &typeMeta{}
	}
	return v.tmeta
}

// SetName sets the type-node leaf name, allocating the typeMeta if
// needed. Exported because the leaf name now lives behind the unexported
// tmeta pointer, so external packages (lang, tests) that build or rename
// an ad-hoc *Type node cannot assign the field directly.
func (v *Value) SetName(name string) {
	v.ensureTMeta().Name = name
}

// Behavior is the nil-safe read of the type node's pluggable dispatch —
// nil for an ordinary value (no tmeta). A non-nil result is the marker of
// a type node, exactly as the inline field was.
func (v Value) Behavior() TypeBehavior {
	if v.tmeta == nil {
		return nil
	}
	return v.tmeta.Behavior
}

// TypeBody returns the structural content the type node was declared
// with, and whether one was recorded — the node-side recovery of the
// declaration's structure (design/TYPE-REPRESENTATION.1.md §N2). A
// kind with no structure (a builtin, a bare refine newtype) and an
// ordinary value both answer false. The returned Value is a copy; the
// stored content is written once at install and never mutated.
func (v Value) TypeBody() (Value, bool) {
	if v.tmeta == nil || v.tmeta.Body == nil {
		return Value{}, false
	}
	return *v.tmeta.Body, true
}

// SetTypeBody records the node's declared structural content. Called
// by InstallType (and kind installers) at mint time; the stamp is
// visible through every copy of the node via the shared typeMeta.
func (v *Value) SetTypeBody(body Value) {
	b := body
	v.ensureTMeta().Body = &b
}

// TypeContentOf returns the structural content a type value stands
// for: a bare node's recorded declaration body (TypeBody), or the
// value itself when it already IS type content (a payload-shaped type
// body). Consumers that operate on a type's STRUCTURE — make, refine
// bases, describe/inspect schema views, the type-algebra words — call
// this at entry so a name (which evaluates to its node after the
// Stage 2 flip) and an inline body are one case
// (design/TYPE-REPRESENTATION.1.md §N2).
func TypeContentOf(v Value) (Value, bool) {
	if IsBareTypeNode(v) {
		return v.TypeBody()
	}
	if v.Data != nil && v.Data.IsTypeContent(&v) {
		return v, true
	}
	return Value{}, false
}

// SetBehavior installs the type node's Behavior, allocating the typeMeta
// if needed. Exported because Behavior now lives behind the unexported
// tmeta pointer (the `behave` word and behavior-registering init code set
// it through here).
func (v *Value) SetBehavior(b TypeBehavior) {
	v.ensureTMeta().Behavior = b
}

// Pos returns the value's source position, or the zero SrcPos (Row/Col 0
// = unknown) when none is set. Nil-safe read of the pos pointer.
func (v Value) Pos() SrcPos {
	if v.pos == nil {
		return SrcPos{}
	}
	return *v.pos
}

// SetPos attaches a source position, allocating a *SrcPos. Internal eng
// code threads a position without allocating by copying the pos pointer
// directly (`v.pos = other.pos`, as WithPos does); SetPos is the exported
// entry point for the parser and lang layer, where the field is not
// reachable and a fresh position is being minted anyway.
func (v *Value) SetPos(p SrcPos) {
	v.pos = &p
}

// DynFrom returns the check-mode binding name a dynamic carrier was
// resolved from, or "" when unset. Nil-safe read of the pointer.
func (v Value) DynFrom() string {
	if v.dynFrom == nil {
		return ""
	}
	return *v.dynFrom
}

// SetDynFrom records (name != "") or clears (name == "") the dynamic
// carrier's origin binding. A check-mode-only path, so the *string it
// allocates is off the runtime hot path.
func (v *Value) SetDynFrom(name string) {
	if name == "" {
		v.dynFrom = nil
		return
	}
	v.dynFrom = &name
}

// ElemConstraint returns the retained element constraint of a concrete
// typed container ({:T} map / [:T] list) and true, or a zero Value and
// false when v carries no tag (the common case). Nil-safe read of the
// pointer. See design/TYPED-CONTAINER-TAG-RETENTION.0.md.
func (v Value) ElemConstraint() (Value, bool) {
	if v.elem == nil {
		return Value{}, false
	}
	return *v.elem, true
}

// SetElemConstraint attaches (c present) or clears (c a zero/bare-Any
// literal) the element constraint carried by a concrete typed container.
// Construction (unifyTyped*WithConcrete) is the sole caller; an Any child
// clears the tag (an untyped container is not element-constrained).
func (v *Value) SetElemConstraint(c Value) {
	if c.Parent == nil || c.Parent.Equal(TAny) {
		v.elem = nil
		return
	}
	v.elem = &c
}

// ModuleNSInfo is the module-namespace facet a bound export map carries
// (the modns field): the export name and the owning Module descriptor.
// It replaces the retired Ideal/ModuleExport wrapper type (NUR038) —
// the namespace VALUE is a plain Map; this is provenance only.
type ModuleNSInfo struct {
	// Name is the declared export name (`X.$name`), which a rename
	// import deliberately does NOT alias.
	Name string
	// Module is the owning Ideal/Module descriptor (`X.$module`),
	// shared by every export of one module. The kernel treats it
	// opaquely (Sealed Payload — the descriptor's ExtensionPayload is
	// never inspected here).
	Module Value
}

// ModuleNSOf returns the module-namespace facet carried by v, or nil for
// the overwhelming majority of values that are not bound module
// namespaces. Nil-safe.
func ModuleNSOf(v Value) *ModuleNSInfo {
	return v.modns
}

// WithModuleNS returns v (a plain export Map) carrying the module-
// namespace facet. The copy discipline matches the other facets: every
// Value copy shares the pointer, so the provenance travels with the
// namespace wherever it flows (re-export, def-binding, container
// storage) without the value ever ceasing to be a plain Map.
func WithModuleNS(v Value, name string, module Value) Value {
	v.modns = &ModuleNSInfo{Name: name, Module: module}
	return v
}

// AscribedType returns the value's dispatch ascription (`v as T`), or nil
// for the overwhelming majority of values that carry none. Nil-safe read
// of the pointer; see the asc field doc for the semantics.
func (v Value) AscribedType() *Type {
	return v.asc
}

// SetAscribed attaches (t non-nil) or clears (t nil) the dispatch
// ascription. The `as` word's handler is the sole attach site; the
// arg-delivery boundaries (execMatch, fn-param binding, the VM call/bind/
// make ops) clear it via StripAscribed.
func (v *Value) SetAscribed(t *Type) {
	v.asc = t
}

// StripAscribed returns v without its dispatch ascription — the
// arg-delivery form of the value. The no-ascription fast path returns v
// unchanged (no copy cost on the hot path).
func StripAscribed(v Value) Value {
	if v.asc != nil {
		v.asc = nil
	}
	return v
}

// Name is the nil-safe read of the type-node leaf name — "" for an
// ordinary value (no tmeta), matching the old empty inline field.
func (v Value) Name() string {
	if v.tmeta == nil {
		return ""
	}
	return v.tmeta.Name
}

// OwnerID is the nil-safe read of the type node's registration/mint
// provenance (design/OPEN-WORDS.1.md §4) — "" for an ordinary value or
// an unowned ad-hoc type. See typeMeta.Owner for the sentinel table.
func (v Value) OwnerID() string {
	if v.tmeta == nil {
		return ""
	}
	return v.tmeta.Owner
}

// SetOwner stamps the type node's provenance. Registration/mint paths
// call it exactly once at the construction boundary; the stamp is
// shared by every copy of the type Value via the tmeta pointer.
func (v *Value) SetOwner(owner string) {
	v.ensureTMeta().Owner = owner
}

// FixedID / Rank / Depth / In / Out are nil-safe reads of the lattice
// integer fields — 0 when tmeta is absent (every ordinary runtime value,
// where these were always zero when they were inline fields).
func (v Value) FixedID() int {
	if v.tmeta == nil {
		return 0
	}
	return v.tmeta.FixedID
}

func (v Value) Rank() int {
	if v.tmeta == nil {
		return 0
	}
	return v.tmeta.Rank
}

func (v Value) Depth() int {
	if v.tmeta == nil {
		return 0
	}
	return v.tmeta.Depth
}

func (v Value) In() int {
	if v.tmeta == nil {
		return 0
	}
	return v.tmeta.In
}

func (v Value) Out() int {
	if v.tmeta == nil {
		return 0
	}
	return v.tmeta.Out
}

// idState is the package-level ID source: a monotone atomic counter
// whose successive values are run through a splitmix64 finalizer to
// spread them across the 48-bit ID space. This is LOCK-FREE — GenerateID
// is called from concurrently-running engine forks (await branches,
// timer callbacks) on the interpreter's hot path, and the previous
// mutex-guarded *rand.Rand serialized every value mint. An atomic
// increment is contention-free; the splitmix64 mix keeps IDs
// well-distributed (not visibly sequential) and collision-free within a
// process. SetIDSeed(s) sets the counter to s, so a given seed
// reproduces the same ID stream (the determinism Options.Seed relies on).
var idState atomic.Uint64

func init() { idState.Store(uint64(time.Now().UnixNano())) }

// idMin is the minimum value for generated IDs (0x100000000000): its top
// hex nibble is non-zero so every ID is a full 12 hex characters.
const idMin uint64 = 0x100000000000

// idMax is the exclusive span above idMin so values fit in 12 hex chars.
const idMax uint64 = 0x1000000000000 - 0x100000000000

// SetIDSeed reseeds the ID counter so the mint stream is reproducible.
func SetIDSeed(seed int64) {
	idState.Store(uint64(seed))
}

// splitmix64 finalizer — a well-distributed bijective mix, so a monotone
// counter yields non-sequential, collision-free outputs.
func mixID(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

const idHexDigits = "0123456789abcdef"

// GenerateID creates a unique ID with the given prefix followed by 12
// lowercase hex characters (value >= 0x100000000000). Lock-free, and
// assembles prefix+hex in a single heap allocation (strings.Builder's
// Grow'd buffer) instead of the previous hex-encode-then-concat pair.
func GenerateID(prefix string) string {
	n := idMin + mixID(idState.Add(1))%idMax
	var hb [12]byte
	for i := 11; i >= 0; i-- {
		hb[i] = idHexDigits[n&0xf]
		n >>= 4
	}
	var sb strings.Builder
	sb.Grow(len(prefix) + 12)
	sb.WriteString(prefix)
	sb.Write(hb[:])
	return sb.String()
}

// IDPrefixForType returns the ID prefix for a given type:
// "S_" for Scalar, "N_" for Node, "W_" for Word, "T_" for Object/Any/None.
//
// Under the Any-root lattice the universal Root() of every connected
// type is Any; the historical category prefix lives one step below —
// the topmost ancestor that's NOT Any. Walk the parent chain until
// the next step would land on Any (or nil) and read the name there.
func IDPrefixForType(t *Type) string {
	if t == nil {
		return "T_"
	}
	for d := t; d != nil; d = d.Parent {
		if d.Parent == nil || d.Parent.FixedID() == anyFixedID {
			switch d.Name() {
			case "Scalar":
				return "S_"
			case "Node":
				return "N_"
			case "Word":
				return "W_"
			}
			return "T_"
		}
	}
	return "T_" //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
}

// checkPassDepth counts the process's live check/compile passes
// (CheckState.Begin / BeginCompilePass). Value IDs exist for exactly one
// consumer class — the emit recorder's provenance maps, which key
// producedBy / locals / captures on the ID minted at value creation and
// shared across copies — and that machinery only runs during a pass. A
// full audit (design/INTERPRETER-PYTHON-PARITY.10.md Phase B) found NO
// run-mode reader of a concrete value's ID, so minting is gated: pure
// runtime execution skips GenerateID entirely (~21% of all interpreter
// allocations), while any live pass anywhere in the process keeps every
// mint (a concurrent runtime engine then pays a mint it doesn't need —
// a pure perf trade, never a correctness one). Bare lattice values
// (data == nil: type literals, carriers) always mint — canon, unify and
// the type registry read their IDs at runtime.
var checkPassDepth atomic.Int64

// CheckPassActive reports whether any check/compile pass is live in the
// process — the condition under which runtime value mints carry IDs.
func CheckPassActive() bool { return checkPassDepth.Load() > 0 }

// BeginIDMintScope arms eager ID minting for a non-pass producer whose
// values must still carry compile identities — the PARSER. Program tokens
// are the compile pass's raw material (a handler that returns its inputs
// hands the emitter the parsed literals themselves; captures snapshot
// parse-minted binding values), yet parsing runs before any pass begins.
// Parsing is once-per-program, so eager IDs there cost nothing on the
// execution hot path. The returned closure disarms exactly once.
func BeginIDMintScope() func() {
	checkPassDepth.Add(1)
	ended := false
	return func() {
		if !ended {
			ended = true
			checkPassDepth.Add(-1)
		}
	}
}

// NewValueRaw creates a Value with an auto-generated ID based on the
// type category. data must be a Payload — the sealed interface
// implemented by all kernel-known payload variants and by every
// eng-defined struct/pointer type used as a payload. After Step 5g,
// passing a raw int64 / string / time.Time / etc. is a compile error
// — wrap it in IntPayload / StrPayload / TimePayload / etc. first.
//
// The ID is elided for concrete values minted outside any check/compile
// pass — see checkPassDepth above. Emit-side consumers treat an empty ID
// as "no identity" (skip, refuse, or dynamic-scope rescue — never a map
// key), so a runtime-minted value flowing into a LATER pass degrades to
// a conservative compile fallback rather than a miscompile.
func NewValueRaw(t *Type, data Payload) Value {
	v := Value{
		Parent: t,
		Data:   data,
	}
	if data == nil || checkPassDepth.Load() > 0 {
		v.ID = GenerateID(IDPrefixForType(t))
	}
	return v
}

// NewString creates a string value tagged with the appropriate
// String subtype: EmptyString for "" (the unique inhabitant of the
// EmptyString singleton type) and ProperString for any non-empty
// payload. Both subtypes match Scalar/String via the type lattice
// (TStringProper.ConformsTo(TString) and TStringEmpty.ConformsTo(TString)
// are true), so signatures declared on TString continue to dispatch
// transparently — the difference is observable only via typeof,
// pattern dispatch, or explicit subtype-equality checks.
//
// Specific-value dispatch is still primarily routed through
// Signature.Patterns; the empty/proper split provides a coarser
// "value-shape at the type level" signal so user code can branch on
// emptiness without resorting to a length comparison.
func NewString(s string) Value {
	if s == "" {
		return NewValueRaw(TStringEmpty, StrPayload{S: s})
	}
	return NewValueRaw(TStringProper, StrPayload{S: s})
}

// NewInteger creates a number/integer value with Parent = Scalar/Number/Integer.
// Specific-value dispatch (e.g. `def fact[0] (1)`) routes through
// Signature.Patterns, not through a per-value type-path leaf. See
// the NewString comment for the rationale.
func NewInteger(n int64) Value {
	return NewValueRaw(TInteger, IntPayload{N: n})
}

// NewFloat creates a number/float value with a float64 payload.
func NewFloat(f float64) Value {
	return NewValueRaw(TFloat, FloatPayload{F: f})
}

// NewBigInteger creates a Scalar/Number/BigInteger value wrapping an
// arbitrary-precision integer. The caller must not mutate n afterwards.
func NewBigInteger(n *big.Int) Value {
	return NewValueRaw(TBigInteger, BigIntPayload{N: n})
}

// NewBigDecimal creates a Scalar/Number/BigDecimal value wrapping an
// arbitrary-precision base-10 decimal. The caller must not mutate d.
func NewBigDecimal(d *apd.Decimal) Value {
	return NewValueRaw(TBigDecimal, DecimalPayload{D: d})
}

// FormatFloat renders a float64 with a guaranteed decimal point so the
// type stays visually distinct from Integer. Uses 'f' format with -1
// precision (shortest round-trip), then appends ".0" when the result
// has neither a fractional part nor an exponent. Float artefacts like
// 0.1 + 0.2 = 0.30000000000000004 are preserved verbatim — see the
// note in spec/SPEC_REPORT.md §2 on the apd-port plan if exact
// decimal arithmetic is required.
func FormatFloat(f float64) string {
	// Special values render as the parseable literals inf / -inf / nan
	// (matching the parser's word-context literals) so print∘parse is
	// identity. The historical +Inf.0 / NaN.0 forms could not be re-read.
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	// Use scientific notation only for genuinely extreme magnitudes, where
	// plain decimal would be gratuitously long (hundreds of digits). The
	// bounds deliberately leave the everyday range — including small
	// decimals like 0.00001 (1e-5) and large values up to ~1e20 — rendered
	// in full, exactly as before. 1e21 (Go's own 'g' upper threshold) and
	// magnitudes below 1e-10 switch to 'e'. Both forms re-parse to the same
	// Float, so the choice is purely about readability.
	if f != 0 {
		if a := math.Abs(f); a >= 1e21 || a < 1e-10 {
			return strconv.FormatFloat(f, 'e', -1, 64)
		}
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// formatFloat is the lowercase alias retained for in-package call
// sites that pre-date the exported form.
func formatFloat(f float64) string { return FormatFloat(f) }

// FormatBigInteger renders a BigInteger as the parseable literal `0d…`,
// with the sign before the marker (`-0d5`), so print∘parse is identity.
func FormatBigInteger(n *big.Int) string {
	if n == nil {
		return "0d0"
	}
	if n.Sign() < 0 {
		return "-0d" + new(big.Int).Abs(n).String()
	}
	return "0d" + n.String()
}

// FormatBigDecimal renders a BigDecimal as the parseable literal `0d…`
// using apd's plain 'f' form (preserving scale, e.g. 0d0.30), with the
// sign before the marker.
func FormatBigDecimal(d *apd.Decimal) string {
	if d == nil {
		return "0d0"
	}
	s := d.Text('f')
	if strings.HasPrefix(s, "-") {
		return "-0d" + s[1:]
	}
	return "0d" + s
}

// NewBoolean creates a boolean value. The boolean payload (true/false) is the
// value; there are no Boolean/True or Boolean/False sub-types.
func NewBoolean(b bool) Value {
	return NewValueRaw(TBoolean, BoolPayload{B: b})
}

// NewList creates a list value from a slice of Values.
func NewList(elems []Value) Value {
	return NewValueRaw(TList, ListPayload{Elems: elems})
}

// NewEvalList creates a list value that is marked for auto-evaluation
// at the end of execution. Used by the parser for source-code lists.
func NewEvalList(elems []Value) Value {
	v := NewValueRaw(TList, ListPayload{Elems: elems})
	v.Eval = true
	return v
}

// NewTypedList creates a typed list value with a child type constraint.
// For example, NewTypedList(NewTypeLiteral(TString)) represents [:string].
func NewTypedList(child Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child})
}

// NewMap creates a map value from an ordered map of string keys to Values.
func NewMap(entries *OrderedMap) Value {
	return NewValueRaw(TMap, MapPayload{M: entries})
}

// NewFlexList creates a mutable FlexList value over a pointer-backed
// element store. Flex nodes are runtime data, never parser output, so
// Eval is never set on them.
func NewFlexList(elems []Value) Value {
	return NewValueRaw(TFlexList, &FlexListData{Elems: elems})
}

// NewXmlElement creates an immutable Node/Xml element value. attr may
// be nil (treated as no attributes); cren may be nil (no children).
// See design/XML-LITERAL.0.md and core_xml.go.
func NewXmlElement(tag string, attr *OrderedMap, cren []Value) Value {
	if attr == nil {
		attr = NewOrderedMap()
	}
	return NewValueRaw(TXml, XmlElementPayload{Tag: tag, Attr: attr, Cren: cren})
}

// NewXmlInterp creates an interpolated XML literal skeleton (Word/__XI).
// The engine evaluates it in place to a concrete Node/Xml at runtime,
// like an InterpString. See payload.go::XmlInterpPayload and
// engine.go::EvalXmlInterp.
func NewXmlInterp(tmpl XmlTmpl) Value {
	return NewValueRaw(TXmlInterp, XmlInterpPayload{Tmpl: tmpl})
}

// IsXmlInterp reports whether v is an interpolated XML literal skeleton.
func IsXmlInterp(v Value) bool {
	return v.Parent.Equal(TXmlInterp)
}

// AsXmlInterp returns the template backing an interpolated XML skeleton.
func AsXmlInterp(v Value) (XmlTmpl, error) {
	if xi, ok := v.Data.(XmlInterpPayload); ok {
		return xi.Tmpl, nil
	}
	return XmlTmpl{}, fmt.Errorf("AsXmlInterp: not an xml-interp value (got %T)", v.Data)
}

// NewFlexMap creates a mutable FlexMap value. It reuses MapPayload —
// the *OrderedMap is pointer-backed, so in-place mutation is visible
// through every Value copy sharing the payload. Like NewFlexList,
// Eval is never set.
func NewFlexMap(entries *OrderedMap) Value {
	return NewValueRaw(TFlexMap, MapPayload{M: entries})
}

// NewEvalMap creates a map value marked for auto-evaluation at end of
// execution. Used by the parser for source-code maps.
func NewEvalMap(entries *OrderedMap) Value {
	v := NewValueRaw(TMap, MapPayload{M: entries})
	v.Eval = true
	return v
}

// NewTypedListWithElements creates a typed list value carrying both
// concrete elements and a child-type constraint. Used by the parser
// for `[v0 :T v1]` syntax. Each element is validated against the
// child constraint by `is` and similar predicates.
func NewTypedListWithElements(child Value, elems []Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child, Elements: elems})
}

// NewTypedMapWithEntries creates a typed map value carrying both
// concrete (key, value) entries and a child-type constraint. Used by
// the parser for `{k:v :T}` syntax.
func NewTypedMapWithEntries(child Value, entries []ChildEntry) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child, Entries: entries})
}

// NewImplicitMap creates a map value marked as implicit (from pair syntax).
// In fn signatures, implicit maps are treated as named parameter declarations
// (e.g., [x:Integer]), while explicit maps are structural patterns.
func NewImplicitMap(entries *OrderedMap) Value {
	entries.Implicit = true
	return NewValueRaw(TMap, MapPayload{M: entries})
}

// IsImplicitMap reports whether v is a Map value whose backing
// OrderedMap was constructed from implicit-pair syntax (e.g.
// `{x:Integer}` or `[x:Integer]` inside an fn sig). Used to
// discriminate record-shape patterns from concrete maps.
func IsImplicitMap(v Value) bool {
	if !v.Parent.Equal(TMap) || v.Data == nil {
		return false
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M != nil && mp.M.Implicit
	}
	return false
}

// NewTypedMap creates a typed map value with a child type constraint.
// For example, NewTypedMap(NewTypeLiteral(TString)) represents {:string}.
func NewTypedMap(child Value) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child})
}

// NewRecordType creates a record type value from a field schema.
// The fields map contains field names as keys and type-constraint Values as values.
// For example, record{x:number, y:number} constrains maps to have exactly
// keys x and y with number-typed values.
func NewRecordType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, RecordTypeInfo{Fields: fields})
}

// NewOptionsType creates an options type value from a field schema map.
func NewOptionsType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, OptionsTypeInfo{Fields: fields})
}

// NewTableType creates a table type value from a record type.
// A table type constrains a list so that each element is a map conforming
// to the given record schema.
func NewTableType(record RecordTypeInfo) Value {
	return NewValueRaw(TList, TableTypeInfo{Record: record})
}

// NewAtom creates an atom value from a bare unquoted word.
func NewAtom(name string) Value {
	return NewValueRaw(TAtom, AtomPayload{Name: name})
}

// AtomReferent returns the value an atom's name was snapshotted to refer to,
// if one was captured (by `quote` or the run-start resolution pass). ok=false
// when v is not an atom or carries no referent.
func AtomReferent(v Value) (Value, bool) {
	ap, ok := v.Data.(AtomPayload)
	if !ok || ap.Referent == nil {
		return Value{}, false
	}
	return *ap.Referent, true
}

// SetAtomReferent returns a copy of atom v carrying ref as its referent (a
// snapshot of what its name refers to). Name, Quoted, and Pos are preserved.
// Returns v unchanged when it is not an atom.
func SetAtomReferent(v Value, ref Value) Value {
	ap, ok := v.Data.(AtomPayload)
	if !ok {
		return v
	}
	snap := ref
	ap.Referent = &snap
	v.Data = ap
	return v
}

// NewPathon creates a Pathon value from parts and an absolute flag.
func NewPathon(parts []string, abs bool) Value {
	return NewPathonVol("", parts, abs)
}

// NewPathonVol builds a Pathon carrying a Windows drive volume ("C:"), or ""
// for a POSIX / driveless path. Use this (not NewPathon) whenever a Pathon is
// reconstructed from an existing PathonInfo, so a drive path's volume is not
// silently dropped (e.g. an IO word returning its input path).
func NewPathonVol(volume string, parts []string, abs bool) Value {
	p := make([]string, len(parts))
	copy(p, parts)
	return NewValueRaw(TPathon, PathonPayload{Info: PathonInfo{Volume: volume, Parts: p, Abs: abs}})
}

// NewTypeLiteral returns the type t expressed as a Value. After the
// type/value merge a type literal IS its lattice node, so this
// returns a by-value copy of the node itself: the literal's Parent
// is the supertype (so typeof is uniformly Parent), its Name is the
// type's own name, its Data is nil.
func NewTypeLiteral(t *Type) Value {
	if t == nil {
		return Value{}
	}
	return *t
}

// typeNodeOf returns the lattice node a Data==nil value stands for.
// A type literal IS its node. A carrier's node is its Parent — a
// carrier keeps Parent pointing at the type it carries. Used by the
// renderers to recover the type name from either shape.
func TypeNodeOf(v Value) *Type {
	if v.Carrier {
		return v.Parent
	}
	return &v
}

// TypeNameOf returns the leaf name of the type v represents: a type
// literal's own name, or any other value's Parent leaf. Used by
// inspect and rendering paths that display a type's name.
func TypeNameOf(v Value) string {
	if IsBareTypeNode(v) {
		return v.Leaf()
	}
	return v.Parent.Leaf()
}

// TypePathOf returns the slash path of the type v represents: a type
// literal's own path, or any other value's Parent path.
func TypePathOf(v Value) string {
	if IsBareTypeNode(v) {
		return v.Path()
	}
	return v.Parent.Path()
}

// ValueType returns the lattice node that is v's type: v itself when
// v is a bare type literal (a type literal IS a lattice node), and
// v's Parent for any concrete value or carrier.
func ValueType(v Value) *Type {
	if IsBareTypeNode(v) {
		return &v
	}
	return v.Parent
}

// noneSentinel is the non-nil Data payload that distinguishes the
// VALUE `none` (the unique inhabitant of None) from the TYPE LITERAL
// `None` (which has Data == nil like every other type literal).
// Renderers and the matcher use the Data!=nil discriminator to print
// the value as "none" and the type literal as "None".
type noneSentinel struct{}

// NewNone creates the value `none` — the unique inhabitant of the
// None type. Distinct from NewTypeLiteral(TNone) (the type itself).
func NewNone() Value {
	return NewValueRaw(TNone, NonePayload{})
}

// Is reports whether v satisfies type t, routed through t.Behavior.
// The canonical dispatch point for "is v a T?" — used by handlers,
// the matcher, and `is` / `guard`. Default Behavior delegates to the
// lattice walk (v.Parent.ConformsTo(t)); types with custom Behavior
// override (predicate types invoke their body, record types check
// field-by-field conformance, etc.).
//
// Safe on nil t: returns false. Safe on nil Behavior: callers should
// not encounter this state because every Type registered through
// the kernel paths carries a non-nil Behavior, but the method
// defends against it anyway.
func (v Value) Is(t *Type) bool {
	if t == nil {
		return false
	}
	if t.Behavior() == nil {
		return v.Parent.ConformsTo(t)
	}
	return t.Behavior().Match(v, t)
}

// IsNone reports whether v is the value `none` (not the None type
// literal). The check distinguishes the inhabitant from the type:
// `none` carries a NonePayload/noneSentinel marker; the bare type
// literal `None` is just a by-value copy of TNone with Data=nil.
// Use IsNoneShape for the broader "is this any form of None" check.
func IsNone(v Value) bool {
	if !v.Parent.Equal(TNone) {
		return false
	}
	if _, ok := v.Data.(NonePayload); ok {
		return true
	}
	_, ok := v.Data.(noneSentinel)
	return ok
}

// IsNoneShape reports whether v is any form of None — the canonical
// "did we get back None?" check that covers:
//
//   - the sentinel value `none` (IsNone — Data is NonePayload or
//     noneSentinel, Parent=TNone).
//   - the bare type literal `None` (NewTypeLiteral(TNone), Data=nil,
//     value IS the TNone lattice node).
//   - manually-constructed Value{Parent: TNone} carrier-shaped
//     values (Data=nil, Parent=TNone — used by some host code and
//     test fixtures as a "no value" sentinel).
//
// Renderers that want to distinguish "none" (lowercase, the value)
// from "None" (capital, the type) use IsNone for the strict check;
// dispatch and comparison sites use IsNoneShape.
func IsNoneShape(v Value) bool {
	if IsNone(v) {
		return true
	}
	if v.Data != nil {
		return false
	}
	if v.Parent.Equal(TNone) {
		return true
	}
	return !v.Carrier && (&v).Equal(TNone)
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return NewValueRaw(TWord, WordInfo{Name: name, ArgCount: -1})
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forceStack, forceForward bool) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:         name,
		ArgCount:     argCount,
		ForceStack:   forceStack,
		ForceForward: forceForward,
	})
}

// NewWordRef creates a word value marked with the /v modifier: when
// reached at the pointer it resolves the name to its bound Function
// value without entering function dispatch. /v is legal ONLY for
// function words — a name bound to a non-fn value (plain value, type
// body) raises [boru/illegal_ref] (see eng.IsFunctionRef). ArgCount
// stays unspecified because /v short-circuits argument collection.
func NewWordRef(name string) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:     name,
		ArgCount: -1,
		ForceVal: true,
	})
}

// NewWordUsurp creates a word value marked with the /u modifier: when
// reached at the pointer it resolves the name to its bound Function value
// and wraps it so its signature argument order is reversed (usurped a b c
// ≡ f c b a). Like /v, /u is legal ONLY for function words. The usurped
// wrapper is left UNQUOTED, so it dispatches immediately when args are
// available; combine with /v (name/uv) to leave it as inert data instead.
func NewWordUsurp(name string, forceVal bool) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:       name,
		ArgCount:   -1,
		ForceUsurp: true,
		ForceVal:   forceVal,
	})
}

// NewForward creates a forward primitive value for forward argument tracking.
func NewForward(info ForwardInfo) Value {
	return NewValueRaw(TForward, info)
}

// newMarkerValue mints a payload-less STRUCTURAL marker (paren / end).
// Markers are recognised by Parent identity and carry no payload, so
// NewValueRaw's data==nil "always mint" rule — which exists for type
// literals, whose IDs canon and the type registry read at runtime —
// does not apply; a marker's ID follows the pass rule instead. The
// interpreter re-expands paren groups per evaluation (expandParenExpr),
// so eager marker IDs were a per-op allocation for nothing; during a
// parse or check/compile pass (checkPassDepth > 0) markers mint exactly
// as before.
func newMarkerValue(t *Type) Value {
	v := Value{Parent: t}
	if checkPassDepth.Load() > 0 {
		v.ID = GenerateID(IDPrefixForType(t))
	}
	return v
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return newMarkerValue(TOpenParen)
}

// NewCloseParen creates a close-paren marker value. Emitted by the
// parser for `)` so the engine can recognise it by Parent identity
// instead of by string compare.
func NewCloseParen() Value {
	return newMarkerValue(TCloseParen)
}

// NewEnd creates an end-marker value (the `end` / `;` keyword).
// Emitted by the parser so the engine can recognise it by Parent
// identity instead of by string compare.
func NewEnd() Value {
	return newMarkerValue(TEnd)
}

// NewParenExpr creates a paren expression value containing items to evaluate.
// Used by the parser for paren groups in map/list value positions.
// autoEvalMap evaluates these by running the items in a sub-engine with
// paren markers, producing a single result value.
func NewParenExpr(items []Value) Value {
	return NewValueRaw(TParenExpr, ParenExprPayload{Toks: items})
}

// NewReach creates an Ideal/Reach value — a first-class dot-access node
// (m.a.b). receiver is the base expression's tokens (nil/empty for a
// receiverless reach); segments are the .key / !.key steps; eval marks it
// evaluate-by-default. See design/REACH.10.md.
func NewReach(info ReachInfo) Value {
	if info.unit == nil {
		info.unit = &lensUnit{} // one shared cache per constructed lens
	}
	return NewValueRaw(TReach, info)
}

// NewReachFromKeys builds an inert (non-evaluating, Eval=false) Reach over a
// concrete receiver value with literal `get` segments — the programmatic
// `reach` constructor. The result is data (a lens): it does not auto-evaluate
// like a parsed m.a.b. See design/REACH.10.md §7.
func NewReachFromKeys(receiver Value, keys []Value) Value {
	segs := make([]ReachSeg, len(keys))
	for i, k := range keys {
		segs[i] = ReachSeg{KeyLit: k}
	}
	return NewReach(ReachInfo{Receiver: []Value{receiver}, Segments: segs, Eval: false})
}

// IsReach reports whether v is an Ideal/Reach value.
func IsReach(v Value) bool {
	return v.Parent.Equal(TReach)
}

// AsReach returns the ReachInfo of a Reach value.
func AsReach(v Value) (ReachInfo, error) {
	if ri, ok := v.Data.(ReachInfo); ok {
		return ri, nil
	}
	return ReachInfo{}, fmt.Errorf("AsReach: not a reach value (got %T)", v.Data)
}

// InterpPart represents one segment of an interpolated string.
// If Expr is nil, Lit is a literal string segment.
// If Expr is non-nil, it contains parsed boru values to evaluate.
type InterpPart struct {
	Lit  string
	Expr []Value
}

// renderInterpParts renders an interpolated string's parts in source
// form: literal segments single-quoted, expression parts as `${…}` with
// their tokens' String renders space-joined. Shared by Value.String's
// InterpString arm; the TS port's renderer mirrors it byte for byte.
func renderInterpParts(parts []InterpPart) string {
	var b strings.Builder
	b.WriteString("interp(")
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		if len(p.Expr) > 0 {
			b.WriteString("${")
			for k, t := range p.Expr {
				if k > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(t.String())
			}
			b.WriteString("}")
		} else {
			b.WriteString("'" + p.Lit + "'")
		}
	}
	b.WriteString(")")
	return b.String()
}

// renderXmlTmplSrc renders an XML template skeleton in source form —
// `<tag attr="lit${expr}">text${hole}<child/></tag>` — for the
// XmlInterp String arm. Mirrored by the TS port's renderer.
func renderXmlTmplSrc(t XmlTmpl) string {
	var b strings.Builder
	b.WriteString("<" + t.Tag)
	for _, a := range t.Attr {
		b.WriteString(" " + a.Name + "=\"")
		for _, p := range a.Parts {
			if len(p.Expr) > 0 {
				b.WriteString("${")
				for k, tok := range p.Expr {
					if k > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(tok.String())
				}
				b.WriteString("}")
			} else {
				b.WriteString(p.Lit)
			}
		}
		b.WriteString("\"")
	}
	if len(t.Cren) == 0 {
		b.WriteString("/>")
		return b.String()
	}
	b.WriteString(">")
	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenLit:
			b.WriteString(c.Lit)
		case XmlCrenExpr:
			b.WriteString("${")
			for k, tok := range c.Expr {
				if k > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(tok.String())
			}
			b.WriteString("}")
		case XmlCrenChild:
			if c.Child != nil {
				b.WriteString(renderXmlTmplSrc(*c.Child))
			}
		}
	}
	b.WriteString("</" + t.Tag + ">")
	return b.String()
}

// NewInterpString creates an interpolated string value from alternating
// literal and expression parts. The engine evaluates expression parts in
// a sub-engine, converts results to strings, and concatenates everything.
func NewInterpString(parts []InterpPart) Value {
	return NewValueRaw(TInterpString, InterpStringPayload{Parts: parts})
}

// NewMark creates a mark value with the given unique ID and the body to
// replay when the corresponding move fires. The body should contain
// the original values between the mark and its paired move.
func NewMark(id string, body ...Value) Value {
	b := make([]Value, len(body))
	copy(b, body)
	return NewValueRaw(TMark, MarkInfo{ID: id, Body: b})
}

// NewMove creates a move value targeting the mark with the given ID.
// The reason string describes why this move exists (used in error messages).
func NewMove(to string, reason string) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason})
}

// NewMoveCont creates a move value with for-loop continuation state.
func NewMoveCont(to, reason string, cont *ForCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, Cont: cont})
}

// NewMoveIf creates a move value with if-statement continuation state.
func NewMoveIf(to, reason string, ifCont *IfCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, IfCont: ifCont})
}

// NewFunction creates a function reference value. The underlying data is a
// FnDefInfo, but the type is TFunction so it can be matched by function-typed
// parameters and passed to other functions without being called.
//
// It also mints the payload's identity token when it has none (NUR031). This
// is the one seam every fn VALUE passes through, so minting here — rather
// than at each of the two dozen construction sites — is what makes "a fn
// value always has an identity" true by construction. A payload that already
// carries a token keeps it, which is what lets identity survive rebinding:
// the reach path (ResolveRef → Lookup → NewFunction) re-wraps a copy on every
// mention of the name and must not re-mint.
func NewFunction(info FnDefInfo) Value {
	if info.ident == nil {
		info.ident = &fnIdent{}
	}
	return NewValueRaw(TFunction, info)
}

// NewFnUndef creates a function undef spec value for targeted signature removal.
func NewFnUndef(info FnUndefInfo) Value {
	return NewValueRaw(TFnUndef, info)
}

// NewReturnCheck creates a return-check marker for fn return type validation.
func NewReturnCheck(info ReturnCheckInfo) Value {
	return NewValueRaw(TReturnCheck, info)
}

// DefCleanupInfo holds a snapshot of DefStacks lengths taken before fn body
// execution. When the engine encounters a DefCleanup marker, it pops any
// defs that were added during body execution back to the snapshot state.
type DefCleanupInfo struct {
	Snapshot map[string]int
	Registry *Registry
	// SkipCleanup marks a frame whose body provably installs no
	// body-local defs (buildFnBodyHandler's bodyNeedsFrameState analysis):
	// the marker stays on the tape so the frame shape and the
	// break/continue unwind and TCO paths are unchanged, but stepDefCleanup
	// short-circuits and no Snapshot map is allocated. With no body-local
	// defs the truncation would remove nothing, so TCO treats the frame as
	// eagerly-teardown-eligible without consulting the (nil) Snapshot.
	SkipCleanup bool
	// EvalResidual marks a MULTI-TOKEN fn body: the frame's residual
	// pending containers evaluate IN-frame at this marker — before the
	// body-local defs pop — so the spliced dispatch path agrees with the
	// CallBoru sub-run drain (which evaluates the residual at sub-run end,
	// while the frame is live). A SINGLE-LITERAL container body leaves
	// this false and keeps the no-closures transparency: the returned
	// container resolves names in the CONSUMER's scope
	// (lang/spec/def-node-binding.tsv §3).
	EvalResidual bool
}

// NewDefCleanup creates a def-cleanup marker for fn body local def cleanup.
func NewDefCleanup(info DefCleanupInfo) Value {
	return NewValueRaw(TDefCleanup, info)
}

// NewDisjunct creates a disjunction type value from a list of
// alternatives. The low-level constructor preserves the caller's order —
// some internal callers build order-bearing disjuncts (a DepScalar
// `[low, high]` bound, a dynamic-dispatch reachable-returns partition).
// Canonical tcmp ordering for `tor` type UNIONS is applied one layer up,
// in SimplifyDisjunctAlts (core_helpers.go), which every user-facing
// union construction routes through — so `None tor 1` and `1 tor None`
// build equal values that print identically and compare equal under
// `tcmp`. (NewEnum also preserves order — an enum's member order is
// significant.)
func NewDisjunct(alternatives []Value) Value {
	return NewValueRaw(TDisjunct, DisjunctInfo{Alternatives: alternatives})
}

// disjunctAltLess is the canonical ordering predicate for disjunct
// alternatives: primarily CompareValues (the `tcmp` total order); a CanonValue
// lexical tiebreak resolves both incomparable pairs (CompareValues errs) AND
// value-equal-but-distinct pairs (CompareValues returns 0). The tie case is
// real: cross-leaf numeric magnitude equality (`1 cmp 1.0 == 0`) leaves two
// type-distinct alts the primary order cannot separate, so without the tiebreak
// `1 tor 1.0` and `1.0 tor 1` would keep the caller's order and render/store
// differently despite `tor` being commutative. CanonValue distinguishes them
// (`1` vs `1.0`), keeping the sort a deterministic strict weak ordering.
func disjunctAltLess(a, b Value) bool {
	if c, err := CompareValues(a, b); err == nil && c != 0 {
		return c < 0
	}
	return CanonValue(a) < CanonValue(b)
}

// NewEnum creates an Enum value (Type/Disjunct/Enum) — a fixed
// enumeration of named values. Structurally identical to a Disjunct
// (same DisjunctInfo payload) but tagged with the more specific Enum
// type so `typeof` reports `Enum` and the value can be distinguished
// from a general type-disjunct.
func NewEnum(alternatives []Value) Value {
	return NewValueRaw(TEnum, DisjunctInfo{Alternatives: alternatives})
}

// NewNegation creates a negation (complement) type value whose
// inhabitants are exactly the values that do NOT satisfy inner.
func NewNegation(inner Value) Value {
	return NewValueRaw(TNegation, NegationInfo{Inner: inner})
}

// NewClassType creates an object type value. The caller must
// provide the canonical *Type identity — typically minted via
// r.Types.MintType for named types being installed, or for anonymous
// `object {…}` declarations. The info's Type field is set to t for
// downstream code that needs the parent's *Type when extending.
func NewClassType(t *Type, info ClassTypeInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// NewClassInstance creates an object instance value of the given
// type. The caller must provide the type's *Type identity (typically
// info.TypeRef.Type for the type currently being instantiated).
func NewClassInstance(t *Type, info ClassInstanceInfo) Value {
	return NewValueRaw(t, info)
}

// NewResourceType creates a Resource/Entity type value; sets info.Type to t.
func NewResourceType(t *Type, info ResourceTypeInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// NewResourceInstance creates a resource instance value of the given type.
func NewResourceInstance(t *Type, info ResourceInstanceInfo) Value {
	return NewValueRaw(t, info)
}

// NewStore creates a Store value of the given type. Pass TStore for
// the builtin Object/Store, TStoreSystem for Object/Store/System, or
// a user-defined *Type for a custom store subtype. The Value's
// TypeName is derived from t.Path() for legacy prototype-chain code
// that compares store types by path string.
func NewStore(t *Type) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName: t.Path(),
		Data:     make(map[string]Value),
	})
}

// NewStoreValue wraps an existing StoreInstanceInfo into a Value.
// Pass t = nil to default to TStore.
func NewStoreValue(t *Type, si *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, si)
}

// NewStoreWithPrototype creates a Store value of the given type with
// a prototype chain. Pass t = nil to default to TStore.
func NewStoreWithPrototype(t *Type, prototype *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName:  t.Path(),
		Data:      make(map[string]Value),
		Prototype: prototype,
	})
}

// As* accessors for Scalar/Time/* moved to
// lang/go/engine/native_temporal.go (Step 6/7). The kernel no longer
// carries methods named for types it doesn't own. CalDurationData
// stays here because the payload struct is kernel-owned (it has the
// payloadMarker) — only the user-facing constructor / accessor
// surface moved.

// CalDurationData holds a calendar duration (years, months, days).
type CalDurationData struct {
	Years  int
	Months int
	Days   int
}

// ErrorInfo holds the details of a boru error value.
type ErrorInfo struct {
	Message string // the short error description (a BoruError's Detail)
	// Code is the stable, dispatchable error code ("user_error",
	// "type_error", …) when the source was a BoruError (native or
	// `raise`d); empty for plain Go errors. Handlers branch on it via
	// `e.code` / `convert Map`.
	Code string
	// Data carries the extra keys of a `raise {code:… message:… …}`
	// spec map for programmatic handlers; nil otherwise. The formatter
	// prints code + message only.
	Data *OrderedMap
}

// NewError creates an error value from a Go error. A BoruError (the
// engine's structured error, including everything `raise` produces)
// contributes its stable Code, its SHORT Detail as the message (not
// the formatted multi-line report), and any raise payload; a plain Go
// error contributes only its text.
func NewError(err error) Value {
	info := ErrorInfo{Message: err.Error()}
	var ae *BoruError
	if errors.As(err, &ae) {
		info.Code = ae.Code
		info.Message = ae.Detail
		info.Data = ae.Data
	}
	return NewValueRaw(TError, info)
}

// IsError reports whether this value is an error.
func IsError(v Value) bool {
	return v.Parent.Equal(TError)
}

// AsError returns the ErrorInfo for an error value.
func AsError(v Value) (ErrorInfo, error) {
	info, ok := v.Data.(ErrorInfo)
	if !ok {
		return ErrorInfo{}, fmt.Errorf("AsError: not an error value (got %T)", v.Data)
	}
	return info, nil
}

// TimeoutInfo holds a pending timeout handle.
type TimeoutInfo struct {
	ID    string      // unique identifier
	Ms    int64       // delay in milliseconds
	Timer *time.Timer // underlying Go timer (nil after cancel)
}

// NewTimeout, IsTimeout, AsTimeout moved to lang/go/engine/native_misc.go
// (Step 8). Callers that need them use engine.NewTimeout /
// engine.AsTimeout, etc. The IsTimeout method is replaced by
// `v.Parent.Equal(engine.TTimeout)` at call sites.

// IntervalInfo holds a repeating interval handle.
type IntervalInfo struct {
	ID     string        // unique identifier
	Ms     int64         // interval in milliseconds
	Ticker *time.Ticker  // underlying Go ticker (nil after cancel)
	Done   chan struct{} // closed to signal cancellation
}

// NewInterval moved to lang/go/engine/native_misc.go (Step 8).

// IsInterval and AsInterval moved to lang/go/engine/native_misc.go (Step 8).

// IsWord reports whether this value is a word (function reference).
func IsWord(v Value) bool {
	return v.Parent.Equal(TWord)
}

// IsForward reports whether this value is a forward primitive.
func IsForward(v Value) bool {
	return v.Parent.Equal(TForward)
}

// IsBoolean reports whether this value is a boolean type.
func IsBoolean(v Value) bool {
	return v.Parent.ConformsTo(TBoolean)
}

// IsOpenParen reports whether this value is an open-paren marker.
func IsOpenParen(v Value) bool {
	return v.Parent.Equal(TOpenParen)
}

// IsCloseParen reports whether this value is a close-paren marker.
func IsCloseParen(v Value) bool {
	return v.Parent.Equal(TCloseParen)
}

// IsEnd reports whether this value is an end-marker.
func IsEnd(v Value) bool {
	return v.Parent.Equal(TEnd)
}

// IsParenExpr reports whether this value is a paren expression.
func IsParenExpr(v Value) bool {
	return v.Parent.Equal(TParenExpr)
}

// AsParenExpr returns the items in a paren expression value.
func AsParenExpr(v Value) ([]Value, error) {
	if pp, ok := v.Data.(ParenExprPayload); ok {
		return pp.Toks, nil
	}
	return nil, fmt.Errorf("AsParenExpr: not a paren-expr value (got %T)", v.Data)
}

// IsInterpString reports whether this value is an interpolated string.
func IsInterpString(v Value) bool {
	return v.Parent.Equal(TInterpString)
}

// AsInterpString returns the parts of an interpolated string value.
func AsInterpString(v Value) ([]InterpPart, error) {
	if ip, ok := v.Data.(InterpStringPayload); ok {
		return ip.Parts, nil
	}
	return nil, fmt.Errorf("AsInterpString: not an interp-string value (got %T)", v.Data)
}

// IsMark reports whether this value is a mark.
func IsMark(v Value) bool {
	return v.Parent.Equal(TMark)
}

// AsMark returns the MarkInfo, panics if not a mark.
func AsMark(v Value) (MarkInfo, error) {
	info, ok := v.Data.(MarkInfo)
	if !ok {
		return MarkInfo{}, fmt.Errorf("AsMark: not a mark value (got %T)", v.Data)
	}
	return info, nil
}

// IsMove reports whether this value is a move.
func IsMove(v Value) bool {
	return v.Parent.Equal(TMove)
}

// NewSplice wraps a value in an __SP (splice) marker. When the marker reaches
// the engine pointer its payload is spliced, unevaluated, into the stack.
func NewSplice(v Value) Value {
	return NewValueRaw(TSplice, SpliceInfo{Data: v})
}

// IsSplice reports whether this value is an __SP splice marker.
func IsSplice(v Value) bool {
	return v.Parent.Equal(TSplice)
}

// AsSplice returns the SpliceInfo, erroring if not a splice value.
func AsSplice(v Value) (SpliceInfo, error) {
	info, ok := v.Data.(SpliceInfo)
	if !ok {
		return SpliceInfo{}, fmt.Errorf("AsSplice: not a splice value (got %T)", v.Data)
	}
	return info, nil
}

// DispatchModInfo carries a `/`-modifier applied to a paren / dotted-path
// RESULT (a value), e.g. `(m.f)/s`, `path/3`, `m.a/v`. The parser emits a
// Word/__DM marker right after the group; execFnDefLiteral peeks and
// consumes it to dispatch the result function with these flags (or, for
// Ref/Quote, to leave it as inert data). ArgCount is -1 when unset. The
// `/u` (usurp) modifier is NOT carried here — it is emitted as the `usurp`
// word.
type DispatchModInfo struct {
	Val   bool // /v — take the binding's VALUE, disabling any call
	Quote bool // /q — treat the result as data
}

func (DispatchModInfo) payloadMarker() {}

// NewDispatchMod builds a Word/__DM dispatch-modifier marker.
func NewDispatchMod(info DispatchModInfo) Value {
	return NewValueRaw(TDispatchMod, info)
}

// IsDispatchMod reports whether v is a Word/__DM marker.
func IsDispatchMod(v Value) bool { return v.Parent.Equal(TDispatchMod) }

// AsDispatchMod returns the DispatchModInfo if v is a Word/__DM marker.
func AsDispatchMod(v Value) (DispatchModInfo, bool) {
	info, ok := v.Data.(DispatchModInfo)
	return info, ok
}

// AsMove returns the MoveInfo, panics if not a move.
func AsMove(v Value) (MoveInfo, error) {
	info, ok := v.Data.(MoveInfo)
	if !ok {
		return MoveInfo{}, fmt.Errorf("AsMove: not a move value (got %T)", v.Data)
	}
	return info, nil
}

// IsReturnCheck reports whether this value is a return-check marker.
func IsReturnCheck(v Value) bool {
	return v.Parent.Equal(TReturnCheck)
}

// AsReturnCheck returns the ReturnCheckInfo, panics if not a return-check.
func AsReturnCheck(v Value) (ReturnCheckInfo, error) {
	info, ok := v.Data.(ReturnCheckInfo)
	if !ok {
		return ReturnCheckInfo{}, fmt.Errorf("AsReturnCheck: not a return-check value (got %T)", v.Data)
	}
	return info, nil
}

// IsDefCleanup reports whether this value is a def-cleanup marker.
func IsDefCleanup(v Value) bool {
	return v.Parent.Equal(TDefCleanup)
}

// AsDefCleanup returns the DefCleanupInfo, panics if not a def-cleanup.
func AsDefCleanup(v Value) (DefCleanupInfo, error) {
	info, ok := v.Data.(DefCleanupInfo)
	if !ok {
		return DefCleanupInfo{}, fmt.Errorf("AsDefCleanup: not a def-cleanup value (got %T)", v.Data)
	}
	return info, nil
}

// IsDisjunct reports whether this value is a disjunction type — a
// plain Disjunct (Type/Disjunct) or any subtype such as an Enum
// (Type/Disjunct/Enum).
func IsDisjunct(v Value) bool {
	_, ok := v.Data.(DisjunctInfo)
	return ok && v.Parent.ConformsTo(TDisjunct)
}

// AsDisjunct returns the DisjunctInfo, panics if not a disjunct.
func AsDisjunct(v Value) (DisjunctInfo, error) {
	info, ok := v.Data.(DisjunctInfo)
	if !ok {
		return DisjunctInfo{}, fmt.Errorf("AsDisjunct: not a disjunct value (got %T)", v.Data)
	}
	return info, nil
}

// IsNegation reports whether v is a negation (complement) type value.
func IsNegation(v Value) bool {
	_, ok := v.Data.(NegationInfo)
	return ok && v.Parent.ConformsTo(TNegation)
}

// AsNegation returns the NegationInfo payload, or an error if v is not
// a negation value.
func AsNegation(v Value) (NegationInfo, error) {
	info, ok := v.Data.(NegationInfo)
	if !ok {
		return NegationInfo{}, fmt.Errorf("AsNegation: not a negation value (got %T)", v.Data)
	}
	return info, nil
}

// IsClassType reports whether this value is an object type definition.
// The test is payload-based: any value carrying ClassTypeInfo is an
// object type, regardless of where its Parent sits in the lattice — a
// builtin like Resource is an object type whose Parent is the peer
// Ideal kind Ideal/Resource, not a descendant of Ideal/Object.
func IsClassType(v Value) bool {
	_, ok := v.Data.(ClassTypeInfo)
	return ok
}

// AsClassType returns the ClassTypeInfo, panics if not an object type.
func AsClassType(v Value) (ClassTypeInfo, error) {
	info, ok := v.Data.(ClassTypeInfo)
	if !ok {
		return ClassTypeInfo{}, fmt.Errorf("AsClassType: not an object type value (got %T)", v.Data)
	}
	return info, nil
}

// IsStore reports whether this value is a Store instance.
func IsStore(v Value) bool {
	_, ok := v.Data.(*StoreInstanceInfo)
	return ok && v.Parent.ConformsTo(TStore)
}

// AsStore returns the StoreInstanceInfo pointer. Returns an error if not a store.
func AsStore(v Value) (*StoreInstanceInfo, error) {
	si, ok := v.Data.(*StoreInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("AsStore: not a store value (got %T)", v.Data)
	}
	return si, nil
}

// IsClassInstance reports whether this value is an object instance.
// Payload-based — see IsClassType.
func IsClassInstance(v Value) bool {
	_, ok := v.Data.(ClassInstanceInfo)
	return ok
}

// AsClassInstance returns the ClassInstanceInfo, panics if not an object instance.
func AsClassInstance(v Value) (ClassInstanceInfo, error) {
	info, ok := v.Data.(ClassInstanceInfo)
	if !ok {
		return ClassInstanceInfo{}, fmt.Errorf("AsClassInstance: not an object instance value (got %T)", v.Data)
	}
	return info, nil
}

// IsResourceType reports whether v is a Resource/Entity type descriptor.
func IsResourceType(v Value) bool {
	_, ok := v.Data.(ResourceTypeInfo)
	return ok
}

// AsResourceType returns the ResourceTypeInfo, or an error if v is not one.
func AsResourceType(v Value) (ResourceTypeInfo, error) {
	info, ok := v.Data.(ResourceTypeInfo)
	if !ok {
		return ResourceTypeInfo{}, fmt.Errorf("AsResourceType: not a resource type value (got %T)", v.Data)
	}
	return info, nil
}

// IsResourceInstance reports whether v is a Resource/Entity instance.
func IsResourceInstance(v Value) bool {
	_, ok := v.Data.(ResourceInstanceInfo)
	return ok
}

// AsResourceInstance returns the ResourceInstanceInfo, or an error if v is not one.
func AsResourceInstance(v Value) (ResourceInstanceInfo, error) {
	info, ok := v.Data.(ResourceInstanceInfo)
	if !ok {
		return ResourceInstanceInfo{}, fmt.Errorf("AsResourceInstance: not a resource instance value (got %T)", v.Data)
	}
	return info, nil
}

// IsFlatInstance reports whether v is a flat field-map instance — a
// class instance or a Resource/Entity instance. Both resolve every
// field (own + inherited) into a single map at make time with no
// prototype chain and carry an optional schema TypeRef, so container-
// shaped operations (field projection, size, deq, eq, serialization)
// treat them uniformly. Sites that special-case one MUST use this (or
// FlatInstanceFields / flatInstanceParts) so the two representations
// stay in lockstep.
func IsFlatInstance(v Value) bool {
	switch v.Data.(type) {
	case ClassInstanceInfo, ResourceInstanceInfo:
		return true
	}
	return false
}

// FlatInstanceFields returns a fresh copy of a flat instance's field
// map (class or Resource/Entity) and true; (nil, false) otherwise.
// Field order follows the instance's own order.
func FlatInstanceFields(v Value) (*OrderedMap, bool) {
	fields, _, ok := FlatInstanceParts(v)
	if !ok {
		return nil, false
	}
	out := NewOrderedMap()
	if fields != nil {
		for _, k := range fields.Keys() {
			val, _ := fields.Get(k)
			out.Set(k, val)
		}
	}
	return out, true
}

// flatInstanceParts returns the LIVE (shared) flat Fields map and the
// declared schema key order (own + inherited) for a class or resource
// instance. The Fields map is not copied — callers that mutate must
// copy first (see FlatInstanceFields).
func FlatInstanceParts(v Value) (fields *OrderedMap, schemaKeys []string, ok bool) {
	switch d := v.Data.(type) {
	case ClassInstanceInfo:
		if d.TypeRef != nil {
			schemaKeys = d.TypeRef.AllFields().Keys()
		}
		return d.Fields, schemaKeys, true
	case ResourceInstanceInfo:
		if d.TypeRef != nil {
			schemaKeys = d.TypeRef.AllFields().Keys()
		}
		return d.Fields, schemaKeys, true
	}
	return nil, nil, false
}

// IsAtom reports whether this value is an atom.
// IsPathon reports whether this value is a Path.
func IsPathon(v Value) bool {
	_, ok := v.Data.(PathonPayload)
	return ok && v.Parent.Equal(TPathon)
}

// AsPathon returns the PathonInfo, or an error if the value is not a path.
func AsPathon(v Value) (PathonInfo, error) {
	if pp, ok := v.Data.(PathonPayload); ok {
		return pp.Info, nil
	}
	return PathonInfo{}, fmt.Errorf("AsPathon: not a path value (got %T)", v.Data)
}

func IsAtom(v Value) bool {
	return v.Parent.ConformsTo(TAtom)
}

// payloadOf unwraps the payload variant P from v — the ONE nil-check +
// type-assertion + error shape every scalar accessor shares. name is the
// accessor name and kind the payload description for the error text
// ("AsString: not a string value (got %T)"), kept identical to the
// hand-rolled messages the accessors carried before unification.
func payloadOf[P Payload](v Value, name, kind string) (P, error) {
	var zero P
	if v.Data == nil {
		return zero, fmt.Errorf("%s: nil data", name)
	}
	p, ok := v.Data.(P)
	if !ok {
		return zero, fmt.Errorf("%s: not %s value (got %T)", name, kind, v.Data)
	}
	return p, nil
}

// AsAtom returns the string payload. Returns "" if Data is nil.
func AsAtom(v Value) (string, error) {
	p, err := payloadOf[AtomPayload](v, "AsAtom", "an atom")
	return p.Name, err
}

// IsTypedList reports whether this value is a typed list (has child type constraint).
func IsTypedList(v Value) bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.Parent.Equal(TList)
}

// IsTypedMap reports whether this value is a typed map (has child type constraint).
func IsTypedMap(v Value) bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// IsRecordType reports whether this value is a record type (map with field schema).
func IsRecordType(v Value) bool {
	_, ok := v.Data.(RecordTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// AsRecordType returns the RecordTypeInfo, panics if not a record type.
func AsRecordType(v Value) (RecordTypeInfo, error) {
	info, ok := v.Data.(RecordTypeInfo)
	if !ok {
		return RecordTypeInfo{}, fmt.Errorf("AsRecordType: not a record type value (got %T)", v.Data)
	}
	return info, nil
}

// IsOptionsType reports whether this value is an options type (map with defaults/constraints).
func IsOptionsType(v Value) bool {
	_, ok := v.Data.(OptionsTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// AsOptionsType returns the OptionsTypeInfo, panics if not an options type.
func AsOptionsType(v Value) (OptionsTypeInfo, error) {
	info, ok := v.Data.(OptionsTypeInfo)
	if !ok {
		return OptionsTypeInfo{}, fmt.Errorf("AsOptionsType: not an options type value (got %T)", v.Data)
	}
	return info, nil
}

// IsTableType reports whether this value is a table type (list with record schema).
func IsTableType(v Value) bool {
	if v.Parent.Equal(TList) {
		if _, ok := v.Data.(TableTypeInfo); ok {
			return true
		}
		if _, ok := v.Data.(TableData); ok {
			return true
		}
		if _, ok := v.Data.(MaterializerPayload); ok {
			return true
		}
		if _, ok := v.Data.(Materializer); ok {
			return true
		}
	}
	return false
}

// AsTableType returns the TableTypeInfo, panics if not a table type.
func AsTableType(v Value) (TableTypeInfo, error) {
	if td, ok := v.Data.(TableData); ok {
		return TableTypeInfo{Record: td.Record}, nil
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		return TableTypeInfo{Record: mp.M.SourceRecord()}, nil
	}
	if mz, ok := v.Data.(Materializer); ok {
		return TableTypeInfo{Record: mz.SourceRecord()}, nil
	}
	info, ok := v.Data.(TableTypeInfo)
	if !ok {
		return TableTypeInfo{}, fmt.Errorf("AsTableType: not a table type value (got %T)", v.Data)
	}
	return info, nil
}

// AsChildType returns the ChildTypeInfo, panics if not a typed list or typed map.
func AsChildType(v Value) (ChildTypeInfo, error) {
	info, ok := v.Data.(ChildTypeInfo)
	if !ok {
		return ChildTypeInfo{}, fmt.Errorf("AsChildType: not a child type value (got %T)", v.Data)
	}
	return info, nil
}

// AsWord returns the WordInfo, panics if not a word.
func AsWord(v Value) (WordInfo, error) {
	info, ok := v.Data.(WordInfo)
	if !ok {
		return WordInfo{}, fmt.Errorf("AsWord: not a word value (got %T)", v.Data)
	}
	return info, nil
}

// AsForward returns the ForwardInfo, panics if not a forward.
func AsForward(v Value) (ForwardInfo, error) {
	info, ok := v.Data.(ForwardInfo)
	if !ok {
		return ForwardInfo{}, fmt.Errorf("AsForward: not a forward value (got %T)", v.Data)
	}
	return info, nil
}

// AsString returns the string payload. Returns "" if Data is nil (type literal).
func AsString(v Value) (string, error) {
	p, err := payloadOf[StrPayload](v, "AsString", "a string")
	return p.S, err
}

// AsInteger returns the int64 payload. Returns 0 if Data is nil (type literal).
func AsInteger(v Value) (int64, error) {
	p, err := payloadOf[IntPayload](v, "AsInteger", "an integer")
	return p.N, err
}

// AsFloat returns the float64 payload. Returns 0.0 if Data is nil (type literal).
func AsFloat(v Value) (float64, error) {
	p, err := payloadOf[FloatPayload](v, "AsFloat", "a float")
	return p.F, err
}

// AsBigInteger returns the *big.Int payload of a BigInteger value.
func AsBigInteger(v Value) (*big.Int, error) {
	p, err := payloadOf[BigIntPayload](v, "AsBigInteger", "a BigInteger")
	return p.N, err
}

// AsBigDecimal returns the *apd.Decimal payload of a BigDecimal value.
func AsBigDecimal(v Value) (*apd.Decimal, error) {
	p, err := payloadOf[DecimalPayload](v, "AsBigDecimal", "a BigDecimal")
	return p.D, err
}

// AsNumber returns the numeric value as float64. It is defined ONLY for
// the fixed-width leaves Integer and Float; the arbitrary-precision Big
// leaves return an error rather than silently projecting to float64 (that
// would reopen the precision-loss trap across ~60 callers). Code that
// genuinely wants a lossy Big→float64 projection must call AsFloatApprox.
func AsNumber(v Value) (float64, error) {
	if v.Parent.ConformsTo(TFloat) {
		return AsFloat(v)
	}
	if v.Parent.ConformsTo(TBigInteger) || v.Parent.ConformsTo(TBigDecimal) {
		return 0, fmt.Errorf("AsNumber: %s is arbitrary-precision; use AsFloatApprox for a lossy float64", v.Parent)
	}
	n, err := AsInteger(v)
	return float64(n), err
}

// AsFloatApprox projects any numeric leaf to float64, accepting the
// precision loss for the Big leaves. This is the explicit, auditable
// home for the lossy projection (e.g. transcendental math on Big values,
// IEEE classifiers) — never reach for it where exactness is required.
func AsFloatApprox(v Value) (float64, error) {
	switch {
	case v.Parent.ConformsTo(TBigInteger):
		n, err := AsBigInteger(v)
		if err != nil {
			return 0, err
		}
		f := new(big.Float).SetInt(n)
		out, _ := f.Float64()
		return out, nil
	case v.Parent.ConformsTo(TBigDecimal):
		d, err := AsBigDecimal(v)
		if err != nil {
			return 0, err
		}
		return d.Float64()
	default:
		return AsNumber(v)
	}
}

// AsBoolean returns the bool payload. Returns false if Data is nil (type literal).
func AsBoolean(v Value) (bool, error) {
	p, err := payloadOf[BoolPayload](v, "AsBoolean", "a boolean")
	return p.B, err
}

// AsList returns the []Value payload, or nil if the data is not a []Value.
// Also works for TableData and Materializer, returning the rows.
// For Materializer, this triggers materialization.
// AsList returns a read-only view of the list payload.
// Returns an error if the value lacks a list-shaped payload.
// Type literals (Data == nil) and unsupported payload types are
// reported as errors so callers can't silently treat a non-list
// value as an empty list.
func AsList(v Value) (ReadList, error) {
	if v.Data == nil {
		return ReadList{}, fmt.Errorf("AsList: nil data")
	}
	// Post Step 5c variant first; legacy []Value second.
	if lp, ok := v.Data.(ListPayload); ok {
		return ReadList{elems: lp.Elems}, nil
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return ReadList{elems: fd.Elems}, nil
	}
	// Weak flex list: swept strong snapshot (see AsMap's weak arm).
	if wd, ok := v.Data.(*WeakFlexListData); ok {
		return ReadList{elems: wd.snapshotElems()}, nil
	}
	if td, ok := v.Data.(TableData); ok {
		return ReadList{elems: td.Rows}, nil
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			return ReadList{}, fmt.Errorf("AsList: materialize: %w", err)
		}
		return ReadList{elems: td.Rows}, nil
	}
	if mz, ok := v.Data.(Materializer); ok {
		td, err := mz.Materialize()
		if err != nil {
			return ReadList{}, fmt.Errorf("AsList: materialize: %w", err)
		}
		return ReadList{elems: td.Rows}, nil
	}
	// Typed list carrying both a child constraint and concrete
	// elements (`[v0 :T v1]`). Surface the elements so list-aware
	// operations (lengthq, firstq, is) see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Elements) > 0 {
		return ReadList{elems: ci.Elements}, nil
	}
	return ReadList{}, fmt.Errorf("AsList: not a list payload (got %T)", v.Data)
}

// AsMutableList returns the underlying []Value slice for mutation.
// Only valid for internal construction paths — never for immutable Node values.
// Returns an error if the value lacks a mutable list payload.
func AsMutableList(v Value) ([]Value, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMutableList: nil data")
	}
	if lp, ok := v.Data.(ListPayload); ok {
		return lp.Elems, nil
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return fd.Elems, nil
	}
	return nil, fmt.Errorf("AsMutableList: not a list payload (got %T)", v.Data)
}

// AsFlexList returns the pointer-backed element store of a FlexList
// value. Mutating words need the pointer — element growth (append,
// push) must reassign fd.Elems through it so every Value copy sharing
// the payload observes the change. Returns an error for anything that
// is not a concrete FlexList.
func AsFlexList(v Value) (*FlexListData, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsFlexList: nil data")
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return fd, nil
	}
	return nil, fmt.Errorf("AsFlexList: not a flex list payload (got %T)", v.Data)
}

// NewFlexXml creates a mutable Node/Xml/FlexXml element. attr may be nil
// (treated as no attributes); cren may be nil (no children). The payload
// is pointer-backed so append/set mutate in place. See core_flex.go.
func NewFlexXml(tag string, attr *OrderedMap, cren []Value) Value {
	if attr == nil {
		attr = NewOrderedMap()
	}
	return NewValueRaw(TFlexXml, &FlexXmlData{Tag: tag, Attr: attr, Cren: cren})
}

// AsFlexXml returns the pointer-backed FlexXmlData of a FlexXml value;
// mutators reassign through it so every Value copy observes the change.
func AsFlexXml(v Value) (*FlexXmlData, error) {
	if fd, ok := v.Data.(*FlexXmlData); ok {
		return fd, nil
	}
	return nil, fmt.Errorf("AsFlexXml: not a flex xml payload (got %T)", v.Data)
}

// AsXmlElement returns the XmlElementPayload backing a Node/Xml value,
// or an error when v is a type literal or a non-Xml value.
func AsXmlElement(v Value) (XmlElementPayload, error) {
	if x, ok := v.Data.(XmlElementPayload); ok {
		return x, nil
	}
	return XmlElementPayload{}, fmt.Errorf("AsXmlElement: not an Xml element payload (got %T)", v.Data)
}

// AsMap returns a read-only view of the map payload.
// Returns an error if the value lacks a map-shaped payload.
// Type literals (Data == nil) and unsupported payload types are
// reported as errors so callers can't silently treat a non-map
// value as an empty map.
func AsMap(v Value) (ReadMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMap: nil data")
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M, nil
	}
	// Weak flex map: sweep dead slots and materialize a STRONG
	// snapshot, so the whole read vocabulary observes one consistent
	// world per operation (design/FLEX-ATTRS.1.md §4.3).
	if wd, ok := v.Data.(*WeakFlexMapData); ok {
		return wd.snapshot(), nil
	}
	// Typed map carrying both a child constraint and concrete entries
	// (`{k:v :T}`). Surface the entries as an OrderedMap so map-aware
	// operations see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Entries) > 0 {
		om := NewOrderedMap()
		for _, e := range ci.Entries {
			om.Set(e.Key, e.Value)
		}
		return om, nil
	}
	return nil, fmt.Errorf("AsMap: not a map payload (got %T)", v.Data)
}

// AsMutableMap returns the underlying *OrderedMap for mutation. Only valid
// for Object instances and internal construction — never for Node values.
// Returns an error if the value lacks a mutable map payload.
func AsMutableMap(v Value) (*OrderedMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMutableMap: nil data")
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M, nil
	}
	return nil, fmt.Errorf("AsMutableMap: not a map payload (got %T)", v.Data)
}

// String returns a human-readable representation.
func (v Value) String() string {
	// A dynamic carrier renders as dynamic(<bound>) so the gradual
	// modality is legible in traces / `boru check` output instead of
	// masquerading as its bare bound (design/dynamic-modality-report.10.md).
	// Render the bound by clearing the flag and recursing.
	if v.Dynamic {
		inner := v
		inner.Dynamic = false
		return "dynamic(" + inner.String() + ")"
	}
	// Behavior-driven format delegation: types that supply a custom
	// TypeBehavior route through their Format. Walks the Parent
	// chain so descendants of a type with a custom Behavior inherit
	// it (e.g. TInspect inherits mapFormatBehavior via its TMap
	// parent). The DefaultBehavior sentinel — and any Behavior that
	// embeds defaultBehavior without overriding Format (e.g. the
	// scalar Comparer behaviors) — falls through to the kernel
	// renderer below.
	//
	// Type literals (Data==nil) are NOT delegated — they render as
	// their leaf type name uniformly across all types, including
	// types with custom Behaviors. See the Data==nil arm in
	// kernelFormatDefault.
	if v.Data != nil && v.Parent != nil {
		for t := v.Parent; t != nil; t = t.Parent {
			if t.Behavior() == nil || t.Behavior() == DefaultBehavior {
				continue
			}
			if delegatesFormat(t.Behavior()) {
				continue
			}
			return t.Behavior().Format(v)
		}
	}
	return kernelFormatDefault(v)
}

// formatDelegatesToDefault is an opt-in marker for TypeBehavior impls
// that embed defaultBehavior without overriding Format. Without it,
// Value.String would dispatch into the embedded defaultBehavior.Format,
// which calls v.String() — a stack-overflow. Marker-tagged Behaviors
// are skipped so the walk falls through to kernelFormatDefault.
type formatDelegatesToDefault interface {
	formatDelegate()
}

// FormatDelegate is the EXPORTED counterpart of formatDelegatesToDefault,
// for external (lang-layer / plugin) Behaviors that delegate Format to
// DefaultBehavior and want canon / Value.String to fall through to the
// kernel switch — so the value renders by its lattice family (e.g. an
// Atom subtype as `name` / `name/q`) instead of routing through the
// delegating Format. Unexported interface methods can only be satisfied
// from this package, so out-of-package Behaviors need this hook.
type FormatDelegate interface {
	FormatDelegate()
}

// delegatesFormat reports whether b opts out of Format routing via
// either the internal or exported marker.
func delegatesFormat(b TypeBehavior) bool {
	if _, ok := b.(formatDelegatesToDefault); ok {
		return true
	}
	_, ok := b.(FormatDelegate)
	return ok
}

// kernelFormatDefault renders v using the kernel's canonical switch.
// Used both as the terminal of Value.String's Behavior walk and from
// defaultBehavior.Format so the two paths produce identical output
// without recursing through Value.String.
func kernelFormatDefault(v Value) string {
	switch {
	case IsBoundedType(v):
		// `Type of [B]` renders as the suffix sugar B/t — the shortest
		// round-trippable spelling, mirroring atoms rendering as name/q.
		n, err := AsBoundedType(v)
		if err != nil {
			return "Type"
		}
		return n.Leaf() + "/t"
	case IsWord(v) && !IsSugar(v):
		w, _ := AsWord(v)
		return fmt.Sprintf("word(%s)", w.Name)
	case IsSugar(v):
		info, _ := AsSugar(v)
		switch info.Kind {
		case SugarForceArity:
			return fmt.Sprintf("sugar(%s %d)", info.Kind, info.N)
		case SugarMini:
			return fmt.Sprintf("sugar(%s %s '%s')", info.Kind, info.Name, info.Src)
		case SugarAngle:
			return fmt.Sprintf("sugar(%s %s %v)", info.Kind, info.Name, info.Items)
		case SugarTypeBound:
			return fmt.Sprintf("sugar(%s %v)", info.Kind, info.Items)
		}
		return fmt.Sprintf("sugar(%s)", info.Kind)
	case IsForward(v):
		f, _ := AsForward(v)
		return fmt.Sprintf("forward(%s,%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs)
	case IsOpenParen(v):
		return "("
	case IsCloseParen(v):
		return ")"
	case IsEnd(v):
		return "end"
	case IsParenExpr(v):
		_pe, _ := AsParenExpr(v)
		return fmt.Sprintf("paren(%v)", _pe)
	case IsInterpString(v):
		// A readable structural render (`interp('a ' ${word(x)})`) —
		// previously this fell to the %v fallback and dumped the raw
		// payload struct (`word()({[{ [42]}]})`), which leaked into
		// debug output and the parser stream oracle.
		parts, _ := AsInterpString(v)
		return renderInterpParts(parts)
	case IsXmlInterp(v):
		// Same repair for the XML template skeleton: render the source
		// form with ${…} holes instead of the payload struct dump.
		tmpl, _ := AsXmlInterp(v)
		return "interp-xml(" + renderXmlTmplSrc(tmpl) + ")"
	case IsMark(v):
		_as2, _ := AsMark(v)
		return fmt.Sprintf("mark(%s)", _as2.ID)
	case IsMove(v):
		m, _ := AsMove(v)
		return fmt.Sprintf("move(%s,%s)", m.To, m.Reason)
	case IsReturnCheck(v):
		rc, _ := AsReturnCheck(v)
		return fmt.Sprintf("returncheck(%s)", rc.FuncName)
	case IsDefCleanup(v):
		return "__dc"
	case IsError(v):
		_as3, _ := AsError(v)
		return fmt.Sprintf("error(%s)", _as3.Message)
	case IsNone(v):
		// The VALUE none (NonePayload, Data non-nil) — lowercase, like
		// the source literal and eng.Canon. The None TYPE literal has
		// Data==nil and takes the next arm, rendering capital `None`.
		return "none"
	case v.Data == nil:
		// Type literal (or carrier) with no specific value — render as
		// the type's leaf name; type names are globally unique so the
		// leaf alone is unambiguous.
		return TypeNodeOf(v).Leaf()
	case v.IsDepScalar():
		// Must come before TString / TInteger / TFloat matches: the
		// lattice override makes DepString.ConformsTo(TString) (and the
		// numeric counterparts) true, so without this case the value
		// payload would be cast to the wrong concrete type.
		return renderDepScalar(v)
	case v.Parent.ConformsTo(TString):
		s, _ := AsString(v)
		return fmt.Sprintf("'%s'", s)
	case v.Parent.ConformsTo(TAtom):
		s, _ := AsAtom(v)
		return s
	// Big leaves come before Integer/Float: they don't conform to either,
	// but listing them first keeps the numeric arms grouped and guards
	// against a future widening of those checks.
	case v.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(v)
		return FormatBigInteger(n)
	case v.Parent.ConformsTo(TBigDecimal):
		d, _ := AsBigDecimal(v)
		return FormatBigDecimal(d)
	case v.Parent.ConformsTo(TFloat):
		_as4, _ := AsFloat(v)
		return formatFloat(_as4)
	case v.Parent.ConformsTo(TInteger):
		n, _ := AsInteger(v)
		return fmt.Sprintf("%d", n)
	case v.Parent.ConformsTo(TBoolean):
		_as5, _ := AsBoolean(v)
		if _as5 {
			return "true"
		}
		return "false"
	// Domain types (Instant, DateTime, Date, TimeOfDay, CalendarDuration,
	// ClockDuration, Timezone, Matrix, Timeout, Interval) now render
	// via their per-Type Behavior installed by
	// coretype_format_behaviors.go and dispatched at the top of this
	// function. Their old switch arms have been removed.
	case IsPathon(v):
		_as6, _ := AsPathon(v)
		return _as6.String()
	case IsMicronValue(v) && micronRenderBridge != nil:
		// Backstop for Micron instances whose minted node carries a
		// wrapper Behavior that delegates Format to the kernel default
		// (a bare-nominal newtype's bareRefineUnifier wraps the fresh
		// mint's DefaultBehavior) — without this arm they would fall
		// to the %v branch and render the payload struct. The render
		// itself is family content, reached through the bridge the
		// content layer installs at init; with no content layer linked
		// the guard falls through to the generic arms below.
		return micronRenderBridge(v)
	// TList rendering moved to listFormatBehavior in
	// coretype_list_map_behaviors.go (Step 10). The top-of-function
	// Behavior dispatch routes List values there.
	// Timeout / Interval render via their per-Type Behavior — see
	// coretype_format_behaviors.go. Their arms have been removed
	// from this switch.
	case IsClassInstance(v):
		oi, _ := AsClassInstance(v)
		// A class instance normally carries its schema TypeRef; guard
		// defensively against a nil/anonymous TypeRef by rendering under
		// the lattice leaf or ID instead of dereferencing an absent
		// schema (was a SIGSEGV).
		name := ""
		switch {
		case oi.TypeRef != nil && oi.TypeRef.Name != "":
			name = oi.TypeRef.Name
		case oi.TypeRef != nil:
			name = "Ideal/Class/" + oi.TypeRef.ID
		case v.Parent != nil:
			name = v.Parent.Leaf()
		default:
			name = "Class"
		}
		return formatFieldBag(name, oi.AllFields())
	case IsResourceInstance(v):
		ri, _ := AsResourceInstance(v)
		name := "Resource"
		switch {
		case ri.TypeRef != nil && ri.TypeRef.Name != "":
			name = ri.TypeRef.Name
		case v.Parent != nil:
			name = v.Parent.Leaf()
		}
		return formatFieldBag(name, ri.Fields)
	case IsClassType(v):
		ot, _ := AsClassType(v)
		name := ot.Name
		if name == "" {
			name = "Ideal/Class/" + ot.ID
		}
		return formatFieldBag("object<"+name+">", ot.AllFields())
	case IsDisjunct(v):
		di, _ := AsDisjunct(v)
		parts := make([]string, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			parts[i] = alt.String()
		}
		return strings.Join(parts, " tor ")
	case IsNegation(v):
		ni, _ := AsNegation(v)
		// Parenthesise a compound inner so `tnot (A tor B)` doesn't misread
		// as `(tnot A) tor B`.
		if IsDisjunct(ni.Inner) || IsNegation(ni.Inner) {
			return "tnot (" + ni.Inner.String() + ")"
		}
		return "tnot " + ni.Inner.String()
	// A function value (TFunction, payload FnDefInfo) renders
	// as a compact `fn name(sig…)` summary. Crucially it does NOT fall
	// through to the `%v` default, which would dump the whole FnDefInfo
	// — including its captured *Registry and the module's entire exports
	// map (a 600-line spill into error messages). See formatFnDef.
	case isFnDefValue(v):
		fd, _ := v.Data.(FnDefInfo)
		return FormatFnDef(fd)
	case func() bool { cl, ok := v.Data.(ClosurePayload); return ok && cl.Render != "" }():
		// A compiled closure carrying its source fn's interpreter rendering
		// (CompiledFn.Render via OpPushClosure) — byte-identical to the
		// interpreter's fn value in interpolation holes and print output.
		cl, _ := v.Data.(ClosurePayload)
		return cl.Render
	// TMap rendering moved to mapFormatBehavior in
	// coretype_list_map_behaviors.go (Step 10). The top-of-function
	// Behavior dispatch routes Map values (and TMap-rooted subtypes
	// like RecordType/OptionsType) there.
	default:
		return fmt.Sprintf("%v(%v)", v.Parent, v.Data)
	}
}

// isFnDefValue reports whether v carries an FnDefInfo payload (a fn or
// lambda value, Parent TFunction).
func isFnDefValue(v Value) bool {
	_, ok := v.Data.(FnDefInfo)
	return ok
}

// formatFnDef renders a function value as a compact one-line summary:
// `fn name(argTypes…)` per overload, e.g. `fn inc(Integer)` or
// `fn (Integer, String) or (Float)`. It reads ONLY Name and the
// argument types of each Signature — never Registry, Captured, or the
// raw FnSig bodies — so a function value can appear in an error message
// or a printed list without spilling its closure environment (the
// module exports map in particular). Anonymous lambdas have no Name and
// render as `fn(args…)`.
func FormatFnDef(fd FnDefInfo) string {
	name := fd.Name
	// Summarise the argument shapes. Prefer non-empty signatures; a
	// synthesized 0-arg fallback alongside real overloads is noise here
	// (it is not shown by `help` either), so drop empty sigs when any
	// non-empty one exists.
	var sigParts []string
	for i := range fd.Signatures {
		if fd.Signatures[i].TotalArgs() > 0 {
			sigParts = append(sigParts, describeSigArgs(&fd.Signatures[i]))
		}
	}
	sigPart := strings.Join(sigParts, " or ")
	switch {
	case name == "" && sigPart == "":
		return "fn"
	case name == "":
		return "fn " + sigPart
	case sigPart == "":
		return "fn " + name
	default:
		return "fn " + name + sigPart
	}
}

// IsTypeValue returns true if v is a type literal, an Options instance,
// or a Node that contains a leaf that is a type.
func IsTypeValue(v Value) bool {
	// Type literal: Data==nil with a real type (not None).
	if v.Data == nil && !IsNoneShape(v) {
		return true
	}

	// Options type, record type, typed list/map, table type, object type.
	if IsOptionsType(v) || IsRecordType(v) || IsTypedList(v) ||
		IsTypedMap(v) || IsTableType(v) || IsClassType(v) {
		return true
	}

	// Concrete list: check each element recursively.
	if v.Parent.ConformsTo(TList) && v.Data != nil {
		elems, _ := AsList(v)
		if !elems.IsNil() {
			for _, elem := range elems.Slice() {
				if IsTypeValue(elem) {
					return true
				}
			}
		}
	}

	// Concrete map: check each value recursively.
	if v.Parent.ConformsTo(TMap) && v.Data != nil {
		m, _ := AsMap(v)
		if m != nil {
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				if IsTypeValue(val) {
					return true
				}
			}
		}
	}

	return false
}
