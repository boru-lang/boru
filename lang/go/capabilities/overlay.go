package capabilities

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// OverlayFileOps composes a writable UPPER FileOps over a read-only LOWER
// one — the standard union-mount shape (upper=mem over lower=real is the
// "partially in-memory" unit-test configuration). Semantics:
//
//   - reads / stat / list consult the upper first; a path absent there
//     falls through to the lower unless a WHITEOUT hides it;
//   - every mutation lands in the upper: writing over a lower file
//     copies nothing up front (the new upper entry simply shadows), and
//     metadata mutations (chmod/chtimes/truncate/append-style rewrites)
//     COPY UP the lower file first, then mutate the upper copy;
//   - deleting a path that exists in the lower records a whiteout, so
//     the union stops showing it; deleting an upper-only path is a plain
//     upper delete. Whiteouts are stored in the overlay itself (not the
//     upper), keyed by resolved path;
//   - directory listings merge both layers name-wise (upper entry wins a
//     collision; whiteouts subtract), sorted like every FileOps.ReadDir.
//
// The zero value is not usable; build with NewOverlay. The overlay is a
// full FileOps, so it installs anywhere one does (SetHostFileOps, the
// io module's mount word) and nests (an overlay may itself be a layer).
type OverlayFileOps struct {
	Upper FileOps
	Lower FileOps
	// whiteouts records union-deleted paths (resolved through Upper's
	// ResolvePath, which the overlay uses as its path authority). A
	// whiteout hides the lower entry AND any lower subtree below it.
	// wmu guards it: IO.watch runs its callback on a concurrent registry
	// fork while the main program keeps doing I/O, so both sides can touch
	// the map at once — without the lock that is Go's fatal "concurrent map
	// read and map write". Every access goes through the helpers below.
	wmu       sync.Mutex
	whiteouts map[string]bool
}

// isWhiteout / setWhiteout / clearWhiteout are the only ways whiteouts is
// touched, each under wmu, so concurrent overlay users never race the map.
func (o *OverlayFileOps) isWhiteout(p string) bool {
	o.wmu.Lock()
	defer o.wmu.Unlock()
	return o.whiteouts[p]
}

func (o *OverlayFileOps) setWhiteout(p string) {
	o.wmu.Lock()
	defer o.wmu.Unlock()
	o.whiteouts[p] = true
}

func (o *OverlayFileOps) clearWhiteout(p string) {
	o.wmu.Lock()
	defer o.wmu.Unlock()
	delete(o.whiteouts, p)
}

// NewOverlay builds an overlay of upper (writable) over lower (read-only).
func NewOverlay(upper, lower FileOps) *OverlayFileOps {
	return &OverlayFileOps{Upper: upper, Lower: lower, whiteouts: map[string]bool{}}
}

// deref follows TRAILING symlinks at the UNION level: a symlink in one
// layer may point at an entry in the other, so single-layer resolution is
// not enough. Intermediate-component symlinks that cross layers are an
// accepted overlay limitation (each layer still resolves chains that live
// wholly within it); the practical partially-in-mem test surface needs
// only the trailing hop. Bounded like the kernel (ELOOP).
func (o *OverlayFileOps) deref(path string) (string, error) {
	for depth := 0; ; depth++ {
		if depth > 40 {
			return "", &os.PathError{Op: "open", Path: path, Err: errMemTooManyLinks}
		}
		fi, err := o.Stat(path, false)
		if err != nil || !fi.Symlink {
			return path, nil // absent or concrete: the caller owns the outcome
		}
		t := fi.Target
		if !isAbsPath(t) {
			t = filepath.Join(filepath.Dir(path), t)
		}
		path = filepath.Clean(t)
	}
}

// resolve is the overlay's path authority: the UPPER layer's resolution
// (for a mem upper that is lexical cleaning, matching the differential
// harness's absolute-path convention).
func (o *OverlayFileOps) resolve(path string) string {
	resolved, err := o.Upper.ResolvePath(path)
	if err != nil {
		// Upper resolution can only fail on a relative path with an
		// unreadable cwd; degrade to the lexical form so the whiteout
		// keys stay deterministic.
		return filepath.Clean(path)
	}
	return resolved
}

