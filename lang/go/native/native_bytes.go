package native

import (
	"fmt"
	"math"
	"unicode/utf8"

	core "github.com/boru-lang/boru/core/go"
)

// A binary frame layout is modelled as a sealed CLASS that carries a wire
// layout (ClassTypeInfo.BinaryLayout): `def Header (refine BinarySpec
// [layout])` builds it, `make Header {fields}` produces a field-accessible
// INSTANCE (reusing the object-instance machinery wholesale), and the instance
// serialises to `Bytes` via `convert Bytes`. Two membership types name the two
// roles, mirroring `BinarySpec : Binary :: Class : Object`:
//
//   - TBinarySpec — matches a spec TYPE: a class value carrying a BinaryLayout.
//     It is the `refine` base (`refine BinarySpec [layout]`) and the answer to
//     `Header is BinarySpec`.
//   - TBinary — matches an INSTANCE: an object instance whose type carries a
//     BinaryLayout. The answer to `p is Binary`.
//
// Because instances reuse the class machinery they are ALSO `is Class` (they
// are sealed records under the hood) — a deliberate simplification that keeps
// field access / make / convert Map / typeof entirely free of new wiring.
// Binary and BinarySpec are defined per-registry as MEMBERSHIP types in
// installIdeals (via DefineMemberType) — the parity-proven path that answers
// `is`/dispatch through a Go predicate. They are not global builtins (no
// FixedID): a binary frame is realised on the class machinery, so the types
// are membership predicates over class instances / class spec-types.
//
// binaryMembers builds the two predicates (instance / spec) for installIdeals.

// binarySpecLayout returns the wire layout carried by a binary-frame SPEC type
// (a class value with BinaryLayout set), and whether v is such a type.
func binarySpecLayout(v Value) (Value, bool) {
	// Both callers deliver CONTENT, never a node: frameInstance resolves
	// a named spec's node before probing, and the BinarySpec member
	// predicate only ever sees concrete candidates (matchMembership
	// defers bare nodes to the lattice walk) — so no node-resolution
	// prelude is needed here.
	if !IsClassType(v) {
		return Value{}, false
	}
	ot, err := AsClassType(v)
	if err != nil || !IsConcrete(ot.BinaryLayout) {
		return Value{}, false
	}
	return ot.BinaryLayout, true
}

// binaryInstanceLayout returns the wire layout of a binary INSTANCE (an object
// instance whose type carries a BinaryLayout), and whether v is such a value.
func binaryInstanceLayout(v Value) (Value, bool) {
	if !IsClassInstance(v) {
		return Value{}, false
	}
	oi, err := AsClassInstance(v)
	if err != nil || oi.TypeRef == nil || !IsConcrete(oi.TypeRef.BinaryLayout) {
		return Value{}, false
	}
	return oi.TypeRef.BinaryLayout, true
}

