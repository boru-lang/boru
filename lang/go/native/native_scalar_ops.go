package native

// Within-type core operations for scalars and Microns.
//
// Every scalar type and every Micron kind has WELL-DEFINED behaviour
// under the binary arithmetic words add / sub / mul / div / mod / pow —
// applied WITHIN a type, never across it (a cross-type pair stays a
// type error). The definitions here complete the coverage the numeric
// tower (native_math.go) and Bytes/String concat leave open:
//
//   - String / Atom: the OCCURRENCE PACKAGE — sub removes every
//     occurrence, div counts them, mod takes the tail after the last,
//     mul is the operand-major Cartesian character product, pow repeats
//     the left operand once per character of the right. add stays
//     concatenation (String rides the existing [String Scalar] sig;
//     Atom concatenates names here). Atom mirrors String on the name,
//     producing an Atom (div a count Integer).
//   - Bytes: the same occurrence package over byte subsequences.
//   - Boolean: arithmetic is a DEFINED ERROR — Boolean carries the
//     logical words (and / or / xor / not), not arithmetic; the erroring
//     signature pins a clear message (the setMicron precedent) where
//     sig-absence would give an opaque dispatch failure.
//   - Micron: a same-kind pair with no more-specific signature falls to
//     the FIELD-WISE DEFAULT — the op runs field by field over the two
//     primary field maps (each field pair dispatching the same op by its
//     own scalar type), a field present on one side only passes through,
//     and the result map is rebuilt through the kind's single `make`
//     validator, so the result is the same kind (a map the validator
//     declines is a loud make error). Qion (same-currency money add/sub)
//     and Pathon (add = join, sub = strip trailing segments) are
//     hand-written; the other builtin kinds and every user kind use the
//     default. The Micron default is installed UNLOCKED
//     (installMicronOpDefaults), so a user's own `def add fn [[a:Kindon
//     b:Kindon] …]` — a more specific signature — replaces it (the
//     open-words merge), which is how "for user microns, the user adds
//     new signatures" and "where none is defined, use the map default"
//     coexist.

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	core "github.com/boru-lang/boru/core/go"
)

// arithOps is the set of binary arithmetic words this file gives
// within-type definitions to.
var arithOps = []string{"add", "sub", "mul", "div", "mod", "pow"}

// ---- the recursion engine (used by the Micron field-wise default) ----

// applyScalarBinaryOp computes `left OP right` in natural order for two
// same-type operands, dispatching on their (identical) family. It is the
// engine the Micron field-wise default drives per field; a cross-type
// pair is a type error ("within a type, not across"). Nested Map / List
// field values recurse element-wise (Maps and Lists are not exposed on
// the public op words — only reachable inside a Micron field).
func applyScalarBinaryOp(r *Registry, op string, left, right Value) (Value, error) {
	if !IsConcrete(left) || !IsConcrete(right) {
		return Value{}, opCrossTypeError(r, op, left, right)
	}
	lp, rp := left.Parent, right.Parent
	switch {
	case lp.ConformsTo(TNumber) && rp.ConformsTo(TNumber):
		ops, _ := towerOpsFor(op)
		// towerApply computes args[1] OP args[0]; pass (right, left) so
		// the result is the natural left OP right.
		out, err := towerApply(ops, right, left, r)
		if err != nil {
			return Value{}, err
		}
		return out[0], nil
	case lp.ConformsTo(TString) && rp.ConformsTo(TString):
		// The recursion only ever reaches here with concrete leaf field
		// values, so the projections cannot fail; the top-level handlers
		// keep the error guard for their own (sig-matched) callers.
		a, _ := left.AsConcreteString()
		b, _ := right.AsConcreteString()
		return seqOp(r, op, a, b, NewString)
	case lp.ConformsTo(TAtom) && rp.ConformsTo(TAtom):
		a, _ := AsAtom(left)
		b, _ := AsAtom(right)
		return seqOp(r, op, a, b, NewAtom)
	case lp.ConformsTo(TBoolean) && rp.ConformsTo(TBoolean):
		return Value{}, booleanArithError(r, op)
	case lp.ConformsTo(TBytes) && rp.ConformsTo(TBytes):
		a, _ := asBytes(left)
		b, _ := asBytes(right)
		return bytesSeqOp(r, op, a, b)
	case lp.ConformsTo(TMicron) && rp.ConformsTo(TMicron):
		return micronBinaryOp(r, op, left, right)
	case lp.ConformsTo(TMap) && rp.ConformsTo(TMap):
		return mapFieldwiseOp(r, op, left, right)
	case lp.ConformsTo(TList) && rp.ConformsTo(TList):
		return listElementwiseOp(r, op, left, right)
	default:
		return Value{}, opCrossTypeError(r, op, left, right)
	}
}

