// Registry holds per-name function tables, def stacks, and the
// host-installed capability slot.
//
// PARITY NOTE: the Go Registry has many fields the spec subset
// doesn't exercise (NativeModResolver, ModuleInitFunc, ParseFunc,
// SDKCache, Manager, BaseDir, OnRegisterHook, Check state, args
// stack). Those are intentionally omitted here — adding them would
// inflate the port without helping the specs run. A full-parity
// version would mirror them one-for-one.
import { CheckState } from './check-state.ts'
import type { NativeFunc, NativeSig, Signature } from './signature.ts'
import { sortSignatures } from './signature.ts'
import type { Value } from './value.ts'

/** A registered word with its overloaded signatures + dispatch flags. */
export interface FunctionEntry {
  name: string
  signatures: Signature[]
  /** Signatures in registration order (inspect renders these). */
  declOrder: Signature[]
  forwardPrecedence: boolean
  /** Longest forward arg count across all sigs (used by stepWord). */
  maxForwardArgs: number
}

/** Snapshot of mutable registry state for speculative compile/check passes. */
export interface RegistrySnapshot {
  defStacks: Map<string, Value[]>
  capabilities: Map<string, unknown>
  argsStack: Value[][]
  check: CheckState
}

export class Registry {
  private functions = new Map<string, FunctionEntry>()
  private defStacks = new Map<string, Value[]>()
  private capabilities = new Map<string, unknown>()
  /**
   * Per-call args frames. dispatchFnDef pushes a frame containing
   * the matched args (in sig order) before running the body, and
   * pops after. The `args` word returns the top frame so a body can
   * read positional args by index without naming each param.
   */
  private argsStack: Value[][] = []

  /**
   * Sugar-role table (ADR-012 rule 3, 2026-08-04 amendment): maps a
   * sugar marker's role to the word name the engine lowers it to. The
   * language layer binds roles at registration; a stepped marker whose
   * role is unbound raises `sugar_unbound`. Mirrors Go
   * Registry.sugarWords / BindSugarWord / SugarWord.
   */
  private sugarWords = new Map<string, string>()

  /** Static type-checker state. Inactive until check.begin() is called. */
  readonly check = new CheckState()

  /** Bind a sugar role to the word name the engine expands it to. */
  bindSugarWord(kind: string, word: string): void {
    this.sugarWords.set(kind, word)
  }

  /** The word name bound to a sugar role, or undefined when unbound. */
  sugarWord(kind: string): string | undefined {
    return this.sugarWords.get(kind)
  }

  // ── Capabilities ──────────────────────────────────────────────────────

  /** Returns true iff a capability is registered under `name`. */
  hasCapability(name: string): boolean {
    return this.capabilities.has(name)
  }

  /**
   * Look up the value stored under `name`. Returns a tuple analogous
   * to Go's (value, ok) so a stored null/undefined value is
   * distinguishable from "no capability registered".
   */
  capability(name: string): [unknown, true] | [undefined, false] {
    if (!this.capabilities.has(name)) return [undefined, false]
    return [this.capabilities.get(name), true]
  }

  /**
   * Install or replace the capability value under `name`.
   *
   * Storing `null` or `undefined` is a real STORE — the capability is
   * present and its value is null/undefined. Use deleteCapability to
   * remove an entry. The previous version conflated the two; passing
   * a possibly-undefined argument silently became a delete instead of
   * an "install undefined" call.
   */
  setCapability(name: string, value: unknown): void {
    this.capabilities.set(name, value)
  }

  /** Remove the capability for `name`. Returns true if one was present. */
  deleteCapability(name: string): boolean {
    return this.capabilities.delete(name)
  }

  capabilityNames(): string[] {
    return [...this.capabilities.keys()]
  }

  // ── Function table ────────────────────────────────────────────────────

  /** Look up a registered word entry, or undefined if not registered. */
  lookup(name: string): FunctionEntry | undefined {
    return this.functions.get(name)
  }

