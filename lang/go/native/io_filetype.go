package native

import (
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// fileTypeNames is the closed set of FileType atom names — the kind field of
// a `stat` record. Membership is name-based (like StreamKind), so an atom
// minted by one import is admitted by another import's FileType too.
var fileTypeNames = map[string]bool{
	"file":    true,
	"dir":     true,
	"symlink": true,
	"other":   true,
}

// isFileTypeAtom is the FileType membership rule: a concrete atom whose name
// is one of file/dir/symlink/other. As with StreamKind, this single
// predicate defines the type — MemberBehavior derives Match/Unify/canon and
// `is`/dispatch agreement, so no hand-written TypeBehavior is needed.
func isFileTypeAtom(v Value) bool {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return false
	}
	return fileTypeNames[name]
}

// MintFileType mints the FileType type into r's type table and returns the
// node. Like StreamKind it is owned by the boru:io module (minted per import
// into the sub-registry by BuildIOModule), has no FixedID, and is reachable
// from boru only through the module export `IO.FileType`. It is an Atom
// subtype whose inhabitants are exactly file/dir/symlink/other.
func MintFileType(r *Registry) *Type {
	return r.Types.MintMemberType("FileType", core.TAtom, isFileTypeAtom)
}

// NewFileType mints a standalone FileType (into its own dynamic type table)
// for test helpers that register the io words under bare names without a host
// registry to mint into (mirrors NewStreamKind).
func NewFileType() *Type {
	return core.NewDynamicTypeTable().MintMemberType("FileType", core.TAtom, isFileTypeAtom)
}

// newFileTypeAtom returns the FileType atom for a name, tagged with the given
// FileType node so its static and runtime types agree.
func newFileTypeAtom(name string, fileType *Type) Value {
	atom := core.NewAtom(name)
	return core.MintRetagged(&atom, fileType)
}

// fileTypeName classifies a FileInfo into its FileType atom name.
func fileTypeName(fi capabilities.FileInfo) string {
	switch {
	case fi.Symlink:
		return "symlink"
	case fi.IsDir:
		return "dir"
	case fi.Mode.IsRegular():
		return "file"
	default:
		return "other"
	}
}