func opCrossTypeError(r *Registry, op string, left, right Value) error {
	return r.BoruError("type_error",
		fmt.Sprintf("%s: no operation defined between %s and %s — core operations apply within a type, not across",
			op, typeLeaf(left), typeLeaf(right)), op)
}

func typeLeaf(v Value) string {
	if v.Parent == nil {
		return "?"
	}
	return v.Parent.Leaf()
}

// ---- String / Atom occurrence package ----

// seqOp computes the occurrence-package op on two strings, natural
// a OP b, wrapping a string result with mk (NewString for String,
// NewAtom for Atom). div always yields an Integer count.
func seqOp(r *Registry, op, a, b string, mk func(string) Value) (Value, error) {
	switch op {
	case "add":
		return mk(a + b), nil
	case "sub":
		if b == "" {
			return mk(a), nil
		}
		return mk(strings.ReplaceAll(a, b, "")), nil
	case "mul":
		return seqMul(r, op, a, b, mk)
	case "div":
		if b == "" {
			return Value{}, seqEmptyError(r, op, "division")
		}
		return NewInteger(int64(strings.Count(a, b))), nil
	case "mod":
		if b == "" {
			return Value{}, seqEmptyError(r, op, "modulo")
		}
		if idx := strings.LastIndex(a, b); idx >= 0 {
			return mk(a[idx+len(b):]), nil
		}
		return mk(a), nil
	default: // pow
		return seqPow(r, op, a, b, mk)
	}
}

// seqMul builds the operand-major Cartesian character product: for each
// rune of a, for each rune of b, emit the a-rune then the b-rune
// ("ab" mul "xy" → "axaybxby").
func seqMul(r *Registry, op, a, b string, mk func(string) Value) (Value, error) {
	ra, rb := []rune(a), []rune(b)
	// The exact output size in BYTES: every byte of a is emitted once per
	// rune of b, and every byte of b once per rune of a. Counting runes as
	// one byte each would let two multi-byte-rune inputs pass the guard
	// yet build a result up to 4x the cap (PR #292 review).
	if err := seqSizeGuard(r, op, int64(len(a))*int64(len(rb))+int64(len(b))*int64(len(ra))); err != nil {
		return Value{}, err
	}
	var sb strings.Builder
	for _, ca := range ra {
		for _, cb := range rb {
			sb.WriteRune(ca)
			sb.WriteRune(cb)
		}
	}
	return mk(sb.String()), nil
}

// seqPow repeats a once per character of b (a "power" whose exponent is
// the right operand's length): "ab" pow "xyz" → "ababab".
func seqPow(r *Registry, op, a, b string, mk func(string) Value) (Value, error) {
	n := utf8.RuneCountInString(b)
	if err := seqSizeGuard(r, op, int64(len(a))*int64(n)); err != nil {
		return Value{}, err
	}
	return mk(strings.Repeat(a, n)), nil
}

func seqEmptyError(r *Registry, op, verb string) error {
	return r.BoruError("arith_error", op+": "+verb+" by an empty string", op)
}

func seqSizeGuard(r *Registry, op string, estBytes int64) error {
	if estBytes > int64(maxStringResultBytes) {
		return r.BoruError("value_error",
			fmt.Sprintf("%s: result would exceed %d bytes", op, maxStringResultBytes), op)
	}
	return nil
}