// bytesNatives registers the Bytes operations as SIGNATURE OVERLOADS of
// existing words (no new core words except unpack-prefix). Conversions go
// through `convert`, sub-ranges through `slice`, concatenation through
// `add`; the bit-syntax builds with `make Bytes`, decodes with `unpack`,
// and streams with `unpack-prefix`. RegisterNativeFunc APPENDS these sigs
// to the existing words (eng/go/registry.go upsertFnDef); a more-specific
// sig wins dispatch, so the Bytes overloads take precedence over the
// generic `[Scalar Scalar]` / `[Scalar Any]` forms.
var bytesNatives = []NativeFunc{
	{
		Name: "convert",
		Signatures: []Signature{
			// String <-> Bytes (UTF-8), List <-> Bytes (0-255 ints), and
			// Bytes -> Bytes (compact copy). Target type is the literal arg0.
			// There is no Bytes literal, so `convert Bytes "m"` is how a
			// program writes a Bytes constant: over a const string the compile
			// pass folds it (CompileScalarFold), so a refinement bounded by one
			// (`def Hi (Bytes gte (convert Bytes "m"))`) has a KNOWN bound
			// (NUR009, NUR231).
			{Args: []*Type{TBytes, TString}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertStringToBytes), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1, CompileEffect: CompileScalarFold},
			{Args: []*Type{TString, TBytes}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertBytesToString), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TBytes, TList}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertListToBytes), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TList, TBytes}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertBytesToList), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TBytes, TBytes}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertBytesToBytes), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
		},
	},
	{
		Name: "slice",
		Signatures: []Signature{
			// `slice start end b` (end-exclusive, zero-copy view); data last.
			{Args: []*Type{TInteger, TInteger, TBytes}, Impl: Go(bytesSliceStartEnd), Returns: []*Type{TBytes}, BarrierPos: -1},
			{Args: []*Type{TInteger, TBytes}, Impl: Go(bytesSliceStart), Returns: []*Type{TBytes}, BarrierPos: -1},
			{Args: []*Type{TBytes}, Impl: Go(bytesSliceAll), Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "add",
		Signatures: []Signature{
			// `a add b` concatenates two byte strings (mirrors String add).
			{Args: []*Type{TBytes, TBytes}, Impl: Go(addBytesHandler), Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		// `make Header {fields}` reuses the object/class make path (Header is a
		// sealed class), so no Bytes-specific `make` overload is needed here —
		// see installIdeals' BinarySpec constructor. The result is a Binary
		// INSTANCE, field-accessible like any object instance.
		Name: "convert",
		Signatures: []Signature{
			// `convert Bytes <Binary>` serialises a Binary instance to wire
			// bytes via its spec's layout (the Binary→Bytes direction). Binary
			// instances are sealed class instances, so dispatch is on TClass;
			// the handler verifies it is a binary instance (declines otherwise).
			{Args: []*Type{TBytes, TClass}, TypeArgs: map[int]bool{0: true}, Impl: Go(convertBinaryToBytes), ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
		},
	},
	{
		Name: "unpack",
		Signatures: []Signature{
			// `unpack <BinarySpec> b` decodes Bytes `b` into a Binary instance.
			// A spec is a sealed class, so dispatch is on TClass; the handler
			// verifies the class carries a layout (errors otherwise).
			{Args: []*Type{TClass, TBytes}, TypeArgs: map[int]bool{0: true}, Impl: Go(unpackFrameHandler), Returns: []*Type{TClass}, BarrierPos: -1},
		},
	},
	{
		Name: "unpack-prefix",
		Signatures: []Signature{
			// `unpack-prefix <BinarySpec> b` -> {ok: <Binary> rest: <Bytes>} | {need n}.
			{Args: []*Type{TClass, TBytes}, TypeArgs: map[int]bool{0: true}, Impl: Go(unpackPrefixFrameHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
}

// ---- convert overloads -----------------------------------------------------

func convertStringToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("bytes_error", "convert Bytes: expected a String", "convert")
	}
	return []Value{newBytes([]byte(s))}, nil
}

func convertBytesToString(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.BoruError("bytes_error", "convert String: expected Bytes", "convert")
	}
	if !utf8.Valid(b) {
		return nil, r.BoruError("bad-encoding", "convert String: bytes are not valid UTF-8", "convert")
	}
	return []Value{NewString(string(b))}, nil
}

func convertListToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	lst, err := RequireConcreteList(args[1], "convert")
	if err != nil {
		return nil, err
	}
	out := make([]byte, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		n, ierr := lst.Get(i).AsConcreteInteger()
		if ierr != nil {
			return nil, r.BoruError("expected-byte", fmt.Sprintf("convert Bytes: element %d is not an Integer", i), "convert")
		}
		if n < 0 || n > 255 {
			return nil, r.BoruError("expected-byte", fmt.Sprintf("convert Bytes: element %d = %d is out of range 0-255", i, n), "convert")
		}
		out[i] = byte(n)
	}
	return []Value{newBytes(out)}, nil
}

func convertBytesToList(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.BoruError("bytes_error", "convert List: expected Bytes", "convert")
	}
	out := make([]Value, len(b))
	for i, c := range b {
		out[i] = NewInteger(int64(c))
	}
	return []Value{NewList(out)}, nil
}

// convertBytesToBytes is `convert Bytes <bytes>` — a compacting copy that
// drops any large backing array a zero-copy slice view was pinning.
func convertBytesToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.BoruError("bytes_error", "convert Bytes: expected Bytes", "convert")
	}
	return []Value{newBytes(append([]byte(nil), b...))}, nil
}

// ---- slice overloads -------------------------------------------------------

// clampRange normalises a [start,end) window to byte bounds (slice never
// errors on out-of-range; it clamps, matching the String/List slice).
func clampRange(start, end, n int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > n {
		start = n
	}
	if end < start {
		end = start
	}
	return start, end
}

func bytesSliceStartEnd(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[2])
	s64, _ := args[0].AsConcreteInteger()
	e64, _ := args[1].AsConcreteInteger()
	s, e := clampRange(int(s64), int(e64), len(b))
	return []Value{newBytes(b[s:e:e])}, nil
}

