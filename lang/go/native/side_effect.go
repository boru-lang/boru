package native

import core "github.com/boru-lang/boru/core/go"

// side_effect.go — which native words have an observable effect
// (core.CompileSideEffect, NUR356).
//
// A native word that returns a value is taken to be QUIET unless it
// declares the flag: the same value, and nothing else, whenever it runs. The
// compiler relies on that where it runs a word earlier than the interpreter
// would — a list or map literal a call matched at run time takes is
// assembled before the match, where the interpreter evaluates it only once a
// signature takes it — so every word whose run the program or its host can
// observe must say so: a read or write of the world outside the run (files,
// the environment, stdin, the terminal, the network, a database, the vault),
// the clock, a random source, printing and logging, another process, and
// state that outlives the call (a flex container mutated in place, a
// matcher, a service, a stored registration, a counter). The census that
// pins the classification is effect_census_test.go.

// SideEffecting is fns with core.CompileSideEffect declared on every word
// but the named quiet ones — the word-level flag RegisterNativeFunc folds
// onto each signature. fns is not modified.
func SideEffecting(fns []NativeFunc, quiet ...string) []NativeFunc {
	skip := make(map[string]bool, len(quiet))
	for _, name := range quiet {
		skip[name] = true
	}
	out := make([]NativeFunc, len(fns))
	for i, fn := range fns {
		if !skip[fn.Name] {
			fn.CompileEffect |= core.CompileSideEffect
		}
		out[i] = fn
	}
	return out
}
