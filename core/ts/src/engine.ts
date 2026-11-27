// Engine.run is the interpreter loop. It walks left-to-right through
// the input values, dispatching words via signature matching, passing
// literals through, and pre-evaluating paren groups so a function
// word's forward scan sees fully-resolved values.
//
// PARITY GAP: the Go engine has step budgets, check-mode, trace,
// loop break/continue handling, mark/move continuation, args-stack,
// context-store push/pop, interpolated strings, parser-eval lists,
// and module sub-engines. The TS port here is the interpreter slice
// that the current TSV specs reach.
import { AnalysisImpl } from "./analysis-hooks.ts";
import type { RecorderOperand } from "./emit-recorder.ts";
import { coerceBoolean } from "./coretype.ts";
import { BoruError } from "./error.ts";
import { valToString } from "./canon.ts";
import { isRefWord, matchEntry, sigTypeMatches } from "./match.ts";
import type { Registry } from "./registry.ts";
import { resolveWordsDeep } from "./resolve.ts";
import { sugarExpansion } from "./sugar.ts";
import {
  TAny,
  TAtom,
  TBoolean,
  TDispatchMod,
  TFloat,
  TFunction,
  TInteger,
  TList,
  TMap,
  TNumber,
  TParenExpr,
  TString,
  TXml,
  TWord,
  resolveBuiltinTypeName,
} from "./type.ts";
import {
  type FnDefInfo,
  type ForwardMarker,
  type MoveInfo,
  asSugar,
  isCloseParen,
  isEnd,
  isOpenParen,
  isSplice,
  isSugar,
  newAtom,
  newBoolean,
  newCloseParen,
  newFloat,
  newCarrier,
  newDynamicCarrier,
  newEnd,
  newForwardMarker,
  newInteger,
  newInterpString,
  newList,
  newMap,
  newNone,
  newOpenParen,
  newString,
  newTypeLiteral,
  newTypedList,
  newTypedMap,
  newXml,
  OrderedMap,
  spliceExpand,
  spliceIsData,
  Value,
  type WordInfo,
} from "./value.ts";
import type { BoruType } from "./type.ts";

/**
 * Quote a list param at frame binding so the body treats it as a data
 * value, not a code body to splice — core_helpers.go's binding rule
 * ("Quote list params so they're treated as data values"). The flag
 * survives the return, so `f [1 2]` returning its param yields
 * `(quote [1 2])` on both engines.
 */
function quoteListArg(v: Value): Value {
  if (!v.vType.equal(TList) || v.quoted || v.data === null) return v;
  return new Value(v.vType, v.data, {
    eval: v.eval,
    quoted: true,
    carrier: v.carrier,
    dynamic: v.dynamic,
  });
}

/**
 * Base value for an omitted optional fn param, by its declared type.
 * Mirrors core_helpers.go::BaseValue (empty containers for List/Map,
 * the empty atom for Atom).
 */
function baseValue(t: BoruType): Value {
  // Float is checked before the Integer/Number branch: a Float param
  // matches TNumber too, so the order matters for its base value.
  if (t.matches(TFloat)) return newFloat(0);
  if (t.matches(TInteger) || t.matches(TNumber)) return newInteger(0n);
  if (t.matches(TString)) return newString("");
  if (t.matches(TBoolean)) return newBoolean(false);
  if (t.matches(TList)) return newList([]);
  if (t.matches(TMap)) return newMap(new OrderedMap());
  if (t.matches(TAtom)) return newAtom("");
  return newNone();
}
import type { FunctionEntry } from "./registry.ts";
import type { Signature } from "./signature.ts";

/** A boru-bodied signature matched against the arguments at the pointer. */
interface FnMatch {
  /** Where the arguments were found, and the arguments in signature order. */
  result: import("./match.ts").MatchResult;
  /** The authored signature that matched. */
  sig: import("./value.ts").FnSig;
  /** How many of its parameters the call supplied (the rest are optional). */
  usedK: number;
}

/**
 * Would this value, stepped at the pointer, be CALLED rather than placed:
 * an unquoted function value. The one home of that test — the literal
 * step asks it to decide between a call and a push, and a collapsing paren
 * group asks it to decide whether its lone survivor must be stepped past.
 * Mirrors Go's fnValueDispatchesAtPointer.
 */
function callsAtPointer(v: Value): boolean {
  return v.vType.equal(TFunction) && v.isFnDef() && !v.quoted;
}

/**
 * The order a function's overloads are tried in: the longer arity first,
 * then — position by position — the more specific parameter type first.
 * Stable, so overloads that compare equal keep the order they were bound
 * in, newest definition first. Mirrors Go's SortSignatures
 * (core/go/signature.go), which is what makes `def f fn [[n:Any] …]`
 * stacked over `def f fn [[n:Integer] …]` still answer an Integer with
 * the Integer overload.
 */
function moreSpecificFirst(
  a: import("./value.ts").FnSig,
  b: import("./value.ts").FnSig,
): number {
  if (a.params.length !== b.params.length) {
    return b.params.length - a.params.length;
  }
  for (let i = 0; i < a.params.length; i++) {
    const d = b.params[i]!.type.specificity() - a.params[i]!.type.specificity();
    if (0 !== d) return d;
  }
  return 0;
}

/**
 * What a run IS, which decides how it ends. core/go steps every one of
 * these on the caller's tape, between markers that say what the stretch
 * is; here each is an engine of its own, and this is what that engine
 * knows about where it stands.
 *
 *   program  the whole program. Collections still parked are refused, an
 *            unmatched `(` is a syntax error, and then the pending
 *            container literals left on the stack are evaluated.
 *   region   a SEALED REGION: a container literal's elements, a map
 *            member (core/go/sealed.go). The same three steps in another
 *            order — the pending containers in the residual are evaluated
 *            FIRST, so a fault inside one surfaces before the region is
 *            refused for an unmatched `(`.
 *   group    a paren group's contents. A group only produces values: the
 *            pending containers among them are left for whoever takes
 *            them, or for the run the group stands in.
 *   frame    a call frame's body. It ends as a region does — its
 *            pending containers are evaluated while the parameters are
 *            still bound, and an unmatched `(` is then refused. (core/go
 *            has no such check in a frame: the paren is handed to the
 *            return check, which counts it as a value. NUR392 records
 *            that as a defect of the kernel's, and it is not ported.)
 */
type RunKind = "program" | "region" | "group" | "frame";

/**
 * Is the named word a BINDER, whose operand is a word's binding rather
 * than its expansion: `def y xs`, with `xs` bound to a splice, rebinds the
 * marker itself, so the new name aliases the splice. The one deliberate
 * exception to `f w ≡ f (w)`; write `def y ( xs )` to force the
 * expansion. Mirrors Go's bindsReferent.
 */
function bindsReferent(name: string): boolean {
  return "def" === name;
}

const STEP_LIMIT = 22222;

export class Engine {
  readonly registry: Registry;
  private stack: Value[] = [];
  private pointer = 0;
  /**
   * Set of currently-active mark IDs. A Move only fires its replay
   * if its target ID is here — orphaned Moves (whose Mark was
   * removed by some controller) are silently dropped. Mirrors the
   * `marks map[string]bool` field on borueng/go/engine.go's Engine.
   */
  private markIds: Set<string> = new Set();
  /**
   * Set by preEvalParens when a paren in the just-scanned forward
   * window resolved to zero values — a void argument expression. A
   * following signature-match failure blames the void instead of
   * reporting a generic mismatch. Mirrors borueng/go/engine.go's
   * voidGroups tracking.
   */
  private lastPreEvalHadVoid = false;
  /** What this run is — see RunKind. */
  private kind: RunKind = "program";

  constructor(registry: Registry) {
    this.registry = registry;
  }

  /**
   * Run the input value sequence. Returns the residual stack.
   * Throws BoruError on undefined word, signature mismatch, etc.
   */
  run(input: Value[]): Value[] {
    try {
      return this.runSteps(input);
    } catch (e) {
      // A program that never settles — a list bound to a name the list
      // itself holds, a function that calls itself with no way out —
      // nests evaluation without bound. core/go runs every region on the
      // one tape and reports the tape exhausted; here each region is an
      // engine of its own, and what runs out is the host's call stack.
      // The same failure, under the same code — not a host exception
      // escaping the taxonomy.
      if (e instanceof RangeError) {
        throw new BoruError(
          "tape_exhausted",
          "evaluation nested too deeply — the program does not settle " +
            "(a binding that holds its own name, or unbounded recursion?)",
        );
      }
      throw e;
    }
  }

  private runSteps(input: Value[]): Value[] {
    // Check mode: strip concrete literals to type-only carriers before
    // execution. The same dispatch/matching machinery then runs over
    // carriers; stepWord short-circuits handlers to carrier returns.
    if (this.registry.check.isActive()) {
      const stripped = AnalysisImpl.stripToCarriers(input);
      // When recording bytecode, remember each stripped literal's
      // concrete original so the compiler can materialise it as a const.
      const emit = this.registry.check.emit;
      if (emit) {
        for (let i = 0; i < input.length; i++) {
          const s = stripped[i]!;
          if (s !== input[i] && s.carrier) emit.rememberStripped(s, input[i]!);
        }
      }
      this.stack = stripped;
    } else {
      this.stack = [...input];
    }
    this.pointer = 0;

    for (let step = 0; step < STEP_LIMIT; step++) {
      if (this.pointer >= this.stack.length) break;

      const val = this.stack[this.pointer]!;
      // A paren-expression VALUE (the parser's nested `( … )` node)
      // expands back to its OpenParen … CloseParen marker span in
      // place, then re-processes — the IsOpenParen branch below
      // collapses it. Mirrors Go stepLiteral's ParenExpr expansion
      // (design/legacy/PAREN-REPRESENTATION.9.ignore Step 3). Quoted paren-exprs
      // stay data.
      if (
        val.vType.equal(TParenExpr) &&
        !val.quoted &&
        Array.isArray(val.data)
      ) {
        this.stack.splice(
          this.pointer,
          1,
          newOpenParen(),
          ...(val.data as Value[]),
          newCloseParen(),
        );
        continue;
      }
      if (isOpenParen(val)) {
        // An UNMATCHED `(` is a scan boundary, not an error where it
        // stands: the pointer walks past it, and it then bounds what the
        // tokens after it can see — exactly as it bounds a forward window
        // (preEvalParens). `( boomq` is boomq failing for want of an
        // argument, and `( boomq 1` is boomq running. The open paren is
        // refused when the run ENDS with it still unmatched.
        if (this.findMatchingClose(this.pointer) < 0) {
          this.pointer++;
          continue;
        }
        const produced = this.evalParenAt(this.pointer);
        // THE PAREN RE-STEP RULE. A group's results are re-stepped where
        // the group stood — except that a group which collapses to ONE
        // value, a function, PLACES it: the pointer steps past, and the
        // function is not called on what stands outside the group.
        // `( negq/v ) 5` is the function and a 5; `( negq/v 5 )` is -5.
        // With more than one survivor the rewind lands on the leading
        // value and steps it as usual. Mirrors Go's fnReturnPark.
        if (1 === produced && callsAtPointer(this.stack[this.pointer]!)) {
          this.pointer++;
        }
        continue;
      }
      if (isCloseParen(val)) {
        throw new BoruError("syntax_error", `unmatched ')'`, ")");
      }
      if (isEnd(val)) {
        this.stepEnd();
        continue;
      }
      if (val.isWord()) {
        // `name/v` is read BEFORE any capture: the reference denotes the
        // bound value whatever slot is waiting, so a `/q` name slot is
        // handed that value and not the word — Go's stepWord takes its
        // ForceVal arm ahead of hasPendingForwardQuoteArg.
        if (isRefWord(val)) {
          this.stepWordVal(val.asWord().name);
          continue;
        }
        // If a pending forward marker is waiting for a Word-typed
        // arg, capture this word as data instead of executing it.
        // Mirrors borueng/go/engine.go's hasPendingForwardExpectingWord
        // check at the top of stepWord — without this the engine would
        // dispatch the word and prematurely consume its forward args.
        if (this.pendingExpectsWord()) {
          this.stepLiteral();
          continue;
        }
        this.stepWord(val);
      } else if (val.isForward()) {
        // Forward markers are passive — the pointer just walks past
        // them. They consume incoming literals via stepLiteral.
        this.pointer++;
      } else if (val.isMark()) {
        // Marks are passive too — record the id (so a subsequent
        // matching Move can find the body), advance.
        this.markIds.add(val.asMark().id);
        this.pointer++;
      } else if (val.isMove()) {
        this.stepMove(val);
      } else if (val.isInterpString()) {
        this.stack[this.pointer] = this.evalInterpString(val);
        // Don't advance: re-process the resulting string as a literal
        // so a pending forward marker can collect it.
      } else if (val.isXmlInterp()) {
        const emit = this.registry.check.emit;
        if (emit !== undefined && this.xmlCaptureFree(val.asXmlTmpl())) {
          // Self-contained xml template: island it (re-runs to the element).
          const out = newCarrier(TXml);
          emit.recordValueIsland(val, out, "xml");
          this.stack[this.pointer] = out;
        } else {
          this.stack[this.pointer] = newXml(
            this.resolveXmlTmpl(val.asXmlTmpl()),
          );
        }
      } else if (isSugar(val) && !val.quoted) {
        // A sugar marker fires at the pointer: splice its role-resolved
        // expansion in place and re-step, exactly like the __SP splice.
        // The Angle marker picks its head form when a parked forward is
        // waiting to capture a name (the binder's /q name slot).
        // Mirrors Go stepSugar (eng/go/sugar.go).
        this.stepSugar(val);
      } else {
        this.stepLiteral();
      }
    }

    // THE END OF THE RUN, by what the run is (RunKind). A collection
    // still parked is a failed dispatch everywhere, and is refused first.
    this.refuseUnfilledForward();
    switch (this.kind) {
      case "group":
        break;
      case "frame":
      case "region":
        this.autoEvalStack();
        this.refuseUnmatchedOpen();
        break;
      default:
        this.refuseUnmatchedOpen();
        this.autoEvalStack();
    }
    return this.stack;
  }

