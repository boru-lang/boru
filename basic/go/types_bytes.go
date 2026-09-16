package basic

import (
	"bytes"
	"fmt"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// Scalar/Bytes — the immutable byte-string content type, moved out of
// lang's word file (ADR-013 staged follow-up). The type identity,
// Behavior (render / equality / ordering / size / const-bakeability),
// Go-bridge wiring, and the constructor/unwrapper pair live here; the
// bytes WORDS and the binary-frame (BinarySpec / Binary) machinery
// stay in lang with the class codec they build on.

// TBytes is the Scalar/Bytes type — an immutable byte string and the
// foundation for all binary-adjacent functionality (design/go-modules/
// BYTES.10.md). FixedID 1009 comes from the Scalar band
// (1000-1999, alongside the Scalar/Time family — both owned by this
// component since the ADR-013 staged move); it is pinned in
// lang/go/test/fixedid_stability_test.go. Registered as an external
// builtin in the var initialiser so package-level signature slices that
// reference TBytes see a non-nil pointer at slice-init time.
var TBytes = registerBytesType()

func registerBytesType() *core.Type {
	t, err := core.Builtin.RegisterType("Scalar/Bytes", 1009, core.OwnerKernel, BytesBehavior{})
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		recordTypeInitErr(fmt.Errorf("types_bytes: register Scalar/Bytes: %w", err))
	}
	return t
}

// init teaches the Go bridge how to convert []byte <-> Bytes. eng owns
// the bridge but not the type, so the conversion is installed here. The
// from-side copies (copy-on-ingest): the caller may reuse its slice.
// Done in init() rather than inside registerBytesType so the closures'
// reference to NewBytes (-> TBytes) does not form a var-initialisation
// cycle — same reason lang builds miscNatives in init().
func init() {
	core.RegisterBytesBridge(
		func(b []byte) Value { return NewBytes(append([]byte(nil), b...)) },
		// copy-on-export: a host that calls core.ToNative gets a fresh copy,
		// so mutating it cannot corrupt the immutable Bytes value's backing
		// array (the export twin of FromNative's copy-on-ingest, §4.2).
		func(v Value) ([]byte, bool) {
			b, ok := AsBytes(v)
			if !ok {
				return nil, false
			}
			return append([]byte(nil), b...), true
		},
	)
}

// NewBytes wraps an OWNED []byte as an immutable Bytes value. The caller
// must not retain or mutate the slice afterwards: Bytes shares its
// backing array zero-copy on clone/fork/send (no DeepCloner), which is
// safe precisely because no word mutates a Bytes in place (BYTES.10.md
// §4). Construction at a trust boundary (the Go bridge, a future socket
// recv) must hand newBytes a fresh copy.
func NewBytes(b []byte) Value { return core.NewExtension(TBytes, b) }

// NewBytesValue wraps an OWNED []byte as a Bytes value without copying —
// same contract as newBytes (the caller must never retain or mutate the
// slice). Exported for the boru:net socket words, whose recv path hands
// over freshly-read buffers (the copy-on-ingest boundary NewBytes's doc
// anticipates).
func NewBytesValue(b []byte) Value { return NewBytes(b) }

// AsBytesValue unwraps a Bytes value's backing slice (shared, not
// copied); ok=false when v is not a concrete Bytes. Exported for boru:net.
func AsBytesValue(v Value) ([]byte, bool) { return AsBytes(v) }

// AsBytes extracts the backing slice of a Bytes value.
func AsBytes(v Value) ([]byte, bool) {
	if v.Parent == nil || !v.Parent.ConformsTo(TBytes) {
		return nil, false
	}
	ep, ok := v.Data.(ExtensionPayload)
	if !ok {
		return nil, false
	}
	b, ok := ep.Body.([]byte)
	return b, ok
}

// BytesBehavior renders Bytes as hex, compares byte-lexicographically,
// and reports its byte length. Match/Equal fall back to the kernel
// default for non-Bytes operands.
type BytesBehavior struct{}

func (BytesBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }

// BakeableConst opts Bytes into the compiler's const pool
// (core.ConstBakeable): Bytes is immutable — no word mutates the backing
// array in place (BYTES.10.md §4; NewBytes's ownership contract) — and
// already shares that array zero-copy on clone/fork/send, so a pooled
// const shares exactly as the interpreter does. This is what let a
// module-scope Bytes binding (mini-s3's s3-crlf delimiter) bake into a
// stored-fn unit instead of refusing the unit; since the seventy-first
// increment a stored unit reads such a binding live instead, and the
// pool takes Bytes wherever else a const is baked.
func (BytesBehavior) BakeableConst(_ Value) bool { return true }

// Format renders Bytes as length-capped hex, e.g. Bytes<68 65 6c 6c 6f>.
func (BytesBehavior) Format(v Value) string {
	b, ok := AsBytes(v)
	if !ok {
		return "Bytes<?>"
	}
	const capN = 32
	var sb strings.Builder
	sb.WriteString("Bytes<")
	show := len(b)
	if show > capN {
		show = capN
	}
	for i := 0; i < show; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02x", b[i])
	}
	if show < len(b) {
		fmt.Fprintf(&sb, " … (%d)", len(b))
	}
	sb.WriteByte('>')
	return sb.String()
}

func (BytesBehavior) Equal(a, b Value) bool {
	ab, aok := AsBytes(a)
	bb, bok := AsBytes(b)
	if !aok || !bok {
		return core.DefaultBehavior.Equal(a, b)
	}
	return bytes.Equal(ab, bb)
}

// Compare orders Bytes byte-lexicographically (the Comparer capability),
// opening with the type-literal-first rule so a bare `Bytes` literal
// sorts below every concrete value (two literals bubble to Rank).
func (BytesBehavior) Compare(a, b Value) (int, error) {
	aLit, bLit := IsBareTypeNode(a), IsBareTypeNode(b)
	switch {
	case aLit && bLit:
		return 0, core.ErrNoComparer
	case aLit:
		return -1, nil
	case bLit:
		return 1, nil
	}
	ab, aok := AsBytes(a)
	bb, bok := AsBytes(b)
	if !aok || !bok {
		return 0, core.ErrNoComparer
	}
	return bytes.Compare(ab, bb), nil
}

// Size reports the byte length (the Sizer capability) so the generic
// `size` word works on Bytes with no dedicated word.
func (BytesBehavior) Size(v Value) int {
	if b, ok := AsBytes(v); ok {
		return len(b)
	}
	return 0
}