// ---- Bytes occurrence package ----

// bytesSeqOp mirrors seqOp over byte subsequences.
func bytesSeqOp(r *Registry, op string, a, b []byte) (Value, error) {
	switch op {
	case "add":
		return newBytes(concatBytes(a, b)), nil
	case "sub":
		if len(b) == 0 {
			return newBytes(cloneBytes(a)), nil
		}
		return newBytes(bytes.ReplaceAll(a, b, nil)), nil
	case "mul":
		return bytesMul(r, op, a, b)
	case "div":
		if len(b) == 0 {
			return Value{}, seqEmptyError(r, op, "division")
		}
		return NewInteger(int64(bytes.Count(a, b))), nil
	case "mod":
		if len(b) == 0 {
			return Value{}, seqEmptyError(r, op, "modulo")
		}
		if idx := bytes.LastIndex(a, b); idx >= 0 {
			return newBytes(cloneBytes(a[idx+len(b):])), nil
		}
		return newBytes(cloneBytes(a)), nil
	default: // pow
		if err := seqSizeGuard(r, op, int64(len(a))*int64(len(b))); err != nil {
			return Value{}, err
		}
		return newBytes(bytes.Repeat(a, len(b))), nil
	}
}

func bytesMul(r *Registry, op string, a, b []byte) (Value, error) {
	if err := seqSizeGuard(r, op, int64(len(a))*int64(len(b))*2); err != nil {
		return Value{}, err
	}
	out := make([]byte, 0, len(a)*len(b)*2)
	for _, ca := range a {
		for _, cb := range b {
			out = append(out, ca, cb)
		}
	}
	return newBytes(out), nil
}

func concatBytes(a, b []byte) []byte {
	out := make([]byte, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func cloneBytes(b []byte) []byte { return append([]byte(nil), b...) }

// ---- Boolean: arithmetic is a defined error ----

func booleanArithDetail(op string) string {
	return op + ": arithmetic is not defined on Boolean"
}

func booleanArithError(r *Registry, op string) error {
	return r.BoruErrorHint("type_error", booleanArithDetail(op), op,
		"Boolean carries the logical words (and / or / xor / not), not arithmetic")
}

func booleanArithHandler(op string) Handler {
	return func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return nil, booleanArithError(r, op)
	}
}

// booleanArithReturns is the check-mode mirror: Boolean arithmetic over
// two CONCRETE Boolean operands on the top-level straight line is a
// GUARANTEED runtime error, flagged with the byte-identical detail (the
// setMicron immutability-error pattern). Both gates are load-bearing
// (PR #292 review): reachability (atUncaughtTopLevel — a mirror inside
// an uncalled fn body / branch is not unconditionally reached), and
// concreteness — a Boolean-typed CARRIER's runtime value can carry a
// strict-subtype tag (`refine Boolean`) that a more-specific user
// overload claims at run time (the refinement escape), so the
// CoreDefault match over a carrier proves nothing.
func booleanArithReturns(op string) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		if atUncaughtTopLevel(r) && len(args) == 2 && IsConcrete(args[0]) && IsConcrete(args[1]) {
			core.CheckAddUniqueDiagnostic(r, "type_error", booleanArithDetail(op), op, args[0].Pos())
		}
		return []Value{}
	}
}

// ---- Micron: field-wise default + Qion / Pathon ----

// micronBinaryOp computes `left OP right` for two Microns. Requires the
// same kind (core operations apply within a kind, not across); Qion and
// Pathon are hand-written, every other kind uses the field-wise default.
func micronBinaryOp(r *Registry, op string, left, right Value) (Value, error) {
	lk, rk := left.Parent, right.Parent
	if lk == nil || rk == nil || !lk.Equal(rk) {
		return Value{}, r.BoruError("type_error",
			fmt.Sprintf("%s: %s and %s are different Micron kinds — core operations apply within a kind, not across",
				op, typeLeaf(left), typeLeaf(right)), op)
	}
	switch {
	case lk.ConformsTo(TQion):
		return qionOp(r, op, left, right)
	case lk.ConformsTo(TPathon):
		return pathonOp(r, op, left, right)
	default:
		return micronFieldwiseOp(r, op, left, right)
	}
}

