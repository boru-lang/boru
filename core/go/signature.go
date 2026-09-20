package core

import "sort"

// MaxArgs is the maximum number of arguments a signature may declare.
const MaxArgs = 32

// Handler is the unified function handler type for all boru words.
// It receives the matched arguments, the current context map, the
// resolved stack (only populated for FullStack signatures), and the
// registry.
type Handler func(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error)

// Signature describes one way a function can be called. Params lists the
// per-position descriptors (name + type + pattern + optional), ordered
// top-first (Params[0] = top of stack for stack matching).
//
// Params[0..BarrierPos-1] are forward-eligible — the engine collects them
// from the tokens following the word, then dispatches once all are
// present. Params[BarrierPos..N-1] are matched from the stack in reverse.
type Signature = FnSig

// CheckFullStackFunc produces the full base..pointer replacement
// for a FullStack signature in check mode. args are the matched
// carrier args in signature order; stack is the preserved carrier
// stack segment below the args; r is the registry the analysis is
// running against (for emitting diagnostics, reading defs, etc.).
type CheckFullStackFunc func(args []Value, stack []Value, r *Registry) []Value

// ReturnsFunc computes the carrier return values for a signature in
// static type-check mode. args are the carrier-typed input values in
// signature order; r is the registry (for emitting diagnostics,
// reading defs, running sub-analyses, etc.) — the same one passed to
// the runtime Handler.
type ReturnsFunc func(args []Value, r *Registry) []Value

// TotalArgs returns the number of arguments. Backed by Params (the
// unified per-position descriptor. Falls back to len(Args) for a
// Signature built positionally via the Args constructor-convenience
// field that hasn't been normalized into Params yet.
func (s *Signature) TotalArgs() int {
	if len(s.Params) == 0 {
		return len(s.Args)
	}
	return len(s.Params)
}

// usesLegacyArgs reports that this Signature was built through the
// positional Args/Patterns constructor-convenience fields and has not
// yet been normalized into Params (normalizeSig hasn't run). It is the
// SINGLE definition of the "fall back to the Args mirror" condition the
// per-position accessors share, so the predicate lives in one place
// instead of being re-spelled at each reader.
func (s *Signature) usesLegacyArgs() bool {
	return len(s.Params) == 0 && len(s.Args) > 0
}

// ArgTypes returns the per-position declared types of the signature,
// derived from Params (the unified source of arg shape). This is the
// EXPORTED accessor external consumers (the lang help/inspect surface)
// should use instead of the legacy Args field, so that field can later
// be retired without a public-API break. Order is sig order (position 0
// = top of stack). Funnels through sigArgType / TotalArgs so the
// Params-or-mirror fallback has one implementation.
func (s *Signature) ArgTypes() []*Type {
	out := make([]*Type, s.TotalArgs())
	for i := range out {
		out[i] = SigArgType(s, i)
	}
	return out
}

// normalizeSig makes Params authoritative for a Signature that may have
// been built via the positional Args/Patterns constructor-convenience
// fields. If Params is empty but Args is set, it derives Params from
// Args+Patterns. It then refreshes the Args/Patterns mirrors from Params
// so introspection that reads either view stays consistent. Idempotent.
func NormalizeSig(s *Signature) {
	if s.usesLegacyArgs() {
		s.Params = make([]FnParam, len(s.Args))
		for i, t := range s.Args {
			s.Params[i] = FnParam{Type: t, Quote: s.QuoteArgs[i]}
			if s.Patterns != nil {
				if pat, ok := s.Patterns[i]; ok {
					p := pat
					s.Params[i].Pattern = &p
				}
			}
		}
	}
	// Refresh the exported mirrors from Params (the source of truth).
	s.Args = make([]*Type, len(s.Params))
	var patterns map[int]Value
	for i, p := range s.Params {
		s.Args[i] = p.Type
		if p.Pattern != nil {
			if patterns == nil {
				patterns = make(map[int]Value)
			}
			patterns[i] = *p.Pattern
		}
		// boru-declared /q params (FnParam.Quote, from `name:Atom/q`)
		// merge INTO QuoteArgs — the field every dispatch-side reader
		// consults — without disturbing native-set entries (two
		// declaration surfaces, one per-position property).
		//
		// Only write a NEW entry: a native /q sig already carries the
		// position in QuoteArgs (its FnParam.Quote was DERIVED from that map
		// in the legacy-Args branch above), so re-writing it would mutate the
		// package-level author map that every Registry shares — a data race
		// under concurrent registration. Skipping the redundant write keeps
		// the shared native map read-only; the boru path (QuoteArgs nil/absent)
		// still populates its own per-parse map.
		if p.Quote && !s.QuoteArgs[i] {
			if s.QuoteArgs == nil {
				s.QuoteArgs = make(map[int]bool)
			}
			s.QuoteArgs[i] = true
		}
	}
	s.Patterns = patterns
}

