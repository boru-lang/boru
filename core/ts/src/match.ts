// Unified signature matching (post §1.4).
//
// Every sig declares its boundary via `barrierPos`: the count of
// leading args that may be collected from forward tokens. Args at
// positions [barrierPos..N-1] always come from the stack, top-down.
// There is no stack-only/forward-precedence distinction at the word
// level — registration just sets the per-sig boundary.
//
// Algorithm for one sig with N args and forward limit B:
//
//   1. Forward phase: walk sig[0..B-1] in order, consuming forward
//      tokens one at a time from positions immediately after the
//      word. Stop on a structural boundary, a function word, or a
//      type mismatch.
//   2. Stack phase: walk the remaining sig positions [fwd..N-1] in
//      order, consuming stack values from the TOP DOWN. sig[fwd] =
//      top-of-stack, sig[fwd+1] = next-deeper, etc.
//
// This is ONE rule at every arity — two-arg words are not a special
// case, and there is no "swap form". A call form only chooses where
// the split between the two phases falls; the handler convention
// (args[1] OP args[0]) then makes the infix spelling read as written.
//
// Examples (sub: subtract; not commutative, handler returns
// args[1] - args[0]):
//
//   sub 10 3   → fwd=2 [10,3]                  → sig[0]=10, sig[1]=3   → -7
//   3 sub 10   → fwd=1 [10], stack top=3       → sig[0]=10, sig[1]=3   → -7
//   3 10 sub   → fwd=0, stack top=10, next=3   → sig[0]=10, sig[1]=3   → -7
//   10 sub 3   → fwd=1 [3],  stack top=10      → sig[0]=3,  sig[1]=10  → 7  (infix)
//   10 3 sub   → fwd=0, stack top=3, next=10   → sig[0]=3,  sig[1]=10  → 7

import type { FunctionEntry, Registry } from "./registry.ts";
import type { Signature } from "./signature.ts";
import { TAtom, TNode, TScalar, TType, TWord, typeNameTable } from "./type.ts";
import {
  isSugar,
  newAtom,
  newBoolean,
  newNone,
  newTypeLiteral,
  type Value,
  type WordInfo,
} from "./value.ts";

/**
 * isTypeArg reports whether v may fill a typeArgs slot: a bare type
 * literal (data===null, not the none value) or a structural type body
 * (a value whose VType lives under Type — Function/Disjunct/Enum/…).
 * Concrete scalars/lists/maps and the none value are rejected. Mirrors
 * eng/go/signature.go::sigTypeMatchesAsType.
 */
function isTypeArg(v: Value): boolean {
  if (v.isNone()) return false;
  if (v.data === null) return true;
  return v.vType.matches(TType);
}

/**
 * Per-position signature match. A typeArgs slot wants a type (literal or
 * type body) whose lattice node conforms to the expected type; an
 * ordinary slot uses the value matcher (which rejects bare type
 * literals at Scalar/Node slots).
 */
function argMatches(
  sig: Signature,
  idx: number,
  v: Value,
  expected: import("./type.ts").BoruType,
): boolean {
  if (sig.typeArgs?.has(idx)) return isTypeArg(v) && v.vType.matches(expected);
  return sigTypeMatches(v, expected);
}

export interface MatchResult {
  sig: Signature;
  /** Args in sig order. */
  args: Value[];
  /** Number of forward tokens consumed (after the word). */
  forwardCount: number;
  /** Number of prefix (stack) tokens consumed (before the word). */
  prefixCount: number;
}

/**
 * Match N already-evaluated operands against a word's signatures, the
 * runtime poly path (OpCallNativePoly). `window[i]` is sig position i —
 * the operand-stack convention where the top of stack is position 0. This
 * is the all-stack case of `tryMatch` (no forward tokens), iterating the
 * entry's signatures in the same specificity order as dispatch so poly
 * takes the IDENTICAL first-match the interpreter would. Mirrors
 * eng/go/vm.go::callPoly + MatchSignature.
 */
export function matchValues(
  fn: FunctionEntry,
  window: readonly Value[],
): MatchResult | null {
  for (const sig of fn.signatures) {
    const n = sig.args.length;
    if (n !== window.length) continue;
    let ok = true;
    for (let i = 0; i < n; i++) {
      if (!argMatches(sig, i, window[i]!, sig.args[i]!)) {
        ok = false;
        break;
      }
    }
    if (!ok) continue;
    const args = [...window];
    if (!patternsOk(sig, args, 0)) continue; // all-stack: forwardCount 0
    return { sig, args, forwardCount: 0, prefixCount: n };
  }
  return null;
}

