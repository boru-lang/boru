package check

import (
	"errors"

	core "github.com/boru-lang/boru/core/go"
)

// Check-mode dry pass for PURE words over concrete arguments.
//
// A word whose handler is a pure function of its arguments — an assertion
// comparison, a codec decode of a literal, a bounds-checked list edit, a
// parse of a literal source — fails at runtime iff it fails on the same
// concrete arguments at check time. DryPassReturns runs the real handler
// once during analysis when EVERY argument is concrete and the position is
// the top-level straight line, and mirrors a handler error into a check
// diagnostic with the byte-identical code + detail. The modelled residual
// is unchanged either way (the declared result carriers), so the dry pass
// gates without altering typing or the compile pass.
//
// Precedents: miniMicronLitReturns' lenient dry pass (boru:minilang),
// parseSpecReturns (boru:parse), CheckMicronConstruction (make).

// CheckAtUncaughtTopLevel reports whether the current analysis position is
// the check pass's top-level straight line: outside every fn body and
// nested branch/loop/quotation body. A guaranteed runtime fault detected
// here is unconditionally reached, so it may be flagged as a program
// error; anywhere else reachability is conditional. Inside `do` bodies
// AddDiagnostic re-attributes error findings to caught info centrally,
// and the compile pass no longer needs excluding — mirror diagnostics
// (RuntimeMirror) do not trip the compile pipeline's compile failure.
func CheckAtUncaughtTopLevel(r *core.Registry) bool {
	return r != nil && r.Check.IsActive() &&
		r.Check.FnBodyDepth == 0 && r.Check.NestedBodyDepth == 0
}

// DryPassReturns builds a ReturnsFunc that models the declared result
// carriers and, on the top-level straight line with all-concrete args,
// dry-runs the (pure) handler to surface its guaranteed runtime error as a
// check diagnostic. h MUST be pure: no I/O, no registry mutation, no body
// execution — the dry pass runs it during analysis. An uncoded handler
// error is reported as type_error (the CheckMicronConstruction fallback).
func DryPassReturns(h func(args []core.Value, named map[string]core.Value, body []core.Value, r *core.Registry) ([]core.Value, error), results ...*core.Type) core.ReturnsFunc {
	return DryPassWrap(h, ReturnsStatic(results...))
}

// DryPassWrap is DryPassReturns over an EXISTING result model: the base
// ReturnsFunc keeps full ownership of the residual (a precise per-arg
// narrowing like ReturnsPreserveListAt), and the dry pass only contributes
// the guaranteed-error diagnostic.
func DryPassWrap(h func(args []core.Value, named map[string]core.Value, body []core.Value, r *core.Registry) ([]core.Value, error), base core.ReturnsFunc) core.ReturnsFunc {
	return func(args []core.Value, r *core.Registry) []core.Value {
		if CheckAtUncaughtTopLevel(r) && allConcreteArgs(args) {
			var pos core.SrcPos
			if len(args) > 0 {
				pos = args[0].Pos()
			}
			if _, err := h(dryPassOperands(args), nil, nil, r); err != nil {
				code, detail := "type_error", err.Error()
				var ae *core.BoruError
				if errors.As(err, &ae) {
					code, detail = ae.Code, ae.Detail
				}
				core.CheckAddUniqueDiagnostic(r, code, detail, "", pos)
			}
		}
		return base(args, r)
	}
}

// allConcreteArgs gates the dry pass on arguments whose runtime value the
// analysis provably holds: concrete payloads, plus a strict (non-dynamic)
// none — None is a singleton, so the checked literal IS the runtime value
// (the orderingDeterminate rule) — plus a bare type node, for the same
// reason: a type literal is inert and self-evaluating, so the checked
// node IS the runtime operand (the TypeArgs slot of `convert` and its
// kin; without this the dry pass never fires for any word whose
// signature carries a type-literal slot).
func allConcreteArgs(args []core.Value) bool {
	for _, a := range args {
		if core.IsConcrete(a) || core.IsBareTypeNode(a) {
			continue
		}
		if core.IsNoneShape(a) && !a.Dynamic {
			continue
		}
		return false
	}
	return true
}

