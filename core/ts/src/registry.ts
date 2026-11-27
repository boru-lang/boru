// Registry holds per-name function tables, def stacks, and the
// host-installed capability slot.
//
// PARITY NOTE: the Go Registry has many fields the spec subset
// doesn't exercise (NativeModResolver, ModuleInitFunc, ParseFunc,
// SDKCache, Manager, BaseDir, OnRegisterHook, Check state, args
// stack). Those are intentionally omitted here — adding them would
// inflate the port without helping the specs run. A full-parity
// version would mirror them one-for-one.
import { CheckState } from "./check-state.ts";
import type { NativeFunc, NativeSig, Signature } from "./signature.ts";
import { sortSignatures } from "./signature.ts";
import {
  type FnDefInfo,
  type FnSig,
  newFnDef,
  newParenExpr,
  type Value,
} from "./value.ts";

/** A registered word with its overloaded signatures + dispatch flags. */
export interface FunctionEntry {
  name: string;
  signatures: Signature[];
  /** Signatures in registration order (inspect renders these). */
  declOrder: Signature[];
  forwardPrecedence: boolean;
  /** Longest forward arg count across all sigs (used by stepWord). */
  maxForwardArgs: number;
}

/** Snapshot of mutable registry state for speculative compile/check passes. */
export interface RegistrySnapshot {
  defStacks: Map<string, Value[]>;
  capabilities: Map<string, unknown>;
  argsStack: Value[][];
  check: CheckState;
}

export class Registry {
  private functions = new Map<string, FunctionEntry>();
  private defStacks = new Map<string, Value[]>();
  private capabilities = new Map<string, unknown>();
  /**
   * Per-call args frames. dispatchFnDef pushes a frame containing
   * the matched args (in sig order) before running the body, and
   * pops after. The `args` word returns the top frame so a body can
   * read positional args by index without naming each param.
   */
  private argsStack: Value[][] = [];

  /**
   * Sugar-role table (ADR-012 rule 3, 2026-08-04 amendment): maps a
   * sugar marker's role to the word name the engine lowers it to. The
   * language layer binds roles at registration; a stepped marker whose
   * role is unbound raises `sugar_unbound`. Mirrors Go
   * Registry.sugarWords / BindSugarWord / SugarWord.
   */
  private sugarWords = new Map<string, string>();

  /** Each registered word's function payload, by its entry (nativeFn). */
  private nativeFns = new WeakMap<FunctionEntry, FnDefInfo>();

  /** Static type-checker state. Inactive until check.begin() is called. */
  readonly check = new CheckState();

  /** Bind a sugar role to the word name the engine expands it to. */
  bindSugarWord(kind: string, word: string): void {
    this.sugarWords.set(kind, word);
  }

  /** The word name bound to a sugar role, or undefined when unbound. */
  sugarWord(kind: string): string | undefined {
    return this.sugarWords.get(kind);
  }

  // ── Capabilities ──────────────────────────────────────────────────────

  /** Returns true iff a capability is registered under `name`. */
  hasCapability(name: string): boolean {
    return this.capabilities.has(name);
  }

  /**
   * Look up the value stored under `name`. Returns a tuple analogous
   * to Go's (value, ok) so a stored null/undefined value is
   * distinguishable from "no capability registered".
   */
  capability(name: string): [unknown, true] | [undefined, false] {
    if (!this.capabilities.has(name)) return [undefined, false];
    return [this.capabilities.get(name), true];
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
    this.capabilities.set(name, value);
  }

  /** Remove the capability for `name`. Returns true if one was present. */
  deleteCapability(name: string): boolean {
    return this.capabilities.delete(name);
  }

  capabilityNames(): string[] {
    return [...this.capabilities.keys()];
  }

  // ── Function table ────────────────────────────────────────────────────

  /** Look up a registered word entry, or undefined if not registered. */
  lookup(name: string): FunctionEntry | undefined {
    return this.functions.get(name);
  }

  /**
   * Register a NativeFunc. Each signature gets a boundary normalised:
   * if the NativeSig leaves `barrierPos` unset, it defaults to the
   * sig's arg count when `forwardPrecedence: true` (old forward-prec
   * semantics → all args forward-eligible) and to 0 otherwise (old
   * stack-only → all args from stack).
   */
  registerNativeFunc(fn: NativeFunc): void {
    const forwardPrec = fn.forwardPrecedence ?? false;
    const entry = this.upsert(fn.name, forwardPrec);
    for (const ns of fn.signatures) {
      const sig = this.toSignature(ns, forwardPrec);
      entry.signatures.push(sig);
      entry.declOrder.push(sig);
    }
    sortSignatures(entry.signatures);
    entry.maxForwardArgs = computeMaxForward(entry);
  }

  private upsert(name: string, forward: boolean): FunctionEntry {
    let entry = this.functions.get(name);
    if (!entry) {
      entry = {
        name,
        signatures: [],
        declOrder: [],
        forwardPrecedence: forward,
        maxForwardArgs: 0,
      };
      this.functions.set(name, entry);
    } else {
      entry.forwardPrecedence = entry.forwardPrecedence || forward;
    }
    return entry;
  }

