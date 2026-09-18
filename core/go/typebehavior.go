package core

// TypeBehavior is the per-type operation bundle that the kernel
// consults when it needs to act on a value of a given type. Each
// *Type carries a Behavior; the well-known dispatch points
// (`v.Is(t)`, `Value.String`, `ValuesEqual`) route through it
// rather than switching on a closed enumeration of type identities.
//
// A nil Behavior on *Type means "use defaultBehavior" — the
// TypeTable installs the default on every type that doesn't supply
// its own (see TypeTable.MintType, registerBuiltin). Types provide
// a custom Behavior only when their semantics demand it: predicate
// types, dependent scalars, structural records, domain payloads
// like Date or Matrix, etc.
//
// The three required operations mirror the kernel's three fundamental
// type-system questions:
//
//   - Match: does value v satisfy this type? Used by `v.Is(t)`, the
//     signature matcher, `is` / `guard` / `typeof`, and the unifier.
//   - Format: how is v rendered as a string? Used by Value.String,
//     error messages, the canon writer, and the spec runner.
//   - Equal: are two values of this type semantically equal? Used
//     by ValuesEqual and the `eq` / `neq` words.
//
// Adding a new operation to TypeBehavior is a breaking change for
// every implementor. Optional capability sub-interfaces (Comparer,
// Hasher, Walker, Sizer — see below) let a type opt into extra
// operations without expanding the required surface.
// ContentMembership marks a TypeBehavior whose Match admits values by
// CONTENT — a predicate, member function, union, negation, schema, or
// type-shape test — rather than by construction tag. A content-based
// type cannot ANCHOR a word-extension signature
// (design/OPEN-WORDS.1.md §3.1): its members can exist before the type
// does, so an anchored signature could capture pre-existing calls and
// break the reachability theorem. Every new content-deciding Behavior
// MUST add the marker; TestContentMembershipInventory pins the set.
type ContentMembership interface{ ContentMembership() }

// MatchDelegating is implemented by a wrapper Behavior whose Match
// DELEGATES to an inner behavior rather than defining its own membership
// rule — `behave`'s userBehavior is the one such wrapper (it layers
// compare/canon/nodify over a type without touching Match). IsNominalAnchor
// unwraps it (behaviorIsContent) so a CONTENT behavior hidden beneath a
// Match-delegating wrapper is still found: without this, `behave`-ing a
// predicate/surface and then anchoring a core-word extension on it would
// slip past the marker check and break the reachability theorem (the same
// hole as an unmarked content Behavior, reached through `behave`).
//
// Match-OVERRIDING wrappers (bareRefineUnifier and the membership unifiers,
// which embed behaviorWrapper but define their own nominal/content Match)
// deliberately do NOT implement this: their own Match decides membership,
// so a nominal refine-of-a-predicate stays a valid anchor even though its
// wrapped parent is content-based.
type MatchDelegating interface{ DelegatesMatchTo() TypeBehavior }

// behaviorIsContent reports whether b decides membership by content —
// either it carries the ContentMembership marker directly, or it is a
// Match-delegating wrapper (MatchDelegating) over a behavior that does.
// The single source of truth for IsNominalAnchor.
func behaviorIsContent(b TypeBehavior) bool {
	for b != nil {
		if _, ok := b.(ContentMembership); ok {
			return true
		}
		d, ok := b.(MatchDelegating)
		if !ok {
			return false
		}
		b = d.DelegatesMatchTo()
	}
	return false
}

type TypeBehavior interface {
	// Match reports whether v conforms to the type t. The
	// canonical default is a lattice walk (v.Parent is t or a
	// descendant). Predicate types override to invoke the
	// predicate body; refinement types override to check the
	// refinement clause; record types override to do field-by-
	// field conformance.
	Match(v Value, t *Type) bool

	// Format renders v as a string. The canonical default
	// delegates to Value.String (which uses the kernel's
	// existing switch). Domain types override to produce a
	// type-specific rendering (e.g. CalendarDuration → "P1Y2M3D").
	Format(v Value) string

	// Equal reports semantic equality. The canonical default is
	// the existing ValuesEqual deep-compare. Types with
	// normalisation semantics (CalendarDuration, DepScalar) override
	// to do their type-specific compare.
	Equal(a, b Value) bool
}

