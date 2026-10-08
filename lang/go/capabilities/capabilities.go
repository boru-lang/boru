// Package capabilities provides the abstraction for host-side capabilities
// the boru engine needs to talk to the outside world. At present it covers
// file-system access (the FileOps interface and its OS-backed and
// in-memory implementations); future host capabilities (network,
// process spawn, …) go here too. All dangerous I/O routes through these
// interfaces so it can be replaced for testing or sandboxing without
// touching the Go os/net/etc. packages directly.
package capabilities

import (
	"errors"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Clock is the host capability that supplies "the current time" to boru's
// temporal words (`now`, the boru:time `time-now*` family) and to the
// default seed of boru:rand. The default implementation reads the wall
// clock; a FixedClock can be installed for deterministic tests/specs so
// `now` and time-dependent output are reproducible.
type Clock interface {
	Now() time.Time
}

// WallClock is the default Clock: it returns the real system time.
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }

// FixedClock is a Clock frozen at a single instant — used by the spec
// runner and tests so temporal words produce deterministic results.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }

// StepAction is what a StepController decides to do at a paused step.
type StepAction int

const (
	// StepInto runs the next single step, then pauses again.
	StepInto StepAction = iota
	// StepContinue resumes until the next breakpoint (or completion).
	StepContinue
	// StepQuit abandons the stepped run.
	StepQuit
)

// StepFrame is the rendered state handed to a StepController at a pause.
// It carries only stdlib types so this package stays engine-agnostic — the
// stack is pre-rendered to strings by the caller.
type StepFrame struct {
	Step    int      // 0-based step index (-1 for a one-shot Debug.break pause)
	Pointer int      // index of the value about to execute
	Stack   []string // the tape, rendered, one entry per slot
	AtBreak bool     // true when paused on a Debug.break marker
	// Row / Col locate the value about to execute in its source (1-based;
	// 0 = unknown — synthetic tokens and one-shot break pauses carry no
	// position), and File is the executing registry's resolved source
	// file ("" if none). Additive widening (design/BORU-DEBUGGER.0.md §10
	// Phase 1): source-aware hosts — the `boru debug` CLI, a future DAP
	// adapter — need each pause located in source, and file identity must
	// come from the registry, not the value (a SrcPos carries no file).
	// Plain ints/string keep this package engine-agnostic.
	Row  int
	Col  int
	File string
}

// StepController is the host capability that drives interactive stepping
// (Debug.step). At each pause the engine renders the frame and calls
// OnStep; the returned StepAction decides what happens next. A TTY host
// reads a keypress; a test supplies a scripted list of actions; a
// non-interactive host returns StepContinue to run straight through.
type StepController interface {
	OnStep(frame StepFrame) StepAction
}

// DebugOps is the host capability backing the effectful parts of
// boru:debug that the in-process module cannot supply itself: the
// interactive step controller (and, in future, runtime hooks). Absent a
// DebugOps, Debug.step falls back to printing the trace.
type DebugOps interface {
	Controller() StepController
}

// FileInfo is the host-agnostic result of Stat / a ReadDir entry. It
// carries only stdlib types so the capabilities package stays engine-
// agnostic; the `boru:io` handlers turn it into a boru FileInfo record.
type FileInfo struct {
	Name    string      // base name (final path segment)
	Size    int64       // length in bytes (0 for directories/symlinks)
	Mode    os.FileMode // permission + type bits
	ModTime time.Time   // last-modified time
	IsDir   bool        // true for directories
	Symlink bool        // true when the entry itself is a symlink (lstat view)
	Target  string      // symlink target, when Symlink
	// UID / GID are the owning user and group ids, or -1 where the
	// backend cannot know them (Windows, a mounted boru filesystem whose
	// stat handler omits them). Construct FileInfo values through a path
	// that sets them explicitly — the Go zero value 0 is root's uid, so
	// an accidental zero is a real (wrong) owner, not "unset".
	UID int
	GID int
}

// FsInfo is the host-agnostic Statfs result — the volume/disk view
// (Java FileStore / C# DriveInfo parity). Values are BACKEND-DEFINED:
// the OS reports the real filesystem, MemFileOps a synthetic fixed-size
// volume — consumers must treat the numbers as advisory shape, and the
// parity harness compares shape, never values.
type FsInfo struct {
	TotalBytes uint64 // volume capacity
	FreeBytes  uint64 // free space (superuser view)
	AvailBytes uint64 // space available to this process (≤ FreeBytes)
	BlockSize  int64  // allocation block size
	Type       string // filesystem type name ("ext4", "tmpfs", "mem", …)
}

// OpenOpts controls how Open acquires a stateful file handle. The zero
// value is a read-only open of an existing file. Write/Append/Create/
// Truncate/Exclusive map onto the os.OpenFile flags (O_WRONLY|O_APPEND|
// O_CREATE|O_TRUNC|O_EXCL); Read alongside Write yields O_RDWR. Perm is
// the creation mode when Create is set (0 defaults to 0644).
type OpenOpts struct {
	Read      bool
	Write     bool
	Append    bool
	Create    bool
	Exclusive bool // with Create: fail if the path already exists (O_EXCL)
	Truncate  bool
	Perm      os.FileMode
}

// FileHandle is a stateful open file: a byte cursor plus positioned and
// whole-stream I/O. The OS backend returns *os.File verbatim (it already
// satisfies this interface); the mem backend indexes its maps per call so
// a handle's writes are visible to a stateless read before Close.
type FileHandle interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
	ReadAt(p []byte, off int64) (int, error)
	WriteAt(p []byte, off int64) (int, error)
	Truncate(size int64) error
	Sync() error
	Name() string
}

// ErrLockBusy is returned by Lock with block=false when the advisory
// lock is already held by another holder (the non-blocking contention
// signal — mapped to a `none` result at the word layer).
var ErrLockBusy = errors.New("file lock is held")

// MmapRegion is a memory-mapped view of a file's bytes. Bytes returns the
// mapped slice (the caller must COPY before letting it escape a Close —
// after Close the memory is unmapped); Flush writes pending changes back;
// Close unmaps. The portable contract promises no live sharing between a
// region and stateless reads — Flush is what makes writes durable.
type MmapRegion interface {
	Bytes() []byte
	Flush() error
	Close() error
}