func bytesSliceStart(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[1])
	s64, _ := args[0].AsConcreteInteger()
	s, e := clampRange(int(s64), len(b), len(b))
	return []Value{newBytes(b[s:e:e])}, nil
}

func bytesSliceAll(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[0])
	return []Value{newBytes(b[0:len(b):len(b)])}, nil
}

// ---- add overload ----------------------------------------------------------

// addBytesHandler concatenates two byte strings. Like addConcatHandler it
// joins args[1] ++ args[0], so the infix form `a add b` yields a ++ b.
func addBytesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b0, _ := asBytes(args[0])
	b1, _ := asBytes(args[1])
	out := make([]byte, 0, len(b1)+len(b0))
	out = append(out, b1...)
	out = append(out, b0...)
	return []Value{newBytes(out)}, nil
}

// ---- bit-syntax: segment spec ---------------------------------------------

// bitSeg is one parsed segment of a pack/unpack spec.
type bitSeg struct {
	name   string // bind/scope name; "" when the segment is a literal
	isLit  bool   // key was a numeric literal (pack: write it; unpack: guard)
	litVal int64
	typ    string // u8..u64,i8..i64,f32,f64,bits,bytes,utf8,pad
	width  int    // byte width for fixed ints/floats (1/2/4/8)
	signed bool
	little bool // endianness (default big = network order)
	// size for bytes/utf8/bits/pad
	hasSize  bool
	sizeLit  int
	sizeName string // size given as a previously-bound name
}

var intWidths = map[string]struct {
	width  int
	signed bool
}{
	"u8": {1, false}, "u16": {2, false}, "u32": {4, false}, "u64": {8, false},
	"i8": {1, true}, "i16": {2, true}, "i32": {4, true}, "i64": {8, true},
}

// binaryFieldType maps a segment's wire type to the boru value type a decoded
// field carries — used to build the spec class's field schema so a Binary
// instance validates and type-narrows like any object instance.
func binaryFieldType(seg bitSeg) *Type {
	switch seg.typ {
	case "f32", "f64":
		return TFloat
	case "bytes":
		return TBytes
	case "utf8":
		return TString
	default: // u8..u64, i8..i64, bits
		return TInteger
	}
}

// binaryFieldSchema builds the spec class's field schema (name → type literal)
// from the NAMED segments of a layout (literal-guard and pad segments carry no
// field).
func binaryFieldSchema(segs []bitSeg) *OrderedMap {
	fields := NewOrderedMap()
	for _, seg := range segs {
		if seg.isLit || seg.typ == "pad" {
			continue
		}
		fields.Set(seg.name, NewTypeLiteral(binaryFieldType(seg)))
	}
	return fields
}

// readBitSegments reads the spec, a `List` of segment `Map`s. Each Map is a
// fully-structured, JSON-representable descriptor that this code only READS —
// there is NO token-level / sub-language parsing (ADR-007: no secondary
// parsing; every boru structure is plain Node data a macro could construct).
// Segment Map keys:
//   - name (String) | value (Integer) — exactly one. `name` binds (unpack) or
//     reads from scope (pack); `value` is a pack constant / unpack match-guard.
//   - type (String, required) — u8..u64, i8..i64, f32, f64, bits, bytes,
//     utf8, pad.
//   - endian (String, optional) — "be" (default, network order) or "le".
//   - signed (Boolean, optional) — overrides the u/i prefix default.
//   - size (Integer | String, optional) — a literal bit/byte count, or a
//     String naming a previously-bound field; required for bits/pad, optional
//     for bytes/utf8 (omitted = "the rest").
func readBitSegments(spec Value, r *Registry, word string) ([]bitSeg, error) {
	lst, err := RequireConcreteList(spec, word)
	if err != nil {
		return nil, err
	}
	var segs []bitSeg
	for i := 0; i < lst.Len(); i++ {
		seg, perr := readOneSeg(lst.Get(i), i, r, word)
		if perr != nil {
			return nil, perr
		}
		segs = append(segs, seg)
	}
	for i := range segs {
		if (segs[i].typ == "bits" || segs[i].typ == "pad") && !segs[i].hasSize {
			return nil, r.BoruError("bytes_error", fmt.Sprintf("%s: %s segment needs a `size` (bit count)", word, segs[i].typ), word)
		}
	}
	return segs, nil
}

