// Static type-checker (check mode), ported from eng/go/check.go +
// carrier.go. In check mode the engine runs the same dispatch/matching
// machinery, but concrete payloads are stripped to type-only *carriers*,
// signature handlers are short-circuited to produce carrier return
// values (declared via Signature.returns / returnsFn), and
// signature/return violations surface as diagnostics rather than hard
// errors.
//
// PARITY NOTE: this is the Phase-1 core — carriers, StripToCarriers,
// carrierResults (returnsFn → returns → missing_returns), gradual
// contagion, and the diagnostic store. Fn-body analysis, the step
// budget's fixed-point machinery and disjunct partitioning land in
// later phases.
import { type BoruType, TAny } from '@boru-lang/core'
import type { Signature } from '@boru-lang/core'
import { isSugar, newCarrier, newDynamicCarrier, Value } from '@boru-lang/core'
import type { Registry } from '@boru-lang/core'

// The analysis STATE lives in core (core/ts/src/check-state.ts, the twin of
// core/go/check_state.go); this piece owns the analysis LOGIC. Re-exported
// here so every existing consumer of `check.ts` keeps working unchanged.
import { installAnalysisImpl } from '@boru-lang/core'
export {
  CheckState,
  DEFAULT_CHECK_STEP_BUDGET,
  severityFor,
} from '@boru-lang/core'
export type { CheckDiagnostic, CheckSeverity } from '@boru-lang/core'


/**
 * toCarrier strips a concrete value to a type-only carrier. Structural
 * and control values pass through unchanged (lists/maps keep their
 * payload so positional matching still works; words/markers stay as
 * dispatch tokens; type literals stay literals). Everything else
 * collapses to a bare carrier of its type. Mirrors carrier.go::toCarrier
 * (Phase-1 subset).
 */
export function toCarrier(v: Value): Value {
  // Already a carrier — leave its modality intact.
  if (v.carrier) return v
  // Control / structural tokens are not values to strip. A sugar
  // marker MUST keep its SugarInfo payload — a stripped marker can
  // never lower, so the sugar would be opaque in check mode (the Go
  // carrier-strip has the same exemption, check/go/carrier.go).
  if (v.isWord() || v.isForward() || v.isMark() || v.isMove() || v.isParenExpr()) return v
  if (v.isInterpString() || v.isXmlInterp() || isSugar(v)) return v
  // Lists and maps keep their concrete payload — matching needs it.
  if (Array.isArray(v.data)) return v
  if (v.isMap() || v.isTypedList() || v.isTypedMap()) return v
  // Bare type literals (Data === null, not a carrier) stay literals so
  // type-argument slots and typeof still see them as types.
  if (v.data === null) return v
  // Fn definitions keep their body/params for closure analysis.
  if (v.isFnDef()) return v
  // Everything concrete else → a bare carrier of its type.
  return newCarrier(v.vType)
}

/** StripToCarriers maps every input value through toCarrier. */
export function stripToCarriers(input: Value[]): Value[] {
  return input.map(toCarrier)
}

/**
 * carrierResults produces the carrier return values for a matched
 * signature in check mode. Resolution order (mirrors
 * carrier.go::carrierResults, Phase-1 subset):
 *   1. sig.returnsFn(args, r) — custom check-mode logic; results coerced
 *      to carriers.
 *   2. sig.returns — one fresh carrier per declared type.
 *   3. neither → emit missing_returns and return a dynamic(Any) carrier.
 *
 * Gradual contagion: when any arg is a dynamic carrier, the produced
 * carriers are dynamic too (a statically-unknown input cannot yield a
 * statically-known output).
 */
export function carrierResults(
  registry: Registry,
  word: string,
  sig: Signature,
  args: Value[],
): Value[] {
  const contagious = args.some((a) => a.carrier && a.dynamic)
  // A declared return type of Any is statically unknown, so its carrier
  // is dynamic; a known type stays strict unless a dynamic operand makes
  // the whole result contagious. Mirrors carrierResults' Any handling.
  const mk = (t: BoruType): Value =>
    contagious || t.equal(TAny) ? newDynamicCarrier(t) : newCarrier(t)

  if (sig.returnsFn) {
    return sig.returnsFn(args, registry).map((v) => {
      const c = toCarrier(v)
      if (c.carrier && !c.dynamic && (contagious || c.vType.equal(TAny))) {
        return newDynamicCarrier(c.vType)
      }
      return c
    })
  }
  if (sig.returns) {
    return sig.returns.map(mk)
  }
  registry.check.addDiagnostic({
    code: 'missing_returns',
    detail: `${word} has no return-type annotation; result type is unknown`,
    word,
  })
  return [newDynamicCarrier(TAny)]
}

// Install this piece's implementations over core's NAMED inactive defaults —
// the TS twin of check/go's init replacing core/go's AnalysisImpl. Importing
// check.ts is what arms analysis; a core-only build runs the inactive
// defaults instead (pinned by core/ts/src/seams.test.ts).
installAnalysisImpl({ stripToCarriers, carrierResults })