// FileOps defines the file operations that boru's io words use. The
// default implementation delegates to the os package. Replace with a
// custom implementation for testing or sandboxing.
//
// The interface covers the batch/stateless filesystem surface
// (read/write/append, directory create/list, stat, remove, rename,
// links, metadata mutation) plus Open, a stateful file handle. `copy` is
// deliberately absent — it is composed in the handler layer from
// Stat/ReadFile/WriteFile/ReadDir/MkdirAll. Append is likewise composed
// (read-then-write) in the handler, so it needs no method here.
type FileOps interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	// Stat returns metadata for path. When follow is true a symlink is
	// dereferenced (stat); when false the link itself is described (lstat).
	Stat(path string, follow bool) (FileInfo, error)
	// ReadDir lists the direct children of a directory, name-sorted.
	ReadDir(path string) ([]FileInfo, error)
	// Remove deletes path. When recursive is true a non-empty directory
	// and its contents are removed; otherwise removing a non-empty
	// directory errors.
	Remove(path string, recursive bool) error
	// Rename moves oldPath to newPath.
	Rename(oldPath, newPath string) error
	// Symlink creates a symbolic link at linkPath pointing at target.
	Symlink(target, linkPath string) error
	// Link creates a hard link at linkPath referring to target.
	Link(target, linkPath string) error
	// Chmod sets path's permission bits.
	Chmod(path string, mode os.FileMode) error
	// Chtimes sets path's access and modification times.
	Chtimes(path string, atime, mtime time.Time) error
	// Truncate changes the size of the file at path.
	Truncate(path string, size int64) error
	// Chown changes path's owning user and group. A -1 uid or gid leaves
	// that id unchanged (the chown(2) convention). When follow is false a
	// symlink itself is re-owned (lchown); otherwise its target is.
	Chown(path string, uid, gid int, follow bool) error
	// XattrGet reads one extended attribute. Absent attributes error with
	// the platform's not-found class (ENODATA / ENOATTR shaped).
	XattrGet(path, name string) ([]byte, error)
	// XattrSet writes one extended attribute (create or replace).
	XattrSet(path, name string, value []byte) error
	// XattrList returns the names of path's extended attributes, sorted.
	XattrList(path string) ([]string, error)
	// XattrRemove deletes one extended attribute. Removing an absent
	// attribute errors like XattrGet.
	XattrRemove(path, name string) error
	// TempFile creates a new unique file and returns its path. dir "" means
	// the backend's temp root; a "*" in pattern is replaced by a unique
	// token, otherwise the token is appended (the os.CreateTemp contract).
	TempFile(dir, pattern string) (string, error)
	// TempDir creates a new unique directory, same contract as TempFile.
	TempDir(dir, pattern string) (string, error)
	// Statfs reports the filesystem holding path (which must exist).
	Statfs(path string) (FsInfo, error)
	// Watch subscribes to change events for path — a file, or a
	// directory's children (its direct children under inotify semantics,
	// or the whole subtree when opts.Recursive is set). Returns the event
	// stream and a stop function that releases the watch and closes the
	// stream. Watching an absent path errors. See WatchEvent and WatchOpts
	// (watch.go) for the shared op vocabulary and options.
	Watch(path string, opts WatchOpts) (<-chan WatchEvent, func() error, error)
	// Open acquires a stateful file handle per OpenOpts (read/write/
	// append/create/exclusive/truncate). The caller must Close it.
	Open(path string, opts OpenOpts) (FileHandle, error)
	// Lock takes an advisory lock on path: shared (read) vs exclusive
	// (write), blocking vs non-blocking. A non-blocking call on a held
	// lock returns ErrLockBusy. The returned Closer releases it.
	Lock(path string, shared, block bool) (io.Closer, error)
	// Mmap maps [offset, offset+length) of path into memory. writable
	// permits writes (durable on Flush/Close). The caller must Close it.
	Mmap(path string, offset int64, length int, writable bool) (MmapRegion, error)
	ResolvePath(path string) (string, error)
}

// OSFileOps is the default implementation using the real file system.
// The unexported getwd field allows tests to inject a failing os.Getwd.
type OSFileOps struct {
	getwd    func() (string, error)
	mkdirAll func(string, os.FileMode) error
}

func (o *OSFileOps) ReadFile(path string) ([]byte, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (o *OSFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(resolved)
	mkdirFn := o.mkdirAll
	if mkdirFn == nil {
		mkdirFn = os.MkdirAll
	}
	if err := mkdirFn(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, perm)
}

// MkdirAll creates a directory and all parents. Idempotent.
func (o *OSFileOps) MkdirAll(path string, perm os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	mkdirFn := o.mkdirAll
	if mkdirFn == nil {
		mkdirFn = os.MkdirAll
	}
	return mkdirFn(resolved, perm)
}

// ResolvePath resolves a relative path against the process working directory.
func (o *OSFileOps) ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	getwdFn := o.getwd
	if getwdFn == nil {
		getwdFn = os.Getwd
	}
	wd, err := getwdFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, path), nil
}

// osFileInfo projects an os.FileInfo onto the host-agnostic FileInfo.
// Ownership comes from the platform arm (xattr_unix.go / xattr_other.go)
// — -1/-1 where the host cannot know it.
func osFileInfo(fi os.FileInfo) FileInfo {
	uid, gid := osFileOwner(fi)
	return FileInfo{
		Name:    fi.Name(),
		Size:    fi.Size(),
		Mode:    fi.Mode(),
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
		Symlink: fi.Mode()&os.ModeSymlink != 0,
		UID:     uid,
		GID:     gid,
	}
}

// Stat returns metadata for path. follow=true dereferences a symlink
// (os.Stat); follow=false describes the link itself (os.Lstat) and fills
// Target for a symlink.
func (o *OSFileOps) Stat(path string, follow bool) (FileInfo, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return FileInfo{}, err
	}
	if follow {
		fi, err := os.Stat(resolved)
		if err != nil {
			return FileInfo{}, err
		}
		return osFileInfo(fi), nil
	}
	fi, err := os.Lstat(resolved)
	if err != nil {
		return FileInfo{}, err
	}
	info := osFileInfo(fi)
	if info.Symlink {
		// os.Lstat reported a symlink, so Readlink succeeds barring a
		// filesystem race; on the (unreproducible) race we degrade to an
		// empty Target rather than failing the stat.
		if target, rerr := os.Readlink(resolved); rerr == nil {
			info.Target = target
		}
	}
	return info, nil
}

// ReadDir lists the direct children of a directory, name-sorted (os.ReadDir
// already returns sorted entries).
func (o *OSFileOps) ReadDir(path string) ([]FileInfo, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	infos := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		// DirEntry.Info fails only if the entry vanished between ReadDir
		// and Info (a race); such an entry is simply skipped.
		if fi, ierr := e.Info(); ierr == nil {
			infos = append(infos, osFileInfo(fi))
		}
	}
	return infos, nil
}

// Remove deletes path. recursive=true removes a directory tree (os.RemoveAll);
// recursive=false uses os.Remove, which errors on a non-empty directory.
func (o *OSFileOps) Remove(path string, recursive bool) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	if recursive {
		return os.RemoveAll(resolved)
	}
	return os.Remove(resolved)
}

// Rename moves oldPath to newPath.
func (o *OSFileOps) Rename(oldPath, newPath string) error {
	oldResolved, err := o.ResolvePath(oldPath)
	if err != nil {
		return err
	}
	newResolved, err := o.ResolvePath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldResolved, newResolved)
}