// hidden reports whether resolved (or an ancestor of it) is whited out.
func (o *OverlayFileOps) hidden(resolved string) bool {
	p := resolved
	for {
		if o.isWhiteout(p) {
			return true
		}
		parent := filepath.Dir(p)
		if parent == p || isMemRoot(parent) {
			return false
		}
		p = parent
	}
}

// inUpper reports whether the upper layer knows the path at all (any
// kind — file, dir, symlink).
func (o *OverlayFileOps) inUpper(path string) bool {
	_, err := o.Upper.Stat(path, false)
	return err == nil
}

// copyUp materialises a lower FILE into the upper layer so a mutation can
// apply there. Directories need no copy-up (upper auto-creates parents).
func (o *OverlayFileOps) copyUp(path string) error {
	fi, err := o.Lower.Stat(path, true)
	if err != nil {
		return err
	}
	if fi.IsDir {
		return o.Upper.MkdirAll(path, fi.Mode.Perm())
	}
	data, err := o.Lower.ReadFile(path)
	if err != nil {
		return err
	}
	if err := o.Upper.WriteFile(path, data, fi.Mode.Perm()); err != nil {
		return err
	}
	// Preserve the lower metadata on the fresh copy: timestamps strictly;
	// ownership and xattrs BEST-EFFORT (kernel overlayfs copies these with
	// privilege — an unprivileged upper may refuse chown with EPERM, and
	// that must not fail the mutation that triggered the copy-up).
	if fi.UID != -1 || fi.GID != -1 {
		_ = o.Upper.Chown(path, fi.UID, fi.GID, true)
	}
	if names, xerr := o.Lower.XattrList(path); xerr == nil {
		for _, name := range names {
			if v, gerr := o.Lower.XattrGet(path, name); gerr == nil {
				_ = o.Upper.XattrSet(path, name, v)
			}
		}
	}
	return o.Upper.Chtimes(path, fi.ModTime, fi.ModTime)
}

func (o *OverlayFileOps) ReadFile(path string) ([]byte, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, err
	}
	if o.inUpper(path) {
		return o.Upper.ReadFile(path)
	}
	if o.hidden(o.resolve(path)) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return o.Lower.ReadFile(path)
}

func (o *OverlayFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	// A write lands in the upper and unhides the path: the new content
	// shadows whatever the lower held. Writing through a union symlink
	// lands on its (union-resolved) target, like open(2).
	path, err := o.deref(path)
	if err != nil {
		return err
	}
	if err := o.Upper.WriteFile(path, data, perm); err != nil {
		return err
	}
	o.clearWhiteout(o.resolve(path))
	return nil
}

func (o *OverlayFileOps) MkdirAll(path string, perm os.FileMode) error {
	if err := o.Upper.MkdirAll(path, perm); err != nil {
		return err
	}
	o.clearWhiteout(o.resolve(path))
	return nil
}