func readOneSeg(el Value, i int, r *Registry, word string) (bitSeg, error) {
	var seg bitSeg
	segErr := func(detail string) error {
		return r.BoruError("bytes_error", fmt.Sprintf("%s: segment %d %s", word, i, detail), word)
	}
	m, merr := RequireConcreteMap(el, word)
	if merr != nil {
		return seg, segErr("is not a Map")
	}

	// Bind vs literal: exactly one of `name` / `value`.
	nameV, hasName := m.Get("name")
	valV, hasValue := m.Get("value")
	switch {
	case hasName && hasValue:
		return seg, segErr("has both `name` and `value`")
	case hasName:
		s, e := nameV.AsConcreteString()
		if e != nil {
			return seg, segErr("`name` must be a String")
		}
		seg.name = s
	case hasValue:
		n, e := valV.AsConcreteInteger()
		if e != nil {
			return seg, segErr("`value` must be an Integer")
		}
		seg.isLit = true
		seg.litVal = n
	default:
		return seg, segErr("needs a `name` or `value`")
	}

	// type (required).
	typeV, hasType := m.Get("type")
	if !hasType {
		return seg, segErr("has no `type`")
	}
	seg.typ, merr = typeV.AsConcreteString()
	if merr != nil {
		return seg, segErr("`type` must be a String")
	}
	if iw, ok := intWidths[seg.typ]; ok {
		seg.width = iw.width
		seg.signed = iw.signed
	} else {
		switch seg.typ {
		case "f32":
			seg.width = 4
		case "f64":
			seg.width = 8
		case "bits", "bytes", "utf8", "pad":
			// size comes from the `size` key
		default:
			return seg, segErr(fmt.Sprintf("has unknown type %q", seg.typ))
		}
	}

	// A `value` literal (a pack constant / unpack guard) is only meaningful
	// for integer-family and bits segments: packing writes an Integer and an
	// unpack guard compares one. Reject it up front on float / bytes / utf8 /
	// pad — a float constant would silently pack 0.0 (the Integer→float
	// conversion fails and is ignored), and a bytes/utf8/pad literal has no
	// decode-side guard — rather than emit a self-inconsistent frame.
	if seg.isLit {
		if _, isInt := intWidths[seg.typ]; !isInt && seg.typ != "bits" {
			return seg, segErr(fmt.Sprintf("a `value` literal is not valid on a %s segment", seg.typ))
		}
	}

	// endian (optional; default big = network order).
	if endV, ok := m.Get("endian"); ok {
		es, e := endV.AsConcreteString()
		if e != nil {
			return seg, segErr("`endian` must be a String")
		}
		switch es {
		case "be":
			seg.little = false
		case "le":
			seg.little = true
		default:
			return seg, segErr(fmt.Sprintf("has unknown endian %q (want \"be\" or \"le\")", es))
		}
	}

	// signed (optional; overrides the u/i prefix, ints only).
	if sgV, ok := m.Get("signed"); ok {
		b, e := sgV.AsConcreteBoolean()
		if e != nil {
			return seg, segErr("`signed` must be a Boolean")
		}
		if _, isInt := intWidths[seg.typ]; !isInt {
			return seg, segErr(fmt.Sprintf("`signed` is not valid on %s", seg.typ))
		}
		seg.signed = b
	}

	// size (optional; Integer literal or String field-name reference).
	if szV, ok := m.Get("size"); ok {
		seg.hasSize = true
		if n, e := szV.AsConcreteInteger(); e == nil {
			if n < 0 {
				return seg, segErr("`size` must not be negative")
			}
			seg.sizeLit = int(n)
		} else if s, e := szV.AsConcreteString(); e == nil {
			seg.sizeName = s
		} else {
			return seg, segErr("`size` must be an Integer or a field-name String")
		}
	}
	return seg, nil
}