// Symlink creates a symbolic link at linkPath pointing at target. The
// target is stored verbatim (it may be relative to the link's directory).
func (o *OSFileOps) Symlink(target, linkPath string) error {
	linkResolved, err := o.ResolvePath(linkPath)
	if err != nil {
		return err
	}
	return os.Symlink(target, linkResolved)
}

// Link creates a hard link at linkPath referring to target. If target is a
// symlink, the hard link refers to the symlink itself, without following it.
func (o *OSFileOps) Link(target, linkPath string) error {
	targetResolved, err := o.ResolvePath(target)
	if err != nil {
		return err
	}
	linkResolved, err := o.ResolvePath(linkPath)
	if err != nil {
		return err
	}
	return linkNoFollow(targetResolved, linkResolved)
}

// Chmod sets path's permission bits.
func (o *OSFileOps) Chmod(path string, mode os.FileMode) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Chmod(resolved, mode)
}

// Chtimes sets path's access and modification times.
func (o *OSFileOps) Chtimes(path string, atime, mtime time.Time) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Chtimes(resolved, atime, mtime)
}

// Truncate changes the size of the file at path.
func (o *OSFileOps) Truncate(path string, size int64) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return os.Truncate(resolved, size)
}

// Chown changes path's owning user and group (lchown when follow=false).
func (o *OSFileOps) Chown(path string, uid, gid int, follow bool) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	if follow {
		return os.Chown(resolved, uid, gid)
	}
	return os.Lchown(resolved, uid, gid)
}

// XattrGet reads one extended attribute (platform impl in xattr_*.go).
func (o *OSFileOps) XattrGet(path, name string) ([]byte, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return osXattrGet(resolved, name)
}

// XattrSet writes one extended attribute.
func (o *OSFileOps) XattrSet(path, name string, value []byte) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return osXattrSet(resolved, name, value)
}

// XattrList returns the sorted names of path's extended attributes.
func (o *OSFileOps) XattrList(path string) ([]string, error) {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return nil, err
	}
	return osXattrList(resolved)
}

// XattrRemove deletes one extended attribute.
func (o *OSFileOps) XattrRemove(path, name string) error {
	resolved, err := o.ResolvePath(path)
	if err != nil {
		return err
	}
	return osXattrRemove(resolved, name)
}

// NewDefault returns the default OS-backed file operations.
func NewDefault() FileOps {
	return &OSFileOps{}
}

// Errors returned by MemFileOps for conditions with no os.Err* constant.
// The messages mirror the kernel errno strings (ENOTDIR / EISDIR /
// ENOTEMPTY / ELOOP) so an error-CLASS comparison against the real
// filesystem — the differential parity harness's classify() — matches.
var (
	errMemNotDir       = errors.New("not a directory")
	errMemIsDir        = errors.New("is a directory")
	errMemNotEmpty     = errors.New("directory not empty")
	errMemTooManyLinks = errors.New("too many levels of symbolic links")
)

// memMeta is the per-path metadata MemFileOps tracks alongside Files/Dirs.
// It is created for every path on write/mkdir/symlink/link, so its presence
// is authoritative for an existing path's mode/mtime (Mode holds permission
// bits only — the type bits are composed at read time). A non-empty Target
// marks the path as a symlink (which then lives ONLY in Meta, not Files/Dirs).
type memMeta struct {
	Mode   os.FileMode
	MTime  time.Time
	ATime  time.Time
	Target string
	// UID/GID record ownership; OwnerSet distinguishes them from the Go
	// zero (uid 0 is root — a REAL id, so a bare zero must not be read
	// as "unset"; the no-zero-value-overload rule). Fresh metadata is
	// stamped with the creating identity (stampOwner); a meta-less or
	// unstamped entry (a test poking Files directly) defaults to the
	// CURRENT euid/egid at read time via ownerOf.
	UID      int
	GID      int
	OwnerSet bool
	// Xattrs holds extended attributes. It lives on the CANONICAL path's
	// meta, so hard links share attributes — inode fidelity.
	Xattrs map[string][]byte
}

// MemFileOps is an in-memory implementation with REAL-FILESYSTEM fidelity:
// the differential parity harness (differential_test.go) runs identical
// operation sequences against OSFileOps and MemFileOps and asserts equal
// results and error classes. The model covers regular files (Files),
// directories (Dirs), symlinks and metadata (Meta), and hard links (Links —
// alias name → canonical file path, sharing the canonical's bytes and
// metadata exactly like names sharing an inode). Path resolution follows
// symlinks component-wise with an ELOOP budget, an intermediate component
// that is a regular file raises ENOTDIR, and permission bits are enforced
// euid-aware (root bypasses checks, exactly as the kernel does — see
// checkPerm). Documented fidelity exceptions: umask is not applied to
// creation modes, access times are stored but not surfaced (FileInfo has no
// ATime — true of the portable os API too), directory sizes are 0 (real
// values are filesystem-dependent), and unrecorded ancestor directories are
// treated as implicitly writable so tests may poke Files directly.
type MemFileOps struct {
	// mu guards every map below. It is held by all public methods and by
	// live file handles (handles.go), so a handle used from a watch
	// callback goroutine never races a stateless op on the main thread.
	// emit sends non-blocking (watch.go), so holding mu across a mutation
	// + its event fan-out cannot deadlock. TempFile/TempDir hold mu and
	// call the *Locked cores directly (WriteFile/MkdirAll re-lock).
	mu sync.Mutex

	Files map[string][]byte
	Dirs  map[string]bool     // tracked directories
	Meta  map[string]*memMeta // per-path mode/mtime/atime + symlink targets
	Links map[string]string   // hard-link alias name -> canonical file path
	Cwd   string              // simulated working directory; defaults to "." if empty

	// euidFn / egidFn / nowFn are test seams for the effective uid
	// (permission enforcement is skipped for root, mirroring the
	// kernel), the effective gid (ownership stamping), and the clock
	// (mtime stamping). Nil means os.Geteuid / os.Getegid / time.Now.
	euidFn func() int
	egidFn func() int
	nowFn  func() time.Time

	// watchers holds the live Watch subscriptions; every successful
	// mutation fans an event out through emit (watch.go).
	watchers []*memWatcher

	// tempSeq numbers TempFile/TempDir tokens — deterministic uniqueness
	// (the parity harness compares temp-name shape, never tokens).
	tempSeq int

	// lockMu / lockCond / fileLocks model advisory locks (locks.go): a
	// canonical-path reader/writer table with a condition variable, kept
	// SEPARATE from mu so a blocking Lock never entangles the map guard.
	lockMu    sync.Mutex
	lockCond  *sync.Cond
	fileLocks map[string]*memLockState
}