// Comparer is an optional capability interface. Types implementing
// it expose an ordering relation; the `sort` / `min` / `max` /
// `lt` / `gt` words consult this when ordering values of that
// type. Types lacking Comparer produce a clear "type does not
// support compare" error rather than a silent miscompile.
//
// Conventions match cmp.Compare: negative if a < b, zero if a == b,
// positive if a > b. The error return surfaces failures from
// user-defined comparator bodies (the `cmp` word installs a
// Comparer that may return errors propagated from the body); kernel
// and native Comparers return a nil error.
type Comparer interface {
	Compare(a, b Value) (int, error)
}

// Hasher is an optional capability interface. Types implementing it
// produce a stable hash for use in sets and maps keyed by Value.
type Hasher interface {
	Hash(v Value) uint64
}

// Walker is an optional capability interface. Types implementing it
// expose a traversal over contained Values (lists walk elements,
// maps walk entries, structural types walk fields).
type Walker interface {
	Walk(v Value, visit func(Value))
}

// Sizer is an optional capability interface. Types implementing it
// report a natural size — the length of a dominant collection (a
// List's elements, a Map's keys, a Pathon's segments, an Object's
// fields), a number's floored magnitude, a string's length. SizeOf
// consults it; a type with no Sizer in its lattice sizes to 0.
type Sizer interface {
	Size(v Value) int
}

// ConstBakeable is an optional capability interface for extension-backed
// types (ExtensionPayload — the payload the kernel does not inspect).
// Implementing it declares the type's concrete values IMMUTABLE: no word
// mutates the payload in place, and clone/fork/send share the backing
// storage zero-copy — the same sharing a pooled const performs. The
// compiler's const gate (isInertConst) consults it, walking the parent
// chain like every capability dispatch, when deciding whether such a
// value may bake into a compiled program's const pool; pooled consts are
// shared across calls and loop iterations, which is sound exactly when
// the payload can never be written through. Types with any in-place
// mutation path (sockets, listeners, timers, module instances, flex
// containers) must NOT implement it — the extension default is refusal.
type ConstBakeable interface {
	// BakeableConst reports whether this specific value may bake. Most
	// implementors return true unconditionally; the per-value hook exists
	// for families whose kinds differ in mutability.
	BakeableConst(v Value) bool
}

// Unifier is an optional capability interface. Types implementing it
// supply their own structural intersection rule, called by Unify when
// the lowest common ancestor of the two operands' types reaches a
// Unifier in the lattice walk.
//
// Use cases — kernel auto-installs Unifiers for predicate types and
// refine-with-clause types so the predicate body runs at every Unify
// callsite (closing the soundness gap where narrowing into a refined
// type bypassed the constraint check). External plugin types can
// install a Unifier directly via RegisterType; user code
// can install one via `behave unify/q (fn …)`.
//
// Symmetry is required: Unify(a, b) must equal Unify(b, a) up to
// value identity. The kernel does not enforce this — implementations
// must preserve it.
//
// Implementations return ErrNoUnifier when they hold a placeholder
// slot (the wrapped-Behavior case where compare/canon are installed
// but unify is not). The kernel walks past such Behaviors to find the
// next Unifier up the lattice.
//
// r is the Registry of the UnifyExplainR chain the call belongs to,
// nil when the chain started unarmed (Unify / UnifyExplain). An
// implementation that recurses into the unifier passes it on
// (unifyWithin, or the family helpers) so the nested walk stays armed;
// one that decides membership on its own ignores it.
type Unifier interface {
	Unify(a, b Value, r *Registry) (Value, *UnifyError)
}

// HasConstraintUnify reports whether t's Behavior chain carries a
// KERNEL membership-constraint Unifier — a Behavior that is both
// ContentMembership (membership decided from content) and a Unifier
// (predicate, dependent scalar, binding-body, host member). Callers
// with a generic nominal bare-node fallback (the typed-def
// refine-reparent arm) consult it to keep their hands off nodes whose
// kind enforces membership through Unify — reparenting there would bind
// without running the constraint. A user `behave unify/q` wrapper on a
// nominal refine is a Unifier but NOT ContentMembership, so nominal
// newtypes keep the reparent arm; the walk unwraps Match-delegating
// wrappers and behaviorWrapper chains so a kernel constraint hidden
// beneath a `behave` layer is still found.
func HasConstraintUnify(t *Type) bool {
	if t == nil {
		return false
	}
	for b := t.Behavior(); b != nil; {
		_, isUnifier := b.(Unifier)
		_, isContent := b.(ContentMembership)
		if isUnifier && isContent {
			return true
		}
		if d, ok := b.(MatchDelegating); ok {
			b = d.DelegatesMatchTo()
			continue
		}
		if p, ok := PrevBehavior(b); ok {
			b = p
			continue
		}
		break
	}
	return false
}