// segSize resolves a bits/pad/bytes/utf8 size for PACKING: a literal, a field
// named in the fields map, or -1 ("the rest", unused on pack).
func segSize(seg bitSeg, fields ReadMap, r *Registry, word string) (int, error) {
	if !seg.hasSize {
		return -1, nil
	}
	if seg.sizeName == "" {
		return seg.sizeLit, nil
	}
	v, ok := fields.Get(seg.sizeName)
	if !ok {
		return 0, r.BoruError("bytes_error", fmt.Sprintf("%s: size field %q is not provided", word, seg.sizeName), word)
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, r.BoruError("bytes_error", fmt.Sprintf("%s: size field %q is not an Integer", word, seg.sizeName), word)
	}
	return int(n), nil
}

// resolveSize resolves a decode-time size: a literal, "rest" (-1), or a name
// referring to one of THIS frame's already-decoded integer fields (`seen`) —
// the `size:'len'` case, e.g. a body sized by an earlier `len` field.
func resolveSize(seg bitSeg, seen map[string]int64, r *Registry, word string) (int, error) {
	if !seg.hasSize {
		return -1, nil
	}
	if seg.sizeName == "" {
		return seg.sizeLit, nil
	}
	if n, ok := seen[seg.sizeName]; ok {
		return int(n), nil
	}
	return 0, r.BoruError("bytes_error", fmt.Sprintf("%s: size field %q is not an earlier field", word, seg.sizeName), word)
}

// ---- bit writer / reader (MSB-first) --------------------------------------

type bitWriter struct {
	out    []byte
	acc    uint64 // pending sub-byte bits (MSB-first), 0..7 of them
	accCnt int
}

func (w *bitWriter) aligned() bool { return w.accCnt == 0 }

func (w *bitWriter) writeBits(val uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.acc = (w.acc << 1) | ((val >> uint(i)) & 1)
		w.accCnt++
		if w.accCnt == 8 {
			w.out = append(w.out, byte(w.acc))
			w.acc, w.accCnt = 0, 0
		}
	}
}

func (w *bitWriter) writeBytes(b []byte) { w.out = append(w.out, b...) }

type bitReader struct {
	b      []byte
	pos    int // byte index
	bitPos int // 0..7 within b[pos]
}

func (rd *bitReader) aligned() bool { return rd.bitPos == 0 }

func (rd *bitReader) remaining() int {
	if rd.pos >= len(rd.b) {
		return 0
	}
	return len(rd.b) - rd.pos
}

func (rd *bitReader) readBits(n int) (uint64, bool) {
	var v uint64
	for i := 0; i < n; i++ {
		if rd.pos >= len(rd.b) {
			return 0, false
		}
		bit := (uint64(rd.b[rd.pos]) >> uint(7-rd.bitPos)) & 1
		v = (v << 1) | bit
		rd.bitPos++
		if rd.bitPos == 8 {
			rd.bitPos = 0
			rd.pos++
		}
	}
	return v, true
}

func putUint(val uint64, width int, little bool) []byte {
	b := make([]byte, width)
	if little {
		for i := 0; i < width; i++ {
			b[i] = byte(val >> uint(8*i))
		}
	} else {
		for i := 0; i < width; i++ {
			b[width-1-i] = byte(val >> uint(8*i))
		}
	}
	return b
}

func getUint(b []byte, little bool) uint64 {
	var v uint64
	if little {
		for i := len(b) - 1; i >= 0; i-- {
			v = (v << 8) | uint64(b[i])
		}
	} else {
		for i := 0; i < len(b); i++ {
			v = (v << 8) | uint64(b[i])
		}
	}
	return v
}

func signExtend(v uint64, width int) int64 {
	bits := uint(width * 8)
	if bits < 64 && v&(1<<(bits-1)) != 0 {
		return int64(v) - (1 << bits)
	}
	return int64(v)
}

// fitsInt reports whether n is representable in a width-byte integer field of
// the given signedness — so packing rejects out-of-range values (300 for u8,
// -1 for u8, -129 for i8) instead of silently truncating them modulo the width.
// Width 8 from an int64 source is always representable (a u64 field takes any
// non-negative int64; an i64 field takes all of int64), so only sub-8-byte
// widths and the unsigned-negative case need an explicit bound.
func fitsInt(n int64, width int, signed bool) bool {
	if !signed && n < 0 {
		return false
	}
	if width >= 8 {
		return true
	}
	if signed {
		lo := -(int64(1) << uint(8*width-1))
		hi := (int64(1) << uint(8*width-1)) - 1
		return n >= lo && n <= hi
	}
	return n <= (int64(1)<<uint(8*width))-1
}

