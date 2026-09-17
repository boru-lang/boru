package core

import (
	"io"
	"sync"
)

// Concurrency model for forked registries.
//
// The interpreter's Registry is single-threaded shared state: the step
// loop constantly pushes and pops the def table, the context stack, and
// the args stack. Running two engines that share one Registry on
// separate goroutines is therefore a data race (concurrent map / slice
// mutation = undefined behaviour in Go).
//
// The concurrent words (`await`, `timeout`, `interval`) need real
// parallelism, so each concurrent execution gets its OWN registry via
// ForkConcurrent: an isolated copy that shares the read-mostly
// infrastructure (the builtin lattice, ideals, capabilities, modules,
// native registrations, parser, host hooks) but gives the goroutine its
// own value-binding, context, args, and flow-control scopes. Concurrent
// branches then never touch each other's mutable stacks.
//
// Caller contract: ForkConcurrent reads the parent's def table, context
// stack, and dynamic type table to seed the clone, so it must be called
// while the parent is NOT concurrently mutating those — i.e. on the
// goroutine that owns the parent registry. `await` forks up-front on the
// dispatching goroutine before spawning branches; the timer words fork
// at schedule time (also on the owning goroutine), not when the timer
// fires. Output/ErrOutput remain shared, so callers running several
// forks at once should wrap the writer in a SyncWriter first.

// ForkConcurrent returns an isolated registry safe to run on its own
// goroutine concurrently with the parent and with sibling forks. It
// shares the parent's read-only infrastructure but isolates every
// mutable execution scope. The fork observes the parent's bindings and
// context as of the fork call; its later writes stay private.
func (r *Registry) ForkConcurrent() *Registry {
	fork := *r // shallow copy inherits the shared read-only pointers
	fork.Defs = r.Defs.Clone()
	fork.Types = r.Types.CloneDynamic()
	// builtinWords is read-MOSTLY, not read-only: a host Register call
	// on the parent ((*Boru).Register) writes it while a fork's failure
	// path may be ITERATING it for did-you-mean candidates
	// (RegisteredWordNames via SuggestionCandidates) — a fatal
	// concurrent map fault. Snapshot it here (on the parent-owning
	// goroutine, like the Defs clone) so the fork's view is immutable;
	// words the host registers after the fork simply don't appear in
	// the fork's suggestions.
	bw := make(map[string]bool, len(r.builtinWords))
	for name := range r.builtinWords {
		bw[name] = true
	}
	fork.builtinWords = bw
	// sugarWords is read-only after registration, but snapshot it like
	// builtinWords so a parent Register call can never race a fork read.
	sw := make(map[SugarKind]string, len(r.sugarWords))
	for k, v := range r.sugarWords {
		sw[k] = v
	}
	fork.sugarWords = sw
	fork.Contexts = NewContextStack()
	// A private child layer over the inherited context chain: reads walk
	// down into the (parked) parent's stores, writes land in the child.
	fork.Contexts.Push(r.Contexts.Top())
	fork.Args = NewArgsStack()
	fork.FnBaselines = nil
	// debugEngines is the live-engine stack for on-demand Debug.stack
	// introspection (registry.go). It is mutable per-Run state, so the fork
	// must start with an empty stack of its own. Without this reset the shallow
	// copy inherits the parent's slice header — and because the parent is
	// mid-Run at fork time (its backing array has live entries and spare
	// capacity), sibling forks would each append into the SHARED backing array,
	// a data race. Reset alongside Args/FnBaselines for the same reason.
	fork.debugEngines = nil
	// enginePool engines pin their creating registry (Engine.registry
	// points at the PARENT), so the fork must start with an empty pool of
	// its own — inheriting the slice would run fork work against the
	// parent's registry and race with it.
	fork.enginePool = nil
	// dispatchCache is keyed by name against the PARENT DefTable's
	// generation; the fork clones Defs (fresh generations), so sharing
	// the parent's holder would serve parent aggregates at mismatched
	// generations (and contend on its mutex). Give the fork a fresh,
	// empty holder of its own; the reassignment happens here, on the
	// parent-owning goroutine, before the fork is published.
	fork.dispatchCache = newDispatchCache()
	fork.FlowCtrl = FlowNone
	fork.pendingGen = nil
	fork.macroCache = nil
	fork.errs = nil
	fork.SDKCache = make(map[string]any)
	fork.VmRunning = 0 // the fork starts idle, independent of the parent's run
	// The fork is an instance of the parent's module, never a module of its
	// own: a fn the parent minted runs on the fork when invoked there (FnHome).
	fork.home = r.Home()
	return &fork
}

// Home returns the canonical registry of the module r is an instance of —
// r itself unless r is a concurrent fork, in which case the registry it was
// forked from (transitively). Two registries with the same Home are the same
// module: a fn minted in one is at home in the other.
func (r *Registry) Home() *Registry {
	if r != nil && r.home != nil { //sentinel:home the one reading of a nil home: r is its own canonical registry
		return r.home
	}
	return r
}

// SameHome reports whether r and other are instances of the same module.
func (r *Registry) SameHome(other *Registry) bool {
	return r.Home() == other.Home()
}

// IsModule reports whether r is an instance of a MODULE — a registry a
// resolved import gave a stable, policy-addressable id (ModuleRef:
// "boru:time-util", "./lib.boru"), or a concurrent fork of one (the fork
// copies the id, and the per-export policy that keys on it applies on the
// fork exactly as on the module). The main program's registry, a fork of it,
// an inline `module […]` body (no stable id) and a sandbox all answer false.
// Nil-safe, because the VM reads it through a compiled unit's OWNING
// registry, which is nil for a unit that runs on the program's own.
func (r *Registry) IsModule() bool {
	return r != nil && r.ModuleRef != "" //sentinel:home the one reading of an empty ModuleRef: not a module
}

// SyncWriter serializes concurrent Write calls so that several forked
// registries sharing one underlying writer (e.g. os.Stdout, or a test
// buffer) do not race or interleave a single write. Wrap the parent's
// Output once and assign the wrapper to each fork before spawning.
type SyncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewSyncWriter wraps w so concurrent writes are mutually excluded.
func NewSyncWriter(w io.Writer) *SyncWriter {
	return &SyncWriter{w: w}
}

// Write satisfies io.Writer under a mutex.
func (s *SyncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Unwrap returns the writer this SyncWriter serializes. A caller that must
// ASK something about the destination rather than write to it — is it a
// terminal? — needs the thing underneath, because the wrapper is an
// engine-side concurrency detail the program never chose. Without this a
// terminal probe answers "not a terminal" for a real terminal as soon as the
// program runs inside a fork, silently.
func (s *SyncWriter) Unwrap() io.Writer { return s.w }