// defaultBehavior provides the canonical Match / Format / Equal
// implementations every *Type starts with. Each delegates to the
// existing kernel paths so introducing the Behavior seam is
// observably a no-op:
//
//   - Match → v.Parent.ConformsTo(t) (the historical lattice walk plus
//     DepScalar override).
//   - Format → Value.String() (today's full switch).
//   - Equal → ValuesEqual (today's deep-compare).
//
// Types with custom semantics override one or more of these by
// supplying their own Behavior; the default remains the fall-back
// for every type that doesn't.
type defaultBehavior struct{}

// DefaultBehavior is the canonical no-op TypeBehavior. Exported so
// callers writing custom Behaviors can embed or fall through to it.
var DefaultBehavior TypeBehavior = defaultBehavior{}

func (defaultBehavior) Match(v Value, t *Type) bool {
	// A bare type literal IS a lattice node — test it directly. Any
	// other value (concrete, or a carrier) is tested by its Parent.
	if IsBareTypeNode(v) {
		return v.ConformsTo(t)
	}
	return v.Parent.ConformsTo(t)
}

func (defaultBehavior) Format(v Value) string {
	// Bypass Value.String's Behavior walk: the walk skips
	// DefaultBehavior and types tagged formatDelegatesToDefault, so
	// it would always fall through here anyway. Calling
	// kernelFormatDefault directly avoids any chance of recursion
	// via embedded-defaultBehavior Format inheritance.
	return kernelFormatDefault(v)
}

func (defaultBehavior) Equal(a, b Value) bool {
	return valuesEqualDefault(a, b)
}

// behaviorWrapper is the shared embed for the kernel Behaviors that wrap
// a previous Behavior and override only Match (and sometimes Format or
// Unify), delegating Format / Equal / Compare to the wrapped Behavior.
// Every membership/constraint Behavior — predicateUnifier,
// depScalarUnifier, disjunctUnifier, negationUnifier, bareRefineUnifier,
// surfaceUnifier, schemaUnifier, genParamUnifier — embeds it so the
// delegation boilerplate lives once instead of being copied per struct.
//
// Compare delegates via baseCompare, so a wrapper over a Comparer keeps
// that capability while a wrapper over DefaultBehavior opts out
// (ErrNoComparer) and the Comparer walk continues up the lattice — the
// behaviour predicateUnifier already relied on. The embed deliberately
// does NOT supply Match (every wrapper has its own membership rule) or
// Unify (only the wrappers reached by dispatchUnifier's Unifier walk
// define one; supplying it here would make every wrapper a Unifier and
// reroute lattice dispatch).
type behaviorWrapper struct {
	prev TypeBehavior
}

func (w behaviorWrapper) Format(v Value) string           { return baseBehavior(w.prev).Format(v) }
func (w behaviorWrapper) Equal(a, b Value) bool           { return baseBehavior(w.prev).Equal(a, b) }
func (w behaviorWrapper) Compare(a, b Value) (int, error) { return baseCompare(w.prev, a, b) }

// Prev returns the behavior this wrapper was layered over (DefaultBehavior
// if none). Kernel unifiers (bareRefine, depScalar, disjunct, …) embed
// behaviorWrapper, so this exposes a walkable chain: a caller that installed
// a custom Behavior before a `def`/`refine` wrapped it can recover that
// Behavior by following Prev() down the chain. Returns the wrapped behavior,
// not DefaultBehavior, so the walk terminates (nil ends it).
func (w behaviorWrapper) Prev() TypeBehavior { return w.prev }

// PrevBehavior is the chain-walk step for Prev-exposing wrappers: it returns
// the next behavior down and whether one exists. A plain (non-wrapper)
// Behavior returns (nil, false), ending the walk.
func PrevBehavior(b TypeBehavior) (TypeBehavior, bool) {
	if w, ok := b.(interface{ Prev() TypeBehavior }); ok {
		return w.Prev(), true
	}
	return nil, false
}