  /**
   * Register a NativeFunc. Each signature gets a boundary normalised:
   * if the NativeSig leaves `barrierPos` unset, it defaults to the
   * sig's arg count when `forwardPrecedence: true` (old forward-prec
   * semantics → all args forward-eligible) and to 0 otherwise (old
   * stack-only → all args from stack).
   */
  registerNativeFunc(fn: NativeFunc): void {
    const forwardPrec = fn.forwardPrecedence ?? false
    const entry = this.upsert(fn.name, forwardPrec)
    for (const ns of fn.signatures) {
      const sig = this.toSignature(ns, forwardPrec)
      entry.signatures.push(sig)
      entry.declOrder.push(sig)
    }
    sortSignatures(entry.signatures)
    entry.maxForwardArgs = computeMaxForward(entry)
  }

  private upsert(name: string, forward: boolean): FunctionEntry {
    let entry = this.functions.get(name)
    if (!entry) {
      entry = { name, signatures: [], declOrder: [], forwardPrecedence: forward, maxForwardArgs: 0 }
      this.functions.set(name, entry)
    } else {
      entry.forwardPrecedence = entry.forwardPrecedence || forward
    }
    return entry
  }

  private toSignature(ns: NativeSig, forwardPrec: boolean): Signature {
    let barrierPos = ns.barrierPos
    if (barrierPos === undefined) {
      barrierPos = forwardPrec ? ns.args.length : 0
    }
    return {
      args: ns.args,
      handler: ns.handler,
      barrierPos,
      patterns: ns.patterns,
      noEvalArgs: ns.noEvalArgs,
      fallback: ns.fallback,
      fullStack: ns.fullStack,
      typeArgs: ns.typeArgs,
      quoteArgs: ns.quoteArgs,
      returns: ns.returns,
      returnsFn: ns.returnsFn,
      runInCheckMode: ns.runInCheckMode,
      recordsOwnEvent: ns.recordsOwnEvent,
      compileFallback: ns.compileFallback,
    }
  }

  // ── Def stack ─────────────────────────────────────────────────────────
  pushDef(name: string, v: Value): void {
    let stack = this.defStacks.get(name)
    if (!stack) {
      stack = []
      this.defStacks.set(name, stack)
    }
    stack.push(v)
  }

  popDef(name: string): boolean {
    const stack = this.defStacks.get(name)
    if (!stack || stack.length === 0) return false
    stack.pop()
    if (stack.length === 0) this.defStacks.delete(name)
    return true
  }

  topOfDefStack(name: string): Value | undefined {
    const stack = this.defStacks.get(name)
    if (!stack || stack.length === 0) return undefined
    return stack[stack.length - 1]
  }

  hasDef(name: string): boolean {
    const stack = this.defStacks.get(name)
    return !!stack && stack.length > 0
  }

  defStackDepth(name: string): number {
    const stack = this.defStacks.get(name)
    return stack ? stack.length : 0
  }

  defNames(): string[] {
    return [...this.defStacks.keys()]
  }

  // ── Args stack ────────────────────────────────────────────────────────
  pushArgs(args: Value[]): void {
    this.argsStack.push(args)
  }

  popArgs(): Value[] | undefined {
    return this.argsStack.pop()
  }

  topArgs(): Value[] | undefined {
    return this.argsStack[this.argsStack.length - 1]
  }

  /**
   * Capture mutable execution state before a speculative pass. Function
   * registrations are intentionally shared: they are static infrastructure,
   * while defs/capabilities/args/check state are the runtime scopes a check-mode
   * compile may mutate before deciding to fall back.
   */
  snapshot(): RegistrySnapshot {
    const defStacks = new Map<string, Value[]>()
    for (const [name, stack] of this.defStacks) {
      defStacks.set(name, [...stack])
    }
    return {
      defStacks,
      capabilities: new Map(this.capabilities),
      argsStack: this.argsStack.map((frame) => [...frame]),
      check: this.check.clone(),
    }
  }

  /** Restore mutable execution state in place from snapshot(). */
  restore(snapshot: RegistrySnapshot): void {
    this.defStacks = new Map()
    for (const [name, stack] of snapshot.defStacks) {
      this.defStacks.set(name, [...stack])
    }
    this.capabilities = new Map(snapshot.capabilities)
    this.argsStack = snapshot.argsStack.map((frame) => [...frame])
    this.check.restoreFrom(snapshot.check)
  }
}

function computeMaxForward(entry: FunctionEntry): number {
  let max = 0
  for (const sig of entry.signatures) {
    const limit = sig.barrierPos ?? 0
    if (limit > max) max = limit
  }
  return max
}
