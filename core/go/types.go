package core

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Well-known types. Each is a pointer into the package-level Builtin
// TypeTable. Type identity is pointer equality; comparing T*-constants
// against each other or against runtime-constructed types just
// compares the Type pointers.
var (
	TAny            = mustType("Any")
	TNone           = mustType("None")
	TNever          = mustType("Never")
	TAbsent         = mustType("Absent")
	TScalar         = mustType("Scalar")
	TString         = mustType("Scalar/String")
	TStringProper   = mustType("Scalar/String/ProperString")
	TStringEmpty    = mustType("Scalar/String/EmptyString")
	TNumber         = mustType("Scalar/Number")
	TInteger        = mustType("Scalar/Number/Integer")
	TFloat          = mustType("Scalar/Number/Float")
	TBigInteger     = mustType("Scalar/Number/BigInteger")
	TBigDecimal     = mustType("Scalar/Number/BigDecimal")
	TBoolean        = mustType("Scalar/Boolean")
	TMicron         = mustType("Scalar/Micron")
	TPathon         = mustType("Scalar/Micron/Pathon")
	TEmailon        = mustType("Scalar/Micron/Emailon")
	TUrlon          = mustType("Scalar/Micron/Urlon")
	TIpon           = mustType("Scalar/Micron/Ipon")
	THoston         = mustType("Scalar/Micron/Hoston")
	TSemveron       = mustType("Scalar/Micron/Semveron")
	TCidron         = mustType("Scalar/Micron/Cidron")
	TMacon          = mustType("Scalar/Micron/Macon")
	TColoron        = mustType("Scalar/Micron/Coloron")
	TMimon          = mustType("Scalar/Micron/Mimon")
	TQion           = mustType("Scalar/Micron/Qion")
	TPhonon         = mustType("Scalar/Micron/Phonon")
	TNode           = mustType("Node")
	TIdeal          = mustType("Ideal")
	TList           = mustType("Node/List")
	TListArgs       = mustType("Node/List/Args")
	TFlexList       = mustType("Node/List/FlexList")
	TWeakFlexList   = mustType("Node/List/FlexList/WeakFlexList")
	TMap            = mustType("Node/Map")
	TFlexMap        = mustType("Node/Map/FlexMap")
	TWeakFlexMap    = mustType("Node/Map/FlexMap/WeakFlexMap")
	TXml            = mustType("Node/Xml")
	TFlexXml        = mustType("Node/Xml/FlexXml")
	TWeakFlexXml    = mustType("Node/Xml/FlexXml/WeakFlexXml")
	TOptions        = mustType("Ideal/Options")
	TTable          = mustType("Ideal/Table")
	TRecord         = mustType("Ideal/Record")
	TAtom           = mustType("Scalar/Atom")
	TWord           = mustType("Word")
	TFunction       = mustType("Type/Function")
	TForward        = mustType("Word/__FW")
	TOpenParen      = mustType("Word/__OP")
	TCloseParen     = mustType("Word/__CP")
	TEnd            = mustType("Word/__ED")
	TParenExpr      = mustType("Word/__PE")
	TInterpString   = mustType("Word/__IS")
	TXmlInterp      = mustType("Word/__XI")
	TFnUndef        = mustType("Type/FunctionSignature")
	TReturnCheck    = mustType("Word/__RC")
	TDefCleanup     = mustType("Word/__IN/__DC")
	TDisjunct       = mustType("Type/Disjunct")
	TEnum           = mustType("Type/Disjunct/Enum")
	TNegation       = mustType("Type/Negation")
	TMark           = mustType("Word/__MK")
	TMove           = mustType("Word/__MV")
	TSplice         = mustType("Word/__SP")
	TDispatchMod    = mustType("Word/__DM")
	TSugar          = mustType("Word/__SG")
	TInternal       = mustType("Word/__IN")
	TInspect        = mustType("Node/Map/Inspect")
	TClass          = mustType("Ideal/Class")
	TSurface        = mustType("Ideal/Surface")
	TSelf           = mustType("Type/Self")
	TTypeParam      = mustType("Type/TypeParam")
	TGenSpec        = mustType("Type/GenSpec")
	TGenParam       = mustType("Type/GenParam")
	TStore          = mustType("Ideal/Store")
	TStoreSystem    = mustType("Ideal/Store/System")
	TReach          = mustType("Ideal/Reach")
	TResource       = mustType("Ideal/Resource")
	TResourceEntity = mustType("Ideal/Resource/Entity")
	// TFetchFunction / TFetchRequest / TFetchResponse moved to
	// lang/go/native/fetch.go (Step 8 migration); registered via
	// RegisterType at lang/go/native package init.
	TError = mustType("Ideal/Error")
	TType  = mustType("Type")
	// Scalar/Time and descendants moved to
	// lang/go/engine/native_temporal.go (Step 8). They live in
	// lang/go/engine (not lang/go/internal/nativemod/time) because
	// date-arithmetic handlers in lang/go/engine/native_math.go
	// reference these types at package-init time — colocating
	// avoids the import-cycle constraint.
	// TMatrix moved to lang/go/internal/nativemod/matrix.go (Step 8).
	// TTimeout moved to lang/go/engine/native_misc.go (Step 8).
	// TInterval moved to lang/go/engine/native_misc.go (Step 8).
)