// fitsBits reports whether v fits an unsigned size-bit field (0 <= v < 1<<size),
// so a 4-bit field rejects 31 / -1 instead of writing only the low bits.
func fitsBits(v int64, size int) bool {
	if v < 0 {
		return false
	}
	if size >= 64 {
		return true
	}
	return v < (int64(1) << uint(size))
}

// checkSegLen enforces a declared size on a bytes/utf8 segment at pack time:
// when the segment carries an explicit `size` (a literal or a field name), the
// field's byte length MUST equal it, so `convert Bytes` cannot emit a frame
// whose length field disagrees with its payload. No size = "the rest", so the
// length is unconstrained.
func checkSegLen(seg bitSeg, fields ReadMap, have int, r *Registry, word string) error {
	if !seg.hasSize {
		return nil
	}
	n, err := segSize(seg, fields, r, word)
	if err != nil {
		return err
	}
	if n >= 0 && have != n {
		return r.BoruError("bytes_error", fmt.Sprintf("%s: field %q is %d bytes but its declared size is %d", word, seg.name, have, n), word)
	}
	return nil
}

// ---- serialise: convert Bytes <Binary> ------------------------------------

// convertBinaryToBytes serialises a Binary instance to wire bytes: it reads the
// layout off the instance's spec type and packs each segment from the
// instance's fields (a `value` segment packs its constant). The result is plain
// `Bytes` (data).
func convertBinaryToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	rawLayout, ok := binaryInstanceLayout(args[1])
	if !ok {
		return nil, r.BoruError("type_error", "convert Bytes: value is not a Binary instance", "convert")
	}
	segs, err := readBitSegments(rawLayout, r, "convert")
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil, err
	}
	oi, _ := AsClassInstance(args[1])
	fields := oi.AllFields() // field name → value (prototype chain flattened)
	w := &bitWriter{}
	for _, seg := range segs {
		if err := packSeg(w, seg, fields, r, "convert"); err != nil {
			return nil, err
		}
	}
	if !w.aligned() {
		return nil, r.BoruError("unaligned", "convert Bytes: bit segments do not sum to whole bytes", "convert")
	}
	return []Value{newBytes(w.out)}, nil
}

// packValue fetches a segment's source value for packing: a literal constant
// (the `value` key) or the named field from the supplied fields map.
func packValue(seg bitSeg, fields ReadMap) (Value, bool) {
	if seg.isLit {
		return NewInteger(seg.litVal), true
	}
	return fields.Get(seg.name)
}

func packSeg(w *bitWriter, seg bitSeg, fields ReadMap, r *Registry, word string) error {
	missing := func() error {
		return r.BoruError("bytes_error", fmt.Sprintf("%s: field %q is not provided", word, seg.name), word)
	}
	switch {
	case seg.typ == "pad":
		n, err := segSize(seg, fields, r, word)
		if err != nil {
			return err
		}
		w.writeBits(0, n)
		return nil
	case seg.typ == "bits":
		n, err := segSize(seg, fields, r, word)
		if err != nil {
			return err
		}
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		iv, _ := v.AsConcreteInteger()
		if !fitsBits(iv, n) {
			return r.BoruError("bytes_error", fmt.Sprintf("%s: value %d does not fit a %d-bit field", word, iv, n), word)
		}
		w.writeBits(uint64(iv), n)
		return nil
	}
	if !w.aligned() {
		return r.BoruError("unaligned", fmt.Sprintf("%s: %s segment is not byte-aligned", word, seg.typ), word)
	}
	switch seg.typ {
	case "bytes":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		b, bok := asBytes(v)
		if !bok {
			return r.BoruError("bytes_error", fmt.Sprintf("%s: field %q is not Bytes", word, seg.name), word)
		}
		if err := checkSegLen(seg, fields, len(b), r, word); err != nil {
			return err
		}
		w.writeBytes(b)
	case "utf8":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		s, serr := v.AsConcreteString()
		if serr != nil {
			return r.BoruError("bytes_error", fmt.Sprintf("%s: field %q is not a String", word, seg.name), word)
		}
		if err := checkSegLen(seg, fields, len(s), r, word); err != nil {
			return err
		}
		w.writeBytes([]byte(s))
	case "f32":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		f, _ := AsFloat(v)
		w.writeBytes(putUint(uint64(math.Float32bits(float32(f))), 4, seg.little))
	case "f64":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		f, _ := AsFloat(v)
		w.writeBytes(putUint(math.Float64bits(f), 8, seg.little))
	default: // fixed ints
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		n, _ := v.AsConcreteInteger()
		if !fitsInt(n, seg.width, seg.signed) {
			kind := "u"
			if seg.signed {
				kind = "i"
			}
			return r.BoruError("bytes_error", fmt.Sprintf("%s: value %d does not fit %s%d", word, n, kind, seg.width*8), word)
		}
		w.writeBytes(putUint(uint64(n), seg.width, seg.little))
	}
	return nil
}

