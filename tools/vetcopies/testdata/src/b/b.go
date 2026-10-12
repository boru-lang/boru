package b

import core "github.com/boru-lang/boru/core/go"

var (
	IsWord = core.IsWord // want "function variable aliases IsWord, which takes a \\*core.Value: declare a wrapper func"
	Plain  = core.Plain
	// A *Type parameter carries a lattice node, already on the heap.
	NewTypeLiteral = core.NewTypeLiteral
)

func Wrapped(v *core.Value) bool { return core.IsWord(v) }