  /** Run `input` as a run of the given kind, in an engine of its own. */
  private subRun(kind: RunKind, input: Value[]): Value[] {
    const sub = new Engine(this.registry);
    sub.kind = kind;
    return sub.run(input);
  }

  /**
   * An open paren still on the stack when the run ends was never matched:
   * the pointer walked past it as a boundary, and no close came.
   */
  private refuseUnmatchedOpen(): void {
    if (this.stack.some(isOpenParen)) {
      throw new BoruError("syntax_error", `unmatched '('`, "(");
    }
  }

  /**
   * A forward marker that never filled is a FAILED dispatch, not a value.
   * Go raises signature_error for it; core/ts used to hand the marker back
   * as a residual, so `bothq __pa true` rendered `forward(bothq,1/2)` —
   * an engine-internal token escaping as a program result.
   *
   * Reachable whenever a word between the marker and its operands
   * produces NO residual (a frame-tail `__…` word, a handler returning
   * nothing): the plan counted that token as an operand, and one fewer
   * arrives than was promised.
   */
  private refuseUnfilledForward(): void {
    for (const v of this.stack) {
      if (!v.isForward()) continue;
      const m = v.asForward();
      if (m.collected.length >= m.expectedForward) continue;
      const supplied = m.collected.length + m.stackArgs.length;
      throw new BoruError(
        "signature_error",
        `cannot call \`${m.funcName}\` — no signature matches the arguments\n` +
          `  = note: candidate \`${m.funcName}\` takes ${m.sig.args.length} arguments, but ${supplied} was supplied`,
        m.funcName,
      );
    }
  }

  /**
   * Step a `name/v` word: the name resolves to the VALUE bound to it and
   * is never called. For a function binding that suppresses the call —
   * the function arrives as a value — and for any other binding it is the
   * identity, so one spelling reads a name whose kind is not known. Only
   * an unbound name declines. Mirrors Go's stepWordVal / deliverValRead
   * and the `/v` arm of the Lean model's stepWord.
   *
   * The DELIVERY is what keeps the read inert. A reference is data: it
   * arrives at a collection that is still waiting, like any literal, and
   * otherwise it is placed and the pointer steps past it. In neither case
   * is it stepped as a value at the pointer, where a function is a call.
   */
  private stepWordVal(name: string): void {
    const v = this.registry.resolveRef(name);
    if (v === undefined) {
      if (this.registry.check.isActive()) {
        // Lenient, as the bare-word path is: the diagnostic, then a
        // placeholder the next step collects or passes.
        this.undefinedWordPlaceholder(name);
        return;
      }
      throw new BoruError("undefined_word", `undefined word: ${name}`, name);
    }
    this.stack[this.pointer] = v;
    const fwdIdx = this.findPendingMarker();
    if (fwdIdx >= 0) {
      this.collectArrival(fwdIdx);
      return;
    }
    this.pointer++;
  }

  /**
   * Check mode's stand-in for a name nothing binds: the diagnostic, and an
   * Any carrier in the word's cell so analysis continues past it. The
   * pointer is left on the placeholder — it must go through the literal
   * path on the next step so a pending forward can consume it, which is
   * what lets analysis continue past an unknown name inside a call. Go
   * says the same thing in as many words (core/go/engine.go, "the
   * placeholder atom can be consumed by a downstream operation and never
   * reach the result stack") and likewise leaves its pointer alone.
   * Advancing here stranded the marker instead: `idq nosuchword` left
   * `forward(idq,0/1)` on the stack under a lenient pass.
   */
  private undefinedWordPlaceholder(name: string): void {
    this.registry.check.addDiagnostic({
      code: "undefined_word",
      detail: `undefined word: ${name}`,
      word: name,
    });
    const placeholder = newCarrier(TAny);
    placeholder.undefined = true;
    this.stack[this.pointer] = placeholder;
  }

  private stepWord(val: Value): void {
    const w = val.asWord() as WordInfo;
    const name = w.name;

    // Built-in keywords. The opaque parser leaves none/null as Words
    // (the legacy fixture tokenizer resolved them at lex time) — the
    // engine resolves them at consumption, mirroring Go stepWord.
    if (name === "true") {
      this.stack[this.pointer] = newBoolean(true);
      return;
    }
    if (name === "false") {
      this.stack[this.pointer] = newBoolean(false);
      return;
    }
    if (name === "none") {
      this.stack[this.pointer] = newNone();
      return;
    }
    if (name === "null") {
      this.stack[this.pointer] = newAtom("null");
      return;
    }
    const tn = resolveBuiltinTypeName(name);
    if (tn !== undefined) {
      this.stack[this.pointer] = newTypeLiteral(tn);
      return;
    }

    // Def-stack substitution. Two paths:
    //   1. A FUNCTION binding dispatches: the arguments are matched
    //      against its signatures and the body runs in a call frame.
    //   2. Anything else is a VALUE binding: the word is replaced by the
    //      value, which is then stepped as a literal (stepSubstituted).
    const top = this.registry.topOfDefStack(name);
    if (top !== undefined) {
      // Resolving a def is a "use" for check-mode unused-def tracking.
      this.registry.check.recordUse(name);
      if (top.isFnDef()) {
        // The function the NAME dispatches as: every function bound under
        // it, so a name defined twice has the overloads of both.
        const info = this.registry.fnBinding(name)!;
        if (info.native !== undefined) {
          // A name bound to a NATIVE word's function value — a parameter
          // handed `negq/v` — calls that word: a bare name bound to a
          // function CALLS, whichever kind of function it is.
          this.dispatchEntry(name, info.native);
        } else if (this.registry.check.isActive()) {
          this.dispatchFnDefCheck(name, info);
        } else {
          this.dispatchFnDef(name, info);
        }
        return;
      }
      // `f w ≡ f (w)`: a word bound to a DATA splice that a waiting word
      // is collecting stands as the group `( w )`, so the splice's
      // values arrive as arguments — several of them fill several slots,
      // as the written group would. A binder is the one exception:
      // `def y xs` rebinds the marker itself, so `y` aliases the splice.
      // A CODE-bearing splice is a macro over the live stack and is never
      // grouped; it is collected as the marker it is (core/go/engine.go
      // stepWord, spliceIsData, bindsReferent).
      if (isSplice(top) && spliceIsData(top)) {
        const fwdIdx = this.findPendingMarker();
        if (fwdIdx >= 0 && !bindsReferent(this.stack[fwdIdx]!.asForward().funcName)) {
          this.stack.splice(this.pointer, 1, newOpenParen(), val, newCloseParen());
          return;
        }
      }
      this.stack[this.pointer] = top;
      this.stepSubstituted(top);
      return;
    }

    const fn = this.registry.lookup(name);
    if (!fn) {
      // Check mode is lenient: an undefined word becomes an Any carrier
      // (a placeholder so analysis continues) and emits a diagnostic,
      // rather than a hard error. Mirrors stepWord's check-mode path.
      if (this.registry.check.isActive()) {
        this.undefinedWordPlaceholder(name);
        return;
      }
      throw new BoruError("undefined_word", `undefined word: ${name}`, name);
    }

    this.dispatchEntry(name, fn);
  }

  /**
   * A binding holds a VALUE, and the value a name was just replaced by is
   * stepped AS A LITERAL: collected by a waiting word, or placed. It is
   * not read again as a token. So a word bound to a name is that word, as
   * data — never looked up, never called — and a list bound to a name is
   * the list, which is then evaluated where any pending list literal is.
   * core/go's stepWord: "a def'd word binds a VALUE: push it as-is".
   *
   * core/ts used to hand the cell back to the step loop instead, which
   * re-read the value as a program token: a bound word was dispatched,
   * and an unquoted list binding was spliced into the program as a code
   * body. Splicing is what an explicit splice MARKER does (`def name word
   * [ … ]`), and it does it from the literal step, as any marker fires.
   *
   * The forms the literal step itself EXPANDS in place — a paren
   * expression, a sugar marker, an interpolation — are left where they
   * are for the loop to expand, exactly as it expands them anywhere.
   */
  private stepSubstituted(v: Value): void {
    if (v.vType.equal(TParenExpr) && !v.quoted && Array.isArray(v.data)) return;
    if (isSugar(v) && !v.quoted) return;
    if (v.isInterpString() || v.isXmlInterp()) return;
    this.stepLiteral();
  }

  /**
   * Dispatch the word at the pointer through a registered word's
   * signature table: the statement boundary, the forward window's paren
   * groups, the match, and then a parked forward or an immediate run.
   * `name` is what the word is CALLED here — the registered name, or the
   * name a function value of it is bound under.
   *
   * `asValue` is the same dispatch for the word's function VALUE standing
   * at the pointer, and it differs in the one place a value must: what a
   * miss means. A word that matches nothing is a failed call. A value that
   * matches nothing may simply be data (fnValueUnmatched), and it never
   * takes a FALLBACK signature — the catch-all exists to turn a bare
   * word's miss into a clean error, which is not what a value at rest is.
   */
  private dispatchEntry(
    name: string,
    fn: FunctionEntry,
    asValue = false,
  ): void {
    // STATEMENT BOUNDARY. Every bare function word beginning its own
    // dispatch is a forward-collection barrier — uniformly, regardless of
    // arity (REFERENCE.md:364, design/STRICT-FORWARD-BARRIER.0.md). A
    // parked forward that cannot commit with the args it already holds is
    // STRANDED, not left to wait through: `boomq negq 5` is a
    // signature_error, and the grouped `boomq ( negq 5 )` is what fires
    // the outer word. Without this the inner dispatch ran, its result
    // arrived at the marker, and the outer word fired — 51 rows of
    // core/spec/divergent.tsv.
    const stranded = this.strandedForwardError(name);
    if (stranded !== undefined) {
      throw stranded;
    }

    // Pre-evaluate any paren groups in the forward window so the
    // matcher sees concrete values. The window is bounded by the
    // function's largest forward-eligible arg count across all sigs.
    this.preEvalParens(fn.maxForwardArgs, fn.signatures, name);

    const matched = matchEntry(fn, this.stack, this.pointer, this.registry);
    const result = asValue && true === matched?.sig.fallback ? null : matched;
    if (!result) {
      if (asValue) {
        this.fnValueUnmatched(name);
        return;
      }
      // Check mode: a missing signature is a soft diagnostic, not a hard
      // error. Assume a best-fit candidate, synthesise its carrier
      // returns, and splice them over the word + adjacent operands.
      if (this.registry.check.isActive() && fn.signatures.length > 0) {
        this.checkModeAssumeSig(name, fn);
        return;
      }
      if (this.lastPreEvalHadVoid) {
        throw new BoruError(
          "no_value_error",
          `argument expression produced no value for ${name}`,
          name,
        );
      }
      throw new BoruError(
        "signature_error",
        `cannot call \`${name}\` — no signature matches the arguments\n  = expected: ${name} (${describeExpected(fn)})\n  = stack: ${this.describeStack()}`,
        name,
      );
    }

    // Defer FIRST (in both modes): a forward arg that is still a function
    // Word must be dispatched before this word fires, so its result — not the
    // raw word — fills the slot. Doing this before the check-mode short-circuit
    // lets `typeof fnsig […]` / nested `typeof` record their operand's
    // provenance (the marker fires via fireMarker, which records in check mode).
    if (
      this.shouldDeferDispatch(result.args, result.forwardCount, result.sig)
    ) {
      this.beginForward(name, result, fn);
      return;
    }

    // Check mode: short-circuit the handler. A matched signature whose
    // handler must still run in check mode (def/fn/type/… — its side
    // effects feed later analysis) falls through to normal dispatch.
    if (this.registry.check.isActive() && !result.sig.runInCheckMode) {
      // Compile mode: evaluate computed list args (recording their makeList)
      // so they reach the recorder as values with provenance, mirroring the
      // value-mode autoEvalArgs the short-circuit otherwise skips. Gated on
      // emit so plain type-checking is unchanged.
      if (this.registry.check.emit !== undefined)
        this.autoEvalArgs(result.args, result.sig);
      const out = AnalysisImpl.carrierResults(
        this.registry,
        name,
        result.sig,
        result.args,
      );
      // Bytecode recording: a clean native match is a candidate call
      // event. Passive — does not affect `out`. A sig that records its
      // own event (e.g. `if` records a branch from its returnsFn) is
      // skipped here to avoid double-recording. A compileFallback sig is
      // recorded as an interpreter island instead of a native call; if the
      // island isn't recordable, the whole program falls back.
      const emit = this.registry.check.emit;
      if (emit !== undefined && !result.sig.recordsOwnEvent) {
        if (result.sig.compileFallback) {
          if (
            !emit.recordFallback(
              name,
              result.args,
              out,
              this.registry,
              result.sig.noEvalArgs,
            )
          ) {
            emit.markUncompilable(`${name}: fallback island not recordable`);
          }
        } else {
          emit.recordCall(name, result.sig, result.args, out);
        }
      }
      const replaceFrom = this.pointer - result.prefixCount;
      const replaceCount = result.prefixCount + 1 + result.forwardCount;
      this.stack.splice(replaceFrom, replaceCount, ...out);
      // Position at the first result (like value-mode dispatch), not past it,
      // so a pending outer forward marker collects this carrier via
      // stepLiteral — otherwise a deferred word (def whose value is a forward
      // sub-expression, `def n make Integer 42`) never receives it.
      this.pointer = replaceFrom;
      return;
    }

    this.dispatch(result, name);
  }

