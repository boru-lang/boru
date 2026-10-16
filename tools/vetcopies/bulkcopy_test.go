package main

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// The fixture package a exercises every shape the analyzer must report and
// the near misses it must not; testdata's core package stands in for
// core/go at the same import path.
func TestBulkCopy(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), BulkCopy, "a")
}

// Package b holds one function-variable alias of a pointer-taking function
// (reported) beside one of a by-value function and a wrapper (both fine).
func TestFnVarAlias(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), FnVarAlias, "b")
}
