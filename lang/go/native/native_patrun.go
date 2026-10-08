package native

import (
	"fmt"
	"sort"
	"strings"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native/internal/patrun"
)

// Patrun — a mutable pattern→value dispatch table, backed by the vendored
// github.com/rjrodger/patrun trie (lang/go/native/internal/patrun). A pattern
// is a Map of Scalar match-values; `find` returns the value of the MOST
// SPECIFIC pattern that subset-matches a subject — more matched keys win,
// unknown subject keys are ignored — in O(query-key-depth), independent of
// table size.
//
//	def routes (patrun)
//	add {a:1}     "A" routes
//	add {a:1 b:1} "B" routes
//	find {a:1}        routes      # → 'A'
//	find {a:1 b:1}    routes      # → 'B'   (specificity wins)
//	find {a:1 z:9}    routes      # → 'A'   (unknown key z ignored)
//	find {x:9}        routes      # → None
//
// patrun matches on map[string]string. boru match-values are SCALARS compared
// by string coercion (ValToString): 1 and "1" share a rule; 1.0 ("1.0") and
// true ("true") key on their own text. A non-scalar pattern value is a loud
// error. The stored value is any boru value — typically a function, making a
// Patrun a dispatch/router table — so it rides a side table keyed by the
// pattern's canonical signature (patrun's *string data is that signature).

// patrunRule is the boru side of one registered rule: the original pattern Map
// (for `patterns`), the stored value, and a pre-rendered "k=v,…" for Format.
type patrunRule struct {
	raw  Value
	val  Value
	disp string
}

// patrunMatcher wraps the vendored patrun trie plus a side table. The trie
// owns matching (it stores map[string]string patterns and a *string handle —
// the pattern's canonical signature); the side table maps that signature to
// the boru value and raw pattern, and `order` preserves insertion order for
// `patterns`. add/remove mutate in place; a Patrun Value wraps the pointer
// (ExtensionPayload), so mutation is visible through every copy.
type patrunMatcher struct {
	pm      *patrun.Patrun
	side    map[string]patrunRule
	order   []string
	valType *core.Type // the DECLARED type of every stored value (`patrun T`)
}

func newPatrunMatcher(valType *core.Type) *patrunMatcher {
	return &patrunMatcher{pm: patrun.New(), side: map[string]patrunRule{}, valType: valType}
}

// NewPatrun builds a fresh empty Patrun whose stored values are declared to be
// of type valType.
func NewPatrun(valType *core.Type) Value {
	return core.NewExtension(TPatrun, newPatrunMatcher(valType))
}

func asPatrun(v Value) (*patrunMatcher, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	m, ok := ep.Body.(*patrunMatcher)
	return m, ok
}