  /**
   * Run a fully-resolved match: auto-evaluate any list args whose
   * sig position isn't NoEvalArgs-marked, then execute the handler
   * and splice the result over the consumed prefix + word + forward
   * range.
   */
  private dispatch(
    result: import("./match.ts").MatchResult,
    name: string,
  ): void {
    this.autoEvalArgs(result.args, result.sig);

    // FULL-STACK words (`depth`, `pick`, `roll`) take the whole resolved
    // stack of the current paren scope and return its complete
    // REPLACEMENT, rather than N args and their replacement. Mirrors the
    // Go engine's FullStack path (core/go/engine.go).
    //
    // The scope is the nearest OPEN PAREN below the pointer, not the whole
    // stack: `(1 2 depth)` must see two values, not whatever the enclosing
    // program left underneath. The matched args are excluded — they sit
    // between replaceFrom and the pointer and the handler receives them
    // separately.
    if (true === result.sig.fullStack) {
      let base = 0;
      for (let i = this.pointer - 1; i >= 0; i--) {
        if (isOpenParen(this.stack[i]!)) {
          base = i + 1;
          break;
        }
      }
      const argStart = this.pointer - result.prefixCount;
      const scope = this.stack.slice(base, argStart);
      const fsResult = result.sig.handler(
        result.args,
        null,
        scope,
        this.registry,
      );
      if (fsResult instanceof Promise) {
        throw new BoruError(
          "unsupported",
          `async handlers are not supported in the TS port`,
          name,
        );
      }
      const fsOut = fsResult as Value[];
      this.stack.splice(
        base,
        this.pointer + 1 + result.forwardCount - base,
        ...fsOut,
      );
      this.pointer = base;
      return;
    }

    const handlerResult = result.sig.handler(
      result.args,
      null,
      [],
      this.registry,
    );
    if (handlerResult instanceof Promise) {
      throw new BoruError(
        "unsupported",
        `async handlers are not supported in the TS port`,
        name,
      );
    }
    const out = handlerResult as Value[];

    const replaceFrom = this.pointer - result.prefixCount;
    const replaceCount = result.prefixCount + 1 + result.forwardCount;
    this.stack.splice(replaceFrom, replaceCount, ...out);
    // Position the pointer at the first result (or, for empty
    // outputs, whatever the splice left at this index). The next
    // iteration re-processes that slot — letting a pending outer
    // forward marker collect a Value-typed result via stepLiteral,
    // or just advancing past an immediate-dispatch result.
    this.pointer = replaceFrom;
  }

  /**
   * Check-mode recovery for a word whose arguments matched no
   * signature: assume the best-fit (highest-scored) overload, gather
   * the adjacent operands it would consume, emit a no_signature
   * diagnostic, and splice the assumed signature's carrier returns over
   * the word + operands so analysis continues. Mirrors
   * eng/go/engine.go::checkModeAssumeSig (Phase-1 subset — no disjunct
   * partition / poly recovery).
   */
  private checkModeAssumeSig(name: string, fn: FunctionEntry): void {
    const sig = fn.signatures[0]!;
    const n = sig.args.length;
    const isBoundary = (v: Value): boolean =>
      v.isWord() || v.isForward() || v.isMark() || v.isMove();
    // Gather forward operands (after the pointer) up to the arity, then
    // fill the rest from the stack prefix (before the pointer).
    let fwd = 0;
    while (fwd < n && this.pointer + 1 + fwd < this.stack.length) {
      if (isBoundary(this.stack[this.pointer + 1 + fwd]!)) break;
      fwd++;
    }
    let stk = 0;
    while (fwd + stk < n && this.pointer - 1 - stk >= 0) {
      if (isBoundary(this.stack[this.pointer - 1 - stk]!)) break;
      stk++;
    }
    const args: Value[] = [];
    for (let i = 0; i < fwd; i++) args.push(this.stack[this.pointer + 1 + i]!);
    for (let i = 0; i < stk; i++) args.push(this.stack[this.pointer - 1 - i]!);

    this.registry.check.addDiagnostic({
      code: "no_signature",
      detail: `cannot call \`${name}\` — no signature matches the arguments; assuming best-fit candidate for analysis`,
      word: name,
    });
    const out = AnalysisImpl.carrierResults(this.registry, name, sig, args);
    const replaceFrom = this.pointer - stk;
    const replaceCount = stk + 1 + fwd;
    this.stack.splice(replaceFrom, replaceCount, ...out);
    this.pointer = replaceFrom + out.length;
  }

  /**
   * Decide whether the match needs deferred forward collection. We
   * defer when an optimistically-matched forward arg is still a Word
   * that the engine will want to dispatch (e.g. a TAny slot grabbed
   * a function-name Word). When the matcher accepted Word-as-data
   * for a TWord/TAtom slot, we keep the immediate dispatch — those
   * slots intentionally capture names without executing them. Same
   * for /q-style slots (not yet ported); they'd be checked here too.
   */
  private shouldDeferDispatch(
    args: Value[],
    forwardCount: number,
    sig: Signature,
  ): boolean {
    for (let i = 0; i < forwardCount; i++) {
      const a = args[i]!;
      if (!a.isWord()) continue;
      // A `name/v` word still standing here names NOTHING — a bound one
      // was resolved to its value by the forward scan. It defers at every
      // slot, a name-taking one included, so stepping it reports the name.
      if (isRefWord(a)) return true;
      const expected = sig.args[i]!;
      // TWord/TAtom slots: the matcher kept the Word as data on
      // purpose; never defer here even if the name happens to match
      // a registered function (e.g. `quote dup`).
      if (expected.equal(TWord) || expected.equal(TAtom)) continue;
      // ANY Word that survived resolveForwardToken defers, registered or
      // not. A registered one dispatches and its RESULT arrives — that
      // was already the rule. An UNREGISTERED one is an undefined name,
      // and deferring is what lets stepWord reach it and say so: Go's
      // forward PLAN resolves an unknown word to an Atom
      // (engine.go:8177 "Undefined word: always resolves to Atom") but
      // the plan is speculative, so the token is still stepped and
      // raises. core/ts consumed it as data at an `Any` slot instead, so
      // `boomq zzz` fired boomq where Go reports undefined_word — 21
      // rows of core/spec/divergent.tsv, and the reason an undefined
      // name could vanish into a call rather than being reported.
      //
      // Deferring rather than REFUSING is the distinction that matters:
      // an earlier attempt broke forward collection at such a word and
      // took 7 eng/ts rows with it, because `def inc fn […]` needs the
      // keyword to reach its slot.
      return true;
    }
    return false;
  }

  /**
   * Insert a ForwardMarker in place of the function word and
   * advance past it. The optimistically-matched forward args remain
   * at their original positions on the stack; the engine will step
   * them as usual, and their post-evaluation values flow back into
   * the marker via stepLiteral.
   */
  private beginForward(
    name: string,
    result: import("./match.ts").MatchResult,
    _fn: FunctionEntry,
  ): void {
    // Stack args (sig positions [forwardCount..N-1]) stay where they
    // are below the pointer; we capture them in sig order for the
    // final dispatch.
    const stackArgs = result.args.slice(result.forwardCount);
    const marker: ForwardMarker = {
      funcName: name,
      sig: result.sig,
      expectedForward: result.forwardCount,
      collected: [],
      stackArgs,
    };
    // Replace the function word with the marker. Pointer stays —
    // the main loop's "isForward" branch will advance past it.
    this.stack[this.pointer] = newForwardMarker(marker);
    // Stack args (resolved before the pointer) need to be removed
    // here so they're not double-counted at completion time. Mirror
    // Go's insertForward which records prefix args separately.
    const replaceFrom = this.pointer - result.prefixCount;
    this.stack.splice(replaceFrom, result.prefixCount);
    this.pointer = replaceFrom;
  }

  /**
   * Evaluate the paren group whose `(` sits at `idx`. Finds the
   * matching `)` (accounting for nesting), runs a sub-engine on the
   * contents, and splices the results back in place of the
   * `(...)`. The pointer is positioned at the first result so the
   * outer interpreter can pick it up on the next iteration.
   */
  private evalParenAt(idx: number): number {
    // Both callers have found the group's close before asking for the
    // group: an unmatched `(` is a boundary they walk past or stop at,
    // never a group to evaluate.
    const closeIdx = this.findMatchingClose(idx);
    const inner = this.stack.slice(idx + 1, closeIdx);
    const results = this.subRun("group", inner);
    this.stack.splice(idx, closeIdx - idx + 1, ...results);
    // Don't advance the pointer; the next iteration will process the
    // first spliced result. The COUNT is returned because the caller
    // cannot recover it from the stack length: preEvalParens used to
    // derive it as `length - (before - 1)`, arithmetic that only holds
    // for a two-token `( )` group and went negative for every longer
    // one. Harmless while the void branch merely continued, and a
    // 22-row regression the moment it began to break.
    return results.length;
  }

  /**
   * Find the index of the `)` that closes the `(` at `openIdx`.
   * Returns -1 if no matching close is found. Tracks paren depth so
   * nested groups don't break out prematurely.
   */
  private findMatchingClose(openIdx: number): number {
    let depth = 1;
    for (let i = openIdx + 1; i < this.stack.length; i++) {
      const v = this.stack[i]!;
      if (isOpenParen(v)) depth++;
      else if (isCloseParen(v)) {
        depth--;
        if (depth === 0) return i;
      }
    }
    return -1;
  }

