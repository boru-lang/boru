package native

import (
	"io"
	"os"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// permissionedFileOps wraps a FileOps implementation, gating each
// method through a Policy. The wrapper is installed automatically
// by SetHostFileOps when a Policy is present on the registry.
//
// Each method:
//  1. Consults the relevant global hard cap (disk.read / disk.write).
//  2. Consults the fileops scope rule for the op with the path arg.
//  3. Delegates to the inner FileOps on allow.
//
// Returns the *policy.Denied error verbatim on compile failure so callers
// can pattern-match on Code.
type permissionedFileOps struct {
	inner  capabilities.FileOps
	policy policy.Policy
}

// NewPermissionedFileOps wraps ops with a permissions gate. If pol
// is nil, the input is returned unchanged.
func NewPermissionedFileOps(ops capabilities.FileOps, pol policy.Policy) capabilities.FileOps {
	if pol == nil {
		return ops
	}
	return &permissionedFileOps{inner: ops, policy: pol}
}

// ReadFile gates on global.disk.read + fileops.read{path}.
func (p *permissionedFileOps) ReadFile(path string) ([]byte, error) {
	if err := p.policy.Check("fileops", "read", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.ReadFile(path)
}

// WriteFile gates on global.disk.write + fileops.write{path,bytes}.
func (p *permissionedFileOps) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := p.policy.Check("fileops", "write", policy.Args{
		"path":  path,
		"bytes": int64(len(data)),
	}); err != nil {
		return err
	}
	return p.inner.WriteFile(path, data, perm)
}

// MkdirAll gates on global.disk.write + fileops.mkdir{path}.
func (p *permissionedFileOps) MkdirAll(path string, perm os.FileMode) error {
	if err := p.policy.Check("fileops", "mkdir", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.MkdirAll(path, perm)
}

// Stat gates on fileops.stat{path} (a read-like op).
func (p *permissionedFileOps) Stat(path string, follow bool) (capabilities.FileInfo, error) {
	if err := p.policy.Check("fileops", "stat", policy.Args{"path": path}); err != nil {
		return capabilities.FileInfo{}, err
	}
	return p.inner.Stat(path, follow)
}

// ReadDir gates on fileops.list{path} (a read-like op).
func (p *permissionedFileOps) ReadDir(path string) ([]capabilities.FileInfo, error) {
	if err := p.policy.Check("fileops", "list", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.ReadDir(path)
}

// Remove gates on fileops.remove{path} (a write-like op).
func (p *permissionedFileOps) Remove(path string, recursive bool) error {
	if err := p.policy.Check("fileops", "remove", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.Remove(path, recursive)
}

// Rename gates on fileops.rename{path,to} (a write-like op).
func (p *permissionedFileOps) Rename(oldPath, newPath string) error {
	if err := p.policy.Check("fileops", "rename", policy.Args{"path": oldPath, "to": newPath}); err != nil {
		return err
	}
	return p.inner.Rename(oldPath, newPath)
}

// Symlink gates on fileops.link{path,to} (a write-like op).
func (p *permissionedFileOps) Symlink(target, linkPath string) error {
	if err := p.policy.Check("fileops", "link", policy.Args{"path": linkPath, "to": target}); err != nil {
		return err
	}
	return p.inner.Symlink(target, linkPath)
}

// Link gates on fileops.link{path,to} (a write-like op).
func (p *permissionedFileOps) Link(target, linkPath string) error {
	if err := p.policy.Check("fileops", "link", policy.Args{"path": linkPath, "to": target}); err != nil {
		return err
	}
	return p.inner.Link(target, linkPath)
}

// Chmod gates on fileops.chmod{path} (a write-like op).
func (p *permissionedFileOps) Chmod(path string, mode os.FileMode) error {
	if err := p.policy.Check("fileops", "chmod", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.Chmod(path, mode)
}

// Chtimes gates on fileops.chmod{path} (a write-like metadata op).
func (p *permissionedFileOps) Chtimes(path string, atime, mtime time.Time) error {
	if err := p.policy.Check("fileops", "chmod", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.Chtimes(path, atime, mtime)
}

// Truncate gates on fileops.write{path} (it mutates file content).
func (p *permissionedFileOps) Truncate(path string, size int64) error {
	if err := p.policy.Check("fileops", "write", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.Truncate(path, size)
}

// Chown gates on fileops.chmod{path} — a write-like metadata mutation,
// same class as Chmod/Chtimes, so existing policy profiles keep their
// meaning without a new op name.
func (p *permissionedFileOps) Chown(path string, uid, gid int, follow bool) error {
	if err := p.policy.Check("fileops", "chmod", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.Chown(path, uid, gid, follow)
}

// XattrGet gates on fileops.stat{path} (read-like metadata).
func (p *permissionedFileOps) XattrGet(path, name string) ([]byte, error) {
	if err := p.policy.Check("fileops", "stat", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.XattrGet(path, name)
}

// XattrSet gates on fileops.chmod{path} (write-like metadata).
func (p *permissionedFileOps) XattrSet(path, name string, value []byte) error {
	if err := p.policy.Check("fileops", "chmod", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.XattrSet(path, name, value)
}

// XattrList gates on fileops.stat{path} (read-like metadata).
func (p *permissionedFileOps) XattrList(path string) ([]string, error) {
	if err := p.policy.Check("fileops", "stat", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.XattrList(path)
}

// XattrRemove gates on fileops.chmod{path} (write-like metadata).
func (p *permissionedFileOps) XattrRemove(path, name string) error {
	if err := p.policy.Check("fileops", "chmod", policy.Args{"path": path}); err != nil {
		return err
	}
	return p.inner.XattrRemove(path, name)
}

// TempFile / TempDir gate on fileops.write{path} (they create entries).
// The gate argument is the DIRECTORY the entry lands in ("" = the
// backend temp root, gated as that literal token).
func (p *permissionedFileOps) TempFile(dir, pattern string) (string, error) {
	if err := p.policy.Check("fileops", "write", policy.Args{"path": dir}); err != nil {
		return "", err
	}
	return p.inner.TempFile(dir, pattern)
}

func (p *permissionedFileOps) TempDir(dir, pattern string) (string, error) {
	if err := p.policy.Check("fileops", "write", policy.Args{"path": dir}); err != nil {
		return "", err
	}
	return p.inner.TempDir(dir, pattern)
}

// Statfs gates on fileops.stat{path} (read-like metadata).
func (p *permissionedFileOps) Statfs(path string) (capabilities.FsInfo, error) {
	if err := p.policy.Check("fileops", "stat", policy.Args{"path": path}); err != nil {
		return capabilities.FsInfo{}, err
	}
	return p.inner.Statfs(path)
}

// ResolvePath is pure path manipulation — no I/O, no policy check.
// Resolution must succeed even for paths that read/write would
// later deny, so consumers can compute the canonical form before
// hitting a gated method.
func (p *permissionedFileOps) ResolvePath(path string) (string, error) {
	return p.inner.ResolvePath(path)
}

// compile-time interface check.
// Watch gates on fileops.watch{path} (read-like: it observes, never
// mutates), then delegates the live stream to the inner FileOps.
func (p *permissionedFileOps) Watch(path string, opts capabilities.WatchOpts) (<-chan capabilities.WatchEvent, func() error, error) {
	if err := p.policy.Check("fileops", "watch", policy.Args{"path": path}); err != nil {
		return nil, nil, err
	}
	return p.inner.Watch(path, opts)
}

// Open gates by intent: a read-only handle is "read", any write intent
// (write/append/create/truncate) is "write" — the same two names the
// stateless read/write ops use, so existing profiles keep their meaning.
func (p *permissionedFileOps) Open(path string, opts capabilities.OpenOpts) (capabilities.FileHandle, error) {
	op := "read"
	if opts.Write || opts.Append || opts.Create || opts.Truncate {
		op = "write"
	}
	if err := p.policy.Check("fileops", op, policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.Open(path, opts)
}

// Lock gates on fileops.write (an advisory lock guards a mutation).
func (p *permissionedFileOps) Lock(path string, shared, block bool) (io.Closer, error) {
	if err := p.policy.Check("fileops", "write", policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.Lock(path, shared, block)
}

// Mmap gates by intent: a writable mapping is "write", read-only "read".
func (p *permissionedFileOps) Mmap(path string, offset int64, length int, writable bool) (capabilities.MmapRegion, error) {
	op := "read"
	if writable {
		op = "write"
	}
	if err := p.policy.Check("fileops", op, policy.Args{"path": path}); err != nil {
		return nil, err
	}
	return p.inner.Mmap(path, offset, length, writable)
}

var _ capabilities.FileOps = (*permissionedFileOps)(nil)