// patrunNatives installs the Patrun words. `patrun` / `find` / `patterns` are
// new; `add` and `remove` carry a Patrun overload that FOLDS into the existing
// core words (upsertFnDef appends), disambiguated by the Patrun receiver type.
var patrunNatives = []NativeFunc{
	{
		Name: "patrun",
		Signatures: []Signature{
			// patrun T — a dispatch table whose every stored/matched value must
			// be a T (`patrun String`, `patrun Function`). `find` surfaces the
			// DECLARED T (∪ None on a miss), never a poisoned dynamic(Any).
			{Args: []*Type{TAny}, Impl: Go(patrunNewHandler), Returns: []*Type{TPatrun},
				ReturnsFn: patrunNewReturns, BarrierPos: -1},
		},
	},
	{
		Name: "add",
		Signatures: []Signature{
			// add PATTERN VALUE PATRUN — register a rule (in place). The VALUE
			// may be a fn (the dispatch-table pattern: `add {cmd:"sum"} (=>…)
			// pm`), which the handler STASHES in the patrun and never invokes on
			// the VM tape — so a PURE fn literal rides as an inert const
			// (CompileStoresFn; a capturing fn declines at isInertConst and falls
			// back, keeping its real binding).
			{Args: []*Type{TMap, TAny, TPatrun}, Impl: Go(patrunAddHandler), Returns: []*Type{},
				ReturnsFn: patrunAddReturns, BarrierPos: -1, CompileEffect: CompileStoresFn},
		},
	},
	{
		Name: "find",
		Signatures: []Signature{
			// find SUBJECT PATRUN {opts} — opts: {exact:Boolean}.
			{Args: []*Type{TMap, TPatrun, TMap}, Impl: Go(patrunFindHandler), Returns: []*Type{TAny},
				ReturnsFn: patrunFindReturns, BarrierPos: -1},
			// find SUBJECT PATRUN
			{Args: []*Type{TMap, TPatrun}, Impl: Go(patrunFindHandler), Returns: []*Type{TAny},
				ReturnsFn: patrunFindReturns, BarrierPos: -1},
		},
	},
	{
		Name: "remove",
		Signatures: []Signature{
			// remove PATTERN PATRUN — delete a rule by its pattern (in place).
			{Args: []*Type{TMap, TPatrun}, Impl: Go(patrunRemoveHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "patterns",
		Signatures: []Signature{
			// patterns PATRUN — the registered rules as [{pattern value} …].
			{Args: []*Type{TPatrun}, Impl: Go(patrunPatternsHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
}

func patrunNewHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewPatrun(patrunValType(args))}, nil
}

// patrunValType extracts the declared value type from `patrun T`'s type-literal
// argument, defaulting to TAny defensively (the no-signature recovery may call
// with a short window).
func patrunValType(args []Value) *core.Type {
	if len(args) >= 1 {
		if t := core.ValueType(args[0]); t != nil {
			return t
		}
	}
	return TAny
}

// ---- check-mode shape twins (design/legacy/checker-precision-fronts.0.ignore §2) ----
//
// A Patrun is a store-class container: a mutable dispatch table whose
// stored values are unreachable to the checker — `find` declared the
// honest dynamic(Any) hatch. Per-creation-site shaping recovers a BOUND
// without touching that epistemics: `patrun` mints an abstract
// StoreShapeInfo in check mode, `add` joins the WRITTEN VALUE's type
// into its unkeyed join (patterns are map-shaped, not string keys — the
// join is over values, the exact analogue of a typed list's element
// join), and `find` surfaces dynamic(join ∪ None) — a gradual bound,
// never a proof, since WHICH rule matches is runtime dispatch. A
// dispatch-bearing stored value (the lambda-router pattern, whose
// reader re-dispatches the found fn) POISONS the join and find keeps
// today's dynamic(Any) — the pinned patrun.tsv:40 residual is
// deliberately unchanged. Compile passes keep the legacy carriers
// (patrun rows compile natively today; byte-identical lowering).

func patrunNewReturns(args []Value, r *Registry) []Value {
	if r == nil || r.Check.Compiling {
		return []Value{NewCarrier(TPatrun)}
	}
	c := core.NewStoreShapeCarrier(TPatrun, 0)
	if ss, ok := check.StoreShapeOf(c); ok {
		ss.DeclaredVal = patrunValType(args) // the declared value type rides the shape
	}
	return []Value{c}
}

func patrunAddReturns(args []Value, r *Registry) []Value {
	// The stored value's type is DECLARED at construction, so `add` no longer
	// INFERS it — but a CONCRETE value the checker can prove is not the declared
	// type is a static error (the mirror of patrunAddHandler's runtime guard),
	// so the checker flags `add {a:1} 5 (patrun String)` rather than deferring
	// the whole class to runtime. Abstract / carrier values can't be proven and
	// stay a runtime check. Plain-check only — the diagnostic never bakes.
	if r != nil && !r.Check.Compiling && len(args) >= 3 {
		if ss, ok := check.StoreShapeOf(args[2]); ok && ss.DeclaredVal != nil &&
			!ss.DeclaredVal.Equal(TAny) && IsConcrete(args[1]) && !args[1].Is(ss.DeclaredVal) {
			// A proven-concrete wrong-typed value is a GUARANTEED runtime
			// error mirror of patrunAddHandler's guard — stamped and deduped
			// through the helper (NUR058). Plain-check-only today (the
			// !Compiling gate above), so the stamp is classification, not a
			// compile-behavior change.
			core.CheckAddUniqueDiagnostic(r, "patrun_error",
				fmt.Sprintf("add: value must be a %s, got %s", ss.DeclaredVal.Leaf(), args[1].Parent.String()),
				"add", args[1].Pos())
		}
	}
	return nil
}

func patrunFindReturns(args []Value, r *Registry) []Value {
	if r != nil && !r.Check.Compiling && len(args) >= 2 {
		if ss, ok := check.StoreShapeOf(args[1]); ok && ss.DeclaredVal != nil {
			// A typed table: surface the DECLARED element type ∪ None (an
			// unmatched subject reads None, patrunFindHandler's miss path). No
			// poisoning — the type is a declaration, not an inference.
			bound := core.JoinCarriers(NewCarrier(ss.DeclaredVal), NewCarrier(core.TNone))
			return []Value{core.NewDynamicCarrierValue(bound)}
		}
	}
	return []Value{NewDynamicCarrier(TAny)} // unshaped fallback (recovery / no shape)
}

func patrunAddHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[2])
	if !ok {
		return nil, r.BoruError("patrun_error", "add: expected a Patrun, got "+args[2].Parent.String(), "add")
	}
	if m.valType != nil && !m.valType.Equal(TAny) && !args[1].Is(m.valType) {
		return nil, r.BoruErrorHint("patrun_error",
			fmt.Sprintf("add: value must be a %s, got %s", m.valType.Leaf(), args[1].Parent.String()),
			"add", "this Patrun was declared `patrun "+m.valType.Leaf()+"` — its stored values must be that type")
	}
	pat, keys, sig, err := coercePattern(args[0], "add", r)
	if err != nil {
		return nil, err
	}
	if _, exists := m.side[sig]; !exists {
		m.order = append(m.order, sig)
	}
	m.side[sig] = patrunRule{raw: args[0], val: args[1], disp: patrunDisp(keys, pat)}
	h := sig
	m.pm.Add(pat, &h)
	return nil, nil
}

func patrunFindHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[1])
	if !ok {
		return nil, r.BoruError("patrun_error", "find: expected a Patrun, got "+args[1].Parent.String(), "find")
	}
	exact := false
	if len(args) == 3 {
		b, err := patrunOptBool(args[2], "exact")
		if err != nil {
			return nil, r.BoruError("patrun_error", "find: "+err.Error(), "find")
		}
		exact = b
	}
	subj, err := coerceSubject(args[0], "find")
	if err != nil {
		return nil, err
	}
	var h *string
	var found bool
	if exact {
		h, found = m.pm.FindExact(subj)
	} else {
		h, found = m.pm.Find(subj)
	}
	if !found || h == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	rule, ok := m.side[*h]
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{rule.val}, nil
}

func patrunRemoveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[1])
	if !ok {
		return nil, r.BoruError("patrun_error", "remove: expected a Patrun, got "+args[1].Parent.String(), "remove")
	}
	pat, _, sig, err := coercePattern(args[0], "remove", r)
	if err != nil {
		return nil, err
	}
	if _, exists := m.side[sig]; exists {
		delete(m.side, sig)
		for i, s := range m.order {
			if s == sig {
				m.order = append(m.order[:i], m.order[i+1:]...)
				break
			}
		}
		m.pm.Remove(pat)
	}
	return nil, nil
}

func patrunPatternsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, ok := asPatrun(args[0])
	if !ok {
		return nil, r.BoruError("patrun_error", "patterns: expected a Patrun, got "+args[0].Parent.String(), "patterns")
	}
	out := make([]Value, 0, len(m.order))
	for _, sig := range m.order {
		rule := m.side[sig]
		row := NewOrderedMap()
		row.Set("pattern", rule.raw)
		row.Set("value", rule.val)
		out = append(out, NewMap(row))
	}
	return []Value{NewList(out)}, nil
}

// ---- coercion ----

// coercePattern coerces a pattern Map to patrun's map[string]string plus the
// sorted keys and a canonical signature (the trie's *string handle). Every
// match-value must be a concrete Scalar; anything else is a loud error (unlike
// patrun-JS, which silently stringifies objects).
func coercePattern(pv Value, op string, r *Registry) (map[string]string, []string, string, error) {
	mp, err := RequireConcreteMap(pv, op)
	if err != nil {
		return nil, nil, "", err
	}
	mkeys := mp.Keys()
	pat := make(map[string]string, len(mkeys))
	for _, k := range mkeys {
		val, _ := mp.Get(k)
		if !IsConcrete(val) || !val.Parent.ConformsTo(TScalar) {
			return nil, nil, "", r.BoruErrorHint("patrun_error",
				fmt.Sprintf("%s: pattern value for %q must be a Scalar, got %s", op, k, val.Parent.String()),
				op, "patterns match on scalar values (Integer/Float/String/Boolean/Atom)")
		}
		pat[k] = ValToString(val)
	}
	keys := append([]string(nil), mkeys...)
	sort.Strings(keys)
	return pat, keys, patrunSig(keys, pat), nil
}