  /**
   * Pre-evaluate paren groups in the forward window so matchSignature
   * sees concrete values. Scans from pointer+1 forward; for each `(`
   * encountered within the window, evaluates that paren in-place
   * (which splices its results back into the main stack). Stops at a
   * structural boundary or a registered function word.
   *
   * `maxFwd` is the upper bound on how many forward values we might
   * need to resolve — taken from FunctionEntry.maxForwardArgs.
   * `collector` names the word whose window this is.
   */
  private preEvalParens(
    maxFwd: number,
    sigs?: readonly Signature[],
    collector = "",
  ): void {
    this.lastPreEvalHadVoid = false;
    if (maxFwd <= 0) return;
    // Keyword slots are decided by the RAW token at their position —
    // prune keyword overloads before any evaluation below, so a keyword
    // overload's larger arity never widens the scan past the dispatch
    // the non-keyword overloads will actually make: `def x 5 \`${x}\``
    // must not pre-evaluate the template before def binds x, and
    // `def g (fn […]) (g 3)` must not pre-evaluate `(g 3)` before the
    // 2-arg def binds g. Mirrors Go's pruneKeywordViable.
    let viable = sigs !== undefined ? [...sigs] : undefined;
    const windowOf = (ss: readonly Signature[]): number => {
      let max = 0;
      for (const s of ss) {
        const limit = s.barrierPos ?? 0;
        if (limit > max) max = limit;
      }
      return max;
    };
    let resolved = 0;
    let scanIdx = this.pointer + 1;
    let guard = 0;
    while (resolved < maxFwd && scanIdx < this.stack.length && guard < 2222) {
      guard++;
      const tok = this.stack[scanIdx]!;
      if (viable !== undefined) {
        const pos = resolved;
        viable = viable.filter((s) => {
          const pat = s.patterns?.get(pos);
          if (pat === undefined || !pat.vType.equal(TAtom) || pat.data === null)
            return true;
          if (pos >= (s.barrierPos ?? 0)) return true;
          const nm = tok.isWord()
            ? (tok.asWord() as WordInfo).name
            : tok.vType.equal(TAtom) && typeof tok.data === "string"
              ? tok.data
              : undefined;
          return nm === (pat.data as string);
        });
        const bound = windowOf(viable);
        if (resolved >= bound) break;
        if (bound < maxFwd) maxFwd = bound;
      }
      // A paren-expression VALUE in the window expands to its marker
      // span in place and reprocesses — the '(' arm below evaluates
      // it. Mirrors Go resolveForwardArgs' IsParenExpr branch.
      if (
        tok.vType.equal(TParenExpr) &&
        !tok.quoted &&
        Array.isArray(tok.data)
      ) {
        this.stack.splice(
          scanIdx,
          1,
          newOpenParen(),
          ...(tok.data as Value[]),
          newCloseParen(),
        );
        continue;
      }
      // `f w ≡ f (w)`: a word bound to a DATA splice occupies its forward
      // position as the group `( w )`, and the '(' arm below evaluates
      // it — so the splice's values are the arguments. Not where no
      // still-viable overload consumes the position, not where the
      // position captures the token itself (a /q name, a type-arg), and
      // not for a binder, which rebinds the marker. Mirrors Go's
      // CollectForward (core/go/collect_kernel.go).
      if (
        tok.isWord() &&
        !isRefWord(tok) &&
        !bindsReferent(collector) &&
        (viable === undefined ||
          (viable.some((s) => resolved < (s.barrierPos ?? 0)) &&
            !viable.some(
              (s) =>
                (s.quoteArgs?.has(resolved) ?? false) ||
                (s.typeArgs?.has(resolved) ?? false),
            )))
      ) {
        const bound = this.registry.topOfDefStack(tok.asWord().name);
        if (bound !== undefined && isSplice(bound) && spliceIsData(bound)) {
          this.stack.splice(scanIdx, 1, newOpenParen(), tok, newCloseParen());
          continue;
        }
      }
      // A sugar marker in the window expands once per dispatch, BEFORE
      // signature matching (which treats markers as boundaries) —
      // mirror of Go's expandScanSugar. The Angle marker's head form
      // is selected when a still-viable overload /q-captures this
      // position; a selected-head expansion failure is the user's
      // error, surfaced now; any other failure leaves the marker as a
      // boundary (it errors at step time).
      if (isSugar(tok) && !tok.quoted) {
        const consumes =
          viable === undefined ||
          viable.some((s) => resolved < (s.barrierPos ?? 0));
        if (!consumes) break;
        const sinfo = asSugar(tok);
        if (sinfo === undefined) break;
        let headForm = false;
        if (sinfo.kind === "angle" && viable !== undefined) {
          headForm = viable.some(
            (s) =>
              (s.quoteArgs?.has(resolved) ?? false) &&
              resolved < (s.barrierPos ?? 0),
          );
        }
        try {
          const exp = sugarExpansion(this.registry, sinfo, headForm);
          this.stack.splice(scanIdx, 1, ...exp);
          continue;
        } catch (e) {
          if (headForm) throw e;
          break;
        }
      }
      // Internal control markers stop the forward scan — they're
      // boundaries, not data.
      if (tok.isForward() || tok.isMark() || tok.isMove()) break;
      // Interpolated literals in the forward window resolve to their
      // concrete value before the matcher sees them, so a consumer like
      // `typeof <p>${…}</p>` types the resulting Node/Xml, not the
      // template marker.
      if (tok.isXmlInterp()) {
        // A self-contained xml template in the forward window islands here
        // (re-runs to its element at run time) so a consumer like
        // `typeof <p>${…}</p>` compiles — mirroring the main loop's
        // XmlInterp branch, which preEvalParens would otherwise pre-empt.
        const emit = this.registry.check.emit;
        if (emit !== undefined && this.xmlCaptureFree(tok.asXmlTmpl())) {
          const out = newCarrier(TXml);
          emit.recordValueIsland(tok, out, "xml");
          this.stack[scanIdx] = out;
        } else {
          this.stack[scanIdx] = newXml(this.resolveXmlTmpl(tok.asXmlTmpl()));
        }
        resolved++;
        scanIdx++;
        continue;
      }
      if (tok.isInterpString()) {
        this.stack[scanIdx] = this.evalInterpString(tok);
        resolved++;
        scanIdx++;
        continue;
      }
      if (isCloseParen(tok) || isEnd(tok)) break;
      if (!tok.isWord() && !isOpenParen(tok)) {
        resolved++;
        scanIdx++;
        continue;
      }
      // A marker or word with NO payload reaches here — an open-paren
      // TYPE literal, say. Go's AsWord tolerates a nil payload and yields
      // an empty name (`w, _ := AsWord(val)` in stepWord); this used to
      // lean on isWord() being true for any value in the Word branch,
      // payload or not, which is exactly the conflation that let a `Word`
      // type literal loop the step loop into an uncoded crash.
      const wi = tok.data as WordInfo | null;
      const name = wi?.name ?? "";
      if (name === "(") {
        // An UNMATCHED `(` is a scan boundary, not an error. Go's forward
        // scan stops at an open paren it cannot resolve and lets the
        // dispatch fail on its own terms — `addq ( 1 2` is a
        // signature_error there. core/ts called evalParenAt, whose
        // findMatchingClose miss throws syntax_error, so the engine
        // reported a SYNTAX error for a stream of values that never had
        // any syntax. Six rows of core/spec/divergent.tsv.
        if (this.findMatchingClose(scanIdx) < 0) break;
        const produced = this.evalParenAt(scanIdx);
        // After the splice the first result is at `scanIdx`. Each
        // produced value counts toward the resolved budget. If the
        // paren produced zero values, advance scanIdx to skip the
        // (now-empty) slot — but since we removed (openIdx..closeIdx)
        // and inserted N values, scanIdx still points at the first
        // result (or past it if N==0).
        if (produced <= 0) {
          // A paren in the forward window that resolved to zero values
          // is a void argument expression. Record it so a subsequent
          // signature-match failure can blame the void rather than
          // reporting a generic mismatch. Mirrors voidArgErrorFor.
          this.lastPreEvalHadVoid = true;
          // And STOP the scan. The slot cannot be filled, so the matcher
          // falls back to stack form or fails on the void — which is what
          // Go does. Continuing past it let the NEXT group slide into the
          // empty slot and RE-ASSOCIATED the operands: `7 8 addq ( ) ( 5 )`
          // was `7 13` here against Go's `15 5`, the only divergence in the
          // whole ledger that produced a wrong NUMBER rather than a wrong
          // error.
          break;
        }
        resolved += produced;
        scanIdx += produced;
        continue;
      }
      // A registered function word in the forward window is a
      // boundary — leave it for the outer matcher to either consume
      // (e.g. if the sig accepts TWord) or reject — UNLESS a
      // still-viable overload captures this position structurally
      // (/q quoteArgs): there the word is the ARGUMENT (`def trip
      // (fn …)` names trip even though trip is registered), and the
      // scan must walk past it so the following group pre-evaluates.
      // Mirrors Go capturesForwardToken. Simple-def words count as
      // one resolved value.
      const capturedByViable =
        viable !== undefined &&
        viable.some(
          (s) =>
            (s.quoteArgs?.has(resolved) ?? false) &&
            resolved < (s.barrierPos ?? 0),
        );
      //
      // A `name/v` word is never the boundary: it is a reference to the
      // function, an ordinary position, and the scan walks past it.
      if (this.registry.lookup(name) && !capturedByViable && !isRefWord(tok))
        break;
      resolved++;
      scanIdx++;
    }
  }

  /**
   * Dispatch a fn-typed def at the pointer. Synthesises a Signature
   * from the FnDef's params (one overload, all-forward-eligible),
   * matches it against the current stack/forward window, binds each
   * param onto the def stack, runs the body in a sub-engine, then
   * splices the result back in place of the consumed prefix + word
   * + forward args. The bound params are popped after the sub-engine
   * returns so they don't leak into the surrounding scope.
   *
   * Mirrors borueng/go/engine.go's FnDef dispatch path (the "stepWord
   * → execFnDefSig" arc) compressed into a single function.
   */
  private dispatchFnDef(name: string, info: FnDefInfo): void {
    const m = this.matchFnSigs(name, info, true);
    if (null === m) {
      // A group in the forward window that produced nothing is what the
      // call lacked: blame the void, as a native word's miss does.
      if (this.lastPreEvalHadVoid) {
        throw new BoruError(
          "no_value_error",
          `argument expression produced no value for ${name}`,
          name,
        );
      }
      throw new BoruError(
        "signature_error",
        `cannot call \`${name}\` — no signature matches the arguments\n  = stack: ${this.describeStack()}`,
        name,
      );
    }
    this.callFnSig(name, m);
  }

  /**
   * The overload of a boru-bodied function that the arguments around the
   * pointer select, or null when none does.
   *
   * Each overload is tried at each arity from its full parameter count
   * down to its required count — optional trailing parameters may be
   * omitted — and the first match is taken. `reduce` false holds every
   * overload to its FULL count: a function LITERAL standing in the
   * program has no optional forms, where the same function reached
   * through its name does (Go matches a named function's compiled
   * signatures and a bare literal's authored ones, core/go/engine.go
   * `stackMatchSig`).
   */
  private matchFnSigs(
    name: string,
    info: FnDefInfo,
    reduce: boolean,
  ): FnMatch | null {
    const maxParams = Math.max(0, ...info.sigs.map((s) => s.params.length));
    // Pre-evaluate forward parens so the matcher sees concrete values.
    this.preEvalParens(maxParams, undefined, name);

    const m = this.matchFnSigsOn(this.stack, name, info, reduce);
    // A check pass analyses the match as it stands: stranding is a
    // run-time refusal, and analysis is lenient about a call it cannot
    // complete.
    if (null === m || this.registry.check.isActive()) return m;
    return this.settleForwardWord(name, info, reduce, m);
  }

  /** matchFnSigs' overload walk, over the given view of the stack. */
  private matchFnSigsOn(
    stack: readonly Value[],
    name: string,
    info: FnDefInfo,
    reduce: boolean,
  ): FnMatch | null {
    const ordered = [...info.sigs].sort(moreSpecificFirst);
    for (const fsig of ordered) {
      const total = fsig.params.length;
      const required = reduce
        ? fsig.params.filter((p) => !p.optional).length
        : total;
      for (let k = total; k >= required; k--) {
        const sig: Signature = {
          args: fsig.params.slice(0, k).map((p) => p.type),
          barrierPos: k,
          handler: () => [],
        };
        const fakeEntry: FunctionEntry = {
          name,
          signatures: [sig],
          declOrder: [sig],
          forwardPrecedence: true,
          maxForwardArgs: k,
        };
        const r = matchEntry(fakeEntry, stack, this.pointer, this.registry);
        if (r) return { result: r, sig: fsig, usedK: k };
      }
    }
    return null;
  }

  /**
   * A boru-bodied match whose forward arguments include a WORD that is
   * still a word — one the forward scan could not resolve to a value —
   * has claimed a token that must be STEPPED before it can be an
   * argument. Go parks the call, steps the token, and the token decides:
   *
   *   - an unbound name raises `undefined_word`;
   *   - a function word begins its own dispatch, which is a statement
   *     boundary: the parked call COMMITS if an overload takes exactly
   *     what it has claimed so far, and is otherwise STRANDED — a
   *     `signature_error` (design/STRICT-FORWARD-BARRIER.0.md);
   *   - an engine-internal `__…` word is exempt from stranding and simply
   *     runs, leaving the call to take what the stack holds.
   *
   * A boru-bodied call here does not park — it binds and runs at once —
   * so the same three outcomes are decided at the match: the token's
   * kind is known before it is stepped, and the arguments ahead of it
   * are already values. A native word's call does park (beginForward),
   * and reaches the same outcomes by stepping.
   */
  private settleForwardWord(
    name: string,
    info: FnDefInfo,
    reduce: boolean,
    m: FnMatch,
  ): FnMatch | null {
    const { result } = m;
    // A word the scan could not resolve is still the token it was. A word
    // that is an argument because a name was BOUND to it is a value like
    // any other, and settles nothing. A word that merely STEPS to a value
    // — `null`, a type name in its path form — is that value by the time
    // it arrives, so it is the value that is bound.
    let at = -1;
    for (let i = 0; i < result.forwardCount && at < 0; i++) {
      const arg = result.args[i]!;
      if (!arg.isWord() || arg !== this.stack[this.pointer + 1 + i]) continue;
      const stepped = isRefWord(arg) ? undefined : this.wordValue(arg);
      if (stepped !== undefined) result.args[i] = stepped;
      else at = i;
    }
    if (at < 0) return m;
    const word = result.args[at]!;
    const wname = word.asWord().name;
    const isFunction =
      !isRefWord(word) &&
      (this.registry.topOfDefStack(wname) !== undefined ||
        this.registry.lookup(wname) !== undefined);
    if (!isFunction) {
      throw new BoruError("undefined_word", `undefined word: ${wname}`, wname);
    }
    if (wname.startsWith("__")) {
      // The frame-tail word runs on its own; the call is matched as if
      // the window ended there.
      const view = [...this.stack];
      view[this.pointer + 1 + at] = newEnd();
      return this.matchFnSigsOn(view, name, info, reduce);
    }
    // COMMIT: an overload of exactly the claimed arity — the forward
    // arguments ahead of the word, then the stack arguments the plan
    // took — fires with those and leaves the word to begin its own
    // statement.
    const claimed = [
      ...result.args.slice(0, at),
      ...result.args.slice(result.forwardCount),
    ];
    if (claimed.length > 0) {
      for (const fsig of info.sigs) {
        const total = fsig.params.length;
        const required = reduce
          ? fsig.params.filter((p) => !p.optional).length
          : total;
        if (claimed.length < required || claimed.length > total) continue;
        if (!claimed.every((a, i) => sigTypeMatches(a, fsig.params[i]!.type)))
          continue;
        return {
          result: {
            sig: result.sig,
            args: claimed,
            forwardCount: at,
            prefixCount: result.prefixCount,
          },
          sig: fsig,
          usedK: claimed.length,
        };
      }
    }
    throw new BoruError(
      "signature_error",
      `${name} is still waiting for an argument when \`${wname}\` begins its own ` +
        "dispatch — a function word is a barrier and never feeds forward collection " +
        "(strict rule); group the call in parens so its RESULT becomes the argument: " +
        `${name} (${wname} …)`,
      name,
    );
  }

