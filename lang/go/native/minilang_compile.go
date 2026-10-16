package native

import (
	"sync"

	core "github.com/boru-lang/boru/core/go"
)

// Compiled mini-languages (design/MINILANG.5.md §13). A kind may register an
// expansion-time COMPILE HOOK alongside its runtime transducer. When `mini
// <kind> <literal-src>` is expanded, the hook runs ONCE at the call site and
// returns the token list `mini` splices instead of the standard
// `MiniLang.lang_<kind> src opts end` call — so the DSL is parsed ahead of
// time (syntax errors surface with the call-site span) and the per-call cost
// drops to consuming a precompiled carrier (or inlined native ops).
//
// The transducer is never replaced: it is the semantic reference, the
// checker's target, and the fallback whenever src is not concrete. `mini` has
// no expansion cache, so a hook re-runs every time the call is stepped — a
// hook memoizes its own compile (as `re` does via miniCompiledPattern).
//
// Hooks are BUILT-IN (Go-only) machinery: they live in a per-registry table
// (RegisterMiniCompileGoHook) keyed by kind, discovered by miniHandler via
// miniGoHook. The boru hook surface (MiniLang.register-compiled) died with
// the frozen kind namespace, and the kind set is fixed — so a hook can only
// belong to a built-in kind (`re` is the shipping example).

// MiniCompileHook is a Go expansion-time compiler: given the src and the
// (possibly deferred) opts value, it returns the tokens to splice.
type MiniCompileHook func(src string, opts Value, r *Registry) ([]Value, error)

const capMiniCompile = "engine.minilang.compile"

type miniCompileState struct {
	mu      sync.Mutex
	goHooks map[string]MiniCompileHook
}

func miniCompileStateFor(r *Registry, create bool) *miniCompileState {
	if s, ok, _ := core.Cap[*miniCompileState](r, capMiniCompile); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &miniCompileState{goHooks: map[string]MiniCompileHook{}}
	_ = r.Capabilities.Set(capMiniCompile, s)
	return s
}

// RegisterMiniCompileGoHook installs a Go compile hook for kind on r.
func RegisterMiniCompileGoHook(r *Registry, kind string, hook MiniCompileHook) {
	s := miniCompileStateFor(r, true)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goHooks[kind] = hook
}

// miniGoHook returns kind's Go compile hook, if registered.
func miniGoHook(r *Registry, kind string) (MiniCompileHook, bool) {
	s := miniCompileStateFor(r, false)
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.goHooks[kind]
	return h, ok
}
