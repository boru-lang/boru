package core

// NativeFunc describes a built-in native function with its name and
// signatures. All predefined words (core and extension) use this type
// for registration.
//
// The signatures are plain `Signature` values — the SAME unified dispatch
// struct boru `def fn […]` produces. A native author fills the positional
// `Signature.Args` (+ optional `Patterns`) construction-convenience fields
// and a Go `Handler`; `RegisterNativeFunc` → `normalizeSig` derives the
// authoritative `Params` from them. There is no separate native-sig type.
//
// There is no "ForwardArgs" / "stack-only" flag at this level — that
// distinction lives entirely in each `Signature.BarrierPos`:
//
//   - `BarrierPos: BarrierAllForward` (-1)  — default all-forward;
//     resolved to `len(Args)` at registration. The common case for
//     normal forward-collecting words.
//   - `BarrierPos: 0`  — explicit all-stack dispatch. Use for words
//     that take args strictly off the prefix stack (`drop`, `dup`,
//     stack manipulators).
//   - `BarrierPos: N`  — explicit barrier at position N (`|`).
type NativeFunc struct {
	Name       string
	Signatures []Signature

	// CompileEffect is a WORD-level compile-effect declaration OR'd into every
	// one of this word's signatures at registration. Use it for per-word
	// classifications that apply to all overloads (a pure module reader, an
	// island-pure dispatch word, a code-body higher-order word) so the bytecode
	// recorder reads sig.CompileEffect instead of a name-keyed eng table. A
	// single overload that needs an extra flag can still set Signature.CompileEffect.
	CompileEffect CompileEffect
}
