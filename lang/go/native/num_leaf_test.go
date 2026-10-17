package native

import (
	"math/big"
	"testing"

	core "github.com/boru-lang/boru/core/go"

	"github.com/cockroachdb/apd/v3"
)

// TestNumLeafClassifiesEveryLeaf pins numLeaf's identity fast path to the
// conformance chain it short-cuts: each canonical leaf, a refinement of
// each (a Parent of its own, so the chain decides), and a value whose
// Parent is a handle copy of a leaf node rather than the node itself.
func TestNumLeafClassifiesEveryLeaf(t *testing.T) {
	r := mirrorReg(t)
	integer, float := NewInteger(3), NewFloat(1.5)
	bigInt, bigDec := NewBigInteger(big.NewInt(7)), NewBigDecimal(apd.New(15, -1))
	intCopy, floatCopy := *TInteger, *TFloat
	cases := []struct {
		name string
		v    Value
		want int
	}{
		{"Integer", integer, leafInteger},
		{"Float", float, leafFloat},
		{"BigInteger", bigInt, leafBigInteger},
		{"BigDecimal", bigDec, leafBigDecimal},
		{"refined Integer", core.MintRetagged(&integer, r.Types.MintType("LeafInt", TInteger)), leafInteger},
		{"refined Float", core.MintRetagged(&float, r.Types.MintType("LeafFloat", TFloat)), leafFloat},
		{"refined BigInteger", core.MintRetagged(&bigInt, r.Types.MintType("LeafBigInt", TBigInteger)), leafBigInteger},
		{"refined BigDecimal", core.MintRetagged(&bigDec, r.Types.MintType("LeafBigDec", TBigDecimal)), leafBigDecimal},
		{"Integer handle copy", NewValueRaw(&intCopy, IntPayload{N: 3}), leafInteger},
		{"Float handle copy", NewValueRaw(&floatCopy, FloatPayload{F: 1.5}), leafFloat},
	}
	for _, c := range cases {
		if got := numLeaf(&c.v); got != c.want {
			t.Errorf("numLeaf(%s) = %d, want %d", c.name, got, c.want)
		}
	}
	// The leaves are siblings: no leaf's value classifies as another's.
	if numLeaf(&float) == leafInteger || numLeaf(&integer) == leafFloat {
		t.Error("Integer and Float classified as one leaf")
	}
}