  /**
   * The value stepping this word would put in its place, when stepping it
   * does no more than that: a keyword, a builtin type name, a value
   * binding. `undefined` for a word that dispatches a function and for a
   * name nothing binds. The word step's own cascade, asked without
   * stepping.
   */
  private wordValue(word: Value): Value | undefined {
    const name = word.asWord().name;
    if ("true" === name) return newBoolean(true);
    if ("false" === name) return newBoolean(false);
    if ("none" === name) return newNone();
    if ("null" === name) return newAtom("null");
    const tn = resolveBuiltinTypeName(name);
    if (tn !== undefined) return newTypeLiteral(tn);
    const top = this.registry.topOfDefStack(name);
    return top !== undefined && !top.isFnDef() ? top : undefined;
  }

  /**
   * Run a matched boru-bodied signature in a CALL FRAME: evaluate its
   * pending container arguments, bind its parameters, run the body, hold
   * the body's residual to the declared returns, and splice it where the
   * call stood.
   *
   * In Go the frame is a paren group on the caller's tape —
   * `(frame-open) body __DC [__RC] )` — whose cleanup marker unbinds the
   * parameters and whose return check reads the group's residual
   * (core/go/fn_frame.go AppendFrameTail; the Lean model's closeFrame).
   * Here the body runs in an engine of its own and the same two steps
   * bracket that run.
   */
  private callFnSig(name: string, m: FnMatch): void {
    const { result, sig: chosen, usedK } = m;
    // A pending container argument is a PROGRAM, and it runs before the
    // frame binds it — in the caller's scope, as a native word's argument
    // does. Binding first quoted the list as it was written, so
    // `inc [ addq 1 2 ]` handed the body `[addq 1 2]` where Go hands it
    // `[3]`.
    this.autoEvalArgs(result.args, result.sig);
    const total = chosen.params.length;
    // Bind each param on the def stack so the body can reference it.
    // Provided args bind directly; omitted optional params default to
    // their type's base value. Push the args list for the `args` word.
    const boundArgs: Value[] = [];
    for (let i = 0; i < total; i++) {
      const val = quoteListArg(
        i < usedK ? result.args[i]! : baseValue(chosen.params[i]!.type),
      );
      this.registry.pushDef(chosen.params[i]!.name, val);
      boundArgs.push(val);
    }
    this.registry.pushArgs(boundArgs);

    let bodyResult: Value[];
    try {
      bodyResult = this.subRun("frame", [...chosen.body]);
    } finally {
      this.registry.popArgs();
      for (let i = total - 1; i >= 0; i--) {
        this.registry.popDef(chosen.params[i]!.name);
      }
    }
    this.checkFrameReturns(name, chosen, bodyResult);

    // Splice the result over the consumed range (prefix + word +
    // forward args), exactly like a native handler.
    const replaceFrom = this.pointer - result.prefixCount;
    const replaceCount = result.prefixCount + 1 + result.forwardCount;
    this.stack.splice(replaceFrom, replaceCount, ...bodyResult);
    this.pointer = replaceFrom + bodyResult.length;
  }

  /**
   * THE FRAME'S RETURN CHECK. A signature that declares returns is a
   * contract on what its body leaves: exactly that many values, each one
   * a member of its declared type. A body that leaves anything else is a
   * `type_error` raised as the frame closes — the count first, then each
   * value in order. A signature that declares no returns makes no
   * promise and is not checked.
   *
   * Membership is the question the parameter boundary asks, so a value
   * is held to the same rule on the way out as on the way in
   * (core/go/engine.go validateReturnTypes, `v.Is(exp)`).
   */
  private checkFrameReturns(
    name: string,
    sig: import("./value.ts").FnSig,
    results: readonly Value[],
  ): void {
    const returns = sig.returns;
    if (0 === returns.length) return;
    if (results.length !== returns.length) {
      throw new BoruError(
        "type_error",
        `${name}: expected ${returns.length} return value(s), got ${results.length}`,
        name,
      );
    }
    for (let i = 0; i < returns.length; i++) {
      const want = returns[i]!;
      const got = results[i]!;
      if (got.vType.matches(want)) continue;
      throw new BoruError(
        "type_error",
        `${name}: return value ${i + 1}: expected ${want.toString()}, got ${got.toString()}`,
        name,
      );
    }
  }

  /**
   * Check-mode dispatch of a user fn. Binds the (carrier) args to the
   * matched signature's params, analyses the body in a check sub-engine
   * so its diagnostics propagate, and produces the result carriers:
   * declared return types when present (the body run is the proof
   * obligation), else the body's inferred residual. A no-match assumes
   * the first overload and emits no_signature; a recursive self-call is
   * broken via the fnInflight guard. Mirrors AnalyseFnBody +
   * buildFnBodyReturnsFn (Phase-2 subset — no memoised summaries or
   * fixed-point refinement).
   */
  private dispatchFnDefCheck(name: string, info: FnDefInfo): void {
    // Try to match an overload (largest arity first, optional-trailing).
    const m = this.matchFnSigs(name, info, true);

    // Gather the consumed span + args. A clean match uses the matcher's
    // positions; a no-match assumes the first overload, gathers adjacent
    // operands, and emits no_signature (the param types are violated).
    if (null !== m) {
      this.analyseFnMatch(name, m);
      return;
    }
    const chosen = [...info.sigs].sort(
      (a, b) => b.params.length - a.params.length,
    )[0]!;
    const n = chosen.params.length;
    const isBoundary = (v: Value): boolean =>
      v.isWord() || v.isForward() || v.isMark() || v.isMove();
    let fwd = 0;
    while (fwd < n && this.pointer + 1 + fwd < this.stack.length) {
      if (isBoundary(this.stack[this.pointer + 1 + fwd]!)) break;
      fwd++;
    }
    let stk = 0;
    while (fwd + stk < n && this.pointer - 1 - stk >= 0) {
      if (isBoundary(this.stack[this.pointer - 1 - stk]!)) break;
      stk++;
    }
    const args: Value[] = [];
    for (let i = 0; i < fwd; i++) args.push(this.stack[this.pointer + 1 + i]!);
    for (let i = 0; i < stk; i++) args.push(this.stack[this.pointer - 1 - i]!);
    const replaceFrom = this.pointer - stk;
    const replaceCount = stk + 1 + fwd;
    this.registry.check.addDiagnostic({
      code: "no_signature",
      detail: `cannot call \`${name}\` — no signature matches the arguments; assuming best-fit candidate for analysis`,
      word: name,
    });
    // The assumed dispatch analyses the body under args the real match
    // REJECTED — its dispatch failures are cascade noise (mirrors the Go
    // checkModeAssumeSig suppression); the no_signature above is the one
    // honest diagnostic.
    this.registry.check.suppressBodyErrors++;
    try {
      const out = this.analyseFnBody(name, chosen, args);
      this.stack.splice(replaceFrom, replaceCount, ...out);
      this.pointer = replaceFrom + out.length;
    } finally {
      this.registry.check.suppressBodyErrors--;
    }
  }

  /**
   * The check twin of callFnSig: analyse a matched signature's body under
   * the arguments the match found and splice its result carriers where the
   * call stood.
   */
  private analyseFnMatch(name: string, m: FnMatch): void {
    const { result, sig, usedK } = m;
    const out = this.analyseFnBody(name, sig, result.args.slice(0, usedK));
    const replaceFrom = this.pointer - result.prefixCount;
    const replaceCount = result.prefixCount + 1 + result.forwardCount;
    this.stack.splice(replaceFrom, replaceCount, ...out);
    this.pointer = replaceFrom + out.length;
  }

  /**
   * Analyse one fn signature's body under carrier args and return its
   * result carriers. Binds named params to their args (carrier-typed),
   * runs the body in a check sub-engine so body diagnostics propagate,
   * and returns the declared return carriers (Any → dynamic, dynamic
   * operands contagious) when the signature declares returns, else the
   * inferred residual. Recursive self-calls are broken via fnInflight.
   */
  private analyseFnBody(
    name: string,
    sig: import("./value.ts").FnSig,
    args: Value[],
  ): Value[] {
    const declared = sig.returns;
    const contagious = args.some((a) => a.carrier && a.dynamic);
    const declaredOut = (): Value[] =>
      declared.map((t) =>
        contagious || t.equal(TAny) ? newDynamicCarrier(t) : newCarrier(t),
      );

    const key = `${name}#${sig.params.length}`;
    if (this.registry.check.fnInflight.has(key)) {
      // Recursion: declared returns are the induction hypothesis; an
      // unchecked fn breaks the cycle with a dynamic Any.
      return declared.length > 0 ? declaredOut() : [newDynamicCarrier(TAny)];
    }
    this.registry.check.fnInflight.add(key);
    this.registry.check.fnBodyDepth++;
    let bodyResult: Value[];
    const bound: string[] = [];
    const frame: Value[] = [];
    try {
      for (let i = 0; i < sig.params.length; i++) {
        const p = sig.params[i]!;
        // A missing optional param defaults to its type's base value (a
        // concrete, bakeable default — exactly what dispatchFnDef binds at
        // run time), so an inlined body using it still compiles.
        const v = quoteListArg(
          args[i] ?? (p.optional ? baseValue(p.type) : newDynamicCarrier(TAny)),
        );
        if (p.name !== "") {
          this.registry.pushDef(p.name, v);
          bound.push(p.name);
        }
        frame.push(v);
      }
      // Push the args frame so `args` inside the inlined body resolves to the
      // param carriers (recorded as a makeList), matching the interpreter.
      this.registry.pushArgs(frame);
      bodyResult = new Engine(this.registry).run([...sig.body]);
    } finally {
      this.registry.popArgs();
      for (let i = bound.length - 1; i >= 0; i--)
        this.registry.popDef(bound[i]!);
      this.registry.check.fnBodyDepth--;
      this.registry.check.fnInflight.delete(key);
    }
    if (declared.length === 0) return bodyResult;
    const out = declaredOut();
    // Compile path: INLINE the fn. The body already recorded its events
    // into the trace (run above, in check mode with emit active); alias
    // each declared-return carrier to the corresponding body residual so
    // the fn's result threads to those events. Mismatched counts can't be
    // inlined cleanly → refuse (fall back).
    const emit = this.registry.check.emit;
    if (emit !== undefined) {
      if (out.length === bodyResult.length) {
        for (let i = 0; i < out.length; i++)
          emit.alias(out[i]!, bodyResult[i]!);
      } else {
        emit.markUncompilable(
          `fn ${name}: declared/body result count mismatch`,
        );
      }
    }
    return out;
  }

  /**
   * Return true iff the nearest preceding ForwardMarker (within the
   * current paren scope) is waiting for a Word-typed arg at its
   * next collection slot. Mirrors borueng/go/engine.go's
   * hasPendingForwardExpectingWord — the gate that lets `def NAME`
   * capture NAME as data even when it would otherwise dispatch.
   */
  private pendingExpectsWord(): boolean {
    const idx = this.findPendingMarker();
    if (idx < 0) return false;
    const m = this.stack[idx]!.asForward();
    const nextIdx = m.collected.length;
    if (nextIdx >= m.sig.args.length) return false;
    const expected = m.sig.args[nextIdx]!;
    // Only an EXPLICIT TWord/TAtom slot suppresses dispatch, or a slot
    // marked `/q`. TAny also accepts Word values, but at TAny slots we
    // still want the engine to dispatch the word and feed its result
    // back. Mirrors borueng/go/engine.go's hasPendingForwardExpectingWord,
    // which checks `Equal(TWord)` and the /q flag, never
    // TAny.matches(TWord).
    //
    // The /q half was MISSING here, and the strict forward barrier is
    // what exposed it: `def h fn […]` parks def with one arg and then
    // meets `fn`, which Go captures into def's quote-marked slot before
    // any barrier check runs. Without the /q test the word reached
    // stepWord instead, and the barrier — correctly, for a word that
    // really is beginning its own dispatch — stranded def. The keyword
    // forms this protects are load-bearing under the strict rule
    // (design/STRICT-FORWARD-BARRIER.0.md, "two sharp edges").
    if (m.sig.quoteArgs?.has(nextIdx) === true) return true;
    return expected.equal(TWord) || expected.equal(TAtom);
  }