  private toSignature(ns: NativeSig, forwardPrec: boolean): Signature {
    let barrierPos = ns.barrierPos;
    if (barrierPos === undefined) {
      barrierPos = forwardPrec ? ns.args.length : 0;
    }
    return {
      args: ns.args,
      argNames: ns.argNames,
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
    };
  }

  // ── Def stack ─────────────────────────────────────────────────────────
  pushDef(name: string, v: Value): void {
    let stack = this.defStacks.get(name);
    if (!stack) {
      stack = [];
      this.defStacks.set(name, stack);
    }
    stack.push(v);
  }

  popDef(name: string): boolean {
    const stack = this.defStacks.get(name);
    if (!stack || stack.length === 0) return false;
    stack.pop();
    if (stack.length === 0) this.defStacks.delete(name);
    return true;
  }

  topOfDefStack(name: string): Value | undefined {
    const stack = this.defStacks.get(name);
    if (!stack || stack.length === 0) return undefined;
    return stack[stack.length - 1];
  }

  /**
   * What `name/v` denotes: the VALUE bound to the name, never a call.
   * Mirrors Go's ResolveRef (core/go/core_ref.go).
   *
   * A value binding is itself. A function binding — a def-bound fn or a
   * registered native word — comes back as a function VALUE carrying the
   * name it was read through, so it dispatches as the word would wherever
   * it is later stepped and reports under that name when a call to it
   * matches nothing. `undefined` when nothing is bound: the keywords and
   * the builtin type names are not bindings, so `true/v` and `Integer/v`
   * decline exactly as an unknown name does.
   *
   * The reference is the SAME FUNCTION each time it is read: the value
   * holds the binding's own payload, which is the function's identity
   * (FnDefInfo), so one binding read twice is one function — and read
   * through a parameter it was passed to, it is that function still.
   *
   * Go keeps natives in the def table and reads both kinds from it; here
   * the def stack is consulted first, as the word step consults it.
   */
  resolveRef(name: string): Value | undefined {
    const top = this.topOfDefStack(name);
    if (top !== undefined) {
      this.check.recordUse(name);
      if (!top.isFnDef()) return top;
      return newFnDef(this.fnBinding(name)!, name);
    }
    const fn = this.lookup(name);
    if (fn === undefined) return undefined;
    return newFnDef(this.nativeFn(fn), name);
  }

  /**
   * What `name/u` denotes: the bound FUNCTION with its argument order
   * reversed, so that `f/u a b c` is `f c b a`. Mirrors Go's ResolveUsurp
   * and usurpOver (core/go/core_ref.go) and the Lean model's usurpOver.
   *
   * The wrapper is a function of its own. Every signature is one of the
   * original's with its parameters REVERSED and all of them collectable
   * forward, whatever the original's barrier. A parameter keeps its type,
   * its pattern and its /q; a type-arg slot and a no-eval slot are the
   * original's to enforce, on the re-dispatch. Its handler re-dispatches
   * the ORIGINAL — the function as it is bound, identity and all — in a
   * paren group of its own, with what it collected reversed and laid out
   * for the original's barrier: what the original takes from the stack
   * before it, what it takes forward after.
   *
   * A wrapper is a NEW function every time one is made: two `name/uv`
   * are not the same function.
   *
   * Only a function has an argument order to reverse: `"illegal_ref"` for
   * a name bound to anything else, `undefined` for a name nothing binds.
   */
  resolveUsurp(name: string): Value | "illegal_ref" | undefined {
    const top = this.topOfDefStack(name);
    if (top !== undefined && !top.isFnDef()) return "illegal_ref";
    const orig = this.resolveRef(name);
    if (orig === undefined) return undefined;
    const info = orig.asFnDef();
    // The original's signatures, each with the barrier it was bound with.
    // A boru-bodied signature collects every parameter forward.
    const sources: Signature[] =
      info.native !== undefined
        ? info.native.declOrder
        : info.sigs.map((fs) => ({
            args: fs.params.map((p) => p.type),
            argNames: fs.params.map((p) => p.name),
            returns: fs.returns,
            barrierPos: fs.params.length,
            handler: () => [],
          }));
    const wrapped = sources.map((src) => usurpSig(src, orig));
    const entry: FunctionEntry = {
      name,
      signatures: [...wrapped],
      declOrder: wrapped,
      forwardPrecedence: true,
      maxForwardArgs: 0,
    };
    sortSignatures(entry.signatures);
    entry.maxForwardArgs = computeMaxForward(entry);
    return newFnDef({ sigs: wrapped.map(authoredSig), native: entry }, name);
  }

