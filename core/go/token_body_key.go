package core

import (
	"strconv"
	"strings"
)

// token_body_key.go — the name a RAW token body carries in the registry's
// run-time stamp memo (Registry.TokenBodyStamp), shared by every seam that
// compiles a body at run time: the VM's token-body host (eng/go
// vm_token_body.go, a body reaching InvokeBody) and a handler's throwaway
// signature over a body it binds itself (lang/go/native StampBodySig, a
// body reaching CallBoru). Each seam closes the key with its own shape —
// the input count and types, the params' names and types — so one body
// applied over like inputs compiles once, and a heterogeneous stream once
// per shape it meets.

// TokenBodyKey names body — a raw token list — for the stamp memo: its
// tokens rendered with their positions when every token is identity-free
// (TokenBodyContentKeyable), so two bodies of the same text at the same
// positions are the same program text and one unit answers both, error
// positions included; else its value ID, since a body carrying a reference
// value (a flex, a map, an object — a list built by `push` around a live
// store) can render alike around different instances and the unit bakes the
// first. A reference-bearing body with no ID (the mode-gated ID elision) is
// not named at all, and the seam keeps its interpreter path.
func TokenBodyKey(body Value, tokens []Value) (string, bool) {
	var b strings.Builder
	switch {
	case TokenBodyContentKeyable(tokens):
		b.WriteString("txt:")
		for _, tok := range tokens {
			p := tok.Pos()
			b.WriteString(tok.String())
			b.WriteByte('@')
			b.WriteString(strconv.Itoa(p.Row))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(p.Col))
			b.WriteByte(' ')
		}
	case body.ID != "":
		b.WriteString("id:")
		b.WriteString(body.ID)
	default:
		return "", false
	}
	return b.String(), true
}

// TokenBodyContentKeyable reports whether every token is identity-free — a
// word, a scalar, or a list of those, at any depth — so the body's rendered
// text names it (TokenBodyKey).
func TokenBodyContentKeyable(tokens []Value) bool {
	for _, tok := range tokens {
		switch d := tok.Data.(type) {
		case WordInfo:
			continue
		case ListPayload:
			if !TokenBodyContentKeyable(d.Elems) {
				return false
			}
			continue
		}
		if tok.Parent == nil || !tok.Parent.ConformsTo(TScalar) {
			return false
		}
	}
	return true
}

// TokenBodyInputType is the type a run-time stamp declares for one seam
// input: its concrete Parent, Any for a value with none.
func TokenBodyInputType(v Value) *Type {
	if v.Parent == nil {
		return TAny
	}
	return v.Parent
}
