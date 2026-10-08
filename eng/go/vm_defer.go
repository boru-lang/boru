package eng

import core "github.com/boru-lang/boru/core/go"

// The VM piece's interpreter-facing entry helpers: the designed
// defer-to-interpreter choke points (vmDefer / vmDeferAlt) and the
// fallback-island runner with the flow-escape contract. Regrouped in
// Stage 1b of the four-piece split (design/legacy/ENG-FOUR-PIECE.0.ignore) — the
// only callers are vm*.go.

// vmDefer builds a designed-defer error AND reports it to the runtime-bail
// hook — the single choke point every reachable designed-defer site in the VM
// routes through, so the executed bail census sees each defer exactly once
// with a stable site tag.
//
// Nothing resolves it by re-running any more (2026-09-19): it is an
// internal_error, it is a compiler defect, and it surfaces.
func vmDefer(r *core.Registry, curDebug []core.SrcPos, pc int, site, msg string) error {
	r.NoteBail(site, msg)
	return vmErrAt(curDebug, pc, msg)
}

// vmDeferAlt is vmDefer carrying the user-facing raise the defer site built
// over its live values (BoruError.DeferAlt). It used to be the fence-blocked
// arm's consolation prize — what the caller showed when the interpreter
// re-run this defer asked for could not be made. It is the ANSWER now: the
// caller surfaces alt instead of the internal error, which is the "trap that
// raises the interpreter's own error at the same moment" disposition
// (design/SESSION-HANDOVER.0.md), and the only legal one a defer site can
// reach without a lowering.
//
// Which is why the alt takes the defer's POSITION when it has none of its
// own: the site builds it over values rather than tokens. While a re-run
// followed the defer the interpreter's raise supplied the position; with the
// re-run gone an unpositioned alt renders "source position unknown" where the
// interpreter points at the token.
//
// This covers the alts whose defer instruction IS positioned. It does not
// reach NUR171, where neither the recorded no-match spec nor the debug table
// has a position to lend — that one is the recorder's to fix. A nil alt is
// plain vmDefer.
func vmDeferAlt(r *core.Registry, curDebug []core.SrcPos, pc int, site, msg string, alt *core.BoruError) error {
	err := vmDefer(r, curDebug, pc, site, msg)
	if alt == nil {
		return err
	}
	ae, ok := err.(*core.BoruError)
	if !ok { //covergate:allow vmErrAt always builds a *BoruError (§compiler)
		return err
	}
	if alt.Row == 0 && ae.Row > 0 {
		alt.Row, alt.Col, alt.Src = ae.Row, ae.Col, ae.Src
	}
	ae.DeferAlt = alt
	return ae
}

// runIslandResolved is RunResolved with the island flow-escape contract
// (Engine.flowUnwind): a break/continue escaping the run tears down the
// island's live frames and returns no values, leaving the registry FlowCtrl
// flag set for the VM to translate (escapedFlow).
func runIslandResolved(r *core.Registry, inputs, tokens []core.Value) ([]core.Value, error) {
	// Census seam: core.RunResolved emits "RunResolved" here; this is its
	// island twin, so it names itself rather than reporting only the
	// Engine.Run its pooled sub-engine emits.
	r.NoteInterp("vm:island-resolved")
	input := make([]core.Value, len(inputs)+len(tokens))
	copy(input, inputs)
	copy(input[len(inputs):], tokens)
	e := r.TakeSubEngine()
	e.IsTop = false
	e.StartAt = len(inputs)
	e.FlowUnwind = true
	res, err := e.Run(input)
	if len(res) > 0 {
		res = append([]core.Value(nil), res...)
	}
	e.IsTop = false
	e.StartAt = 0
	e.FlowUnwind = false
	r.PutSubEngine(e)
	return res, err
}