  /**
   * Walk backward from the pointer, stopping at open-paren markers,
   * and return the index of the nearest unfilled ForwardMarker. -1
   * if none.
   */
  /**
   * The strict forward barrier's failure case, ported from Go's
   * strandedForwardError (core/go/engine.go:6504). Called from stepWord
   * when a function word is about to begin its own dispatch: the nearest
   * pending forward in the CURRENT paren scope will never be fed by that
   * word under the strict rule, so if it cannot fire with what it holds
   * it is an error rather than a wait-through.
   *
   * The scope scan stops at an open paren, which is what makes
   * `boomq ( negq 5 )` fire and `boomq negq 5` refuse — the distinction
   * the rule exists to create.
   *
   * DELIBERATELY A SUBSET of Go's rule, and the boundary is exact. Go
   * calls commitBarrierForward FIRST: a parked word that can already
   * dispatch with the args it CLAIMS (collected forward + stack) is fired
   * at the boundary instead of stranded, and only a failed commit strands.
   * That half is not ported — it needs the tape rearrangement Go performs
   * and a word with a shorter real overload than the parked plan assumed.
   *
   * So this fires only when the parked word has claimed NOTHING, which is
   * precisely where Go's commitBarrierForward returns false on its first
   * test ("Nothing collected yet — no smaller-arity dispatch to commit").
   * Where it fires it matches Go exactly; where it stays silent the
   * divergence is still in the ledger. Widening it without the commit half
   * is wrong, and measurably so: doing that stranded `def h fn […]` at
   * 1-of-2 args and turned 44 eng/ts rows red.
   *
   * Engine-internal frame-tail words (`__…`) are exempt on both sides:
   * they are not source-level statement boundaries.
   */
  private strandedForwardError(boundary: string): BoruError | undefined {
    if (boundary.startsWith("__")) return undefined;
    const fwdIdx = this.findPendingMarker();
    if (fwdIdx < 0) return undefined;
    const m = this.stack[fwdIdx]!.asForward();
    if (0 !== m.collected.length || 0 !== m.stackArgs.length) return undefined;
    const missing = m.expectedForward - m.collected.length;
    return new BoruError(
      "signature_error",
      `${m.funcName} is still waiting for ${missing} argument(s) when \`${boundary}\` ` +
        "begins its own dispatch — a function word is a barrier and never feeds " +
        "forward collection (strict rule); group the call in parens so its RESULT " +
        `becomes the argument: ${m.funcName} (${boundary} …)`,
      m.funcName,
    );
  }

  private findPendingMarker(): number {
    for (let i = this.pointer - 1; i >= 0; i--) {
      const v = this.stack[i]!;
      if (isOpenParen(v)) return -1;
      if (v.isForward()) {
        // A marker that FILLED is replaced by its results in the same
        // step, so one below the pointer is always still collecting. It
        // is a scope boundary either way — a second marker further back
        // cannot be fed past this one.
        const m = v.asForward();
        return m.collected.length < m.expectedForward ? i : -1;
      }
    }
    return -1;
  }

  /**
   * Handle a literal (or word-treated-as-literal) at the pointer.
   * If a pending marker can absorb it, collect; otherwise advance.
   */
  private stepLiteral(): void {
    const fwdIdx = this.findPendingMarker();
    if (fwdIdx < 0) {
      const val = this.stack[this.pointer]!;
      // A dispatch-modifier marker reaching the pointer on its own stands
      // after a value that was not a function — `( 1 addq 2 )/v`. The
      // modifier has nothing to say about a non-function: it is dropped.
      if (val.vType.equal(TDispatchMod)) {
        this.stack.splice(this.pointer, 1);
        return;
      }
      // A SPLICE marker fires where it stands with nothing collecting
      // it: it is replaced, unevaluated, by what it holds — a plain
      // list's elements, any other value itself — and the pointer stays,
      // so the spliced tokens run against the live stack (core/go
      // stepLiteral, SpliceExpand). With a word collecting, the marker
      // is an argument like any other value and does not fire.
      if (isSplice(val)) {
        this.stack.splice(this.pointer, 1, ...spliceExpand(val));
        return;
      }
      // A function VALUE standing at the pointer is a CALL.
      if (callsAtPointer(val)) {
        this.stepFnValue(val);
        return;
      }
      this.pointer++;
      return;
    }
    this.collectArrival(fwdIdx);
  }

  /**
   * A function VALUE at the pointer dispatches as its word would: the
   * forward window's groups, the match, then a parked forward or an
   * immediate run. Mirrors Go's execFnDefLiteral and the Lean model's
   * dispatchFnValue.
   *
   * The value is DATA in exactly two cases. A `/v` or `/q` modifier
   * marker right after it — what the parser emits for `( … )/v` — says
   * so, and the value is placed. And a value that matches nothing with
   * nothing to be called on is a function being passed around, not a
   * failed call; with anything at all around it, a NAMED function that
   * matches nothing is an error (fnValueUnmatched).
   */
  private stepFnValue(val: Value): void {
    const info = val.asFnDef();
    const next = this.stack[this.pointer + 1];
    if (next !== undefined && next.vType.equal(TDispatchMod)) {
      this.stack.splice(this.pointer + 1, 1);
      this.pointer++;
      return;
    }
    if (info.native !== undefined) {
      this.dispatchEntry(val.fnName ?? info.native.name, info.native, true);
      return;
    }
    // A function reached through a NAME is the function its word calls,
    // optional parameters and all; a bare literal is held to the
    // signatures as written (matchFnSigs).
    const name = val.fnName ?? "";
    const m = this.matchFnSigs(name, info, val.fnName !== undefined);
    if (null === m) {
      this.fnValueUnmatched(val.fnName);
      return;
    }
    if (this.registry.check.isActive()) this.analyseFnMatch(name, m);
    else this.callFnSig(name, m);
  }

  /**
   * A function value was called and matched no signature.
   *
   * A NAMED function with anything to be called on — a value beneath it
   * in its paren scope, or a token after it up to the next boundary — is
   * an `uncalled_function` error, raised here at the dispatch site: the
   * program wrote a call and the call failed. With nothing at all around
   * it, and for a nameless function always, the value is simply placed,
   * which is how a function is passed as data. Mirrors the tail of Go's
   * ExecFnDefSigStackMatch.
   *
   * PARITY NOTE: a check pass places the value in every case. Go reports
   * the finding as a diagnostic where the position is unconditionally
   * reached; the TS check piece has no reachability model to gate it on.
   */
  private fnValueUnmatched(name: string | undefined): void {
    if (name !== undefined && !this.registry.check.isActive()) {
      let candidates = 0;
      for (let i = this.pointer - 1; i >= 0; i--) {
        const v = this.stack[i]!;
        if (isOpenParen(v)) break;
        if (!v.isForward()) candidates++;
      }
      let bounded = false;
      for (let i = this.pointer + 1; i < this.stack.length && !bounded; i++) {
        const v = this.stack[i]!;
        if (v.isForward()) continue;
        if (isCloseParen(v) || isEnd(v) || v.isMark() || v.isMove())
          bounded = true;
        else candidates++;
      }
      // In a call frame the body is followed by the frame's own cleanup
      // marker, and Go counts it: the walk over what follows stops at a
      // close paren, an `end`, a mark or a move, and the marker is none
      // of them. So a named function that matches nothing at the END of a
      // frame's body is a failed call even with nothing else around it.
      // (That reads as an accident of the walk rather than a rule — the
      // marker is no argument — and is reported as such; it is what
      // core/go and the Lean model both answer.)
      if ("frame" === this.kind && !bounded) candidates++;
      if (candidates > 0) {
        throw new BoruError(
          "uncalled_function",
          `call to '${name}' matched no signature\n` +
            `  = hint: check the call's argument types and arity — or use ${name}/v ` +
            "to push the function as a value deliberately",
          name,
        );
      }
    }
    this.pointer++;
  }

  /**
   * The value at the pointer ARRIVES at the collection parked at
   * `fwdIdx`: it fills the next slot if it fits, and the collection
   * fails if it does not.
   */
  private collectArrival(fwdIdx: number): void {
    const m = this.stack[fwdIdx]!.asForward();
    const nextIdx = m.collected.length;
    const expected = m.sig.args[nextIdx]!;
    const val = this.stack[this.pointer]!;

    // Type-check. Words at TWord/TAtom slots match directly; other
    // values check via sigTypeMatches.
    let matches: boolean;
    if (val.isWord()) {
      // A Word can fill a TWord/TAtom slot, or a TAny slot (data).
      // For other slot types it must resolve via def-sub first; that
      // happens at stepWord, so reaching here with a Word means
      // either we want it as data (TWord/TAtom/TAny) or it's a
      // mismatch.
      matches =
        expected.equal(TWord) || expected.equal(TAtom) || expected.equal(TAny);
    } else {
      matches = val.vType.matches(expected);
    }
    if (!matches) {
      // Implicit end of forward collection — fail the dispatch.
      throw new BoruError(
        "signature_error",
        `forward arg ${nextIdx} type mismatch for ${m.funcName}: expected ${expected.toString()}, got ${val.toString()}`,
        m.funcName,
      );
    }

    // Collect: remove value from current position, append to marker.
    this.stack.splice(this.pointer, 1);
    m.collected.push(val);
    // pointer stays — it now points at what came after the value.

    if (m.collected.length === m.expectedForward) {
      this.completeForward(fwdIdx);
    }
  }

  /**
   * Handle the `end` keyword. If a forward marker is pending in the
   * current paren scope, complete it with whatever's been collected
   * so far (forward args + stack args from the original match) — the
   * collection short-circuits before all expected slots arrive.
   * Otherwise just remove the `end` token. Mirrors stepEnd in
   * borueng/go/engine.go.
   */
  /**
   * Fire a sugar marker at the pointer: splice the marker's role-
   * resolved expansion in place and re-step. The Angle marker lowers
   * to its generic-def head form when a pending forward is waiting to
   * capture a name (the binder's name slot), the use-site paren
   * otherwise. Mirrors Go stepSugar / SugarExpansion.
   */
  private stepSugar(val: Value): void {
    const info = asSugar(val);
    if (info === undefined) {
      // A sugar TYPE with no SugarInfo payload. core's own constructor
      // cannot build one, but the type is exported and eng/ts builds the
      // shape directly, so the token is stepped over rather than crashing
      // the pass. (Asserting the payload here instead was tried and took
      // an eng/ts row with it.)
      this.pointer++;
      return;
    }
    const headForm = info.kind === "angle" && this.pendingExpectsWord();
    const exp = sugarExpansion(this.registry, info, headForm);
    this.stack.splice(this.pointer, 1, ...exp);
  }

  private stepEnd(): void {
    const fwdIdx = this.findPendingMarker();
    if (fwdIdx < 0) {
      // No pending forward — just drop the end token.
      this.stack.splice(this.pointer, 1);
      return;
    }
    // Drop the end token first so the marker is no longer "pending"
    // when completeForward runs.
    this.stack.splice(this.pointer, 1);
    this.completeForwardPartial(fwdIdx);
  }

  /**
   * Variant of completeForward that fires a marker with fewer
   * collected forward args than expected. Used by stepEnd. The args
   * list is built from whatever's collected so far (any unfilled
   * forward slots stay missing, which the handler must tolerate).
   * For the spec subset this is rarely meaningful, but the path
   * keeps shape parity with Go's stepEnd → implicitEnd flow.
   */
  private completeForwardPartial(fwdIdx: number): void {
    const m = this.stack[fwdIdx]!.asForward();
    const args = [...m.collected, ...m.stackArgs];
    if (args.length < m.sig.args.length) {
      throw new BoruError(
        "signature_error",
        `${m.funcName}: 'end' before all forward args collected (have ${args.length}, need ${m.sig.args.length})`,
        m.funcName,
      );
    }
    this.fireMarker(fwdIdx, m, args);
  }

  /**
   * Build the full args list (forward then stack, in sig order) and
   * dispatch the marker's sig. The marker is replaced by the result.
   */
  private completeForward(fwdIdx: number): void {
    const m = this.stack[fwdIdx]!.asForward();
    const args = [...m.collected, ...m.stackArgs];
    this.fireMarker(fwdIdx, m, args);
  }

