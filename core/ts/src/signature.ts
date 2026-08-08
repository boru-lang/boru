// Signature describes one calling convention of a registered word.
// Mirrors borueng/go/signature.go but trimmed to the spec subset:
// no checker mode, no Patterns, no FullStack, no Returns lists for
// type-check propagation.

import type { BoruType } from './type.ts'
import { TAny } from './type.ts'
import { newCarrier, type Value } from './value.ts'

/** A handler receives matched args and the registry, returns the values to push. */
export type Handler = (
  args: Value[],
  ctx: Map<string, Value> | null,
  stack: Value[],
  registry: Registry,
) => Value[] | Promise<Value[]>

export interface Signature {
  /** Argument types in sig order (sig[0] is the first arg the handler sees). */
  args: BoruType[]
  handler: Handler
  /**
   * Position of the boundary marker `|` in the sig (post-§1.4).
   * Args before the boundary may be collected from forward tokens or
   * fall back to the stack; args from the boundary onward must come
   * from the stack. 0 = all stack (old "stack-only"); N = all
   * forward-eligible (old "forward-precedence"); 0 < B < N = mixed.
   * Set automatically by Registry.registerNativeFunc when the
   * NativeSig leaves it unset.
   */
  barrierPos?: number
  /**
   * Optional value-patterns indexed by arg position. A concrete-scalar
   * pattern fires the §1.1 literal-dispatch path: the matched arg
   * must have the same Data as the pattern. See match.ts for the
   * forward-vs-stack rules.
   */
  patterns?: Map<number, Value>
  /**
   * NoEvalArgs marks positions where list auto-evaluation is suppressed.
   * Unused in the spec subset; included for shape parity.
   */
  noEvalArgs?: Set<number>
  /** Fallback marker — true for the generic 0-arg fallback. */
  fallback?: boolean
  /**
   * FULL-STACK words (`depth`, `pick`, `roll`): the handler receives the
   * whole resolved stack of the current paren scope and returns its
   * complete REPLACEMENT, rather than receiving N args and returning their
   * replacement. Mirrors Go's FullStack() dispatch knob
   * (core/go/sigimpl.go).
   *
   * Scoped to the nearest open paren so a full-stack word inside a group
   * cannot reach values below it — `(1 2 depth)` sees two, not whatever
   * the enclosing program left underneath.
   */
  fullStack?: boolean
  /**
   * Positions that must be filled by a bare type literal (data === null),
   * not a concrete value — used by `make` to require a type argument.
   * Mirrors NativeSig.TypeArgs in the Go matcher.
   */
  typeArgs?: Set<number>
  /**
   * Positions that capture the next forward Word as an Atom (the name),
   * suppressing evaluation. Used by quote / inspect. Mirrors
   * NativeSig.QuoteArgs.
   */
  quoteArgs?: Set<number>
  /**
   * Declared return types, used by the static checker to synthesise
   * carrier return values when the handler is short-circuited in check
   * mode. Mirrors NativeSig.Returns.
   */
  returns?: BoruType[]
  /**
   * Computes the carrier return values for this signature in check
   * mode, given the (carrier-typed) args. Takes precedence over
   * `returns`. Mirrors NativeSig.ReturnsFn / ReturnsFunc.
   */
  returnsFn?: ReturnsFunc
  /**
   * When true, the handler runs even in check mode (its side effects —
   * bindings, type registration — are prerequisites for later
   * analysis). Mirrors NativeSig.RunInCheckMode.
   */
  runInCheckMode?: boolean
  /**
   * When true, this signature's returnsFn records its own bytecode event
   * (e.g. `if` records a branch), so the engine's generic per-dispatch
   * recordCall is skipped to avoid double-recording.
   */
  recordsOwnEvent?: boolean
  /**
   * When true, the bytecode recorder compiles this dispatch as an
   * interpreter ISLAND (OpFallback) rather than a native call: the word +
   * its baked args are captured as re-runnable tokens and re-executed
   * through a sub-engine at VM runtime, with the surrounding compiled code
   * still running. Marks a construct the recorder can't lower natively
   * (a dynamic dispatch, a code-body higher-order word). Mirrors the
   * CompileFallbackBody / CompileIslandPure CompileEffect flags in eng/go.
   */
  compileFallback?: boolean
}

/** Computes carrier return values for a signature in check mode. */
export type ReturnsFunc = (args: Value[], registry: Registry) => Value[]

/**
 * returnsIdentity is the ReturnsFunc for words that PRESERVE their inputs —
 * the stack vocabulary (dup, swap, over, rot, …), where the output types
 * are expressible as a permutation of the input types. The twin of Go's
 * ReturnsIdentity (core/go/carrier_new.go).
 *
 * The mapping is a permutation description: result[i] = args[mapping[i]].
 * swap is returnsIdentity(0, 1); over is returnsIdentity(1, 0, 1). An
 * index outside the args range yields an Any carrier rather than throwing,
 * matching Go.
 *
 * PARITY NOTE: Go's version additionally mints a FRESH Value.ID for a
 * DUPLICATED source index, so `dup`'s two outputs stay distinct for the
 * bytecode emitter's per-value provenance. core/ts Values carry no ID —
 * there is no compiler in the TS pieces to consume one — so that half has
 * no analogue here and is deliberately absent rather than stubbed.
 */
export function returnsIdentity(...mapping: number[]): ReturnsFunc {
  return (args: Value[]): Value[] =>
    mapping.map((m) => (m < 0 || m >= args.length ? newCarrier(TAny) : args[m]!))
}

export interface NativeSig {
  args: BoruType[]
  /** See Signature.fullStack. */
  fullStack?: boolean
  handler: Handler
  barrierPos?: number
  patterns?: Map<number, Value>
  noEvalArgs?: Set<number>
  fallback?: boolean
  typeArgs?: Set<number>
  quoteArgs?: Set<number>
  returns?: BoruType[]
  returnsFn?: ReturnsFunc
  runInCheckMode?: boolean
  recordsOwnEvent?: boolean
  compileFallback?: boolean
}

export interface NativeFunc {
  name: string
  forwardPrecedence?: boolean
  signatures: NativeSig[]
}

/**
 * Score a signature by sum of part-counts across its arg types
 * (argument specificity). Higher score = more specific. This mirrors
 * the Go engine's heuristic: a signature `[Integer, Integer]` scores
 * higher than `[Any, Any]`.
 */
export function signatureScore(sig: Signature): number {
  let s = 0
  for (const t of sig.args) s += t.specificity()
  // Concrete-value patterns make the sig more specific than one
  // with the same arg types but no pattern (parity with Go's
  // post-§1.1 score boost).
  if (sig.patterns) {
    for (const v of sig.patterns.values()) {
      if (v.data !== null) s += 10
    }
  }
  return s
}

/**
 * Sort signatures by descending specificity. The first matching sig
 * wins, so more-specific overloads must be tried first.
 */
export function sortSignatures(sigs: Signature[]): void {
  sigs.sort((a, b) => signatureScore(b) - signatureScore(a))
}

// Forward-declared type — the actual class lives in registry.ts.
import type { Registry } from './registry.ts'