// typeNames is the user-facing name → Type map for bare-word type
// resolution. Built at init from the Builtin TypeTable; refreshed
// whenever a new externally-registered builtin is added via
// RegisterType. The parser and engine consult this map
// to resolve bare type-name words (e.g. `Integer`, `Date`, plugin-
// supplied names) to their *Type.
var TypeNames = buildTypeNames()

func buildTypeNames() map[string]*Type {
	m := make(map[string]*Type, len(Builtin.byName))
	for name, def := range Builtin.byName {
		if def == nil {
			continue
		}
		m[name] = def
	}
	return m
}

// refreshTypeNames rebuilds the typeNames snapshot. Called by
// RegisterType so freshly-installed types are immediately
// resolvable by bare-name lookup in the parser.
func refreshTypeNames() {
	TypeNames = buildTypeNames()
}

// TypeNameTable returns the canonical mapping of all well-known type
// names to their Type. Used by both the parser and engine.
func TypeNameTable() map[string]*Type {
	return TypeNames
}

// TypeNameByID returns the canonical user-facing name for a Type ID
// (e.g. the ID for "Type/FunctionSignature" → "FunctionSignature").
// Returns the empty string if no entry exists or the type is internal.
func TypeNameByID(id string) string {
	def := Builtin.LookupByID(id)
	if def == nil || IsInternal(def) {
		return ""
	}
	// Only return a name for types that participate in user-facing
	// name resolution (present in byName).
	if _, ok := Builtin.byName[def.Name()]; !ok {
		return ""
	}
	return def.Name()
}

// mustType resolves a well-known builtin type constant at init time. An
// invalid path can only mean a programmer error in the T* constant list
// or builtinDecls; rather than panic, it records the error (surfaced by
// NewRegistry via BuiltinInitError) and returns a degenerate placeholder
// so other package-level var initialisers stay non-nil. The name is kept
// for source familiarity; it no longer panics. See ADR-005.
func mustType(path string) *Type {
	return mustBuiltinType(path)
}

// NewType resolves a slash-separated path to a builtin *Type, e.g.
// "String/ProperString". Short names are auto-expanded to their full
// hierarchy path: "String/ProperString" becomes
// "Scalar/String/ProperString", etc. Every alphabetic part must begin
// with an uppercase letter; lowercase is an error.
//
// NewType resolves *builtin* paths only. User-defined types must be
// minted via TypeTable.MintType — an otherwise-unknown path is a hard
// error. Test scaffolding that needs to synthesise arbitrary type
// hierarchies should use MintTestType, which mints intermediate
// *Types under the builtin root as needed.
func NewType(path string) (*Type, error) {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		r, _ := utf8.DecodeRuneInString(p)
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return nil, fmt.Errorf("boru: type part %q in %q must start with an uppercase letter", p, path)
		}
	}

	// Auto-expand short names to full hierarchy paths via the Builtin
	// table. If the first part is already a root, no expansion needed.
	if _, isRoot := Builtin.bypath[parts[0]]; !isRoot {
		if fullPrefix, ok := Builtin.ExpandShortName(parts[0]); ok {
			expanded := fullPrefix
			if len(parts) > 1 {
				expanded += "/" + strings.Join(parts[1:], "/")
			}
			parts = strings.Split(expanded, "/")
		}
	}

	fullPath := strings.Join(parts, "/")
	if def := Builtin.bypath[fullPath]; def != nil {
		return def, nil
	}
	return nil, fmt.Errorf("boru: unknown type %q", fullPath)
}

// ResolveTypePath attempts to resolve a slash-separated path to a
// known builtin Type. Returns the Type and true only if the path
// matches a registered Builtin entry — transient Type values minted
// by NewType for unknown paths do NOT count. Single-part names are
// resolved via the parser's TypeNameTable bare-word lookup instead.
func ResolveTypePath(name string) (*Type, bool) {
	if !strings.Contains(name, "/") {
		return nil, false
	}
	t, err := NewType(name)
	if err != nil {
		return nil, false
	}
	if t != nil && Builtin.bypath[t.Path()] == t {
		return t, true
	}
	return nil, false
}

// ConformsTo reports whether t is a subtype of — conforms to — pattern:
// every value of type t is also a value of type pattern. The relation
// is directional (a partial order, NOT symmetric): a child conforms to
// its parent; a parent does NOT conform to its child.
//   - everything conforms to "Any" (and to a nil pattern, vacuously).
//   - Scalar/String/ProperString conforms to Scalar/String.
//   - Scalar/String does NOT conform to Scalar/String/ProperString.
//
// (Formerly named Matches; renamed because "matches" read as a symmetric
// predicate when this is the asymmetric conformance order. The strict /
// irreflexive form — t below pattern, not equal — is IsSubtypeOf.)
//
// DepScalar values carry their base scalar as Parent (e.g. an
// `Integer gte 0` value has Parent=Integer), so they conform to any
// scalar-typed slot via the normal ancestor walk — no special-case
// override here. Per-value satisfaction (does 5 lie in [10, ∞)?) is
// handled at Unify time; this method answers the type-level question
// only.
func (t *Type) ConformsTo(pattern *Type) bool {
	// Empty pattern (nil Type) is a vacuous match — preserves the
	// old PathSubtype semantics where a zero-iteration prefix loop
	// returned true. Dispatcher / coercion code relies on this.
	if pattern == nil {
		return true
	}
	if pattern.Equal(TAny) {
		return true
	}
	if t == nil {
		return false
	}
	return t.IsAncestor(pattern)
}