  /** Run the marker's handler with `args` and replace it with the result. */
  private fireMarker(fwdIdx: number, m: ForwardMarker, args: Value[]): void {
    this.autoEvalArgs(args, m.sig);
    // Check mode: a DEFERRED dispatch must record like the immediate
    // short-circuit (carrierResults + recordCall/recordFallback) — else a
    // forward-deferred word (typeof fnsig […], nested typeof) produces a
    // result with no provenance and the program refuses.
    if (this.registry.check.isActive() && !m.sig.runInCheckMode) {
      const out = AnalysisImpl.carrierResults(
        this.registry,
        m.funcName,
        m.sig,
        args,
      );
      const emit = this.registry.check.emit;
      if (emit !== undefined && !m.sig.recordsOwnEvent) {
        if (m.sig.compileFallback) {
          if (
            !emit.recordFallback(
              m.funcName,
              args,
              out,
              this.registry,
              m.sig.noEvalArgs,
            )
          ) {
            emit.markUncompilable(
              `${m.funcName}: fallback island not recordable`,
            );
          }
        } else {
          emit.recordCall(m.funcName, m.sig, args, out);
        }
      }
      this.stack.splice(fwdIdx, 1, ...out);
      this.pointer = fwdIdx;
      return;
    }
    const handlerResult = m.sig.handler(args, null, [], this.registry);
    if (handlerResult instanceof Promise) {
      throw new BoruError(
        "unsupported",
        `async handlers are not supported in the TS port`,
        m.funcName,
      );
    }
    const out = handlerResult as Value[];
    this.stack.splice(fwdIdx, 1, ...out);
    this.pointer = fwdIdx;
  }

  /**
   * Handle a Move at the pointer. Walk back to find the matching
   * Mark; replace [Mark .. body .. Move] with the saved body so the
   * body re-runs from the start. If no matching Mark is on the
   * stack, drop the orphaned Move silently. Mirrors borueng/go/engine.go's
   * stepMove (one-shot replay variant — Cont/IfCont continuations
   * aren't ported here).
   */
  private stepMove(val: Value): void {
    const info: MoveInfo = val.asMove();
    const moveIdx = this.pointer;

    if (!this.markIds.has(info.to)) {
      // Mark was removed (or never seen). Drop the orphan.
      this.stack.splice(moveIdx, 1);
      return;
    }

    let markIdx = -1;
    for (let i = 0; i < this.stack.length; i++) {
      const v = this.stack[i]!;
      if (v.isMark() && v.asMark().id === info.to) {
        markIdx = i;
        break;
      }
    }
    if (markIdx < 0) {
      this.markIds.delete(info.to);
      this.stack.splice(moveIdx, 1);
      return;
    }

    // A CONDITIONAL move reads the condition's result instead of
    // replaying the body.
    if (info.ifCont !== undefined) {
      this.stepMoveIf(markIdx, moveIdx, info.ifCont, info.to);
      return;
    }

    const markInfo = this.stack[markIdx]!.asMark();
    this.markIds.delete(info.to);
    const body = [...markInfo.body];
    // Replace [Mark .. body .. Move] with the body copy.
    this.stack.splice(markIdx, moveIdx - markIdx + 1, ...body);
    this.pointer = markIdx;
  }

  /**
   * Fire an if-continuation move: the values between the mark and the
   * move ARE the condition's result, so the LAST of them decides the
   * branch, and the chosen branch replaces the whole span. A condition
   * that produced nothing is an error rather than a silent false — an
   * `if` whose condition vanished has no answer, and quietly taking the
   * else arm would hide the mistake. Mirrors Go stepMoveIf.
   */
  private stepMoveIf(
    markIdx: number,
    moveIdx: number,
    cont: import("./value.ts").IfCont,
    id: string,
  ): void {
    let condResult: Value | undefined;
    for (let j = markIdx + 1; j < moveIdx; j++) condResult = this.stack[j];
    this.markIds.delete(id);
    if (condResult === undefined) {
      this.stack.splice(markIdx, moveIdx - markIdx + 1);
      this.pointer = markIdx;
      throw new BoruError(
        "runtime_error",
        "if: condition produced no value",
        "if",
      );
    }
    const branch = coerceBoolean(condResult) ? cont.then : (cont.else ?? []);
    this.stack.splice(markIdx, moveIdx - markIdx + 1, ...branch);
    this.pointer = markIdx;
  }

  /**
   * Auto-evaluate any TList args carrying `eval=true && !quoted`,
   * unless the sig declares NoEvalArgs for that position. Mirrors
   * borueng/go/engine.go's pre-handler autoEvalList step in execMatch.
   * Mutates `args` in place.
   */
  private autoEvalArgs(args: Value[], sig: Signature): void {
    for (let i = 0; i < args.length; i++) {
      const a = args[i]!;
      if (sig.noEvalArgs?.has(i)) continue;
      // In compile mode, deep-evaluate a map arg through deepEvalData so it
      // records a makeMap and carries provenance (typeof/is over a computed
      // map). Gated on emit; value mode and plain checking are unchanged.
      if (
        this.registry.check.emit !== undefined &&
        a.vType.matches(TMap) &&
        a.data instanceof OrderedMap
      ) {
        // Resolve def-bound / type-name WORDS while the registry is
        // still live (record time): the VM promotes defs to frame
        // locals, so a baked constraint carrying word(M) could never
        // resolve at run time — the compiled `is` then rejected what
        // the interpreter admitted ({x?:M} with a def'd union M).
        args[i] = this.deepEvalData(resolveWordsDeep(a, this.registry));
        continue;
      }
      // A map arg evaluates its VALUES at consumption — the Go
      // autoEvalMap mirror: words resolve through the cascade
      // (`{abs:true}` arrives with word(true) from the opaque parser)
      // and expression values (`{abs:( true )}`, eval-lists) run in a
      // sub-engine so the handler sees the computed value.
      // Gated on `eval` exactly as the LIST arm below is, and as Go's
      // autoEvalStack gates both. Only a container the PARSER built
      // auto-evaluates; one a word handler returned is data. Without the
      // gate core/ts evaluated a runtime map's values at consumption, so
      // `boomq {q a: p( nosuchword ) }` raised undefined_word where Go
      // fires the handler on the map as given.
      if (
        a.vType.matches(TMap) &&
        a.data instanceof OrderedMap &&
        a.eval &&
        !a.quoted
      ) {
        args[i] = this.autoEvalMapValues(a);
        continue;
      }
      if (!a.vType.matches(TList)) continue;
      if (!a.eval || a.quoted) continue;
      if (!a.isConcrete()) continue;
      args[i] = this.autoEvalList(a);
    }
  }

  /**
   * The two INERT shapes whose names resolve without running anything:
   * a typed container (its child constraint) and a disjunct (its
   * alternatives). Returns undefined for every other value, which is
   * then a program. Mirrors Go's resolveInertTypeShape.
   *
   * Go additionally reports whether resolution CHANGED the value, and
   * falls through to the sub-engine when it did not. The two paths agree
   * for these shapes — a typed container and a disjunct are both
   * literals to the step loop, so an unchanged one comes back from the
   * sub-engine identical — so the flag has no analogue here.
   */
  private resolveInertTypeShape(v: Value): Value | undefined {
    if (v.isTypedList() || v.isTypedMap()) {
      const ct = v.asChildType();
      const child = resolveWordsDeep(ct.child, this.registry);
      return v.isTypedMap()
        ? newTypedMap(child, ct.entries)
        : newTypedList(child, ct.elements);
    }
    if (v.isDisjunct()) return resolveWordsDeep(v, this.registry);
    return undefined;
  }

  /**
   * Evaluate a consumed map argument's values — the Go autoEvalMap
   * mirror: a ParenExpr value runs in a sub-engine to its single
   * result, an unquoted eval-list auto-evaluates, nested maps recurse,
   * an inert type shape resolves its names, and anything else is a
   * program that runs in a sub-engine.
   */
  private autoEvalMapValues(m: Value): Value {
    const src = m.asMap();
    const out = new OrderedMap();
    for (const k of src.keys()) {
      const v = src.get(k)!;
      if (v.vType.equal(TParenExpr) && !v.quoted && Array.isArray(v.data)) {
        const res = this.subRun("region", [...(v.data as Value[])]);
        out.set(k, res.length === 1 ? res[0]! : v);
        continue;
      }
      if (
        v.vType.matches(TList) &&
        Array.isArray(v.data) &&
        v.eval &&
        !v.quoted &&
        v.isConcrete()
      ) {
        out.set(k, this.autoEvalList(v));
        continue;
      }
      // Gated on `eval` like every other container arm. Without it a
      // NESTED runtime map had its values evaluated even though its
      // parent's gate had already said no.
      if (
        v.vType.matches(TMap) &&
        v.data instanceof OrderedMap &&
        v.eval &&
        !v.quoted
      ) {
        out.set(k, this.autoEvalMapValues(v));
        continue;
      }
      // An INERT TYPE SHAPE — a typed container's child constraint, a
      // disjunct's alternatives — resolves its names and nothing else.
      // Go's resolveInertTypeShape is this exact pair of cases, and it is
      // the ONLY place a map value's names resolve.
      const inert = this.resolveInertTypeShape(v);
      if (inert !== undefined) {
        out.set(k, inert);
        continue;
      }
      // Everything else is a PROGRAM and runs in a sub-engine: a bare
      // word resolves through a def, a 0-arg fn auto-fires, an unbound
      // name raises undefined_word, and a DATA list comes back untouched
      // because a list is a literal there.
      //
      // This was resolveWordsDeep, which resolves names EVERYWHERE rather
      // than only in the two inert shapes. Two defects came out of that:
      // `{a: nosuchword}` degraded an undefined name to an Atom and fired
      // the consuming word where Go reports the name, and `{a: [q x]}`
      // rewrote a data list's words — `[word(x)]` on Go, `[5]` here.
      const sub = this.subRun("region", [v]);
      if (1 === sub.length) {
        out.set(k, sub[0]!);
        continue;
      }
      // Zero residuals drops the key, as everywhere else.
      if (0 === sub.length) continue;
      out.set(k, this.buildList(sub));
    }
    // Preserve the map's lattice subtype (an Inspect map must stay
    // Inspect, not demote to plain Map) and flags.
    return new Value(m.vType, out, { eval: m.eval, quoted: m.quoted });
  }

  /**
   * Auto-evaluate a single TList: run a fresh sub-engine on its
   * elements and wrap the residual stack as a new (non-eval) list.
   * The result is data — clearing eval=true ensures it doesn't
   * recursively re-evaluate when consumed by an outer caller.
   */
  private autoEvalList(list: Value): Value {
    // In compile mode, evaluate through deepEvalData so a computed list arg
    // records a makeList event and carries provenance (lengthq [1 addq 2]) —
    // otherwise the evaluated list is an opaque value the recorder refuses.
    if (this.registry.check.emit !== undefined) return this.deepEvalData(list);
    const elems = list.asList();
    const result = this.subRun("region", [...elems]);
    return new Value(list.vType, result, { eval: false, quoted: false });
  }

  /**
   * End-of-Run pass: any TList still on the stack with eval=true and
   * !quoted gets auto-evaluated. Mirrors Go's autoEvalStack drain.
   */
  /**
   * Evaluate an interpolated string: literal segments append verbatim;
   * expression segments run in a sub-engine and each residual value is
   * stringified (ValToString) and concatenated. Mirrors
   * eng/go's evalInterpString.
   */
  private evalInterpString(v: Value): Value {
    const segs = v.asInterpSegments();
    // Bytecode recording: an interpolated EXPRESSION assembles a string from
    // runtime values (valToString), which the recorder does not model and
    // which in check mode would stringify embedded carriers to type names.
    // A SELF-CONTAINED template (no embedded reference to a binding) compiles
    // as a single-value island — re-evaluate the token verbatim at run time;
    // one capturing a binding refuses (the binding isn't live in the island).
    const emit = this.registry.check.emit;
    if (emit !== undefined && segs.some((s) => !("lit" in s))) {
      // Substitute each const-bound capture inline (def x 5 -> the 5), keep
      // natives/keywords as-is, then island the SELF-CONTAINED result so it
      // re-runs faithfully. A computed (non-const) capture can't be baked in,
      // so it refuses.
      const subbed = this.substituteInterp(segs);
      if (subbed !== null) {
        const out = newCarrier(TString);
        emit.recordValueIsland(newInterpString(subbed), out, "interp");
        return out;
      }
      emit.markUncompilable("interpolated string: unsubstitutable capture");
    }
    let out = "";
    for (const seg of segs) {
      if ("lit" in seg) {
        out += seg.lit;
      } else {
        const residual = new Engine(this.registry).run([...seg.expr]);
        out += residual.map((r) => valToString(r)).join("");
      }
    }
    return newString(out);
  }

