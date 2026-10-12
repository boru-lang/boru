// Package core stands in for core/go: the analyzer recognises Value by its
// name and this import path.
package core

type Value struct{ n int }

type Type = Value

type Tape []Value