// ---- unpack: decode wire bytes into a Binary instance ---------------------

// frameInstance decodes `b` against the spec type `specVal`'s layout and builds
// a Binary instance (an object instance of the spec class) from the decoded
// fields. Returns the instance, the leftover bytes, and any error (a needMore
// for a short buffer when streaming).
func frameInstance(specVal Value, b []byte, r *Registry, word string) (Value, []byte, error) {
	// A NAMED spec evaluates to its node (the Stage 2 flip); both the
	// layout probe and the AsClassType read below need the recorded
	// class content.
	if body, ok := TypeContentOf(specVal); ok && IsBareTypeNode(specVal) {
		specVal = body
	}
	rawLayout, ok := binarySpecLayout(specVal)
	if !ok {
		return Value{}, nil, frameTypeErr(r, specVal, word)
	}
	segs, err := readBitSegments(rawLayout, r, word)
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return Value{}, nil, err
	}
	m, rest, derr := decodeSegs(b, segs, r, word, false)
	if derr != nil {
		return Value{}, nil, derr
	}
	ot, _ := AsClassType(specVal)
	inst, ierr := core.MakeObject(ot, NewMap(m), r)
	if ierr != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return Value{}, nil, ierr
	}
	return inst[0], rest, nil
}

// unpackFrameHandler decodes `unpack <PacketSpec> b` into a Binary instance.
func unpackFrameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	inst, _, err := frameInstance(args[0], firstBytes(args[1]), r, "unpack")
	if err != nil {
		if _, short := err.(needMore); short {
			return nil, r.BoruError("no_match", "unpack: buffer too short for the frame", "unpack")
		}
		return nil, err
	}
	return []Value{inst}, nil
}

// unpackPrefixFrameHandler decodes a leading frame: {ok: <Binary> rest: <Bytes>} | {need n}.
func unpackPrefixFrameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	inst, rest, err := frameInstance(args[0], firstBytes(args[1]), r, "unpack-prefix")
	if err != nil {
		if ne, ok := err.(needMore); ok {
			out := NewOrderedMap()
			out.Set("need", NewInteger(int64(ne.n)))
			return []Value{NewMap(out)}, nil
		}
		return nil, err
	}
	out := NewOrderedMap()
	out.Set("ok", inst)
	out.Set("rest", newBytes(rest))
	return []Value{NewMap(out)}, nil
}

func firstBytes(v Value) []byte { b, _ := asBytes(v); return b }

func frameTypeErr(r *Registry, t Value, word string) error {
	return r.BoruErrorHint("type_error",
		fmt.Sprintf("%s: %s is not a BinarySpec (frame) type", word, t.String()),
		word,
		"define one with `def P (refine BinarySpec [ {name:'x' type:'u8'} … ])`")
}

// needMore signals a short buffer to unpack-prefix (turned into {need:n}).
type needMore struct{ n int }

func (needMore) Error() string { return "unpack-prefix: need more bytes" }