export function matchEntry(
  fn: FunctionEntry,
  stack: readonly Value[],
  pointer: number,
  registry?: Registry,
): MatchResult | null {
  // The dispatching word lives at stack[pointer]. Its WordInfo may
  // carry /s or /f modifiers that override the sig's BarrierPos.
  const wordInfo = readWordInfo(stack, pointer);
  // The /N modifier restricts dispatch to overloads of exactly N total
  // args (mirrors the Go matcher's argCount filter). When set, only
  // sigs with matching arity are considered in every pass.
  const argCount = wordInfo?.argCount;
  const sigs =
    argCount === undefined
      ? fn.signatures
      : fn.signatures.filter((s) => BigInt(s.args.length) === argCount);
  // Strict pass: forward stops on type mismatch, stack fills the
  // rest. Most calls bind here.
  for (const sig of sigs) {
    const n = sig.args.length;
    if (n === 0) continue;
    const r = tryMatch(sig, n, stack, pointer, wordInfo, registry);
    if (r) return r;
  }
  // Optimistic pass: a forward-side Word that names a registered
  // function is accepted even when the sig wants a different type;
  // engine.shouldDeferDispatch will then defer dispatch via
  // insertForward, and stepLiteral re-type-checks the resolved value
  // A bare function word in the forward window is a hard barrier: forward
  // collection stops there and the remaining args must come from the
  // stack (only parentheses pre-evaluate a nested call). So there is no
  // optimistic cross-barrier deferral — `addq 1 mulq 2 3` is a
  // signature_error, matching the eng/spec nested-forward contract.
  // 0-arg fallback: if the function declares a 0-arg sig (e.g. `args`,
  // bare keywords), match it with no consumed prefix or forward.
  for (const sig of sigs) {
    if (sig.args.length === 0) {
      return { sig, args: [], forwardCount: 0, prefixCount: 0 };
    }
  }
  return null;
}

function readWordInfo(
  stack: readonly Value[],
  pointer: number,
): WordInfo | undefined {
  const v = stack[pointer];
  if (!v || !v.isWord()) return undefined;
  return v.asWord();
}

/**
 * Resolve a forward-side Word token against the expected sig type.
 *
 * A plain (non-function) def-bound word expands into the token stream
 * wherever it stands — the `f w ≡ f (w)` equivalence (engine.go's
 * resolveForwardArgs). So a word naming a simple-value def resolves to
 * its value before the slot is type-checked, which is how `typeof pi`
 * sees the integer 3 rather than the word `pi`.
 *
 * Exception: a TWord slot captures the raw word as data (handlers like
 * `def`/`quote` declare TWord precisely to capture the name), so the
 * word is kept. Function-valued defs are also kept so dispatch /
 * forward-deferral handles them, not substitution.
 */
function resolveForwardToken(
  tok: Value,
  expected: import("./type.ts").BoruType,
  registry: Registry | undefined,
): Value {
  if (!tok.isWord()) return tok;
  if (expected.equal(TWord)) return tok;
  const name = tok.asWord().name;
  if (registry) {
    const top = registry.topOfDefStack(name);
    if (top !== undefined && !top.isFnDef()) {
      // Resolving a def in the forward window is a "use" for check-mode
      // unused-def tracking. No-op outside check mode.
      registry.check.recordUse(name);
      return top;
    }
  }
  // The canonical cascade's builtin arm at consumption (ADR-012 rule
  // 4): the parser is type-name-opaque, so `make Integer 42` collects
  // Word(Integer) — resolve it to the type literal exactly where the
  // slot consumes it. The defs arm ran above; keywords resolve too
  // (`true`/`false`/`none`, mirroring Go resolveWordValue).
  if (name === "true") return newBoolean(true);
  if (name === "false") return newBoolean(false);
  if (name === "none") return newNone();
  const t = typeNameTable().get(name);
  if (t !== undefined) return newTypeLiteral(t);
  return tok;
}

