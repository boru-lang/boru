package eng

import (
	"errors"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The VM's region-descriptor adapter — the SECOND seat on core's collection
// seam (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// core's kernel decides and the host dispatches; the interpreter is one host
// (*Engine over its *Tape) and this is the other, over a RegionDesc and the
// live registry. It has no client yet: `OpCollect` is the client, and this
// lands first so the hardest half is built and gated before an opcode
// executes one — the discipline the bind twins and the region table itself
// were held to.
//
// IT LIVES IN eng, AND THAT IS FORCED RATHER THAN CHOSEN. A descriptor-typed
// function in core would be a module cycle (core requires no boru sibling;
// compiler owns RegionDesc and requires core), and separately `cover-gate-core`
// holds core to 100% by its OWN suite, so an arm only the VM reaches is dead
// code in core's profile whoever calls it. eng is above both.
//
// THE TWO HALVES ARE NOT SYMMETRIC, and the asymmetry is the design.
//
//   - The CLASSIFICATIONS delegate to core's shared free functions and
//     restate nothing. "Compiled code must work exactly the same as
//     interpreted" does not survive two parallel copies of a rule as subtle
//     as "an fn word is a barrier but /v opts out", and the seam exists
//     precisely so both hosts read one implementation. eng's own seam-host
//     test records that an earlier draft reimplemented them, and that is
//     what showed the extraction was needed.
//   - The EVALUATIONS DECLINE. Every one of them is the interpreter running
//     something — a paren group, an interpolation, an XML literal, a sugar
//     lowering — and this host has a descriptor, not a program counter. A
//     walk that needs one gets errRegionCannotEval and the caller defers to
//     the interpreter. That is an UNDER-claim, which is the safe direction:
//     a collection this host cannot finish is a dispatch the VM should not
//     have routed, and deferring says so rather than guessing.
//
// The window is the SLOTS' TOKENS — what the interpreter would have read off
// the tape at this position — over core.NewTape rather than a hand-written
// CollectWindow. That is deliberate: the gap-buffer splice and live-length
// semantics the kernel relies on are then shared by construction instead of
// by agreement, and a second implementation of Splice is a second thing to
// keep correct.
//
// A slot's SOURCE is not the window's business. Source says where the VM
// gets the operand VALUE once collection has decided what is claimed; the
// walk only ever asks what a token PRESENTS to matching, which is the token
// itself resolved against the live binding set. Keeping those apart is what
// lets a word slot stay live (region_desc.go's `k` pair) while its value
// still comes from wherever the lowering put it.

// errRegionCannotEval is returned by every evaluation this host declines. It
// is a sentinel rather than a message because the caller's response is not to
// report it: a collection that needs an evaluation is one the descriptor
// cannot drive, and the dispatch defers to the interpreter, which can.
var errRegionCannotEval = errors.New("region host: collection needs an evaluation this host cannot perform")

// errRegionExhausted is the window's growth ceiling, surfaced as a decline.
//
// It is separate from the evaluation decline because it is a different fact —
// the walk could be driven, and the WINDOW ran out — but it takes the same
// response, so RegionCannotEval covers both.
var errRegionExhausted = errors.New("region host: the descriptor window hit its growth ceiling mid-collection")

// RegionCannotEval reports whether err is the decline above, so a caller can
// tell "this host cannot finish" from a real collection error the kernel
// raised. They demand opposite responses — defer versus surface — and a
// caller that could not distinguish them would report an internal decline to
// the user as their program's error.
func RegionCannotEval(err error) bool {
	return errors.Is(err, errRegionCannotEval) || errors.Is(err, errRegionExhausted)
}

// regionHost is the descriptor-backed CollectHost.
type regionHost struct {
	reg *core.Registry
	win *core.Tape

	// span backs ScratchParenSpan, mirroring the interpreter's per-engine
	// reusable buffer: the contract is that the returned slice is valid only
	// until the caller splices, so one buffer per host is enough and a fresh
	// allocation per call would be waste on the hottest path a collection has.
	span []core.Value
}

// newRegionHost seats a host on a descriptor's tokens and a live registry.
//
// The registry is the RUNNING one, passed in rather than carried on the
// descriptor, and that is a rule the descriptor's own doc states: a Program
// is shared and a run may be handed a different registry — ForkConcurrent
// gives each concurrent execution its own fork — so a registry frozen into
// the table would be both a race and the wrong answer.
func newRegionHost(reg *core.Registry, d *compiler.RegionDesc) *regionHost {
	toks := make([]core.Value, 0, len(d.Slots))
	for i := range d.Slots {
		toks = append(toks, d.Slots[i].Token)
	}
	return newRegionHostOver(reg, toks)
}

// newRegionHostOver seats a host on a window the caller has already built —
// the oracle's, where a claimed slot presents the operand the lowering
// pushed rather than its token (region_oracle.go).
func newRegionHostOver(reg *core.Registry, toks []core.Value) *regionHost {
	return &regionHost{reg: reg, win: core.NewTape(toks, core.StackHeadroom)}
}

func (h *regionHost) Window() core.CollectWindow { return h.win }

// Collected wraps a completed walk and converts a silently TRUNCATED window
// into the decline.
//
// The hazard is real and specific. core.Tape.Splice consumes its `count`
// tokens BEFORE attempting to grow, and on hitting the ceiling it returns
// early — leaving the window short and only a latch to say so; its own
// comment says "engine aborts loudly on the next step". The interpreter has
// that next step and tests the latch (engine.go's step loop, twice). This
// host has no step loop, so nothing would notice: a walk that spliced past
// the ceiling could return nil over a window missing the very tokens the
// splice was replacing, and the caller would route a dispatch against it.
//
// Every entry into the kernel from this host must pass its result through
// here. Wrapping rather than checking at each call site is deliberate: a
// check the caller can forget is a check that will be forgotten, and the
// failure it guards is silent.
func (h *regionHost) Collected(err error) error {
	if err != nil {
		return err
	}
	if h.win.Exhausted() {
		return errRegionExhausted
	}
	return nil
}

// --- evaluations: all declined, see the type doc ---

func (h *regionHost) EvalGroupAt(int) error { return errRegionCannotEval }

func (h *regionHost) EvalInterp(core.Value) (core.Value, error) {
	return core.Value{}, errRegionCannotEval
}

func (h *regionHost) EvalXml(core.Value) (core.Value, error) {
	return core.Value{}, errRegionCannotEval
}

func (h *regionHost) ExpandSugarAt(core.Value, int, int, []core.ViableSig) (bool, error) {
	return false, errRegionCannotEval
}

// FlowInterrupted is the one evaluation-side question this host CAN answer,
// and it answers it honestly rather than declining: flow control is registry
// state (break / continue / return raised inside an evaluation), and the
// registry is live. It reads false in practice for the same reason the
// evaluations decline — nothing this host runs can raise it — but reading the
// real flag costs nothing and cannot go stale if that changes.
func (h *regionHost) FlowInterrupted() bool { return h.reg.FlowCtrl != core.FlowNone }

// ScratchParenSpan wraps items in paren markers, exactly as the
// interpreter's expandParenExprScratch does — the kernel splices the span
// in place of a ParenExpr and expects to meet the OpenParen on its next
// step. The first draft returned the bare items, and the COLLECT oracle's
// first corpus walk found what that does: a word bound to a data splice is
// rewritten to ParenExpr([w]), spliced back to the bare `w`, rewritten
// again, forever (`def vs word [2,3] add vs`). The seam's contract is the
// interpreter's shape, marker for marker.
func (h *regionHost) ScratchParenSpan(items []core.Value) []core.Value {
	h.span = append(h.span[:0], core.NewOpenParen())
	h.span = append(h.span, items...)
	h.span = append(h.span, core.NewCloseParen())
	return h.span
}

// --- classifications: delegated to core's shared implementations ---

func (h *regionHost) DefTop(name string) (core.Value, bool) { return h.reg.Defs.Top(name) }

func (h *regionHost) IsFnWordBarrier(tok core.Value) bool {
	return core.FnWordBarrierOn(h.reg, tok)
}

func (h *regionHost) IsReachCallHead(tok core.Value, viable []core.ViableSig, pos, i int) bool {
	return core.ReachCallHeadBarrierOn(h.win, h.reg, tok, viable, pos, i)
}

func (h *regionHost) StaticForwardType(tok core.Value) (core.Value, core.FwdKind) {
	return core.StaticForwardTypeOf(tok)
}

func (h *regionHost) LookupWord(name string) *core.FnDefInfo { return h.reg.Lookup(name) }

func (h *regionHost) ReachFnWouldClaim(tok core.Value, i int) bool {
	return core.ReachFnWouldClaimOn(h.win, h.reg, tok, i)
}

func (h *regionHost) ExpandSugarTokens(sinfo core.SugarInfo, tok core.Value, head bool) ([]core.Value, error) {
	return nil, errRegionCannotEval
}

// The seat, asserted at compile time. This is the sibling core/go's own
// assertion has been waiting for: `_ CollectHost = (*Engine)(nil)` says the
// interpreter is a host, and this says the VM's descriptor adapter is the
// second one — a fact the compiler checks rather than a claim in a comment.
var _ core.CollectHost = (*regionHost)(nil)