func NewMem() *MemFileOps {
	m := &MemFileOps{
		Files:     make(map[string][]byte),
		Dirs:      make(map[string]bool),
		Meta:      make(map[string]*memMeta),
		Links:     make(map[string]string),
		fileLocks: make(map[string]*memLockState),
	}
	m.lockCond = sync.NewCond(&m.lockMu)
	return m
}

func (m *MemFileOps) euid() int {
	if m.euidFn != nil {
		return m.euidFn()
	}
	return os.Geteuid()
}

func (m *MemFileOps) egid() int {
	if m.egidFn != nil {
		return m.egidFn()
	}
	return os.Getegid()
}

// SetIdentity installs test seams for the effective uid / gid consulted
// by permission enforcement and ownership stamping (nil restores the
// real process identity). Exported so cross-package tests — the io word
// tests in particular — can exercise euid-dependent behaviour (chown
// refusals, permission denials) hermetically regardless of the identity
// the test process really runs under.
func (m *MemFileOps) SetIdentity(euid, egid func() int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.euidFn, m.egidFn = euid, egid
}

func (m *MemFileOps) now() time.Time {
	if m.nowFn != nil {
		return m.nowFn()
	}
	return time.Now()
}

// stampOwner records the creating identity on fresh metadata — the mem
// mirror of the kernel assigning a new inode's owner from the process
// credentials. Returns meta for the inline construction idiom.
func (m *MemFileOps) stampOwner(meta *memMeta) *memMeta {
	meta.UID, meta.GID, meta.OwnerSet = m.euid(), m.egid(), true
	return meta
}

// ownerOf reads an entry's ownership, defaulting an unstamped or absent
// meta (a test poking Files directly) to the CURRENT identity — the same
// convention as modeOf's mode defaulting.
func (m *MemFileOps) ownerOf(meta *memMeta) (uid, gid int) {
	if meta != nil && meta.OwnerSet {
		return meta.UID, meta.GID
	}
	return m.euid(), m.egid()
}

// canon maps a hard-link alias to its canonical file path (identity for
// every non-alias path). Content and metadata live under the canonical
// key, so aliases share them — the inode model.
func (m *MemFileOps) canon(p string) string {
	if c, ok := m.Links[p]; ok {
		return c
	}
	return p
}

// isFileEntry reports whether p names a regular file (canonical or alias).
func (m *MemFileOps) isFileEntry(p string) bool {
	if _, ok := m.Files[p]; ok {
		return true
	}
	_, ok := m.Links[p]
	return ok
}

// modeOf / mtimeOf read a memMeta safely, defaulting when it is absent (a
// path can be poked directly into Files by a test without Meta).
func modeOf(meta *memMeta, def os.FileMode) os.FileMode {
	if meta != nil {
		return meta.Mode
	}
	return def
}

func mtimeOf(meta *memMeta) time.Time {
	if meta != nil {
		return meta.MTime
	}
	return time.Time{}
}

// A root is a fixed point of Dir, including Windows drive and UNC roots.
func isMemRoot(p string) bool { return p == "." || p == "/" || filepath.Dir(p) == p }

// resolve turns path into its final in-model location: lexical resolution
// (ResolvePath), then a component walk that chases symlinks in every
// intermediate component — and in the trailing component when
// followTrailing is set — with a shared depth budget (ELOOP), erroring
// ENOTDIR when an intermediate component is a regular file. Ops that act
// on a link itself (Remove, Rename, Link's target, lstat) pass
// followTrailing=false; ops with open(2) semantics (read/write/truncate/
// chmod/chtimes/readdir/stat) pass true.
func (m *MemFileOps) resolve(path string, followTrailing bool) (string, error) {
	resolved, _ := m.ResolvePath(path) // MemFileOps.ResolvePath never fails
	depth := 0
	return m.resolveSteps(resolved, followTrailing, &depth, path)
}

func (m *MemFileOps) resolveSteps(resolved string, followTrailing bool, depth *int, orig string) (string, error) {
	if isMemRoot(resolved) || resolved == "" {
		return resolved, nil
	}
	sep := string(filepath.Separator)
	acc := filepath.VolumeName(resolved)
	rest := strings.TrimPrefix(resolved, acc)
	if strings.HasPrefix(rest, sep) {
		acc, rest = acc+sep, strings.TrimPrefix(rest, sep)
	}
	comps := strings.Split(rest, sep)
	for i, comp := range comps {
		p := filepath.Join(acc, comp)
		last := i == len(comps)-1
		// Chase symlinks at this component (skipping the trailing one
		// when the op targets the link itself).
		for {
			meta := m.Meta[p]
			if meta == nil || meta.Target == "" || (last && !followTrailing) {
				break
			}
			*depth++
			if *depth > 40 {
				return "", &os.PathError{Op: "open", Path: orig, Err: errMemTooManyLinks}
			}
			t := meta.Target
			if !filepath.IsAbs(t) {
				t = filepath.Join(filepath.Dir(p), t)
			}
			rp, err := m.resolveSteps(filepath.Clean(t), true, depth, orig)
			if err != nil {
				return "", err
			}
			p = rp
		}
		if !last && m.isFileEntry(p) {
			return "", &os.PathError{Op: "open", Path: orig, Err: errMemNotDir}
		}
		acc = p
	}
	return acc, nil
}

// checkPerm enforces an owner permission bit on an existing entry.
// Root (euid 0) bypasses permission checks entirely, exactly as the
// kernel does — so the model agrees with the real filesystem whether the
// process runs privileged or not. Symlinks are always 0777 and absent
// paths are left to the caller's not-exist error.
func (m *MemFileOps) checkPerm(op, path, resolved string, bit os.FileMode) error {
	if m.euid() == 0 {
		return nil
	}
	info, ok := m.infoFor(resolved)
	if !ok || info.Symlink {
		return nil
	}
	if info.Mode&bit == 0 {
		return &os.PathError{Op: op, Path: path, Err: os.ErrPermission}
	}
	return nil
}

