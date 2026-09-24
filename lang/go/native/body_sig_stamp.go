package native

import (
	"strings"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// body_sig_stamp.go — a handler's throwaway signature over a RAW body,
// stamped at run time so the callback seam hosts it on the VM.
//
// A handler that binds a body operand in a frame of its own — check-prop's
// generator under the named `r`, its property over one unnamed param — builds
// a throwaway CallBoru signature around the tokens and dispatches it. The
// recorder compiles a LITERAL operand of such a handler to a stored-param-body
// carrier with the same params (Signature.StoredBodies), which InvokeCallback
// hosts on the VM; a body that reaches the handler RAW — read from a map
// (`Test.prop`'s spec, `p get "gen"`), returned by a fn, or one the recorder's
// carrier compile declined — kept the throwaway frame, an interpreter entry
// per invoke (the interp-entry census's module-test.tsv and
// corpus-modules.tsv check-prop rows). StampBodySig is the run-time twin of
// the carrier: the body is compiled once per param shape as a detached unit
// (compiler.StampDetachedSig over the handler's own params) and the returned
// signature carries its CompiledFnRef, so InvokeCallback runs the unit nested
// on the VM with CallBoru — the very frame the unit models — as its
// per-invoke fallback.
//
// The unit's params are TYPED by the run-time inputs, not by the handler's
// declaration: a body that leaves an `Any` input in its residual — check-prop's
// property `['c','d']` over its generated value, `[0 gte]` over an Integer that
// stays beneath — declines as "unapplied fn-value in body residual" when the
// checker cannot rule the input out as a fn value, the token-body host's
// lesson (eng/go vm_token_body.go); an input that does not conform to the
// declared param keeps the throwaway signature, so CallBoru raises what it
// raised. The unit is memoised on the registry by the body's name
// (core.TokenBodyKey) and the shape — each param's name and concrete type —
// (Registry.TokenBodyStamp), a declined shape remembered so it pays the
// compile once; freshness is the ref's own (DepSnap, the JIT re-stamp).
//
// What stays the interpreter's, byte-identical to before: an interpreter run
// (stamping disarmed), a body with no name, a Go-backed or empty signature,
// an input count that is not the params', and a body the compile declines.

// bodySigDeclined marks a (body, shape) whose stamp declined.
type bodySigDeclined struct{}

// StampBodySig returns the signature to dispatch body with over inputs: sig
// itself when nothing can be stamped, else a copy of sig — its params typed
// by the inputs — carrying the run-time-stamped unit's CompiledFnRef.
func StampBodySig(r *Registry, sig *FnSig, body Value, inputs []Value) *FnSig {
	if r == nil || sig == nil || !r.RuntimeStampingEnabled() || len(inputs) != len(sig.Params) {
		return sig
	}
	impl, isBoru := sig.Impl.(*core.BoruImpl)
	if !isBoru || len(impl.Body) == 0 {
		return sig
	}
	base, named := core.TokenBodyKey(body, impl.Body)
	if !named {
		return sig
	}
	params := make([]FnParam, len(sig.Params))
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("/sig")
	for i, p := range sig.Params {
		t := core.TokenBodyInputType(inputs[i])
		if p.Type != nil && !t.ConformsTo(p.Type) {
			return sig
		}
		params[i] = FnParam{Name: p.Name, Type: t}
		b.WriteByte(',')
		b.WriteString(p.Name)
		b.WriteByte(':')
		b.WriteString(t.Name())
	}
	key := b.String()
	if slot, seen := r.TokenBodyStamp(key); seen {
		if stamped, isSig := slot.(*FnSig); isSig {
			return stamped
		}
		return sig // declined before: the throwaway frame, remembered
	}
	stamped := *sig
	stamped.Params = params
	stamped.Impl = impl.Clone()
	fd := core.FnDefInfo{Name: "codebody", Anonymous: true, Signatures: []core.Signature{stamped}}
	ref, ok := compiler.StampDetachedSig(r, fd, 0, body.Pos())
	if !ok {
		r.SetTokenBodyStamp(key, bodySigDeclined{})
		return sig
	}
	stamped.Impl.(*core.BoruImpl).SetCompiled(ref)
	r.SetTokenBodyStamp(key, &stamped)
	return &stamped
}