// PathSubtype reports whether t is a strict path-prefix subtype of
// pattern. With ID-based identity, this is equivalent to the ancestry
// walk used by ConformsTo — no `Any` bolt-on.
//
// `t.PathSubtype(t)` is always true (identity is a subtype of itself).
func (t *Type) PathSubtype(pattern *Type) bool {
	if t == nil || pattern == nil {
		return false
	}
	return t.IsAncestor(pattern)
}

// Specificity returns the depth of the type. More ancestry = more
// specific. Any is the universal lattice top — skipped here for the
// same reason it's skipped in Path() and PathOf: structural roots
// chain to Any but their declared "depth" (Scalar=1, Integer=3, …)
// is the historical short measure that sig dispatch was tuned
// against. Any itself reports Specificity=1 (it IS the root, not
// an ancestor) so the score for Any-only signatures is unchanged.
func (t *Type) Specificity() int {
	n := 0
	for d := t; d != nil; d = ParentOf(d) {
		if d.FixedID() == anyFixedID && n > 0 {
			break
		}
		n++
	}
	return n
}

// Leaf returns the last part of the type path.
// For example, "Object/Fetch/Request" returns "Request".
func (t *Type) Leaf() string {
	if t == nil {
		return ""
	}
	return t.Name()
}

// IsSubtypeOf reports whether t is a strict subtype of parent.
// For example: Scalar/String/ProperString is a subtype of Scalar/String.
// A type is NOT a subtype of itself.
func (t *Type) IsSubtypeOf(parent *Type) bool {
	if t == nil || parent == nil || t.Equal(parent) {
		return false
	}
	return t.IsAncestor(parent)
}

// Equal reports whether t and other are the same lattice identity.
// Identity is the canonical ID string. A type literal is a by-value
// copy of its lattice node — same ID, different address — so a raw
// pointer compare would miss copies. Pointer equality is kept as the
// fast path and as the fallback for ID-less types (raw &Type{} test
// fixtures, where the empty ID makes ID comparison meaningless).
//
// The shared *typeMeta is a second fast path: every copy of one
// lattice node shares its tmeta pointer (typeMeta is allocated once at
// registration and never re-minted for the same node — TypeTable.Clone
// copies the maps, not the nodes), so a pointer compare settles the
// overwhelmingly common same-node case without touching the 14-char ID
// strings — Equal runs on every ConformsTo step of every dispatch
// (~7% of interpreter CPU was this string compare). tmeta inequality
// is NOT node inequality (a node rebuilt around the same ID keeps
// equality via the ID fallback below).
func (t *Type) Equal(other *Type) bool {
	if t == other {
		return true
	}
	if t == nil || other == nil {
		return false
	}
	if tm, om := t.tmeta, other.tmeta; tm != nil && om != nil {
		if tm == om {
			return true
		}
		// Interval-label fast path. In (the DFS pre-order number stamped by
		// labelIntervals) is assigned once, on the package-level Builtin
		// table, as a single `counter++` per node — so among labelled nodes
		// it is a unique per-node identity and In-equality is exactly
		// node-equality. Minted / refined / external / ad-hoc nodes (and a
		// node rebuilt around an existing ID) are never labelled and carry
		// In == 0, so they fall through to the ID compare below with
		// behaviour unchanged. Reaching here therefore means: whenever both
		// nodes are labelled, an int compare settles identity without the
		// 14-char ID string compare — the hot path for the per-token marker
		// predicates (IsWord / IsOpenParen / IsForward / …), which test a
		// token's builtin Parent against a builtin marker on every step and
		// almost always mismatch.
		if tm.In > 0 && om.In > 0 {
			return tm.In == om.In
		}
	}
	return IDOf(t) != "" && IDOf(t) == IDOf(other)
}

// ValidateTypeNameParts checks that a type name (slash-separated) does not
// reuse any part that is already known per the isKnown callback. Returns
// an error on the first conflict.
func ValidateTypeNameParts(name string, isKnown func(string) bool) error {
	parts := strings.Split(name, "/")
	for _, p := range parts {
		if isKnown(p) {
			return fmt.Errorf("type: name part %q in %q conflicts with an existing type name", p, name)
		}
	}
	return nil
}

// IsCapitalisedName reports whether name starts with an ASCII upper-case
// letter. The naming rule is: type names start with a capital, def names
// don't. Empty names are not capitalised.
func IsCapitalisedName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}