// checkParentWrite enforces the write bit on the nearest RECORDED ancestor
// directory for namespace mutations (create/remove/rename/link). Unrecorded
// ancestors (implicit dirs) impose nothing — the model treats them as
// writable so tests may poke Files directly without recording parents.
func (m *MemFileOps) checkParentWrite(op, path, resolved string) error {
	if m.euid() == 0 {
		return nil
	}
	dir := filepath.Dir(resolved)
	for !isMemRoot(dir) && dir != "" {
		if m.Dirs[dir] {
			if modeOf(m.Meta[dir], 0o755)&0o200 == 0 {
				return &os.PathError{Op: op, Path: path, Err: os.ErrPermission}
			}
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// infoFor builds the FileInfo for a resolved path, or ok=false if absent.
// A hard-link alias reads the canonical entry's data/metadata but reports
// its own base name; a symlink's Size is the target length (lstat fidelity).
func (m *MemFileOps) infoFor(resolved string) (FileInfo, bool) {
	name := filepath.Base(resolved)
	if meta := m.Meta[resolved]; meta != nil && meta.Target != "" {
		uid, gid := m.ownerOf(meta)
		return FileInfo{Name: name, Size: int64(len(meta.Target)), Mode: os.ModeSymlink | 0o777, ModTime: meta.MTime, Symlink: true, Target: meta.Target, UID: uid, GID: gid}, true
	}
	if c := m.canon(resolved); m.isFileEntry(resolved) {
		data := m.Files[c]
		meta := m.Meta[c]
		uid, gid := m.ownerOf(meta)
		return FileInfo{Name: name, Size: int64(len(data)), Mode: modeOf(meta, 0o644), ModTime: mtimeOf(meta), UID: uid, GID: gid}, true
	}
	if m.Dirs[resolved] {
		meta := m.Meta[resolved]
		uid, gid := m.ownerOf(meta)
		return FileInfo{Name: name, Mode: os.ModeDir | (modeOf(meta, 0o755) & os.ModePerm), ModTime: mtimeOf(meta), IsDir: true, UID: uid, GID: gid}, true
	}
	return FileInfo{}, false
}

func (m *MemFileOps) ReadFile(path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if m.Dirs[final] {
		return nil, &os.PathError{Op: "read", Path: path, Err: errMemIsDir}
	}
	data, ok := m.Files[m.canon(final)]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm("open", path, final, 0o400); perr != nil {
		return nil, perr
	}
	return data, nil
}

func (m *MemFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeFileLocked(path, data, perm)
}

// writeFileLocked is WriteFile's body without the lock, so lock-holding
// callers (TempFile) can reuse it without re-entering the mutex.
func (m *MemFileOps) writeFileLocked(path string, data []byte, perm os.FileMode) error {
	final, err := m.resolve(path, true) // writes THROUGH a trailing symlink, like open(2)
	if err != nil {
		return err
	}
	if m.Dirs[final] {
		return &os.PathError{Op: "open", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	_, exists := m.Files[c]
	if exists {
		if perr := m.checkPerm("open", path, final, 0o200); perr != nil {
			return perr
		}
	} else if perr := m.checkParentWrite("open", path, final); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(final))
	buf := make([]byte, len(data))
	copy(buf, data)
	m.Files[c] = buf
	if exists {
		// open(2) on an existing file keeps its mode; only mtime moves.
		meta := m.Meta[c]
		if meta == nil {
			meta = m.stampOwner(&memMeta{Mode: 0o644})
			m.Meta[c] = meta
		}
		meta.MTime = m.now()
		m.emit("write", final)
	} else {
		m.Meta[c] = m.stampOwner(&memMeta{Mode: perm & os.ModePerm, MTime: m.now(), ATime: m.now()})
		m.emit("create", final)
	}
	return nil
}

// MkdirAll records a directory path and its parents. Idempotent — an
// existing directory keeps its mode, exactly like os.MkdirAll.
func (m *MemFileOps) MkdirAll(path string, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mkdirAllLocked(path, perm)
}

// mkdirAllLocked is MkdirAll's body without the lock, for lock-holding
// callers (TempDir).
func (m *MemFileOps) mkdirAllLocked(path string, perm os.FileMode) error {
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	if m.isFileEntry(final) {
		return &os.PathError{Op: "mkdir", Path: path, Err: errMemNotDir}
	}
	if m.Dirs[final] {
		return nil
	}
	if perr := m.checkParentWrite("mkdir", path, final); perr != nil {
		return perr
	}
	m.recordDir(final)
	m.Meta[final] = m.stampOwner(&memMeta{Mode: perm & os.ModePerm, MTime: m.now(), ATime: m.now()})
	m.emit("create", final)
	return nil
}

// recordDir records dir and every ancestor as a directory, stamping default
// metadata where none exists. Filesystem roots are implicit and not recorded.
func (m *MemFileOps) recordDir(dir string) {
	for dir != "" && !isMemRoot(dir) {
		m.Dirs[dir] = true
		if m.Meta[dir] == nil {
			m.Meta[dir] = m.stampOwner(&memMeta{Mode: 0o755, MTime: m.now()})
		}
		dir = filepath.Dir(dir)
	}
}

// Stat returns metadata for path, dereferencing symlinks when follow is true.
func (m *MemFileOps) Stat(path string, follow bool) (FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, follow)
	if err != nil {
		return FileInfo{}, err
	}
	info, ok := m.infoFor(final)
	if !ok {
		return FileInfo{}, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
	}
	return info, nil
}

// ReadDir lists the direct children of a directory, name-sorted. A
// symlink-to-directory path is followed, like os.ReadDir.
func (m *MemFileOps) ReadDir(path string) ([]FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if m.isFileEntry(final) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: errMemNotDir}
	}
	children := map[string]FileInfo{}
	m.collectChildren(final, children)
	if len(children) == 0 && !m.Dirs[final] && !isMemRoot(final) {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm("readdir", path, final, 0o400); perr != nil {
		return nil, perr
	}
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)
	infos := make([]FileInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, children[name])
	}
	return infos, nil
}

// collectChildren fills out with the immediate children of dir.
func (m *MemFileOps) collectChildren(dir string, out map[string]FileInfo) {
	add := func(p string) {
		if p == dir || filepath.Dir(p) != dir {
			return
		}
		info, _ := m.infoFor(p)
		out[info.Name] = info
	}
	for p := range m.Files {
		add(p)
	}
	for p := range m.Dirs {
		add(p)
	}
	for p := range m.Links {
		add(p)
	}
	for p, meta := range m.Meta {
		if meta.Target != "" {
			add(p)
		}
	}
}

// Remove deletes path (recursive removes a directory tree). Mirrors os:
// RemoveAll (recursive) is idempotent on an absent path; os.Remove is not.
// A symlink is removed itself (no follow); removing one hard-link name
// keeps the shared content alive under the remaining names.
func (m *MemFileOps) Remove(path string, recursive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, false)
	if err != nil {
		return err
	}
	info, ok := m.infoFor(final)
	if !ok {
		if recursive {
			return nil
		}
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkParentWrite("remove", path, final); perr != nil {
		return perr
	}
	switch {
	case info.IsDir:
		children := map[string]FileInfo{}
		m.collectChildren(final, children)
		if len(children) > 0 && !recursive {
			return &os.PathError{Op: "remove", Path: path, Err: errMemNotEmpty}
		}
		m.removeTree(final)
		m.emit("remove", final)
	case info.Symlink:
		delete(m.Meta, final)
		m.emit("remove", final)
	default:
		m.removeFileEntry(final)
		m.emit("remove", final)
	}
	return nil
}