function tryMatch(
  sig: Signature,
  n: number,
  stack: readonly Value[],
  pointer: number,
  word: WordInfo | undefined,
  registry: Registry | undefined,
): MatchResult | null {
  // sig.barrierPos: 0 = boundary at start (all stack), N = boundary
  // at end (all forward-eligible). The Registry has already
  // normalised this on registration. /s and /f modifiers on the call
  // site override it: /s → 0, /f → N.
  let forwardLimit = sig.barrierPos ?? 0;
  if (word?.forceStack) forwardLimit = 0;
  else if (word?.forceForward) forwardLimit = n;

  const args: Value[] = new Array(n);

  // Phase 1: forward.
  let fwd = 0;
  let scanIdx = pointer + 1;
  while (fwd < forwardLimit && scanIdx < stack.length) {
    const rawTok = stack[scanIdx];
    if (!rawTok) break;
    if (isStructuralBoundary(rawTok)) break;
    // A quoteArgs slot captures the raw forward Word as an Atom name.
    // A word carrying a container type constraint (`def NAME:[…]`) is
    // kept intact so the constraint survives to the handler.
    //
    // The coercion is CONDITIONAL, exactly as Go's is. Go decides it at
    // collection time (core/go/engine.go's `!matches && QuoteArgs[…] &&
    // val.Parent.Equal(TWord) && TAtom.ConformsTo(SigArgType(…))`), so a
    // slot the raw Word ALREADY fills — a gradual `Any` one — receives
    // the Word itself and never an Atom; and a slot Atom cannot fill at
    // all (`String`, `Integer`, even `Word`) is not a /q capture site,
    // so the scan stops rather than matching. Measured against core/go:
    // `Any` → the Word (which then re-executes, resolving a def-bound
    // name), `Atom`/`Scalar` → the Atom, `Word`/`String`/`Integer` →
    // signature_error. Coercing unconditionally — what this port did —
    // turned the `Any` case into an inert Atom and silently changed what
    // a /q handler receives.
    if (sig.quoteArgs?.has(fwd) && rawTok.isWord()) {
      const expected = sig.args[fwd]!;
      if (!TAtom.matches(expected)) break;
      args[fwd] =
        rawTok.asWord().constraint !== undefined ||
        argMatches(sig, fwd, rawTok, expected)
          ? rawTok
          : newAtom(rawTok.asWord().name);
      fwd++;
      scanIdx++;
      continue;
    }
    const tok = resolveForwardToken(rawTok, sig.args[fwd]!, registry);
    if (!argMatches(sig, fwd, tok, sig.args[fwd]!)) {
      // A type mismatch (including a bare function-word barrier) stops
      // forward collection; the rest must come from the stack.
      break;
    }
    args[fwd] = tok;
    fwd++;
    scanIdx++;
  }

  // /f forbids stack supplementation: every sig position must come
  // from forward. If the forward scan stopped short, this sig fails.
  if (word?.forceForward && fwd < n) return null;

  // Phase 2: stack, top-down.
  const remaining = n - fwd;
  if (pointer < remaining) return null;
  for (let j = 0; j < remaining; j++) {
    const stackVal = stack[pointer - 1 - j];
    if (!stackVal) return null;
    if (isStructuralBoundary(stackVal)) return null;
    const sigIdx = fwd + j;
    if (!argMatches(sig, sigIdx, stackVal, sig.args[sigIdx]!)) return null;
    args[sigIdx] = stackVal;
  }

  if (!patternsOk(sig, args, fwd)) return null;
  return { sig, args, forwardCount: fwd, prefixCount: remaining };
}

export function sigTypeMatches(
  v: Value,
  expected: import("./type.ts").BoruType,
): boolean {
  // Check-mode gradual carrier: a dynamic carrier's type is statically
  // unknown, so it matches any slot optimistically (the contagion then
  // widens the result to dynamic). Mirrors NewDynamicCarrier matching.
  if (v.carrier && v.dynamic) return true;
  if (!v.vType.matches(expected)) return false;
  // Concrete-value rule (mirrors positionalMatch in eng/go): a Scalar
  // or Node (list/map) value slot is filled only by a concrete value
  // (or a check-mode carrier), never by a bare type literal. So
  // `do List` and `addq Integer 1` surface signature errors, while a
  // strict carrier of the right type IS a value and matches.
  if (
    v.data === null &&
    !v.carrier &&
    (expected.matches(TScalar) || expected.matches(TNode))
  ) {
    return false;
  }
  return true;
}

function isStructuralBoundary(v: Value): boolean {
  // Internal control values (forward markers, marks, moves, sugar
  // markers) all share the Word lattice (Word/__FW, Word/__MK,
  // Word/__MV, Word/__SG) but are not data — they must not be picked
  // up as args by either phase of the matcher. A sugar marker fires
  // at the pointer (stepSugar) and its expansion is what dispatch
  // sees; collecting the raw marker as data would skip the lowering.
  if (v.isForward() || v.isMark() || v.isMove() || isSugar(v)) return true;
  if (!v.vType.matches(TWord)) return false;
  const name = (v.data as { name?: string } | null)?.name;
  return name === "(" || name === ")" || name === "end";
}

/**
 * Run sig.patterns against resolved arg values. Concrete-scalar
 * patterns (Data != null on a scalar) are enforced on every
 * position; non-scalar (record/map shape) patterns are enforced
 * only on stack positions, preserving the legacy semantic that
 * handlers may further constrain forward args inside the body.
 * Mirrors match.go's `patternsOk`.
 */
function patternsOk(
  sig: Signature,
  args: Value[],
  forwardCount: number,
): boolean {
  if (!sig.patterns) return true;
  for (const [idx, pattern] of sig.patterns) {
    if (idx < 0 || idx >= args.length) continue;
    const isForward = idx < forwardCount;
    if (isForward && pattern.data === null) continue;
    const val = args[idx]!;
    if (pattern.data === null) {
      if (!val.vType.matches(pattern.vType)) return false;
      continue;
    }
    if (!val.vType.matches(pattern.vType)) return false;
    if (val.data === null) return false;
    if (!dataEqual(val.data, pattern.data)) return false;
  }
  return true;
}

// Pattern payloads are compared by IDENTITY, which for the scalar payloads
// a pattern can carry (bigint, number, string, boolean) is value equality —
// bigint is a JS value type, so `1n === 1n` holds and no separate numeric
// arm is needed. Object payloads (lists, maps) never reach here: patternsOk
// enforces non-scalar patterns only at stack positions via the type test
// above, and a bare type-literal pattern is handled before this call.
function dataEqual(a: unknown, b: unknown): boolean {
  return a === b;
}