// sigArgType returns the declared type at signature position i. It is the
// single accessor every matcher / forward-planner reader uses for a
// signature's per-position type. Storage lives in Params[i].Type; the
// legacy Args slice is still populated at construction sites and used as
// a fallback for legacy/external callers that built a Signature with only
// Args set. Callers must ensure 0 <= i < TotalArgs().
func SigArgType(s *Signature, i int) *Type {
	if s.usesLegacyArgs() {
		return s.Args[i]
	}
	return s.Params[i].Type
}

// sigPattern returns the optional structural pattern at signature
// position i, mirroring sigArgType: storage lives in Params[i].Pattern,
// with a fallback to the legacy Patterns map for callers that built a
// Signature with only Args+Patterns set. ok is false when no pattern is
// declared at i.
func SigPattern(s *Signature, i int) (Value, bool) {
	if i < len(s.Params) && s.Params[i].Pattern != nil {
		return *s.Params[i].Pattern, true
	}
	// Constructor-convenience fallback: a Signature built with only the
	// positional Args/Patterns fields (tests, plugins) hasn't been
	// normalized into Params yet.
	if len(s.Params) == 0 && s.Patterns != nil {
		p, ok := s.Patterns[i]
		return p, ok
	}
	return Value{}, false
}

// MatchResult holds a matched signature and the positionally matched args.
type MatchResult struct {
	Sig       *Signature
	Args      []Value   // args in signature order
	Positions []int     // absolute stack indices of each arg (nil for 0-arg)
	Name      string    // word name being dispatched (for tracing/recording)
	Reg       *Registry // sub-registry owning Sig for a module delegation dispatch (nil = main)
}

// MatchSignature finds the first matching signature for a function given the
// resolved stack and optional word modifiers.
//
// Signatures are assumed to be pre-sorted by SortSignatures (longest and most
// specific first, fallbacks last). The first match wins.
//
// stack is the resolved portion of the stack (index 0 = bottom, last = top).
// modifiers control filtering (forceStack, forceForward, argCount).
//
// Returns nil if no signature matches.
func MatchSignature(sigs []Signature, stack []Value, modifiers WordInfo) *MatchResult {
	for i := range sigs {
		sig := &sigs[i]

		if modifiers.ArgCount >= 0 && sig.TotalArgs() != modifiers.ArgCount {
			continue
		}

		n := sig.TotalArgs()
		if len(stack) < n {
			continue
		}

		// Extract top n values from the stack.
		base := len(stack) - n
		top := stack[base:]

		// Try flexible match.
		ordered, ok := FlexibleMatch(top, sig)
		if !ok {
			continue
		}

		// Check structural patterns (e.g. map literals in fn signatures).
		// Maps use open (subset) matching: the pattern's key-value pairs
		// must be present in the argument, but extra keys are allowed.
		patternOk := true
		for idx := 0; idx < sig.TotalArgs(); idx++ {
			pattern, pok := SigPattern(sig, idx)
			if !pok {
				continue
			}
			// Carrier pattern: a check-mode placeholder for a computed
			// pattern value — unknowable statically, never enforced
			// (see patternsOk in match.go).
			if pattern.Carrier {
				continue
			}
			if pattern.Parent.Equal(TMap) && ordered[idx].Parent.Equal(TMap) &&
				pattern.Data != nil && ordered[idx].Data != nil &&
				!IsOptionsType(pattern) && !IsTypedMap(pattern) &&
				!IsRecordType(ordered[idx]) && !IsTypedMap(ordered[idx]) && !IsOptionsType(ordered[idx]) {
				if !OpenUnifyMap(pattern, ordered[idx]) {
					patternOk = false
					break
				}
			} else {
				if _, uOk := Unify(ordered[idx], pattern); !uOk {
					patternOk = false
					break
				}
			}
		}
		if !patternOk {
			continue
		}

		args := make([]Value, n)
		copy(args, ordered)
		return &MatchResult{Sig: sig, Args: args}
	}

	return nil
}