// removeFileEntry unlinks ONE name of a regular file. Removing an alias
// deletes just that name; removing the canonical name with aliases
// remaining promotes the first alias (sorted, for determinism) to
// canonical and repoints the rest — the content survives until its last
// name goes, exactly like an inode's link count.
func (m *MemFileOps) removeFileEntry(p string) {
	if _, isAlias := m.Links[p]; isAlias {
		delete(m.Links, p)
		return
	}
	var aliases []string
	for a, c := range m.Links {
		if c == p {
			aliases = append(aliases, a)
		}
	}
	if len(aliases) > 0 {
		sort.Strings(aliases)
		a0 := aliases[0]
		m.Files[a0] = m.Files[p]
		if meta := m.Meta[p]; meta != nil {
			m.Meta[a0] = meta
		}
		delete(m.Links, a0)
		for _, a := range aliases[1:] {
			m.Links[a] = a0
		}
	}
	delete(m.Files, p)
	delete(m.Meta, p)
}

// removeTree removes everything under root. Alias names inside the tree
// are plain unlinks; canonical files inside the tree promote any alias
// OUTSIDE the tree (their content survives under the surviving name).
func (m *MemFileOps) removeTree(root string) {
	prefix := root + string(filepath.Separator)
	within := func(p string) bool { return p == root || strings.HasPrefix(p, prefix) }
	for p := range m.Links {
		if within(p) {
			delete(m.Links, p)
		}
	}
	var files []string
	for p := range m.Files {
		if within(p) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	for _, p := range files {
		m.removeFileEntry(p)
	}
	for p := range m.Dirs {
		if within(p) {
			delete(m.Dirs, p)
		}
	}
	for p := range m.Meta {
		if within(p) {
			delete(m.Meta, p)
		}
	}
}

// Rename moves oldPath (and, for a directory, its whole subtree) to
// newPath, applying the rename(2) destination rules: file over file
// replaces; a directory may only replace an EMPTY directory (ENOTEMPTY
// otherwise); renaming a directory over a file is ENOTDIR and a file over
// a directory EISDIR. Neither trailing path is symlink-followed.
func (m *MemFileOps) Rename(oldPath, newPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	oldFinal, err := m.resolve(oldPath, false)
	if err != nil {
		return err
	}
	newFinal, err := m.resolve(newPath, false)
	if err != nil {
		return err
	}
	srcInfo, ok := m.infoFor(oldFinal)
	if !ok {
		return &os.PathError{Op: "rename", Path: oldPath, Err: os.ErrNotExist}
	}
	if perr := m.checkParentWrite("rename", oldPath, oldFinal); perr != nil {
		return perr
	}
	if perr := m.checkParentWrite("rename", newPath, newFinal); perr != nil {
		return perr
	}
	if dstInfo, exists := m.infoFor(newFinal); exists {
		switch {
		case srcInfo.IsDir && !dstInfo.IsDir:
			return &os.PathError{Op: "rename", Path: newPath, Err: errMemNotDir}
		case !srcInfo.IsDir && dstInfo.IsDir:
			return &os.PathError{Op: "rename", Path: newPath, Err: errMemIsDir}
		case srcInfo.IsDir && dstInfo.IsDir:
			children := map[string]FileInfo{}
			m.collectChildren(newFinal, children)
			if len(children) > 0 {
				return &os.PathError{Op: "rename", Path: newPath, Err: errMemNotEmpty}
			}
			m.removeTree(newFinal)
		default: // file over file — replace (alias-correct unlink first)
			m.removeFileEntry(newFinal)
		}
	}
	m.recordDir(filepath.Dir(newFinal))
	oldPrefix := oldFinal + string(filepath.Separator)
	newPrefix := newFinal + string(filepath.Separator)
	remap := func(p string) (string, bool) {
		if p == oldFinal {
			return newFinal, true
		}
		if strings.HasPrefix(p, oldPrefix) {
			return newPrefix + p[len(oldPrefix):], true
		}
		return "", false
	}
	remapMapKeys := func(keys []string, move func(oldKey, newKey string)) {
		for _, p := range keys {
			if np, ok := remap(p); ok {
				move(p, np)
			}
		}
	}
	fileKeys := make([]string, 0, len(m.Files))
	for p := range m.Files {
		fileKeys = append(fileKeys, p)
	}
	remapMapKeys(fileKeys, func(o, n string) { m.Files[n] = m.Files[o]; delete(m.Files, o) })
	dirKeys := make([]string, 0, len(m.Dirs))
	for p := range m.Dirs {
		dirKeys = append(dirKeys, p)
	}
	remapMapKeys(dirKeys, func(o, n string) { m.Dirs[n] = true; delete(m.Dirs, o) })
	metaKeys := make([]string, 0, len(m.Meta))
	for p := range m.Meta {
		metaKeys = append(metaKeys, p)
	}
	remapMapKeys(metaKeys, func(o, n string) { m.Meta[n] = m.Meta[o]; delete(m.Meta, o) })
	linkKeys := make([]string, 0, len(m.Links))
	for p := range m.Links {
		linkKeys = append(linkKeys, p)
	}
	remapMapKeys(linkKeys, func(o, n string) { m.Links[n] = m.Links[o]; delete(m.Links, o) })
	// Aliases pointing INTO the moved tree follow their canonical.
	for a, c := range m.Links {
		if nc, ok := remap(c); ok {
			m.Links[a] = nc
		}
	}
	m.emit("rename", oldFinal)
	m.emit("create", newFinal)
	return nil
}

// Symlink creates a symbolic link at linkPath pointing at target (stored
// verbatim). Errors if linkPath already exists.
func (m *MemFileOps) Symlink(target, linkPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	linkFinal, err := m.resolve(linkPath, false)
	if err != nil {
		return err
	}
	if _, ok := m.infoFor(linkFinal); ok {
		return &os.PathError{Op: "symlink", Path: linkPath, Err: os.ErrExist}
	}
	if perr := m.checkParentWrite("symlink", linkPath, linkFinal); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(linkFinal))
	m.Meta[linkFinal] = m.stampOwner(&memMeta{Mode: os.ModeSymlink | 0o777, MTime: m.now(), Target: target})
	m.emit("create", linkFinal)
	return nil
}

// Link creates a hard link at linkPath referring to target. Like link(2),
// the target is NOT symlink-followed: hard-linking a symlink clones the
// link entry, and hard-linking a directory is refused (EPERM). A file
// link records an alias to the canonical path, sharing bytes and metadata.
func (m *MemFileOps) Link(target, linkPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	targetFinal, err := m.resolve(target, false)
	if err != nil {
		return err
	}
	linkFinal, err := m.resolve(linkPath, false)
	if err != nil {
		return err
	}
	info, ok := m.infoFor(targetFinal)
	if !ok {
		return &os.PathError{Op: "link", Path: target, Err: os.ErrNotExist}
	}
	if _, exists := m.infoFor(linkFinal); exists {
		return &os.PathError{Op: "link", Path: linkPath, Err: os.ErrExist}
	}
	if perr := m.checkParentWrite("link", linkPath, linkFinal); perr != nil {
		return perr
	}
	m.recordDir(filepath.Dir(linkFinal))
	switch {
	case info.IsDir:
		return &os.PathError{Op: "link", Path: target, Err: os.ErrPermission}
	case info.Symlink:
		src := m.Meta[targetFinal]
		m.Meta[linkFinal] = &memMeta{Mode: src.Mode, MTime: src.MTime, ATime: src.ATime, Target: src.Target, UID: src.UID, GID: src.GID, OwnerSet: src.OwnerSet, Xattrs: src.Xattrs}
	default:
		m.Links[linkFinal] = m.canon(targetFinal)
	}
	m.emit("create", linkFinal)
	return nil
}