  /**
   * Rewrite an interpolated string's expression segments so it is
   * self-contained: a const-bound capture (def x 5 → `${x}`) is replaced by
   * its concrete value; a native/keyword reference is kept (it re-dispatches
   * fine in the island). Returns null if a captured binding is a computed
   * (non-const) value that can't be baked in — the caller then refuses.
   */
  /**
   * Report whether an xml template references no currently-bound name (so it
   * re-runs faithfully as an island). Walks attribute segments and child
   * expressions; a bare word resolving to a def-stack binding is a capture.
   */
  private xmlCaptureFree(t: import("./value.ts").XmlTmpl): boolean {
    const walk = (toks: readonly unknown[]): boolean => {
      for (const x of toks) {
        if (!(x instanceof Value)) continue;
        if (x.isWord()) {
          if (this.registry.topOfDefStack(x.asWord().name) !== undefined)
            return false;
        } else if (x.isInterpString()) {
          for (const s of x.asInterpSegments())
            if (!("lit" in s) && !walk(s.expr)) return false;
        } else if (x.isXmlInterp()) {
          if (!this.xmlCaptureFree(x.asXmlTmpl())) return false;
        } else if (Array.isArray(x.data)) {
          if (!walk(x.data)) return false;
        }
      }
      return true;
    };
    for (const a of t.attrs)
      for (const s of a.segs) if (!("lit" in s) && !walk(s.expr)) return false;
    for (const c of t.children) {
      if ("expr" in c && !walk(c.expr)) return false;
      if ("elem" in c && !this.xmlCaptureFree(c.elem)) return false;
    }
    return true;
  }

  private substituteInterp(
    segs: readonly import("./value.ts").InterpSegment[],
  ): import("./value.ts").InterpSegment[] | null {
    const emit = this.registry.check.emit!;
    // subTok rewrites one token to the span that reproduces it inside the
    // island (usually length 1). A captured binding becomes its const value,
    // or — when it was produced by a 0-input token island (`def x quote [..]`)
    // — the producer's own token span, re-run inline. null = unbakeable.
    const subTok = (t: unknown): Value[] | null => {
      if (!(t instanceof Value)) return null;
      if (t.isWord()) {
        const top = this.registry.topOfDefStack(t.asWord().name);
        if (top === undefined) return [t]; // native / keyword — re-dispatches in the island
        const constVal = emit.constValueOf(emit.classify(top));
        if (constVal !== null) return [constVal]; // const-bound → bake
        const isl = emit.islandTokensFor(top); // islanded binding → inline its producer
        return isl !== null ? [...isl] : null;
      }
      if (t.isInterpString()) {
        const sub = this.substituteInterp(t.asInterpSegments());
        return sub === null ? null : [newInterpString(sub)];
      }
      if (t.isXmlInterp()) return null; // nested xml not islanded here
      if (Array.isArray(t.data)) {
        const elems: Value[] = [];
        for (const e of t.data as unknown[]) {
          const s = subTok(e);
          if (s === null || s.length !== 1) return null; // list element must map 1:1
          elems.push(s[0]!);
        }
        return [new Value(t.vType, elems, { eval: t.eval, quoted: t.quoted })];
      }
      return [t];
    };
    const out: import("./value.ts").InterpSegment[] = [];
    for (const s of segs) {
      if ("lit" in s) {
        out.push(s);
        continue;
      }
      const expr: Value[] = [];
      for (const t of s.expr) {
        const st = subTok(t);
        if (st === null) return null;
        expr.push(...st);
      }
      out.push({ expr });
    }
    return out;
  }

  /**
   * Resolve an XML template's holes: attribute segments evaluate and
   * concatenate to strings; a child hole evaluates and contributes a
   * string (scalar), a child element (Xml), or spliced children (list).
   */
  private resolveXmlTmpl(
    t: import("./value.ts").XmlTmpl,
  ): import("./value.ts").XmlElement {
    // The recorder does not model runtime XML assembly — refuse so a program
    // containing an XML template falls back to the interpreter.
    this.registry.check.emit?.markUncompilable("xml template not compilable");
    const evalSegs = (segs: import("./value.ts").InterpSegment[]): string =>
      segs
        .map((seg) =>
          "lit" in seg
            ? seg.lit
            : new Engine(this.registry)
                .run([...seg.expr])
                .map((r) => valToString(r))
                .join(""),
        )
        .join("");
    const children: (string | import("./value.ts").XmlElement)[] = [];
    for (const c of t.children) {
      if ("lit" in c) {
        children.push(c.lit);
      } else if ("elem" in c) {
        children.push(this.resolveXmlTmpl(c.elem));
      } else {
        for (const r of new Engine(this.registry).run([...c.expr])) {
          if (r.isXml())
            children.push(r.data as import("./value.ts").XmlElement);
          else if (Array.isArray(r.data)) {
            for (const e of r.asList()) {
              if (e.isXml())
                children.push(e.data as import("./value.ts").XmlElement);
              else children.push(valToString(e));
            }
          } else children.push(valToString(r));
        }
      }
    }
    return {
      tag: t.tag,
      attrs: t.attrs.map((a) => ({ name: a.name, value: evalSegs(a.segs) })),
      children,
    };
  }

  private autoEvalStack(): void {
    for (let i = 0; i < this.stack.length; i++) {
      this.stack[i] = this.deepEvalData(this.stack[i]!);
    }
  }

  /**
   * Deep-evaluate a residual data value: a map resolves its values
   * (def words, parens, eval-lists collapse to the body's single
   * residual or a list); an eval-list runs in a sub-engine and its
   * elements are deep-evaluated; scalars pass through. Mirrors the
   * parser's eval-map / eval-list data-context semantics.
   */
  private deepEvalData(v: Value): Value {
    // The MAP arm carries the same `eval && !quoted` gate as the list arm
    // below, because Go's autoEvalStack applies it to both: only a
    // container the PARSER built (Eval=true) auto-evaluates, and a value a
    // word handler returned stays exactly as the handler left it. The gate
    // was missing here, so a runtime-created map had its values evaluated
    // in TS and not in Go — `{ a: [ addq 1 2 ] }` handed to the step loop
    // as a non-eval map rendered `{a:[3]}` against Go's
    // `{a:[word(addq) 1 2]}`, and `{ a: [ boomq 1 ] }` RAISED in TS where
    // Go returned the map untouched. Pinned by core/spec/data.tsv.
    if (
      v.data instanceof OrderedMap &&
      v.vType.equal(TMap) &&
      v.eval &&
      !v.quoted
    ) {
      // A value that evaluated to NOTHING drops its key — see
      // evalMapValue. Keys and values are filtered together so buildMap
      // still sees two aligned arrays.
      const keys: string[] = [];
      const vals: Value[] = [];
      for (const k of v.asMap().keys()) {
        const ev = this.evalMapValue(v.asMap().get(k)!);
        if (undefined === ev) continue;
        keys.push(k);
        vals.push(ev);
      }
      return this.buildMap(keys, vals);
    }
    if (
      v.vType.matches(TList) &&
      Array.isArray(v.data) &&
      v.eval &&
      !v.quoted
    ) {
      const sub = this.subRun("region", [...v.asList()]).map((e) =>
        this.deepEvalData(e),
      );
      return this.buildList(sub);
    }
    return v;
  }

  /**
   * Build a computed list from its evaluated elements. In compile mode
   * (check + active EmitState) this records a makeList event and returns
   * a provenance-bearing list carrier so the VM assembles the list via
   * OpMakeList; otherwise it builds the concrete list directly. Falls
   * back (markUncompilable) if an element has no operand provenance.
   */
  private buildList(elems: Value[]): Value {
    const emit = this.registry.check.emit;
    if (emit !== undefined) {
      const ops: RecorderOperand[] = [];
      for (const e of elems) {
        const o = emit.classify(e);
        if (o === null) {
          emit.markUncompilable("makeList: element of unknown provenance");
          return newList(elems, { eval: false });
        }
        ops.push(o);
      }
      // Const-fold a fully-inert DATA/TYPE list: bake the concrete value as one
      // const rather than assembling it via OpMakeList. This keeps the value's
      // structure live (not a carrier), so a consumer that reads it — `inspect`
      // of a def bound to a computed type — can introspect it. Mirrors Go's
      // const-folded computed containers. A list bearing a fn-value member keeps
      // the makeList path (a baked fn-value member dispatches dynamically when
      // the container is later consumed).
      const listConsts = ops.map((o) => emit.constValueOf(o));
      if (
        listConsts.every((c) => c !== null) &&
        !elems.some((e) => this.containsFnDef(e))
      ) {
        return newList(listConsts as Value[], { eval: false });
      }
      return emit.recordMakeList(ops);
    }
    return newList(elems, { eval: false });
  }

  /** Build a computed map; records a makeMap event in compile mode (see buildList). */
  private buildMap(keys: string[], vals: Value[]): Value {
    const emit = this.registry.check.emit;
    if (emit !== undefined) {
      const ops: RecorderOperand[] = [];
      for (const e of vals) {
        const o = emit.classify(e);
        if (o === null) {
          emit.markUncompilable("makeMap: value of unknown provenance");
          return this.rawMap(keys, vals);
        }
        ops.push(o);
      }
      // Const-fold a fully-inert DATA/TYPE map (see buildList): bake the
      // concrete map so a consumer that reads its structure (`inspect` of a
      // def-bound record type) keeps a live value rather than a carrier. A map
      // bearing a fn-value keeps the makeMap path.
      const mapConsts = ops.map((o) => emit.constValueOf(o));
      if (
        mapConsts.every((c) => c !== null) &&
        !vals.some((v) => this.containsFnDef(v))
      ) {
        return this.rawMap(keys, mapConsts as Value[]);
      }
      return emit.recordMakeMap([...keys], ops);
    }
    return this.rawMap(keys, vals);
  }

  /** Whether `v` is, or (recursively) contains, a fn-value member. Such a
   *  container must not be const-folded: a baked fn-value dispatches
   *  dynamically when the container is later consumed (typeof/is/length). */
  private containsFnDef(v: Value): boolean {
    if (v.isFnDef()) return true;
    if (Array.isArray(v.data))
      return (v.data as Value[]).some((e) => this.containsFnDef(e));
    if (v.isMap() && v.data instanceof OrderedMap) {
      const m = v.asMap();
      return m.keys().some((k) => this.containsFnDef(m.get(k)!));
    }
    return false;
  }

  private rawMap(keys: string[], vals: Value[]): Value {
    const out = new OrderedMap();
    keys.forEach((k, i) => out.set(k, vals[i]!));
    return newMap(out);
  }

  /**
   * Evaluate one map value: nested maps recurse; an eval-list (or a
   * paren modelled as one) runs in a sub-engine and collapses to its
   * single residual (or a list); a def-bound word resolves; everything
   * else passes through.
   */
  private evalMapValue(v: Value): Value | undefined {
    if (v.data instanceof OrderedMap && v.vType.equal(TMap))
      return this.deepEvalData(v);
    // An eval-list value evaluates to a LIST of its residuals (no
    // collapse). A paren-expr or a bare word collapses to a single
    // residual (a def resolves; an fn auto-calls).
    if (
      v.vType.matches(TList) &&
      Array.isArray(v.data) &&
      v.eval &&
      !v.quoted
    ) {
      const sub = this.subRun("region", [...v.asList()]).map((e) =>
        this.deepEvalData(e),
      );
      return this.buildList(sub);
    }
    let program: Value[] | null = null;
    if (v.isParenExpr()) program = v.data as Value[];
    // A bare MARKER is a program too, and running it is what drops the
    // key: an End alone leaves no residual, so `{ a: ; }` is `{}`. Go
    // reaches this through AutoEvalMap's general sub-engine tail; core/ts
    // enumerated only paren-exprs and words, so the marker fell through
    // and was stored as DATA, rendering `{a:end}`.
    else if (v.isWord() || isEnd(v) || isOpenParen(v) || isCloseParen(v))
      program = [v];
    if (program === null) return v;
    const sub = this.subRun("region", [...program]).map((e) =>
      this.deepEvalData(e),
    );
    if (sub.length === 1) return sub[0]!;
    // ZERO residuals DROPS the key. Go's AutoEvalMap sets the key only in
    // its `len == 1` and `len > 1` arms, so `{a: ()}` yields `{}` there;
    // this port built an empty list and kept the key, giving `{a:[]}`.
    // Pinned by core/spec/data.tsv.
    if (0 === sub.length) return undefined;
    return this.buildList(sub);
  }

  private describeStack(): string {
    return this.stack
      .map((v, i) =>
        i === this.pointer ? `>>>${v.toString()}<<<` : v.toString(),
      )
      .join(" ");
  }
}

function describeExpected(fn: import("./registry.js").FunctionEntry): string {
  // Pick the first non-fallback signature for the error message.
  const sig = fn.signatures.find((s) => !s.fallback);
  if (!sig) return "";
  return sig.args.map((t) => t.toString()).join(", ");
}

// Suppress "imported but unused" in stricter setups where these are
// referenced only in match.ts. They're re-exported here for users
// that want a single import point.
export { TBoolean, TInteger, TString, TWord, Value };
