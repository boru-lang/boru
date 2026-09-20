package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// Check-mode mirrors for the boru:tui words that validate their option
// and app maps BEFORE touching the terminal — the headless contract
// lang/spec/module-tui.tsv §§3-6 pins, and the same shape the boru:vault
// usage mirrors follow (vault.go). Each mirror runs the handler's OWN
// pure prefix over deep-concrete operands, so the diagnostic carries the
// byte-identical code and detail the run raises.
//
// Why these words and not the rest of the surface: a Tier-1 word takes a
// Terminal handle, which only `open` can mint, so its compile failures are
// backend-state, not argument shape. `open` / `run` / `serve` take maps
// the call site writes literally, and every check below is a pure
// function of those maps.

// tuiPolicyFirstMirror wires the pure prefix of a word whose handler
// consults the terminal policy BEFORE validating anything — open and run,
// both of which gate on `open` as their first statement.
//
// The policy gate is consulted but never reported: when the policy denies
// the word the run raises the compile failure and never reaches the validation,
// so claiming a usage defect there would name the wrong error. Declining
// keeps the mirror firing exactly where the run does.
//
// serve does NOT use this: its handler validates the transport options
// first and only then reaches a policy gate, so it carries its own order
// (tuiServeMirror below). The order is the contract — a mirror that gates
// earlier than its handler silently drops the diagnostics in between.
func tuiPolicyFirstMirror(word, polOp string, arity int,
	validate func([]native.Value, *native.Registry) error, base native.ReturnsFunc) native.ReturnsFunc {
	return native.MirrorReturns(word, concreteArgsGate(arity),
		func(args []native.Value, r *native.Registry) error {
			if polErr := checkTuiPolicy(r, word, polOp); polErr != nil {
				return nil
			}
			return validate(args, r)
		}, base)
}

// tuiDynAnyReturns is the residual `run` / `serve` declare — a DYNAMIC Any
// carrier, exactly what declaredReturnCarriers builds for a declared Any
// (a strict Any conforms to no typed slot and would poison every consumer
// downstream). Attaching a ReturnsFn replaces Returns, so the mirror has
// to reproduce it.
func tuiDynAnyReturns(_ []native.Value, _ *native.Registry) []native.Value {
	c := native.NewCarrier(native.TAny)
	c.Dynamic = true
	return []native.Value{c}
}

// tuiOpenMirror models tuiOpenHandler's option parse: the mouse / title
// readers and the §11.7 alt-screen reservation. It stops where the
// handler stops being pure — acquireTuiBackend, which needs a host.
func tuiOpenMirror() native.ReturnsFunc {
	return tuiPolicyFirstMirror("open", "open", 1,
		func(args []native.Value, r *native.Registry) error {
			mp, _ := native.AsMap(args[0])
			if mp == nil {
				// A value that does not inhabit the Map slot at all reaches
				// this ReturnsFn only through union-combo expansion, and the
				// run never routes it here — decline.
				//
				// A MAP-SHAPED one is different: the structural subtypes
				// (RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo) share
				// Parent=TMap and satisfy the signature, but AsMap cannot
				// expose them, so the handler raises this exact error.
				if !args[0].Parent.Equal(native.TMap) {
					return nil
				}
				return r.BoruError("tui_error", "open: options must be a Map", "open")
			}
			var err error
			if _, err = tuiOptBoolean(mp, "mouse"); err == nil {
				_, err = tuiOptString(mp, "title")
			}
			if err != nil {
				return r.BoruError("tui_error", "open: "+err.Error(), "open")
			}
			return tuiAltScreenReserved(mp, "open", r)
		},
		func(_ []native.Value, _ *native.Registry) []native.Value {
			return []native.Value{native.NewCarrier(TTerminal)}
		})
}

// tuiRunMirror models tuiRunHandler's app-config parse — the whole of
// parseTuiApp, which is the handler's entire prefix before the process
// runtime is touched.
func tuiRunMirror() native.ReturnsFunc {
	return tuiPolicyFirstMirror("run", "open", 1,
		func(args []native.Value, r *native.Registry) error {
			_, err := parseTuiApp(args[0], "run", r)
			return err
		}, tuiDynAnyReturns)
}

// tuiServeMirror walks tuiServeHandler's prefix in the handler's OWN
// order, which is the one place in this file where the policy gates are
// not first: transport options, then the net policy, then the terminal
// policy, then the app config.
//
// That order is load-bearing. A malformed `tcp:` raises tui_error at run
// time even under a policy that denies the terminal, because the handler
// never gets as far as the terminal gate — so a mirror that consulted
// that gate up front would drop a guaranteed diagnostic. Each gate
// declines only the validation that comes AFTER it.
func tuiServeMirror() native.ReturnsFunc {
	return native.MirrorReturns("serve", concreteArgsGate(2),
		func(args []native.Value, r *native.Registry) error {
			opts, err := tuiServeOptsOf(args[0], r)
			if err != nil {
				return err
			}
			if netErr := checkNetPolicy(r, "serve", "listen", "", opts.port); netErr != nil {
				return nil
			}
			if polErr := checkTuiPolicy(r, "serve", "serve"); polErr != nil {
				return nil
			}
			_, err = parseTuiApp(args[1], "serve", r)
			return err
		}, tuiDynAnyReturns)
}