// metaForExisting returns the mutable metadata for an existing path
// (through its canonical name, so hard links share it).
func (m *MemFileOps) metaForExisting(resolved string) (*memMeta, bool) {
	if _, ok := m.infoFor(resolved); !ok {
		return nil, false
	}
	c := m.canon(resolved)
	meta := m.Meta[c]
	if meta == nil {
		meta = &memMeta{}
		m.Meta[c] = meta
	}
	return meta, true
}

// Chmod sets path's permission bits (symlink-followed, like chmod(2)).
func (m *MemFileOps) Chmod(path string, mode os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return &os.PathError{Op: "chmod", Path: path, Err: os.ErrNotExist}
	}
	meta.Mode = mode & os.ModePerm
	m.emit("chmod", final)
	return nil
}

// Chtimes sets path's access and modification times (symlink-followed).
func (m *MemFileOps) Chtimes(path string, atime, mtime time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return &os.PathError{Op: "chtimes", Path: path, Err: os.ErrNotExist}
	}
	meta.MTime = mtime
	meta.ATime = atime
	m.emit("chmod", final)
	return nil
}

// Truncate changes the size of the file at path (grows with zero bytes).
func (m *MemFileOps) Truncate(path string, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return err
	}
	if m.Dirs[final] {
		return &os.PathError{Op: "truncate", Path: path, Err: errMemIsDir}
	}
	c := m.canon(final)
	data, ok := m.Files[c]
	if !ok {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrNotExist}
	}
	if size < 0 {
		return &os.PathError{Op: "truncate", Path: path, Err: os.ErrInvalid}
	}
	if perr := m.checkPerm("truncate", path, final, 0o200); perr != nil {
		return perr
	}
	if int64(len(data)) > size {
		m.Files[c] = data[:size]
	} else {
		grown := make([]byte, size)
		copy(grown, data)
		m.Files[c] = grown
	}
	if meta, ok := m.metaForExisting(final); ok {
		meta.MTime = m.now()
	}
	m.emit("write", final)
	return nil
}

// Chown changes an entry's ownership (lchown when follow=false). The
// kernel rule, faithfully: root re-owns freely; for anyone else an
// actual id change is EPERM (the "owner may hand off to a group they
// belong to" carve-out is NOT modeled — a documented fidelity
// exception), and the -1/-1 no-op succeeds against any accessible path.
func (m *MemFileOps) Chown(path string, uid, gid int, follow bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, follow)
	if err != nil {
		return err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return &os.PathError{Op: "chown", Path: path, Err: os.ErrNotExist}
	}
	if uid == -1 && gid == -1 {
		return nil
	}
	if m.euid() != 0 {
		return &os.PathError{Op: "chown", Path: path, Err: os.ErrPermission}
	}
	// Stamp before the partial update so a -1 arm keeps the CURRENT
	// default identity rather than adopting a later reader's.
	if !meta.OwnerSet {
		m.stampOwner(meta)
	}
	if uid != -1 {
		meta.UID = uid
	}
	if gid != -1 {
		meta.GID = gid
	}
	m.emit("chmod", final)
	return nil
}

// xattrsForWrite returns the mutable attribute map for an existing path,
// enforcing the write bit (the kernel's rule for user.* mutation).
func (m *MemFileOps) xattrsForWrite(op, path string) (map[string][]byte, string, error) {
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, "", err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return nil, "", &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm(op, path, final, 0o200); perr != nil {
		return nil, "", perr
	}
	if meta.Xattrs == nil {
		meta.Xattrs = map[string][]byte{}
	}
	return meta.Xattrs, final, nil
}

// xattrsForRead returns the attribute map (possibly nil) for an existing
// path, enforcing the read bit.
func (m *MemFileOps) xattrsForRead(op, path string) (map[string][]byte, error) {
	final, err := m.resolve(path, true)
	if err != nil {
		return nil, err
	}
	meta, ok := m.metaForExisting(final)
	if !ok {
		return nil, &os.PathError{Op: op, Path: path, Err: os.ErrNotExist}
	}
	if perr := m.checkPerm(op, path, final, 0o400); perr != nil {
		return nil, perr
	}
	return meta.Xattrs, nil
}

