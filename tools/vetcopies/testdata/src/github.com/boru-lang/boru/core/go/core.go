// Package core stands in for core/go: the analyzer recognises Value by its
// name and this import path.
package core

type Value struct{ n int }

type Type = Value

type Tape []Value

func IsWord(v *Value) bool { return v != nil }

func Plain(v Value) bool { return v.n == 0 }

func NewTypeLiteral(t *Type) Value { return *t }