// DeepConcreteOptions is the gate for a mirror whose validator reads an
// OPTIONS MAP at position 0.
//
// Two clauses, each load-bearing:
//
//   - args[0] must be concrete ALL THE WAY DOWN (core.DeepConcrete), not
//     merely concrete. A map literal written at a call site is concrete
//     while a field inside it holds a carrier — `{tcp: p}` for a computed
//     p — and a validator that projects fields with AsConcreteInteger /
//     AsConcreteString reads that carrier as a MALFORMED option and
//     reports a guaranteed error for a perfectly good program. That is a
//     false positive, the one outcome the accuracy ratchet rejects
//     outright, and it is not hypothetical: the corpus is full of
//     `{tcp: (join …)}` rows, and the same shallowness produced a real
//     soundness violation in read's option routing.
//   - no LATER operand may be a dynamic carrier. A dynamic operand was
//     matched optimistically, so the run may land on a different overload
//     — or on no signature at all — and never reach the error being
//     mirrored. A strict carrier at a later position is fine precisely
//     because the validator does not read it (the arg0-only contract each
//     caller must honour).
//
// allConcreteArgs is deliberately left shallow: the dry passes already in
// the tree are gated that way and their handlers cope, so tightening it
// would silence findings that are correct today.
func DeepConcreteOptions(args []core.Value) bool {
	return DeepConcreteOptionsAt(0)(args)
}

// DeepConcreteOptionsAt is DeepConcreteOptions for a word whose options
// map is NOT at position 0 — `open [Pathon, Map]`, `write [Pathon,
// String, Map]`. It deep-checks the argument the validator actually
// READS, so a computed path with a literal options map still gates in
// (PR #348 review: gating position 0 declined those and quietly cost
// precision).
//
// The dynamic rule widens to EVERY position, including the target.
// A dynamic operand was matched optimistically, so the runtime value
// may select a DIFFERENT OVERLOAD than the one being modelled — a
// gradual `IO.write` target that turns out to be a stream at run time
// takes the stream branch, which never reaches the encoder the mirror
// is validating. Modelling one overload's failure while the run takes
// another is a false positive, so any dynamic operand closes the gate.
func DeepConcreteOptionsAt(idx int) func([]core.Value) bool {
	return func(args []core.Value) bool {
		if idx >= len(args) || !core.DeepConcrete(args[idx]) {
			return false
		}
		for _, a := range args {
			if a.Dynamic {
				return false
			}
		}
		return true
	}
}

// MirrorReturns is the concrete-gated guaranteed-error mirror for an
// EFFECTFUL word. On the top-level straight line, when gate accepts the
// operands, it runs validate — a PURE PREFIX of the word's own handler,
// sharing the handler's source lines so the two cannot drift — and
// reports its error as a check diagnostic with the byte-identical code
// and detail. The residual is base's either way, so the mirror gates
// without altering typing or the compile pass.
//
// It differs from DryPassWrap in exactly the two places an effectful
// word requires. The caller supplies the GATE, because allConcreteArgs is
// shallow (see DeepConcreteOptions). And the caller supplies a VALIDATOR
// rather than the whole handler, because the handler must never run
// during analysis: no socket may be opened, no SDK built, no policy
// consulted, no file touched at check time. A word whose handler IS pure
// wants DryPassWrap instead — this is the seam for the ones that are not.
func MirrorReturns(word string, gate func([]core.Value) bool,
	validate func([]core.Value, *core.Registry) error, base core.ReturnsFunc) core.ReturnsFunc {
	return func(args []core.Value, r *core.Registry) []core.Value {
		if CheckAtUncaughtTopLevel(r) && gate(args) {
			if err := validate(args, r); err != nil {
				code, detail := "type_error", err.Error()
				var ae *core.BoruError
				if errors.As(err, &ae) {
					code, detail = ae.Code, ae.Detail
				}
				core.CheckAddUniqueDiagnostic(r, code, detail, word, args[0].Pos())
			}
		}
		return base(args, r)
	}
}

// dryPassOperands canonicalises the gated args for the handler run: a
// strict none-SHAPE (a None carrier or literal) becomes the canonical
// `none` sentinel, so the handler's payload probes (IsNone, truthiness)
// see exactly the value the runtime would hand it.
func dryPassOperands(args []core.Value) []core.Value {
	var out []core.Value
	for i, a := range args {
		if !core.IsConcrete(a) && core.IsNoneShape(a) {
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			out[i] = core.WithPos(core.NewNone(), a)
		}
	}
	if out == nil {
		return args
	}
	return out
}