func (o *OverlayFileOps) Stat(path string, follow bool) (FileInfo, error) {
	if follow {
		derefed, err := o.deref(path)
		if err != nil {
			return FileInfo{}, err
		}
		path = derefed
	}
	if o.inUpper(path) {
		return o.Upper.Stat(path, follow)
	}
	if o.hidden(o.resolve(path)) {
		return FileInfo{}, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	return o.Lower.Stat(path, follow)
}

func (o *OverlayFileOps) ReadDir(path string) ([]FileInfo, error) {
	path, err := o.deref(path) // a symlink-to-dir lists its target, like os.ReadDir
	if err != nil {
		return nil, err
	}
	resolved := o.resolve(path)
	upperInfos, upperErr := o.Upper.ReadDir(path)
	var lowerInfos []FileInfo
	var lowerErr error
	if o.hidden(resolved) {
		lowerErr = &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
	} else {
		lowerInfos, lowerErr = o.Lower.ReadDir(path)
	}
	if upperErr != nil && lowerErr != nil {
		// Neither layer lists it — report the upper's error (the layers
		// agree on the not-exist case, the common one).
		return nil, upperErr
	}
	merged := map[string]FileInfo{}
	for _, fi := range lowerInfos {
		if !o.isWhiteout(filepath.Join(resolved, fi.Name)) {
			merged[fi.Name] = fi
		}
	}
	for _, fi := range upperInfos {
		merged[fi.Name] = fi // upper shadows a lower name collision
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]FileInfo, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func (o *OverlayFileOps) Remove(path string, recursive bool) error {
	resolved := o.resolve(path)
	upperKnows := o.inUpper(path)
	lowerVisible := false
	if !o.hidden(resolved) {
		if _, err := o.Lower.Stat(path, false); err == nil {
			lowerVisible = true
		}
	}
	if !upperKnows && !lowerVisible {
		if recursive {
			return nil // RemoveAll is idempotent on an absent path
		}
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	// A non-recursive remove of a union directory that still shows lower
	// children must refuse like rmdir(2).
	if !recursive {
		if fi, err := o.Stat(path, false); err == nil && fi.IsDir {
			if infos, rerr := o.ReadDir(path); rerr == nil && len(infos) > 0 {
				return &os.PathError{Op: "remove", Path: path, Err: errMemNotEmpty}
			}
		}
	}
	if upperKnows {
		if err := o.Upper.Remove(path, recursive); err != nil {
			return err
		}
	}
	if lowerVisible {
		o.setWhiteout(resolved)
	}
	return nil
}

func (o *OverlayFileOps) Rename(oldPath, newPath string) error {
	// Union rename = materialise the source's VISIBLE lower content into the
	// upper, rename there, and white-out the old lower name. A DIRECTORY that
	// lives in both layers must still copy its lower subtree up (merge-safe,
	// so upper modifications win) — otherwise the old-name whiteout below
	// would drop the lower-only children that the move should have carried.
	oldResolved := o.resolve(oldPath)
	inUpper := o.inUpper(oldPath)
	lowerVisible := false
	if !o.hidden(oldResolved) {
		if lfi, lerr := o.Lower.Stat(oldPath, false); lerr == nil {
			lowerVisible = true
			if lfi.IsDir {
				if err := o.copyUpTree(oldPath); err != nil {
					return err
				}
			} else if !inUpper {
				if err := o.copyUp(oldPath); err != nil {
					return err
				}
			}
		}
	}
	if !inUpper && !lowerVisible {
		return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
	}
	if err := o.Upper.Rename(oldPath, newPath); err != nil {
		return err
	}
	if _, err := o.Lower.Stat(oldPath, false); err == nil {
		o.setWhiteout(oldResolved)
	}
	o.clearWhiteout(o.resolve(newPath))
	return nil
}

// copyUpTree materialises a lower directory subtree into the upper. It is
// MERGE-SAFE: a leaf the upper already shadows is left untouched, so calling
// it on a directory that exists in both layers fills in only the lower-only
// entries and never clobbers an upper modification.
func (o *OverlayFileOps) copyUpTree(path string) error {
	fi, err := o.Lower.Stat(path, true)
	if err != nil {
		return err
	}
	if !fi.IsDir {
		if o.inUpper(path) {
			return nil // an upper entry already shadows this lower leaf
		}
		return o.copyUp(path)
	}
	if err := o.Upper.MkdirAll(path, fi.Mode.Perm()); err != nil {
		return err
	}
	infos, err := o.Lower.ReadDir(path)
	if err != nil {
		return err
	}
	for _, child := range infos {
		if o.isWhiteout(filepath.Join(o.resolve(path), child.Name)) {
			continue
		}
		if err := o.copyUpTree(filepath.Join(path, child.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (o *OverlayFileOps) Symlink(target, linkPath string) error {
	// Union EEXIST: a visible lower entry blocks the link even though the
	// upper would accept it.
	if _, err := o.Stat(linkPath, false); err == nil {
		return &os.PathError{Op: "symlink", Path: linkPath, Err: os.ErrExist}
	}
	if err := o.Upper.Symlink(target, linkPath); err != nil {
		return err
	}
	o.clearWhiteout(o.resolve(linkPath))
	return nil
}

func (o *OverlayFileOps) Link(target, linkPath string) error {
	if _, err := o.Stat(linkPath, false); err == nil {
		return &os.PathError{Op: "link", Path: linkPath, Err: os.ErrExist}
	}
	// A hard link needs its target in the writable layer.
	if !o.inUpper(target) {
		if o.hidden(o.resolve(target)) {
			return &os.PathError{Op: "link", Path: target, Err: os.ErrNotExist}
		}
		if err := o.copyUp(target); err != nil {
			return err
		}
	}
	if err := o.Upper.Link(target, linkPath); err != nil {
		return err
	}
	o.clearWhiteout(o.resolve(linkPath))
	return nil
}

// mutateInUpper union-derefs the path, copies a lower-only entry up, then
// applies fn to the upper at the resolved location.
func (o *OverlayFileOps) mutateInUpper(path, op string, fn func(resolved string) error) error {
	path, err := o.deref(path)
	if err != nil {
		return err
	}
	if !o.inUpper(path) {
		if o.hidden(o.resolve(path)) {
			return &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
		}
		if _, err := o.Lower.Stat(path, false); err != nil {
			return &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
		}
		if err := o.copyUp(path); err != nil {
			return err
		}
	}
	return fn(path)
}

func (o *OverlayFileOps) Chmod(path string, mode os.FileMode) error {
	return o.mutateInUpper(path, "chmod", func(p string) error { return o.Upper.Chmod(p, mode) })
}

func (o *OverlayFileOps) Chtimes(path string, atime, mtime time.Time) error {
	return o.mutateInUpper(path, "chtimes", func(p string) error { return o.Upper.Chtimes(p, atime, mtime) })
}

func (o *OverlayFileOps) Truncate(path string, size int64) error {
	return o.mutateInUpper(path, "truncate", func(p string) error { return o.Upper.Truncate(p, size) })
}

// Chown re-owns in the upper (copy-up first — the union mutation rule).
func (o *OverlayFileOps) Chown(path string, uid, gid int, follow bool) error {
	return o.mutateInUpper(path, "chown", func(p string) error { return o.Upper.Chown(p, uid, gid, follow) })
}

// readSideXattr is the shared upper-first-else-whiteout-checked-lower
// routing for the xattr READ pair.
func (o *OverlayFileOps) readSideXattr(path, op string) (FileOps, string, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, "", err
	}
	if o.inUpper(path) {
		return o.Upper, path, nil
	}
	if o.hidden(o.resolve(path)) {
		return nil, "", &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
	}
	return o.Lower, path, nil
}

func (o *OverlayFileOps) XattrGet(path, name string) ([]byte, error) {
	side, p, err := o.readSideXattr(path, "xattr-get")
	if err != nil {
		return nil, err
	}
	return side.XattrGet(p, name)
}

func (o *OverlayFileOps) XattrSet(path, name string, value []byte) error {
	return o.mutateInUpper(path, "xattr-set", func(p string) error { return o.Upper.XattrSet(p, name, value) })
}

func (o *OverlayFileOps) XattrList(path string) ([]string, error) {
	side, p, err := o.readSideXattr(path, "xattr-list")
	if err != nil {
		return nil, err
	}
	return side.XattrList(p)
}

func (o *OverlayFileOps) XattrRemove(path, name string) error {
	return o.mutateInUpper(path, "xattr-remove", func(p string) error { return o.Upper.XattrRemove(p, name) })
}

// TempFile / TempDir create in the UPPER (all mutations land there) and
// unhide the result — a whiteout at a previously-deleted path must not
// mask the fresh entry.
func (o *OverlayFileOps) TempFile(dir, pattern string) (string, error) {
	path, err := o.Upper.TempFile(dir, pattern)
	if err != nil {
		return "", err
	}
	o.clearWhiteout(o.resolve(path))
	return path, nil
}

func (o *OverlayFileOps) TempDir(dir, pattern string) (string, error) {
	path, err := o.Upper.TempDir(dir, pattern)
	if err != nil {
		return "", err
	}
	o.clearWhiteout(o.resolve(path))
	return path, nil
}

// Statfs reports the UPPER's volume — that is where union writes land,
// so its capacity is the one that constrains the caller.
func (o *OverlayFileOps) Statfs(path string) (FsInfo, error) {
	return o.Upper.Statfs(path)
}

// Open routes by intent: a read-only open reads upper-else-lower (like
// ReadFile, honouring whiteouts); any write intent (write/append/create/
// truncate) materialises the path in the upper first — a lower-only file
// is copied up so an append sees its bytes — then opens the upper and
// clears any whiteout. All mutations land in the upper, consistent with
// the rest of the union.
func (o *OverlayFileOps) Open(path string, opts OpenOpts) (FileHandle, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, err
	}
	writeIntent := opts.Write || opts.Append || opts.Create || opts.Truncate
	if !writeIntent {
		if o.inUpper(path) {
			return o.Upper.Open(path, opts)
		}
		if o.hidden(o.resolve(path)) {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}
		return o.Lower.Open(path, opts)
	}
	lowerVisible := !o.inUpper(path) && !o.hidden(o.resolve(path))
	if lowerVisible {
		_, serr := o.Lower.Stat(path, false)
		lowerVisible = serr == nil
	}
	// An exclusive create must fail on UNION existence BEFORE any copy-up,
	// so a refused O_EXCL open leaves the upper untouched (a failed open
	// must have no side effect — mem/OS parity).
	if opts.Create && opts.Exclusive && (o.inUpper(path) || lowerVisible) {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrExist}
	}
	if lowerVisible {
		if cerr := o.copyUp(path); cerr != nil {
			return nil, cerr
		}
	}
	h, err := o.Upper.Open(path, opts)
	if err != nil {
		return nil, err
	}
	o.clearWhiteout(o.resolve(path))
	return h, nil
}

// Lock copies a lower-only file up, then locks the UPPER — so every
// holder (reader or writer) contends on the same layer, and a lock is
// consistent with where mutations land.
func (o *OverlayFileOps) Lock(path string, shared, block bool) (io.Closer, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, err
	}
	if err := o.copyUpIfLowerOnly(path); err != nil {
		return nil, err
	}
	return o.Upper.Lock(path, shared, block)
}

// Mmap of a writable region copies up then maps the UPPER; a read-only
// region maps upper-else-lower (honouring whiteouts).
func (o *OverlayFileOps) Mmap(path string, offset int64, length int, writable bool) (MmapRegion, error) {
	path, err := o.deref(path)
	if err != nil {
		return nil, err
	}
	if writable {
		if err := o.copyUpIfLowerOnly(path); err != nil {
			return nil, err
		}
		return o.Upper.Mmap(path, offset, length, true)
	}
	if o.inUpper(path) {
		return o.Upper.Mmap(path, offset, length, false)
	}
	if o.hidden(o.resolve(path)) {
		return nil, &os.PathError{Op: "mmap", Path: path, Err: os.ErrNotExist}
	}
	return o.Lower.Mmap(path, offset, length, false)
}

// copyUpIfLowerOnly materialises a lower-only, non-whited-out file into
// the upper (a no-op if it is already upper or absent).
func (o *OverlayFileOps) copyUpIfLowerOnly(path string) error {
	if o.inUpper(path) || o.hidden(o.resolve(path)) {
		return nil
	}
	if _, serr := o.Lower.Stat(path, false); serr != nil {
		return nil
	}
	return o.copyUp(path)
}

func (o *OverlayFileOps) ResolvePath(path string) (string, error) {
	return o.Upper.ResolvePath(path)
}

// Whiteouts returns a sorted snapshot of the union-deleted paths — the
// observable overlay state a test may assert on.
func (o *OverlayFileOps) Whiteouts() []string {
	out := make([]string, 0, len(o.whiteouts))
	for p := range o.whiteouts {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

var _ FileOps = (*OverlayFileOps)(nil)