// decodeSegs walks the segments over b. When bind is true it installs each
// name into scope (unpack); otherwise it collects an ordered map (the
// unpack-prefix {ok} payload) and returns the leftover bytes. A short
// buffer returns a needMore error.
func decodeSegs(b []byte, segs []bitSeg, r *Registry, word string, bind bool) (*OrderedMap, []byte, error) {
	rd := &bitReader{b: b}
	out := NewOrderedMap()
	// seen records integer fields decoded so far so a size like `bytes(len)`
	// resolves against this frame's own earlier segments — needed for the
	// non-binding unpack-prefix where names are not installed into scope.
	seen := map[string]int64{}
	emit := func(name string, v Value) {
		if bind {
			InstallDef(r, name, v)
		} else {
			out.Set(name, v)
		}
	}
	for _, seg := range segs {
		switch seg.typ {
		case "pad":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			bits, ok := rd.readBits(n)
			if !ok {
				return nil, nil, shortErr(word, bind)
			}
			// pad is alignment filler the encoder always writes as zero;
			// nonzero padding is a malformed frame, not a value to discard.
			if bits != 0 {
				return nil, nil, r.BoruError("no_match", fmt.Sprintf("%s: nonzero padding bits", word), word)
			}
			continue
		case "bits":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			v, ok := rd.readBits(n)
			if !ok {
				return nil, nil, shortErr(word, bind)
			}
			if seg.isLit {
				if int64(v) != seg.litVal {
					return nil, nil, r.BoruError("no_match", fmt.Sprintf("%s: guard bits != %d", word, seg.litVal), word)
				}
				continue
			}
			seen[seg.name] = int64(v)
			emit(seg.name, NewInteger(int64(v)))
			continue
		}
		if !rd.aligned() {
			return nil, nil, r.BoruError("unaligned", fmt.Sprintf("%s: %s segment is not byte-aligned", word, seg.typ), word)
		}
		switch seg.typ {
		case "bytes", "utf8":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			if n < 0 {
				n = rd.remaining() // trailing "rest"
			}
			if rd.remaining() < n {
				return nil, nil, shortErr2(word, bind, n-rd.remaining())
			}
			chunk := rd.b[rd.pos : rd.pos+n : rd.pos+n]
			rd.pos += n
			if seg.typ == "utf8" {
				if !utf8.Valid(chunk) {
					return nil, nil, r.BoruError("bad-encoding", word+": utf8 segment is not valid UTF-8", word)
				}
				emit(seg.name, NewString(string(chunk)))
			} else {
				emit(seg.name, newBytes(chunk))
			}
		case "f32", "f64":
			if rd.remaining() < seg.width {
				return nil, nil, shortErr2(word, bind, seg.width-rd.remaining())
			}
			raw := rd.b[rd.pos : rd.pos+seg.width]
			rd.pos += seg.width
			var f float64
			if seg.width == 4 {
				f = float64(math.Float32frombits(uint32(getUint(raw, seg.little))))
			} else {
				f = math.Float64frombits(getUint(raw, seg.little))
			}
			emit(seg.name, NewFloat(f))
		default: // fixed ints
			if rd.remaining() < seg.width {
				return nil, nil, shortErr2(word, bind, seg.width-rd.remaining())
			}
			raw := rd.b[rd.pos : rd.pos+seg.width]
			rd.pos += seg.width
			u := getUint(raw, seg.little)
			var n int64
			if seg.signed {
				n = signExtend(u, seg.width)
			} else {
				// boru Integer is int64; a u64 with the high bit set has no
				// faithful Integer (it would wrap negative), so reject it
				// rather than expose a corrupted value.
				if u > math.MaxInt64 {
					return nil, nil, r.BoruError("bytes_error", fmt.Sprintf("%s: u%d value exceeds Integer range", word, seg.width*8), word)
				}
				n = int64(u)
			}
			if seg.isLit {
				if n != seg.litVal {
					return nil, nil, r.BoruError("no_match", fmt.Sprintf("%s: guard %s != %d", word, seg.typ, seg.litVal), word)
				}
				continue
			}
			seen[seg.name] = n
			emit(seg.name, NewInteger(n))
		}
	}
	if !rd.aligned() {
		return nil, nil, r.BoruError("unaligned", word+": trailing sub-byte bits", word)
	}
	return out, rd.b[rd.pos:], nil
}

func shortErr(word string, bind bool) error {
	if bind {
		return fmt.Errorf("[boru/no_match] %s: buffer too short", word)
	}
	return needMore{n: 1}
}

func shortErr2(word string, bind bool, need int) error {
	if bind {
		return fmt.Errorf("[boru/no_match] %s: buffer too short", word)
	}
	if need < 1 {
		need = 1
	}
	return needMore{n: need}
}
