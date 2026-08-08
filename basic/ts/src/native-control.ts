// The control-flow words — the TS twin of basic/go/native_control.go.
//
// PORT STATUS: the RUNTIME half of `do` only. Two deliberate limits, both
// recorded rather than papered over:
//
//   - No ANALYSIS half. `do`'s Go registration carries a ReturnsFn that
//     runs the body through the carrier lattice (runCarrierBodyKeepDefs),
//     and core/ts has no carrier lattice yet — see design/GO-TS-PARITY.0.md
//     for the capability table that forces the porting order. basic/spec
//     exercises value mode, so the runtime half is independently useful
//     and independently testable; the analysis half follows the lattice.
//
//   - No Callable / compiled-closure seam. Go's `do` declares a
//     CallableSpec so the bytecode emitter can lower the body to a true
//     closure. There is no TS compiler for it to serve.
//
// `if`, `case` and `for` are NOT here: unlike `do`, their runtime halves
// are entangled with their analysis halves (branch joins, loop fixed
// points), so porting a runtime-only version would mean inventing
// semantics rather than mirroring them.

import {
  Engine,
  BoruError,
  newErrorValue,
  TList,
  type NativeFunc,
  type Registry,
  type Value,
} from '@boru-lang/core'

/**
 * do [body] — runs the body with no inputs and returns its ENTIRE
 * residual, catching a body error and surfacing it as an Error VALUE
 * rather than propagating. That escape-hatch semantics is the word's
 * whole point, and it is why the handler returns a value on failure
 * instead of rethrowing.
 *
 * DIVERGENCE FROM GO, deliberate: Go re-raises an `IO.exit` request
 * across a handler-less `do`, because an exit is a control transfer
 * rather than a failure and converting it to data would demote
 * `IO.exit 4` to exit 0. There is no IO module in the TS pieces, so
 * there is nothing to re-raise and no arm for it here. When one arrives,
 * this is where it goes.
 */
function doListHandler(args: Value[], _ctx: unknown, _stack: Value[], r: Registry): Value[] {
  // Go guards here with an IsConcrete check raising do_error. That guard
  // is NOT reachable in the TS pieces: a bare type literal fails to match
  // the TList signature and dispatch raises signature_error first (pinned
  // by the `do List` row). core/ts has no check mode, which is where Go's
  // non-concrete carriers come from, so writing the guard would be
  // unreachable code — ADR-008's rule is that dead code is removed. It
  // comes back with check mode, and the corpus row will change with it.
  const body = args[0]!
  try {
    return new Engine(r).run([...body.asList()])
  } catch (e) {
    // EVERY body failure becomes an Error value, coded or not. An earlier
    // version rethrew anything that was not a BoruError, which diverged:
    // Go's handler converts whatever InvokeBody returns, so an uncoded
    // failure — `pick` out of range raises a bare fmt.Errorf — is caught
    // there and was propagating here. The message is the DETAIL, not the
    // rendered `[boru/code]: …` form, matching Go.
    return [newErrorValue(e instanceof BoruError ? e.detail : (e as Error).message)]
  }
}

export const controlNatives: NativeFunc[] = [
  {
    name: 'do',
    signatures: [
      {
        args: [TList],
        // The body is CODE, not data: without this the list would
        // auto-evaluate before the handler ever sees it, and `do` would
        // run its body twice.
        noEvalArgs: new Set([0]),
        handler: doListHandler,
        // ALL-FORWARD. The two engines encode this differently and the
        // difference is easy to miss: Go writes BarrierPos -1 as a
        // SENTINEL meaning "every arg is forward-eligible", while core/ts
        // encodes the boundary as a POSITION, so all-forward is the arg
        // count. Writing -1 here silently produced a stack-form `do` that
        // matched nothing and raised signature_error on every row.
        barrierPos: 1,
      },
    ],
  },
]
