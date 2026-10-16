package core

// Interned content (design/VALUE-NO-COPY.0.md §6 phase 4). The contents the
// constructors mint most — the small integers, the two booleans, none, the
// empty string, the payload-less structural markers (open and close paren,
// end) and the reach lowering's dot words — are built once and shared by
// every holder of them, so minting one allocates nothing. Sharing is safe
// because content is immutable once published: a holder that needs
// different content gets content of its own (ownContent), and nothing
// writes these.

const (
	internIntMin = -128
	internIntMax = 1023
)

var (
	internedInts        = buildInternedInts()
	internedTrue        = &content{parent: TBoolean, data: BoolPayload{B: true}}
	internedFalse       = &content{parent: TBoolean, data: BoolPayload{B: false}}
	internedNone        = &content{parent: TNone, data: NonePayload{}}
	internedEmptyString = &content{parent: TStringEmpty, data: StrPayload{}}
	internedOpenParen   = &content{parent: TOpenParen}
	internedCloseParen  = &content{parent: TCloseParen}
	internedEnd         = &content{parent: TEnd}
	// The reach lowering's dispatch words (appendLoweredReach): a dot
	// access expands to them at every evaluation.
	internedDotWord  = &content{parent: TWord, data: WordInfo{Name: "dot", ArgCount: -1}}
	internedDotrWord = &content{parent: TWord, data: WordInfo{Name: "dotr", ArgCount: -1}}
)

func buildInternedInts() *[internIntMax - internIntMin + 1]content {
	a := new([internIntMax - internIntMin + 1]content)
	for i := range a {
		a[i].parent = TInteger
		a[i].data = IntPayload{N: int64(i + internIntMin)}
	}
	return a
}

// holderOf is a holder of the content c, whose type is t, with the identity
// NewValueRaw would give a value minted with a payload: one only while a
// check or compile pass (or the parser) is minting identities. c is shared
// (interned) or a cell the caller just built (marker_cell.go).
func holderOf(t *Type, c *content) Value {
	v := Value{c: c}
	if checkPassDepth.Load() > 0 {
		SetID(&v, GenerateID(IDPrefixForType(t)))
	}
	return v
}