// micronFieldwiseOp applies op field by field over the two primary field
// maps and rebuilds the result through the kind's single make validator.
func micronFieldwiseOp(r *Registry, op string, left, right Value) (Value, error) {
	lf, lerr := core.AsMicronFields(left)
	rf, rerr := core.AsMicronFields(right)
	if lerr != nil || rerr != nil { //covergate:allow only non-Pathon Microns reach here (Pathon is special-cased), and every one carries a MicronPayload, so the projection cannot fail
		return Value{}, opCrossTypeError(r, op, left, right)
	}
	result := NewOrderedMap()
	for _, k := range lf.Keys() {
		lv, _ := lf.Get(k)
		if rv, ok := rf.Get(k); ok {
			ov, oerr := applyScalarBinaryOp(r, op, lv, rv)
			if oerr != nil {
				return Value{}, micronFieldError(r, op, left.Parent, k, oerr)
			}
			result.Set(k, ov)
		} else {
			result.Set(k, lv)
		}
	}
	for _, k := range rf.Keys() {
		if _, ok := lf.Get(k); !ok {
			rv, _ := rf.Get(k)
			result.Set(k, rv)
		}
	}
	return remakeMicron(r, left.Parent, result)
}

// remakeMicron reconstructs a Micron of the given kind from a primary
// field map, routing through the kind's single `make` validator (so an
// invalid combination surfaces as a loud make error).
func remakeMicron(r *Registry, kind *Type, fields *OrderedMap) (Value, error) {
	out, err := core.MakeScalarHandler([]Value{NewTypeLiteral(kind), NewMap(fields)}, nil, nil, r)
	if err != nil {
		return Value{}, err
	}
	return out[0], nil
}

func micronFieldError(r *Registry, op string, kind *Type, field string, inner error) error {
	name := "Micron"
	if kind != nil {
		name = kind.Leaf()
	}
	return r.BoruError("type_error",
		fmt.Sprintf("%s: %s field %q: %s", op, name, field, boruErrorDetail(inner)), op)
}

// boruErrorDetail unwraps a BoruError's detail for nesting into a field
// message, falling back to the raw error text.
func boruErrorDetail(err error) string {
	if ae, ok := err.(*BoruError); ok {
		return ae.Detail
	}
	return err.Error()
}

// qionOp is Qion's within-kind arithmetic: add / sub combine two amounts
// of the SAME currency exactly (through the numeric tower); a different
// currency, or any of mul / div / mod / pow (money × money is
// dimensionless), is a defined error.
func qionOp(r *Registry, op string, left, right Value) (Value, error) {
	switch op {
	case "add", "sub":
		lf, _ := core.AsMicronFields(left)
		rf, _ := core.AsMicronFields(right)
		lc, _ := lf.Get("code")
		rc, _ := rf.Get("code")
		lcs, _ := lc.AsConcreteString()
		rcs, _ := rc.AsConcreteString()
		if lcs != rcs {
			return Value{}, r.BoruError("arith_error",
				fmt.Sprintf("%s: cannot combine Qion amounts in different currencies (%s and %s)", op, lcs, rcs), op)
		}
		la, _ := lf.Get("amount")
		ra, _ := rf.Get("amount")
		sum, err := applyScalarBinaryOp(r, op, la, ra)
		if err != nil { //covergate:allow both amounts are concrete BigDecimals of the same currency, so the tower add/sub cannot fault
			return Value{}, err
		}
		m := NewOrderedMap()
		m.Set("code", lc)
		m.Set("amount", sum)
		return remakeMicron(r, left.Parent, m)
	default:
		return Value{}, r.BoruErrorHint("type_error",
			op+": is not defined between two Qion values — only add and sub (same currency) are", op,
			"scale money with a plain Number (e.g. `q mul 2`), not with another Qion")
	}
}