// coerceSubject coerces a subject Map to patrun's map[string]string, keeping
// only its scalar values (a non-scalar subject value is not matchable, so it
// is skipped rather than erroring — only patterns are strict).
func coerceSubject(sv Value, op string) (map[string]string, error) {
	mp, err := RequireConcreteMap(sv, op)
	if err != nil {
		return nil, err
	}
	subj := make(map[string]string, len(mp.Keys()))
	for _, k := range mp.Keys() {
		val, _ := mp.Get(k)
		if IsConcrete(val) && val.Parent.ConformsTo(TScalar) {
			subj[k] = ValToString(val)
		}
	}
	return subj, nil
}

// patrunSig is the canonical, collision-free signature of a coerced pattern:
// sorted key\x00value\x00 pairs. It is the trie's *string handle and the side
// table key, so distinct coerced patterns map to distinct rules and identical
// ones overwrite.
func patrunSig(keys []string, pat map[string]string) string {
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(pat[k])
		b.WriteByte(0)
	}
	return b.String()
}

// patrunDisp renders a coerced pattern as "k=v,k=v" for Format.
func patrunDisp(keys []string, pat map[string]string) string {
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(pat[k])
	}
	return b.String()
}

func patrunOptBool(opts Value, key string) (bool, error) {
	m, _ := AsMap(opts)
	if m == nil {
		return false, nil
	}
	v, ok := m.Get(key)
	if !ok {
		return false, nil
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return false, fmt.Errorf("opts.%s: %w", key, err)
	}
	return b, nil
}

// ---- behavior ----

// FormatPatrun renders the matcher as "Patrun(k=v,… -> value; …)" in
// insertion order — the delegation seam basic.PatrunBehavior formats
// through (the type identity and Behavior shell moved to basic/go with
// the Ideal/Patrun registration; the matcher stays here with the words).
func (m *patrunMatcher) FormatPatrun() string {
	if len(m.order) == 0 {
		return "Patrun()"
	}
	var b strings.Builder
	b.WriteString("Patrun(")
	for i, sig := range m.order {
		if i > 0 {
			b.WriteString("; ")
		}
		rule := m.side[sig]
		b.WriteString(rule.disp)
		b.WriteString(" -> ")
		b.WriteString(ValToString(rule.val))
	}
	b.WriteByte(')')
	return b.String()
}