// FlexibleMatch checks whether values match the given signature positionally.
// Arguments are never permuted — values[i] must match Params[i].
// Returns the values slice unchanged if matched, or false.
func FlexibleMatch(values []Value, sig *Signature) ([]Value, bool) {
	n := sig.TotalArgs()
	if len(values) < n {
		return nil, false
	}

	if positionalMatch(values, sig) {
		return values, true
	}

	return nil, false
}

// sigTypeMatches checks whether a value's type matches a signature
// arg type for an ordinary (non-TypeArgs) slot. Routes the primary
// subtype check through Behavior so per-type custom Match
// implementations participate in signature matching.
//
// A type-literal expectation lives on the sig as TypeArgs[i]=true
// (see sigTypeMatchesAsType); this function is the value-side path.
//
// **The carrier rule.** Carriers have a concrete Parent (e.g.
// TInteger) and nil Data, identical to a type literal at the field
// level — but semantically they are abstract VALUES, not types. They
// satisfy ordinary value slots (Carrier{Integer} matches TInteger)
// and are rejected at TypeArgs slots by sigTypeMatchesAsType.
func SigTypeMatches(v Value, t *Type) bool {
	// Dispatch ascription (`v as T` — design/OPEN-WORDS.1.md §9): match as
	// if the value were tagged with the ascribed ancestor type. The VIEW is
	// a reparented copy used for this test only — the delivered arg is the
	// real value (execMatch / the VM strip the ascription at delivery).
	// Strictly non-dynamic: `as` validated (or will validate, for a dynamic
	// input, at its own runtime step) that the value conforms to the
	// ascribed type, so dispatch resolves at exactly that type — which is
	// what makes an anchored subtype override unable to capture the call
	// (the widened tag no longer conforms to the override's nominal
	// anchor). Every matcher funnels through here — the interpreter's
	// matchSignature arms, positionalMatch, the VM's guarded-native
	// contract and poly re-match — so this one arm is the whole rule.
	if a := v.AscribedType(); a != nil {
		view := ReparentValue(v, a)
		view.SetAscribed(nil)
		view.Dynamic = false
		return SigTypeMatches(view, t)
	}
	// Gradual (dynamic) carrier: matches the slot unless its bound is
	// PROVABLY disjoint from t — the not-disjoint rule, the optimistic
	// dual of strict ConformsTo (design/legacy/dynamic-modality-report.10.ignore).
	// Reuses `tand` for the disjointness proof; dynamic(Any) matches
	// every inhabited slot, dynamic(Integer) fails only provably-disjoint
	// slots (String, Atom, …). Checked first so a dynamic carrier never
	// falls into the strict path below. The flag is cleared on the
	// operand copy so the bound flows through `tand` as an ordinary
	// carrier.
	if v.Dynamic {
		// A bound that conforms to the slot (X ⊑ t — the value IS a t), or a
		// slot that conforms to the bound (t ⊑ X — the value MIGHT be a t, the
		// gradual optimism), matches. This direct conformance check is needed
		// because the `tand` disjointness probe below wrongly reports two
		// container-family carriers as disjoint when one conforms to the other
		// (tand(List, Node) = Never even though List ⊑ Node) — so a value pulled
		// from a fn whose declared return narrowed to a dynamic List/Map carrier
		// failed to match a Node-typed `get`/`set` receiver (the trie walkers'
		// `(nd kid-items)`/`build-row`-result → `get` cascade). The tand probe
		// still handles the cross-family disjoint cases (dynamic(Integer) vs
		// String). Looser, never tighter — a guard discharges the modality back
		// to strict downstream.
		if v.Parent != nil && t != nil && (v.Parent.ConformsTo(t) || t.ConformsTo(v.Parent)) {
			return true
		}
		// A dynamic DISJUNCT bound matches if ANY alternative could — the
		// optimistic dual of the strict-disjunct EVERY rule below. The
		// runtime holds exactly one alternative, and gradual optimism picks
		// the fitting one under the same contract as dynamic(Any): the
		// runtime re-checks the concrete value. Without this arm a declared
		// value-or-sentinel union (dynamic(Map ∪ None) from a stat-shaped
		// word) fails every typed receiver slot — the Disjunct parent
		// conforms to nothing and the whole-bound tand probe below cannot
		// see through to the alternatives — so the union strands forward
		// collection at the first field read.
		if IsDisjunct(v) {
			di, err := AsDisjunct(v)
			if err == nil {
				for _, alt := range di.Alternatives {
					probe := alt
					if IsBareTypeNode(alt) {
						probe = CarrierOfLiteral(alt)
					}
					probe.Dynamic = true
					if SigTypeMatches(probe, t) {
						return true
					}
				}
			}
		}
		bound := v
		bound.Dynamic = false
		return !IsNeverShape(TandValues(bound, NewCarrier(t)))
	}
	if v.Is(t) {
		return true
	}
	// Strict disjunct carrier (check mode only — runtime values are
	// never carriers): matches iff EVERY alternative matches, so
	// dispatch distributes over the abstract domain. The selected
	// signature's returns are then refined per alternative by
	// disjunctPartitionReturns (design/legacy/checker-accuracy-review.10.ignore A1).
	if v.Carrier && !v.Dynamic && IsDisjunct(v) {
		di, err := AsDisjunct(v)
		if err == nil && len(di.Alternatives) > 0 {
			all := true
			for _, alt := range di.Alternatives {
				probe := alt
				if IsBareTypeNode(alt) {
					probe = CarrierOfLiteral(alt)
				}
				if !SigTypeMatches(probe, t) {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
	}
	// Options is structurally a keyword-args map. A parameter typed
	// `Options` (a bare `opts:Options` annotation → TOptions slot)
	// accepts both an Options-tagged value AND a plain concrete map —
	// the latter is how callers actually pass options (`f {a:1}`).
	// Without this, every make-style fn is forced to declare `Map`
	// instead of the more descriptive `Options`. A bare `Map` type
	// literal (Data==nil) is excluded; only a concrete map matches.
	if t.Equal(TOptions) {
		if IsOptionsType(v) {
			return true
		}
		// ConformsTo (not Equal) so a FlexMap also passes where an
		// Options map is expected. A Map-typed CARRIER also matches: it is
		// check mode's stand-in for a value that IS a concrete map at run
		// time, and the runtime rule above accepts that value — without
		// this the check-mode dispatch declines what the interpreter runs
		// (the template `{…} render` → Options-arm → tpl-render-opts
		// compile failure). A bare Map type literal (Data==nil, not a carrier)
		// stays excluded.
		if v.Parent.ConformsTo(TMap) && (IsConcrete(v) || v.Carrier) {
			return true
		}
	}
	// An Options / Record value matches a Map- or Node-family slot. These
	// structural keyword/field-map types are lattice-rooted under Ideal (not
	// Map), so an `opts:Options` or record carrier does NOT ConformsTo Node —
	// yet its VALUE is a map, so get/set/size over such a receiver must match
	// the Map/Node slot (the bloom `opts "n" get` / decision record reads).
	// Runtime already dispatches these (the value's payload Parent is TMap);
	// this aligns the check-mode carrier with that.
	if t != nil && TMap.ConformsTo(t) && v.Parent != nil &&
		(v.Parent.ConformsTo(TOptions) || v.Parent.ConformsTo(TRecord)) {
		return true
	}
	return false
}

// sigTypeMatchesAsType is the TypeArgs-slot match: v must be a type
// literal (or a structural type body) whose denoted lattice node
// matches t. Used for sig positions like the second arg of
// `Integer gte 10` — the "Integer" type literal — or the first arg
// of `make Foo {...}` — the Foo type body.
//
// A type literal is a by-value copy of its lattice node (Data==nil,
// Parent set to the supertype); the denoted node is &v. A structural
// type body (RecordType, OptionsType, TableType, ClassType,
// ChildType) carries non-nil Data but its Parent is the family root
// (TMap, TList, TClass) — we match against the Parent for those.
// Carriers (Data==nil but Carrier=true) are values, not types, and
// are rejected here.
func sigTypeMatchesAsType(v Value, t *Type) bool {
	if v.Carrier {
		return false
	}
	if IsBareTypeNode(v) {
		// Bare None has Parent=TNone; treat it as not-a-type for type
		// args. Lattice roots have Parent=nil but are still valid type
		// literals — &v is the lattice node either way.
		if v.Parent != nil && v.Parent.Equal(TNone) && v.Name() == "" {
			return false
		}
		return (&v).ConformsTo(t)
	}
	// DepScalar bodies are NOT accepted at TypeArgs slots: they're
	// constraints over a base scalar (used as runtime values), not
	// bare scalar type literals — the dep-sig fallthrough would
	// otherwise loop back on itself for `(Integer gt 10) lt
	// (Integer gt 20)`.
	if v.IsDepScalar() {
		return false
	}
	// Other structural type bodies (Record, Options, Table, Object,
	// ChildType, Disjunct, Enum, Function/FnUndef, ImplicitMap
	// record shape) are "types" — accept them when their lattice
	// family matches the slot.
	if IsTypeBody(v) {
		return v.Parent.ConformsTo(t)
	}
	return false
}

// sigArgMatches dispatches a positional sig match to either the
// ordinary value matcher or the TypeArgs (type-literal) matcher
// based on sig.TypeArgs[idx]. Use this at every call site that has
// a *Signature in hand; bare sigTypeMatches stays for the
// no-sig-context paths (carrier promotion, predicate sandbox).
func SigArgMatches(sig *Signature, idx int, v Value) bool {
	if sig.TypeArgs != nil && sig.TypeArgs[idx] {
		return sigTypeMatchesAsType(v, SigArgType(sig, idx))
	}
	return SigTypeMatches(v, SigArgType(sig, idx))
}

// rejectsTypeLiteral reports whether a value with Data==nil should be
// rejected at a concrete-payload sig slot — even if sigTypeMatches
// said the Parent matches.
//
// A type literal (e.g. `Integer` resolved from a bare type-name word)
// has Data==nil, so handlers that read its payload via AsX() would
// silently pull the zero value. That used to make programs like
// `addq Integer 1` quietly compute `addq 0 1` instead of raising. Now
// the matcher rejects type literals at every concrete-payload slot
// and dispatch falls through to a TAny overload (or signature_error
// if none exists).
//
// Type literals are still legitimately accepted at:
//
//   - TAny slots — universal catch-all; the handler is expected to
//     handle both concrete payloads and type literals.
//   - TypeArgs slots — the sig-level "I want a type literal here"
//     marker (the successor to the historical metatype slots).
//     rejectsTypeLiteral has no sig in hand; callers wrap the
//     check with a `!sig.TypeArgs[i]` guard.
//
// Carriers (Data==nil but Carrier=true) are abstract VALUES, not
// types — sigTypeMatches deliberately treats them as values, and
// this rejection check follows suit. The value `none` is also
// legitimate at a TNone slot — None has a single inhabitant and
// that's it. This covers the spec runner's NewNone() (Data != nil
// sentinel value with Parent=TNone) AND production boru's
// `NewTypeLiteral(TNone)` (Data == nil, value IS the TNone lattice
// node — its own Parent is nil since None is a degenerate root).
func rejectsTypeLiteral(v Value, expectedType *Type) bool {
	if v.Data != nil {
		return false
	}
	if v.Carrier {
		return false
	}
	if expectedType.Equal(TAny) {
		return false
	}
	if expectedType.Equal(TNone) {
		// At a TNone slot, the None type literal is the canonical
		// inhabitant; sigTypeMatches has already verified the value
		// is None-typed.
		return false
	}
	if expectedType.Equal(TType) {
		// At a Type slot, type literals are the canonical inhabitants —
		// `t:Type` params (and the bounded `Map/t` pattern over them)
		// exist exactly to receive them. Mirrors the TNone carve;
		// membership was already verified by typeMembershipBehavior.
		return false
	}
	if _, ok := v.TypeBody(); ok {
		// A node carrying recorded type CONTENT is the evaluated name
		// of a structural type (the Stage 2 flip of
		// design/legacy/TYPE-REPRESENTATION.1.ignore): it is admissible exactly
		// where its declared body was — `refine Table R` collects R at
		// the TNode arg slot precisely as it collected R's record body
		// before the flip. Pure nominal literals (builtins, refine
		// newtypes) record no content and keep the rejection.
		return false
	}
	return true
}

// positionalMatch checks whether values match the signature's types in order.
// Handles the /q modifier: a Word value at a QuoteArgs position is treated
// as an Atom for type matching purposes.
//
// /q is a forward-only language rule (see Signature.QuoteArgs doc). The
// Word→Atom branch below is reachable only through the forward-collection
// path, where a raw Word can land at the sig position. For stack-only
// matching the value is never a Word (stepWord has already resolved it),
// so the branch falls through to the regular sigTypeMatches check.
func positionalMatch(values []Value, sig *Signature) bool {
	for i := 0; i < sig.TotalArgs(); i++ {
		t := SigArgType(sig, i)
		v := values[i]
		// /q modifier (forward-only): treat Word as Atom for matching.
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && v.Parent.Equal(TWord) {
			if !TAtom.ConformsTo(t) {
				return false
			}
			continue
		}
		// A /q position captures a LITERAL word/atom. In check mode a computed
		// operand arrives as a NON-CONCRETE carrier (an Any/dynamic value, e.g.
		// `quote (s get k)`); such a value is not a literal word, so it must not
		// ride the Any-conforms-to-everything rule onto a /q overload — it would
		// claim `quote`'s word-capture sig (TAtom, QuoteArgs) and then decline to
		// compile, instead of falling to the value sig (TAny, ReturnsIdentity).
		// At RUNTIME the operand is concrete (no carrier), so this is inert there
		// and merely aligns check-mode sig selection with the runtime's (a
		// concrete non-Atom already fails TAtom and picks the value overload).
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && v.Carrier && !IsConcrete(v) && !v.Parent.ConformsTo(TAtom) {
			return false
		}
		if !SigArgMatches(sig, i, v) {
			return false
		}
		// Reject type literals (Data==nil) for concrete Map/List
		// signatures — including the FlexMap/FlexList subtypes —
		// unless this slot explicitly wants a type literal.
		isTypeArg := sig.TypeArgs != nil && sig.TypeArgs[i]
		if !isTypeArg && IsBareTypeNode(v) && (t.ConformsTo(TMap) || t.ConformsTo(TList)) {
			return false
		}
	}
	return true
}

// sigSlotValue returns the expectation value the sig declares at
// position i: the structural Pattern if one is set, otherwise a bare
// type literal of the declared arg type. The result feeds
// CompareValues so the unified type/value lattice settles per-position
// ordering — a concrete Pattern (Data != nil) sorts strictly above the
// bare type literal of the same family via litVsConcreteOrder, and two
// bare type literals fall through to compareTypes (Rank → depth →
// name → ID).
func sigSlotValue(sig *Signature, i int) Value {
	if p, ok := SigPattern(sig, i); ok {
		return p
	}
	return NewTypeLiteral(SigArgType(sig, i))
}

// CompareSignatures imposes a total order on Signatures using the
// unified type/value lattice, in REVERSE — the more specific sig
// sorts first so MatchSignature's first-match-wins loop picks the
// tightest overload available.
//
// Each sig's args are treated as a List of expectation values
// (per sigSlotValue). Comparison follows the list-comparison contract
// from CompareValues: list size first (longer lists sort below shorter
// in natural order; reversed here, so longer arity wins), then
// element-wise. At each position the reversed CompareValues result
// places the more specific value (concrete pattern, deeper type, …)
// first. BarrierPos breaks the final tie: a sig with a stack barrier
// (non-zero BarrierPos) sorts before an otherwise identical sig
// without one, since the barrier is an additional dispatch constraint.
//
// Fallback sigs need no special-case: a fallback is always 0-arg, so
// the arity-first rule already sinks it to the end.
//
// There is NO tier key: locked and merged signatures sort together by
// the natural specificity order. Dispatch stability under merges is
// guaranteed by ADMISSION instead of ordering — every merged signature
// must be anchored by a nominal type its author owns (the ownership
// rules, design/OPEN-WORDS.1.md §2-3), so an added signature can only
// ever match calls carrying values of the author's own types, which
// cannot predate the merge. `Locked` survives only as the
// replacement-compile failure flag (mergeExtensionSigs) — never as an ordering
// input; the rev-1 locked-first key this replaces is documented in
// design/OPEN-WORDS.0.md §2.3/§3.3.
func CompareSignatures(a, b *Signature) int {
	// CoreDefault overloads (the within-type scalar/Micron field-wise
	// arithmetic, RegisterCoreDefault) sort strictly LAST: they are the
	// kernel's declared catch-afters, dispatched only when no specific
	// overload — kernel, module, or user — claims the call. Under rev 1
	// this fell out of their being unlocked behind the locked tier; rev
	// 2 encodes the role explicitly.
	if a.CoreDefault != b.CoreDefault {
		if a.CoreDefault {
			return 1
		}
		return -1
	}
	if c := cmpInt(b.TotalArgs(), a.TotalArgs()); c != 0 {
		return c
	}
	for i := 0; i < a.TotalArgs(); i++ {
		av := sigSlotValue(a, i)
		bv := sigSlotValue(b, i)
		c, err := CompareValues(av, bv)
		if err != nil || c == 0 {
			continue
		}
		return -c
	}
	// "Has a piped barrier" means an intermediate position — neither
	// all-stack (0) nor all-forward (len(Args)). The two extremes are
	// the default shapes and don't represent an additional dispatch
	// constraint worth sorting on.
	aBarrier := a.BarrierPos > 0 && a.BarrierPos < a.TotalArgs()
	bBarrier := b.BarrierPos > 0 && b.BarrierPos < b.TotalArgs()
	if aBarrier && !bBarrier {
		return -1
	}
	if !aBarrier && bBarrier {
		return 1
	}
	return 0
}

// SortSignatures sorts a slice of signatures in-place by reversed
// lattice order (see CompareSignatures): longer arity first, then per
// position the more specific type/pattern first. Stable: sigs that
// compare equal preserve registration order.
func SortSignatures(sigs []Signature) {
	sort.SliceStable(sigs, func(i, j int) bool {
		return CompareSignatures(&sigs[i], &sigs[j]) < 0
	})
}

// RankSignatures returns the indices of sigs sorted by priority (best
// first), using the same reversed-lattice order as SortSignatures.
func RankSignatures(sigs []Signature) []int {
	indices := make([]int, len(sigs))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return CompareSignatures(&sigs[indices[i]], &sigs[indices[j]]) < 0
	})
	return indices
}
