// Package core stands in for core/go: the analyzers recognise Value and
// Datum by their names and this import path.
package core

type Value struct{ d *Datum }

type Datum struct{ n int }

type Type = Value

type Tape []Value

type Run []Datum

func IsWord(v *Value) bool { return v != nil }

func Plain(v Value) bool { return v.d == nil }

func NewTypeLiteral(t *Type) Value { return *t }
