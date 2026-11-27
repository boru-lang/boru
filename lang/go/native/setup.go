package native

import (
	"fmt"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// DefaultRegistry returns a registry populated with every built-in
// boru word, plus any additional provider functions passed in. Each
// provider is a function that registers extra words.
//
// Post engine→native consolidation (May 2026) there is a single
// built-in provider — native.Register — and it is invoked
// unconditionally; callers no longer have to pass it explicitly. The
// variadic surface is preserved for host extensions.
//
// This is the entry point that wires the host-side capabilities
// (FileOps, format registry, SQLite store) onto the bare
// core.Registry returned by core.NewRegistry. borueng itself knows
// nothing about these — it just stores them in opaque capability slots
// for the host's word handlers to retrieve.
//
// To apply a permissions policy, use DefaultRegistryWithPolicy: the
// policy must be installed before host capabilities so SetHostX
// hooks can consult it for install:false decisions and (post-Phase 3)
// auto-wrapping.
func DefaultRegistry(providers ...func(*Registry)) (*Registry, error) {
	return DefaultRegistryWithPolicy(nil, providers...)
}

// DefaultRegistryWithPolicy is like DefaultRegistry but installs a
// permissions policy before host capabilities. Pass nil for p to
// mean "no policy" (allow-everything, the historical default).
func DefaultRegistryWithPolicy(p policy.Policy, providers ...func(*Registry)) (*Registry, error) {
	// Surface any init-time external-type registration error (recorded at
	// package import instead of panicking — see typeinit.go / ADR-005).
	if err := TypeInitError(); err != nil {
		return nil, err
	}
	r, err := newEngRegistry()
	if err != nil {
		return nil, err
	}

	// Install the policy first so subsequent SetHostX hooks can
	// consult it (e.g. skip-install when install:false, wrap with
	// the permissioned variant once Phase 3 lands).
	if p != nil {
		SetHostPolicy(r, p)
	}

	// Default file operations: OS-backed.
	ops := capabilities.NewDefault()
	SetHostFileOps(r, ops)

	// Default format registry, with the jsonic resolver pointed at
	// the active fileops so @"path" references resolve.
	formats := DefaultFormats()
	if jf, ok := formats["jsonic"].(*JsonicFormat); ok {
		jf.Resolver = MakeFileOpsResolver(ops)
	}
	SetHostFormats(r, formats)
	SetHostExtensions(r, DefaultExtensions())

	// The shared stdin reader is installed EAGERLY, and both reasons matter.
	//
	// It cannot be created lazily on first IO.read-line: the capability store
	// is an unsynchronised Go map (core/go/capability.go:54-71) whose pointer
	// ForkConcurrent shares across goroutines, so a lazy Set would race a
	// concurrent Get — a fatal "concurrent map writes", not a recoverable one.
	//
	// And it must exist before any `import`: RunModuleBody snapshots the
	// ModuleInheritedCaps slots at IMPORT time (native_module_module.go), so a
	// holder created later is not inherited, and a module body that read stdin
	// first would open a SECOND buffer over the importer's reader and eat its
	// bytes. Existing at construction makes the snapshot copy the pointer.
	installStdinLines(r)

	// Default SQLite store.
	sqlStore, err := newSQLiteStoreFn()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}
	SetHostSQLite(r, sqlStore)

	// Register engine-bundled words and any caller-supplied providers.
	Register(r)
	for _, p := range providers {
		p(r)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	r.InitRootContext()
	return r, nil
}