// pathonOp is Pathon's within-kind arithmetic: add joins (the right
// operand's segments appended; it must be relative), sub strips a
// matching trailing run of segments; div / mod / mul / pow are defined
// errors.
func pathonOp(r *Registry, op string, left, right Value) (Value, error) {
	li, lerr := AsPathon(left)
	ri, rerr := AsPathon(right)
	if lerr != nil || rerr != nil { //covergate:allow micronBinaryOp routes here only for Pathon-conforming operands, so both projections succeed
		return Value{}, opCrossTypeError(r, op, left, right)
	}
	switch op {
	case "add":
		if ri.Abs {
			return Value{}, r.BoruErrorHint("type_error",
				"add: cannot join an absolute path onto another — the right operand must be relative", "add",
				"drop the leading separator from the right operand")
		}
		parts := append(append([]string{}, li.Parts...), ri.Parts...)
		return core.NewPathonVol(li.Volume, parts, li.Abs), nil
	case "sub":
		if ri.Abs {
			return Value{}, r.BoruErrorHint("type_error",
				"sub: the right operand of a Pathon subtraction must be relative", "sub",
				"drop the leading separator from the right operand")
		}
		parts := stripSegSuffix(li.Parts, ri.Parts)
		return core.NewPathonVol(li.Volume, parts, li.Abs), nil
	default:
		return Value{}, r.BoruErrorHint("type_error",
			op+": is not defined between two Pathon values — only add (join) and sub (strip trailing segments) are", op,
			"")
	}
}

// stripSegSuffix returns a with a matching trailing run of segments b
// removed; a unchanged when b is empty or is not a suffix of a.
func stripSegSuffix(a, b []string) []string {
	if len(b) == 0 || len(b) > len(a) {
		return append([]string{}, a...)
	}
	off := len(a) - len(b)
	for i := range b {
		if a[off+i] != b[i] {
			return append([]string{}, a...)
		}
	}
	return append([]string{}, a[:off]...)
}

// ---- nested Map / List recursion (Micron fields only) ----

// mapFieldwiseOp applies op key by key over two maps: a shared key
// recurses, a key on one side only passes through.
func mapFieldwiseOp(r *Registry, op string, left, right Value) (Value, error) {
	lm, lerr := RequireConcreteMap(left, op)
	rm, rerr := RequireConcreteMap(right, op)
	if lerr != nil || rerr != nil {
		return Value{}, opCrossTypeError(r, op, left, right)
	}
	result := NewOrderedMap()
	for _, k := range lm.Keys() {
		lv, _ := lm.Get(k)
		if rv, ok := rm.Get(k); ok {
			ov, oerr := applyScalarBinaryOp(r, op, lv, rv)
			if oerr != nil {
				return Value{}, oerr
			}
			result.Set(k, ov)
		} else {
			result.Set(k, lv)
		}
	}
	for _, k := range rm.Keys() {
		if _, ok := lm.Get(k); !ok {
			rv, _ := rm.Get(k)
			result.Set(k, rv)
		}
	}
	return NewMap(result), nil
}

// listElementwiseOp applies op position by position over two lists; the
// longer list's tail passes through unchanged.
func listElementwiseOp(r *Registry, op string, left, right Value) (Value, error) {
	ll, lerr := RequireConcreteList(left, op)
	rl, rerr := RequireConcreteList(right, op)
	if lerr != nil || rerr != nil {
		return Value{}, opCrossTypeError(r, op, left, right)
	}
	n := ll.Len()
	if rl.Len() < n {
		n = rl.Len()
	}
	out := make([]Value, 0, ll.Len())
	for i := 0; i < n; i++ {
		ov, oerr := applyScalarBinaryOp(r, op, ll.Get(i), rl.Get(i))
		if oerr != nil {
			return Value{}, oerr
		}
		out = append(out, ov)
	}
	for i := n; i < ll.Len(); i++ {
		out = append(out, ll.Get(i))
	}
	for i := n; i < rl.Len(); i++ {
		out = append(out, rl.Get(i))
	}
	return NewList(out), nil
}

