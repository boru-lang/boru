package native

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_watch.go — the boru:io filesystem watcher: `IO.watch path [body]`
// subscribes to change events on a Pathon (a file, or a directory's
// direct children — non-recursive, inotify semantics) and runs the body
// once per event, PUSH style, exactly like boru:time-util's `interval`
// runs its callback per tick. The body executes on a concurrent fork of
// the registry with the EVENT RECORD on the stack:
//
//	{op: create/q|write/q|remove/q|rename/q|chmod/q, path: <Pathon>}
//
// `IO.unwatch w` stops the watcher and closes the stream. The backend
// is whatever FileOps is effective at watch time: fsnotify on the real
// filesystem, the in-memory event source under __sys.fs.mem, and the
// union of both layers under __sys.fs.overlay — so watch behaviour is
// part of the same real/mem parity surface as every other io word.

// WatcherInfo is the payload behind a Watcher handle (carried via
// ExtensionPayload — a module-scoped resource, like Timeout/Interval
// but without a global FixedID).
type WatcherInfo struct {
	ID   string
	Path string
	stop func() error
	once sync.Once
	// done closes when the callback pump exits (the event stream
	// closed) — tests use it to join deterministically.
	done chan struct{}
}

// Stop releases the underlying watch exactly once (idempotent, like
// fsnotify's Close) and reports the release error, if any.
func (wi *WatcherInfo) Stop() error {
	var err error
	wi.once.Do(func() { err = wi.stop() })
	return err
}

// Done exposes the pump-exit join point for tests.
func (wi *WatcherInfo) Done() <-chan struct{} { return wi.done }

// watcherFormatBehavior renders a Watcher as "Watcher(id,path)".
type watcherFormatBehavior struct{}

func (watcherFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (watcherFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (watcherFormatBehavior) Format(v Value) string {
	if wi, ok := asWatcherInfo(v); ok {
		return fmt.Sprintf("Watcher(%s,%s)", wi.ID, wi.Path)
	}
	return "Watcher(nil)"
}

// MintWatcherType mints the module-scoped Watcher resource type —
// per-import, like StreamKind/FileType, never a global builtin.
func MintWatcherType(r *Registry) *Type {
	return r.Types.MintTypeWithBehavior("Watcher", core.TIdeal, watcherFormatBehavior{})
}

// NewWatcherType mints a standalone Watcher type (into its own dynamic
// type table) for test helpers that register the io words under bare
// names without a host registry to mint into — the NewStreamKind /
// NewFileType twin. Production code mints per import via MintWatcherType.
func NewWatcherType() *Type {
	return core.NewDynamicTypeTable().MintTypeWithBehavior("Watcher", core.TIdeal, watcherFormatBehavior{})
}

// asWatcherInfo unwraps a Watcher handle's payload.
func asWatcherInfo(v Value) (*WatcherInfo, bool) {
	ext, ok := v.Data.(ExtensionPayload)
	if !ok {
		return nil, false
	}
	wi, ok := ext.Body.(*WatcherInfo)
	return wi, ok
}

// watchEventValue builds the boru event record for one WatchEvent.
func watchEventValue(op, path string) Value {
	om := NewOrderedMap()
	om.Set("op", NewAtom(op))
	om.Set("path", NewPathonFromString(path))
	return NewMap(om)
}

// watchRel returns evPath relative to the watch root for glob matching
// (so {match:"**/*.txt"} spans components under {recursive}); it falls back
// to the base name when evPath is the root itself or lies outside it.
func watchRel(root, evPath string) string {
	rel, err := filepath.Rel(root, evPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return filepath.Base(evPath)
	}
	return rel
}

// doWatchWord implements IO.watch: subscribe via the effective FileOps,
// then pump events into per-event body runs on a concurrent fork. The
// body list is captured unevaluated (NoEvalArgs); each run gets the
// event record pushed beneath it, so `[.path print]`-style bodies read
// the event off the stack. opts (when concrete) supplies {recursive:true}
// (subtree watch, passed to the backend) and {match:"glob"} (a word-layer
// filter on each event's path relative to the root; the coalesced overflow
// marker always bypasses the filter so a drop is never hidden).
func doWatchWord(args []Value, r *Registry, watcherType *Type, opts Value) ([]Value, error) {
	path := extractPath(args[0])
	body := args[1]
	if !IsConcrete(&body) {
		return nil, r.BoruError("watch_error", "watch: the callback body must be a concrete list", "watch")
	}
	bodyList, _ := AsList(body)
	tokens := make([]Value, bodyList.Len())
	copy(tokens, bodyList.Slice())

	recursive, match, hasMatch := false, "", false
	if IsConcrete(&opts) {
		recursive = mapBoolOpt(opts, "recursive", false)
		match, hasMatch = mapStrOpt(opts, "match")
	}

	ch, stop, err := EffectiveFileOps(r).Watch(path, capabilities.WatchOpts{Recursive: recursive})
	if err != nil {
		return nil, r.BoruError("watch_error", fmt.Sprintf("watch: %v", err), "watch")
	}
	info := &WatcherInfo{ID: GenerateID("W_"), Path: path, stop: stop, done: make(chan struct{})}
	// Fork now, on the subscribing goroutine, so every event runs the
	// body on an isolated registry and never races the main interpreter
	// — the same pattern interval's ticker uses.
	fork := r.ForkConcurrent()
	go func() {
		defer close(info.done)
		for ev := range ch {
			if hasMatch && ev.Op != watchOverflowOp && !policy.Glob(match, watchRel(path, ev.Path)) {
				continue
			}
			input := make([]Value, 0, len(tokens)+1)
			input = append(input, watchEventValue(ev.Op, ev.Path))
			input = append(input, tokens...)
			sub := New(fork)
			// Execute and discard results — watch callbacks run for
			// side effects, like timer callbacks.
			_, _ = sub.Run(input)
		}
	}()
	return []Value{core.NewValueRaw(watcherType, ExtensionPayload{Body: info})}, nil
}

// watchOverflowOp names the coalesced buffer-overflow marker event — it must
// never be glob-filtered, so a slow consumer always learns it missed events.
const watchOverflowOp = "overflow"

// doUnwatchWord implements IO.unwatch: stop the watcher (idempotent).
func doUnwatchWord(args []Value, r *Registry) ([]Value, error) {
	wi, ok := asWatcherInfo(args[0])
	if !ok {
		return nil, r.BoruError("unwatch_error", fmt.Sprintf("unwatch: not a Watcher handle (got %s)", args[0].Parent), "unwatch")
	}
	if err := wi.Stop(); err != nil {
		return nil, r.BoruError("unwatch_error", fmt.Sprintf("unwatch: %v", err), "unwatch")
	}
	return nil, nil
}
