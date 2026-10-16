package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// streamSentinels maps each admitted stream atom to the internal routing
// path doRead/doWrite use to reach the host stream. The angle-bracket
// strings are not user-visible — they are an implementation detail
// behind the Stream atoms. Stream membership is decided by NAME (these
// keys), so read/write routing is independent of which StreamKind node
// tags a handle.
var streamSentinels = map[string]string{
	"stdin":  pathStdin,
	"stdout": pathStdout,
	"stderr": pathStderr,
}

// isStreamAtom is the StreamKind membership rule: a concrete atom whose
// name is one of stdin/stdout/stderr. It is the single predicate that
// defines the type — the kernel's MemberBehavior derives Match, Unify,
// canon rendering and `is`/dispatch agreement from it (see
// eng.MintMemberType), so there is no hand-written TypeBehavior here.
// Name-based, so a handle minted by one import is accepted by another's
// StreamKind too.
func isStreamAtom(v Value) bool {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return false
	}
	_, ok := streamSentinels[name]
	return ok
}

// MintStreamKind mints the StreamKind type into r's type table and
// returns the node. StreamKind is owned by the boru:io module —
// BuildIOModule mints one per import into the module's sub-registry (see
// modules/io.go) — and is deliberately NOT a global builtin: it has no
// FixedID, is absent from the builtin name index and the FixedID
// snapshot, and is reachable from boru only through the module export
// `IO.StreamKind`. The returned node tags the stdin/stdout/stderr
// handles and backs that module instance's read/write Stream signatures.
//
// It is an Atom subtype whose inhabitants are EXACTLY the three atoms
// stdin/stdout/stderr — a closed enumeration the read/write signatures
// use to tell a stream target apart from a file path.
func MintStreamKind(r *Registry) *Type {
	return r.Types.MintMemberType("StreamKind", core.TAtom, isStreamAtom)
}

// NewStreamKind mints a standalone StreamKind (into its own dynamic type
// table) for test helpers that register the io words under bare names
// without a host registry to mint into. Production code mints per import
// via MintStreamKind into the module sub-registry. Membership is
// name-based, so a standalone StreamKind tags and admits the handles
// exactly like a per-import one.
func NewStreamKind() *Type {
	return core.NewDynamicTypeTable().MintMemberType("StreamKind", core.TAtom, isStreamAtom)
}

// newStreamAtom returns the Atom handle for a standard stream, tagged
// with the given StreamKind type so its static and runtime types agree
// (the read/write Stream signatures, `typeof`, and the soundness checker
// all see StreamKind). The payload is a plain AtomPayload, so every
// atom-aware path (rendering, comparison, `quote`) treats it as the
// symbol it is.
func newStreamAtom(name string, streamKind *Type) Value {
	atom := core.NewAtom(name)
	return core.MintRetagged(&atom, streamKind)
}

// streamSentinel maps a stream-handle value to its internal routing
// path, reporting whether v is a stream handle at all. Used by
// extractPath / returnPath to route the Stream-typed signatures through
// the shared doRead/doWrite plumbing. Name-based: independent of the
// handle's StreamKind tag.
func streamSentinel(v Value) (string, bool) {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return "", false
	}
	sentinel, ok := streamSentinels[name]
	return sentinel, ok
}