// ---- signature handlers ----

// binaryValueOp builds a sig handler that computes the natural
// `args[1] OP args[0]` (infix-form reading) from a Value-level op fn.
func binaryValueOp(fn func(r *Registry, left, right Value) (Value, error)) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		v, err := fn(r, args[1], args[0])
		if err != nil {
			return nil, err
		}
		return []Value{v}, nil
	}
}

func stringOpHandler(op string) Handler {
	return binaryValueOp(func(r *Registry, left, right Value) (Value, error) {
		a, aerr := left.AsConcreteString()
		b, berr := right.AsConcreteString()
		if aerr != nil || berr != nil {
			return Value{}, opCrossTypeError(r, op, left, right)
		}
		return seqOp(r, op, a, b, NewString)
	})
}

func atomOpHandler(op string) Handler {
	return binaryValueOp(func(r *Registry, left, right Value) (Value, error) {
		a, aerr := AsAtom(left)
		b, berr := AsAtom(right)
		if aerr != nil || berr != nil {
			return Value{}, opCrossTypeError(r, op, left, right)
		}
		return seqOp(r, op, a, b, NewAtom)
	})
}

func bytesOpHandler(op string) Handler {
	return binaryValueOp(func(r *Registry, left, right Value) (Value, error) {
		a, aok := asBytes(left)
		b, bok := asBytes(right)
		if !aok || !bok {
			return Value{}, opCrossTypeError(r, op, left, right)
		}
		return bytesSeqOp(r, op, a, b)
	})
}

func micronOpHandler(op string) Handler {
	return binaryValueOp(func(r *Registry, left, right Value) (Value, error) {
		return micronBinaryOp(r, op, left, right)
	})
}

// seqReturnType is the result type of an occurrence-package signature:
// div yields an Integer count, every other op yields the operand type.
func seqReturnType(op string, elem *Type) *Type {
	if op == "div" {
		return TInteger
	}
	return elem
}

// mirrorOpError emits err as a check-mode guaranteed-error diagnostic
// with the byte-identical runtime code and detail (the
// CheckAddUniqueDiagnostic discipline).
func mirrorOpError(r *Registry, op string, err error, pos core.SrcPos) {
	code, detail := "type_error", err.Error()
	var ae *BoruError
	if errors.As(err, &ae) {
		code, detail = ae.Code, ae.Detail
	}
	core.CheckAddUniqueDiagnostic(r, code, detail, op, pos)
}

// seqOpReturns is the check-mode ReturnsFn for the occurrence-package
// signatures. Two CONCRETE operands make the whole call statically
// decidable — the handlers are pure, so the real one runs at analysis
// time (the CheckMicronConstruction precedent) and a guaranteed error
// (an empty-string divisor) is flagged with the byte-identical runtime
// detail. The residual is the declared result type either way.
func seqOpReturns(op string, elem *Type, h Handler) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		if atUncaughtTopLevel(r) && len(args) == 2 && IsConcrete(args[0]) && IsConcrete(args[1]) {
			if _, err := h(args, nil, nil, r); err != nil {
				mirrorOpError(r, op, err, args[0].Pos())
			}
		}
		return []Value{NewCarrier(seqReturnType(op, elem))}
	}
}