// XattrGet reads one extended attribute; an absent one errors with the
// PLATFORM's not-found errno (errXattrAbsent) so mem and OS classify
// identically under the parity harness.
func (m *MemFileOps) XattrGet(path, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	xs, err := m.xattrsForRead("xattr-get", path)
	if err != nil {
		return nil, err
	}
	v, ok := xs[name]
	if !ok {
		return nil, &os.PathError{Op: "xattr-get", Path: path, Err: errXattrAbsent}
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

// XattrSet writes one extended attribute (create or replace).
func (m *MemFileOps) XattrSet(path, name string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	xs, final, err := m.xattrsForWrite("xattr-set", path)
	if err != nil {
		return err
	}
	buf := make([]byte, len(value))
	copy(buf, value)
	xs[name] = buf
	m.emit("chmod", final)
	return nil
}

// XattrList returns the sorted attribute names.
func (m *MemFileOps) XattrList(path string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	xs, err := m.xattrsForRead("xattr-list", path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(xs))
	for name := range xs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// XattrRemove deletes one extended attribute; removing an absent one
// errors like XattrGet.
func (m *MemFileOps) XattrRemove(path, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	xs, final, err := m.xattrsForWrite("xattr-remove", path)
	if err != nil {
		return err
	}
	if _, ok := xs[name]; !ok {
		return &os.PathError{Op: "xattr-remove", Path: path, Err: errXattrAbsent}
	}
	delete(xs, name)
	m.emit("chmod", final)
	return nil
}

// memTempRoot is the synthetic temp root MemFileOps uses when a temp
// call passes dir "" — the mem twin of os.TempDir.
const memTempRoot = "/tmp"

// tempName resolves one unique name for the os.CreateTemp pattern
// contract: a "*" is replaced by a deterministic per-instance counter
// token, otherwise the token is appended. Deterministic on purpose —
// the parity harness compares SHAPE (prefix/suffix/containment), never
// the random token itself.
func (m *MemFileOps) tempName(dir, pattern string) (string, error) {
	if dir == "" {
		dir = memTempRoot
	}
	resolved, _ := m.ResolvePath(dir) // never fails — see ReadFile
	// os.CreateTemp/MkdirTemp raise ENOENT for an absent parent — mirror
	// that (the temp root itself is implicit, like "." and "/").
	if fi, ok := m.infoFor(resolved); resolved != filepath.Clean(memTempRoot) && !isMemRoot(resolved) {
		if !ok {
			return "", &os.PathError{Op: "createtemp", Path: dir, Err: os.ErrNotExist}
		}
		if !fi.IsDir {
			return "", &os.PathError{Op: "createtemp", Path: dir, Err: errMemNotDir}
		}
	}
	for {
		m.tempSeq++
		token := fmt.Sprintf("%d", m.tempSeq)
		name := pattern + token
		if i := strings.LastIndexByte(pattern, '*'); i >= 0 {
			name = pattern[:i] + token + pattern[i+1:]
		}
		candidate := filepath.Join(resolved, name)
		if _, exists := m.infoFor(candidate); !exists {
			return candidate, nil
		}
	}
}

// TempFile creates a unique file (0600) under dir, per the
// os.CreateTemp pattern contract.
func (m *MemFileOps) TempFile(dir, pattern string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.tempName(dir, pattern)
	if err != nil {
		return "", err
	}
	if err := m.writeFileLocked(path, []byte{}, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// TempDir creates a unique directory (0700) under dir.
func (m *MemFileOps) TempDir(dir, pattern string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.tempName(dir, pattern)
	if err != nil {
		return "", err
	}
	if err := m.mkdirAllLocked(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// Statfs reports a SYNTHETIC fixed-size volume: 1 GiB total, usage the
// sum of file contents, a 4096 block size, type "mem". The path must
// exist (statfs(2) semantics); the mem root ("." / "/" and the temp
// root's ancestors) always does.
func (m *MemFileOps) Statfs(path string) (FsInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	final, err := m.resolve(path, true)
	if err != nil {
		return FsInfo{}, err
	}
	if _, ok := m.infoFor(final); !ok && !isMemRoot(final) {
		return FsInfo{}, &os.PathError{Op: "statfs", Path: path, Err: os.ErrNotExist}
	}
	const total = uint64(1) << 30
	var used uint64
	for _, data := range m.Files {
		used += uint64(len(data))
	}
	free := total - min(used, total)
	return FsInfo{TotalBytes: total, FreeBytes: free, AvailBytes: free, BlockSize: 4096, Type: "mem"}, nil
}

func (m *MemFileOps) ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	base := m.Cwd
	if base == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, path)), nil
}

// EnvOps is the host's view of the process environment, read-only.
//
// It is a capability rather than a direct os.Getenv call for the same reason
// FileOps is: the runtime must never reach for ambient process state on its
// own. A hermetic spec runner installs nothing and sees an empty environment;
// an embedded host can install a filtered view exposing an allowlist; the CLI
// installs the real one. Nothing about `IO.env` changes between those cases
// except what the host chose to hand it.
//
// Read-only by design: there is no SetEnv. Mutating one's own environment is
// almost always a smell, and the real use case -- giving a child process a
// different environment -- belongs to whatever spawns the child, which can
// take it explicitly.
type EnvOps interface {
	// Get returns the value and whether the name is set. An empty string is
	// a real value, distinct from unset, so the bool is load-bearing.
	Get(name string) (string, bool)
	// All returns every visible name, in no guaranteed order.
	All() []string
}

// OSEnvOps is the production EnvOps: the real process environment.
type OSEnvOps struct{}

func (OSEnvOps) Get(name string) (string, bool) { return os.LookupEnv(name) }

func (OSEnvOps) All() []string {
	pairs := os.Environ()
	names := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			names = append(names, kv[:i])
		}
	}
	return names
}

// MapEnvOps is a fixed EnvOps over a map — the deterministic fake for tests
// and specs, mirroring FixedClock's role for Clock. It is what makes the
// host-dependent half of IO.env testable at all, which ADR-008 requires.
type MapEnvOps struct{ Vars map[string]string }

func (m MapEnvOps) Get(name string) (string, bool) {
	v, ok := m.Vars[name]
	return v, ok
}

func (m MapEnvOps) All() []string {
	names := make([]string, 0, len(m.Vars))
	for k := range m.Vars {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// StreamProbe is the host capability that answers "is this stream a
// terminal?" for the boru:io `tty` word.
//
// It is a capability rather than a direct isatty call for the reason every
// other one here is: the runtime must not decide by itself what the outside
// world looks like. A hermetic spec runner installs nothing and every stream
// answers false; the CLI installs the real probe; a host that redirected a
// stream somewhere unusual can answer for itself.
//
// Absence means FALSE, not an error. A program asking whether it is talking
// to a terminal is choosing between two valid behaviours (colour or not,
// prompt or not), so "no" is a usable answer and an error would force every
// caller to handle a failure that is not one.
type StreamProbe interface {
	// IsTerminal reports whether the named stream — "stdin", "stdout" or
	// "stderr" — is a terminal.
	//
	// endpoint is the io.Reader or io.Writer the runtime currently holds for
	// that stream. Both arguments are supplied because the two kinds of
	// implementation want different ones: an OS-backed probe inspects the
	// endpoint (and so tells the truth about a redirection the host applied,
	// for free), while a scripted fake answers from the name without caring
	// what the endpoint is.
	IsTerminal(stream string, endpoint any) bool
}

// OSStreamProbe is the production StreamProbe: it asks the operating system
// about the endpoint the runtime actually holds. The test is the same one
// native.ResolveColor uses for its auto-colour decision — a character device —
// so `tty` and automatic colouring can never disagree about a stream.
//
// A non-*os.File endpoint (a buffer, a pipe wrapped in a custom type) is not
// a terminal, which is the honest answer: the host redirected it.
type OSStreamProbe struct{}

func (OSStreamProbe) IsTerminal(_ string, endpoint any) bool {
	f, ok := endpoint.(*os.File)
	if !ok {
		return false
	}
	// term.IsTerminal, not os.ModeCharDevice. A character device is not the
	// same question: /dev/null, /dev/zero and /dev/urandom are all character
	// devices, so the mode test answered "terminal" for the single most common
	// redirection there is. `prog > /dev/null` would then take the colour
	// branch, which is precisely the case --color=auto exists to avoid.
	// IsTerminal asks the kernel (a termios ioctl) instead of guessing from
	// the file type.
	return term.IsTerminal(int(f.Fd()))
}

// FixedStreamProbe is a scripted StreamProbe — the deterministic fake, and
// the only way the "yes, a terminal" arm is reachable in a test suite that
// runs with its streams redirected. Names absent from the map answer false.
type FixedStreamProbe struct{ Terminal map[string]bool }

func (p FixedStreamProbe) IsTerminal(stream string, _ any) bool {
	return p.Terminal[stream]
}
