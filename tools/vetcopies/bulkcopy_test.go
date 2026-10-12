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