// micronOpReturns narrows a Micron op to the operand kind when both
// operands are statically the same kind (else the family root), and
// mirrors the guaranteed errors of a call whose operands are BOTH
// CONCRETE by running the real (pure) handler — full validation,
// byte-identical detail.
//
// Carrier operands are deliberately NOT mirrored, even for shapes that
// look statically decidable (a cross-kind pair, a same-kind Qion/Pathon
// mul): a CoreDefault is an unlocked overload, so a carrier's runtime
// value can arrive tagged with a strict SUBTYPE the static type admits
// (a fn declared to return the base kind reparenting to a newtype — the
// refinement escape), and a more-specific user overload over those tags
// pre-empts the default at run time. Only concrete operands' tags are
// final (PR #292 review — the same reasoning as booleanArithReturns and
// the compile-side coreDefaultCarrier poly routing). Value-dependent
// failures (currency mismatch, validator compile failures, an absolute
// right-hand path) stay with the runtime handler either way.
func micronOpReturns(op string) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		if len(args) != 2 || args[0].Parent == nil || args[1].Parent == nil {
			return []Value{NewCarrier(TMicron)}
		}
		if atUncaughtTopLevel(r) && IsConcrete(args[0]) && IsConcrete(args[1]) {
			if _, err := micronBinaryOp(r, op, args[1], args[0]); err != nil {
				mirrorOpError(r, op, err, args[0].Pos())
			}
		}
		if args[0].Parent.Equal(args[1].Parent) {
			return []Value{NewCarrier(args[0].Parent)}
		}
		return []Value{NewCarrier(TMicron)}
	}
}

// scalarOpsNatives are the within-type signatures for the scalar leaves,
// installed as CoreDefault overloads (installScalarOpDefaults). String add
// and Bytes add already concatenate (the existing [String Scalar] /
// [Bytes Bytes] sigs), so add is omitted for those; every other (op, type)
// pair is defined here. CoreDefault (not a locked native sig) so a user's
// refine-type extension — e.g. a module extending `add` for a `refine
// Boolean` — wins by specificity instead of being pre-empted by the base
// [Boolean Boolean] sig (the locked-first ordering theorem).
var scalarOpsNatives = func() []NativeFunc {
	var out []NativeFunc
	add := func(name string, args []*Type, h Handler, rf ReturnsFunc) {
		out = append(out, NativeFunc{Name: name, Signatures: []Signature{
			{Args: args, Impl: Go(h), ReturnsFn: rf, BarrierPos: -1},
		}})
	}
	for _, op := range arithOps {
		op := op
		// String: sub/mul/div/mod/pow (add is concat via [String Scalar]).
		if op != "add" {
			h := stringOpHandler(op)
			add(op, []*Type{TString, TString}, h, seqOpReturns(op, TString, h))
		}
		// Atom: all six.
		ah := atomOpHandler(op)
		add(op, []*Type{TAtom, TAtom}, ah, seqOpReturns(op, TAtom, ah))
		// Boolean: defined error, all six.
		add(op, []*Type{TBoolean, TBoolean}, booleanArithHandler(op), booleanArithReturns(op))
		// Bytes: sub/mul/div/mod/pow (add is concat via bytesNatives).
		if op != "add" {
			bh := bytesOpHandler(op)
			add(op, []*Type{TBytes, TBytes}, bh, seqOpReturns(op, TBytes, bh))
		}
	}
	return out
}()

// installScalarOpDefaults installs every within-type scalar overload
// (String / Atom / Boolean / Bytes) plus the [Micron Micron] field-wise
// default as CoreDefault overloads on the arithmetic words. They must
// dispatch AFTER any more-specific overload — so a user's own `def add fn
// [[a:Kindon b:Kindon] …]` (or a module extending `add` for a refine
// type) wins — which rules out locked native sigs (locked sigs pre-empt
// every unlocked override — the locked-first ordering theorem).
// CoreDefault gives exactly that: unlocked overloads that still live on
// the native definition (so `undef add` declines) and are skipped by the
// export transplant (so they never ride a module's word extension into an
// importer, where their builtin-only tuple would trip the module-scope
// user-type rule).
func installScalarOpDefaults(r *Registry) {
	for _, nf := range scalarOpsNatives {
		r.RegisterCoreDefault(nf.Name, nf.Signatures...)
	}
	for _, op := range arithOps {
		r.RegisterCoreDefault(op, Signature{
			Args: []*Type{TMicron, TMicron}, Impl: Go(micronOpHandler(op)), ReturnsFn: micronOpReturns(op), BarrierPos: -1,
		})
	}
}
