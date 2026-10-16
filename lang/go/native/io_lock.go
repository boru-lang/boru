package native

import (
	"errors"
	"fmt"
	"io"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_lock.go — advisory file locks. IO.lock acquires a Lock resource (an
// io.Closer behind an ExtensionPayload, the Watcher/File pattern); a
// non-blocking {block:false} call on a held lock returns `none` rather
// than a Lock. IO.unlock releases it, and the polymorphic IO.close
// covers it too.

// LockInfo is the payload behind a Lock handle: a live advisory lock
// released exactly once.
type LockInfo struct {
	ID       string
	Path     string
	Shared   bool
	closer   io.Closer
	once     sync.Once
	closeErr error
}

// Release drops the lock exactly once and reports the release error.
func (li *LockInfo) Release() error {
	li.once.Do(func() { li.closeErr = li.closer.Close() })
	return li.closeErr
}

// lockFormatBehavior renders a Lock as "Lock(id,path)".
type lockFormatBehavior struct{}

func (lockFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (lockFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (lockFormatBehavior) Format(v Value) string {
	if li, ok := asLockInfo(v); ok {
		return fmt.Sprintf("Lock(%s,%s)", li.ID, li.Path)
	}
	return "Lock(nil)"
}

// MintLockType / NewLockType mint the module-scoped Lock resource type
// (per-import, like Watcher/File).
func MintLockType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("Lock", core.TIdeal, lockFormatBehavior{})
}
func NewLockType() *Type {
	return core.NewDynamicTypeTable().MintTypeWithBehavior("Lock", core.TIdeal, lockFormatBehavior{})
}

// asLockInfo unwraps a Lock handle's payload.
func asLockInfo(v Value) (*LockInfo, bool) {
	ext, ok := v.Data().(ExtensionPayload)
	if !ok {
		return nil, false
	}
	li, ok := ext.Body.(*LockInfo)
	return li, ok
}

// doLockWord implements IO.lock p {shared, block}: acquire an advisory
// lock. {shared:true} is a read lock (default exclusive); {block:false}
// returns `none` on contention rather than waiting (default blocking).
func doLockWord(args []Value, r *Registry, lockType *Type) ([]Value, error) {
	path := extractPath(args[0])
	shared, block := false, true
	if len(args) > 1 {
		shared = mapBoolOpt(args[1], "shared", false)
		block = mapBoolOpt(args[1], "block", true)
	}
	r.NoteEffect()
	closer, err := EffectiveFileOps(r).Lock(path, shared, block)
	if err != nil {
		if errors.Is(err, capabilities.ErrLockBusy) {
			// Non-blocking contention → none, not an error.
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return nil, r.BoruError("lock_error", fmt.Sprintf("lock: %v", err), "lock")
	}
	info := &LockInfo{ID: GenerateID("L_"), Path: path, Shared: shared, closer: closer}
	return []Value{core.NewValueRaw(lockType, ExtensionPayload{Body: info})}, nil
}

// doUnlockWord implements IO.unlock l: release an advisory lock.
func doUnlockWord(args []Value, r *Registry) ([]Value, error) {
	li, ok := asLockInfo(args[0])
	if !ok {
		return nil, r.BoruError("unlock_error", fmt.Sprintf("unlock: not a Lock handle (got %s)", args[0].Parent()), "unlock")
	}
	if err := li.Release(); err != nil {
		return nil, r.BoruError("unlock_error", fmt.Sprintf("unlock: %v", err), "unlock")
	}
	return nil, nil
}