  /**
   * The function a def-bound name DISPATCHES as: every function bound
   * under the name, newest first — a name defined twice has the overloads
   * of both definitions (Go's lookupUncached / aggregateDispatch).
   * `undefined` when the name binds no function.
   *
   * With ONE definition that is the definition itself, identity and all.
   * With several it is a COMPOSITE no single authored function
   * identifies, built anew at each read and so equal to nothing, itself
   * included — which is what core/go answers (NUR391 records it as a
   * defect of the kernel's; it is ported as the kernel has it).
   */
  fnBinding(name: string): FnDefInfo | undefined {
    const stack = this.defStacks.get(name) ?? [];
    const entries: FnDefInfo[] = [];
    for (let i = stack.length - 1; i >= 0; i--) {
      if (stack[i]!.isFnDef()) entries.push(stack[i]!.asFnDef());
    }
    if (entries.length <= 1) return entries[0];
    // A native word's function is its own dispatch table, not a list of
    // bodies to pool: stacked under a boru-bodied definition it is simply
    // shadowed.
    if (entries.some((e) => e.native !== undefined)) return entries[0];
    return { sigs: entries.flatMap((e) => e.sigs) };
  }

  /**
   * A registered word's function payload: its signatures as authored and
   * the dispatch table it calls through. ONE payload per word, so every
   * reference to the word is the same function; it is rebuilt only when a
   * later registration adds a signature to the word.
   */
  private nativeFn(fn: FunctionEntry): FnDefInfo {
    const held = this.nativeFns.get(fn);
    if (held !== undefined && held.sigs.length === fn.declOrder.length) {
      return held;
    }
    const info: FnDefInfo = { sigs: fn.declOrder.map(authoredSig), native: fn };
    this.nativeFns.set(fn, info);
    return info;
  }

  hasDef(name: string): boolean {
    const stack = this.defStacks.get(name);
    return !!stack && stack.length > 0;
  }

  defStackDepth(name: string): number {
    const stack = this.defStacks.get(name);
    return stack ? stack.length : 0;
  }

  defNames(): string[] {
    return [...this.defStacks.keys()];
  }

  // ── Args stack ────────────────────────────────────────────────────────
  pushArgs(args: Value[]): void {
    this.argsStack.push(args);
  }

  popArgs(): Value[] | undefined {
    return this.argsStack.pop();
  }

  topArgs(): Value[] | undefined {
    return this.argsStack[this.argsStack.length - 1];
  }

  /**
   * Capture mutable execution state before a speculative pass. Function
   * registrations are intentionally shared: they are static infrastructure,
   * while defs/capabilities/args/check state are the runtime scopes a check-mode
   * compile may mutate before deciding to fall back.
   */
  snapshot(): RegistrySnapshot {
    const defStacks = new Map<string, Value[]>();
    for (const [name, stack] of this.defStacks) {
      defStacks.set(name, [...stack]);
    }
    return {
      defStacks,
      capabilities: new Map(this.capabilities),
      argsStack: this.argsStack.map((frame) => [...frame]),
      check: this.check.clone(),
    };
  }

  /** Restore mutable execution state in place from snapshot(). */
  restore(snapshot: RegistrySnapshot): void {
    this.defStacks = new Map();
    for (const [name, stack] of snapshot.defStacks) {
      this.defStacks.set(name, [...stack]);
    }
    this.capabilities = new Map(snapshot.capabilities);
    this.argsStack = snapshot.argsStack.map((frame) => [...frame]);
    this.check.restoreFrom(snapshot.check);
  }
}

/**
 * A native signature as its function value presents it: the parameters by
 * name and type, the declared returns, and no body — the twin of the
 * body-less own signature a Go native's FnDefInfo carries.
 */
function authoredSig(sig: Signature): FnSig {
  return {
    params: sig.args.map((type, i) => ({
      name: sig.argNames?.[i] ?? "",
      type,
    })),
    returns: sig.returns ?? [],
    body: [],
  };
}

/**
 * One signature of a usurp wrapper: `src` with its parameters reversed and
 * every one of them collectable forward, re-dispatching `orig`.
 */
function usurpSig(src: Signature, orig: Value): Signature {
  const n = src.args.length;
  const flip = (i: number): number => n - 1 - i;
  // How many of the original's parameters come off the STACK: those from
  // its barrier on. They are the first the wrapper collected.
  const stackSide = n - Math.min(Math.max(src.barrierPos ?? 0, 0), n);
  return {
    args: [...src.args].reverse(),
    argNames: src.args.map((_, i) => src.argNames?.[flip(i)] ?? ""),
    returns: src.returns,
    barrierPos: n,
    patterns:
      src.patterns === undefined
        ? undefined
        : new Map([...src.patterns].map(([i, p]) => [flip(i), p])),
    quoteArgs:
      src.quoteArgs === undefined
        ? undefined
        : new Set([...src.quoteArgs].map(flip)),
    handler: (args: Value[]): Value[] => [
      newParenExpr([
        ...args.slice(0, stackSide),
        orig,
        ...args.slice(stackSide).reverse(),
      ]),
    ],
  };
}

function computeMaxForward(entry: FunctionEntry): number {
  let max = 0;
  for (const sig of entry.signatures) {
    const limit = sig.barrierPos ?? 0;
    if (limit > max) max = limit;
  }
  return max;
}
