package core

// The typed-def `make` record (ADR-013, 2026-08-08 amendment).
// `def b:Type {map}` means exactly `def b (make Type map)`, but the
// typed-def handler constructs the instance directly instead of
// dispatching the make WORD — so under a recording pass the instance
// would carry no provenance and a downstream `b typeof` would decline.
// This file records the make event that construction skips.
//
// Core, not check: the recorder is core's EmitRecorder interface, the
// signature lookup is core's registry, and `basic`'s typed-def handler
// is the sole caller.

// RecordTypedDefMake records the synthetic `make` event a typed-def
// object-instance construction (`def b:Type {map}`) skips. That form is
// exactly `def b (make Type map)`, but the typed-def handler builds the
// instance by calling MakeObject directly, bypassing the make WORD dispatch —
// so the instance never gets the make event that gives an explicit make its
// provenance, and a downstream `b typeof` then declines with an operand the
// lowerer cannot resolve.
//
// In active emit mode this records make over [typeArg, body] (both inert
// consts — the instantiated type body and the scalar map) and returns the
// fresh instance carrier to bind in place of the concrete result, so the
// binding carries make-equivalent provenance; the VM re-runs make's
// MakeObjHandler at run time, producing the identical instance. Outside emit
// mode it returns (Value{}, false) and the caller binds the concrete value.
func RecordTypedDefMake(r *Registry, typeArg, body Value, pos SrcPos) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	es := r.Check.Recorder()
	if !es.Active() {
		return Value{}, false
	}
	sig := objectMakeSig(r)
	if sig == nil {
		return Value{}, false
	}
	t := CanonicalType(r, ValueType(typeArg))
	// The check piece wraps this in toCarrier; here that call is provably
	// the IDENTITY and is elided rather than dragging the strip machinery
	// down with it. NewCarrier(t) leaves Data nil for every t except
	// TList/TMap (where it is a ChildTypeInfo), and toCarrier returns its
	// argument unchanged on both of those shapes — the `Parent is TList or
	// TMap` early-out, and the `v.Data == nil` type-literal early-out. t is
	// a class / resource node here, so it takes the second.
	carrier := NewCarrier(t)
	es.RecordCall("make", sig, []Value{typeArg, body}, []Value{carrier}, pos, false, false)
	return carrier, true
}

// objectMakeSig returns make's `[Ideal Map]` overload (MakeObjHandler) — the
// one a typed-def object construction would have dispatched. Looked up by
// arg shape so it tracks the registered native rather than a fabricated sig
// (the VM calls Sig.Handler directly at OpCallNative).
func objectMakeSig(r *Registry) *Signature {
	fd := r.Lookup("make")
	if fd == nil {
		return nil
	}
	for i := range fd.Signatures {
		s := &fd.Signatures[i]
		if s.TotalArgs() == 2 && SigArgType(s, 0) != nil && SigArgType(s, 1) != nil &&
			SigArgType(s, 0).Equal(TIdeal) && SigArgType(s, 1).Equal(TMap) {
			return s
		}
	}
	return nil
}
