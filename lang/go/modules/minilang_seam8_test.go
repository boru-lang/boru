package modules

import (
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// (The micron grammar-map / grammar-builder / finalize seams died with the
// MiniLang.micron tombstone — the +m literal grammar is fixed to the
// builtin Micron leaves; TestMiniCovMicronTombstone pins the frozen
// surface.)

// TestW8MicronHandlerSrcError drives miniMicronHandler's src compile failure.
func TestW8MicronHandlerSrcError(t *testing.T) {
	r := mcovReg(t)
	if _, err := miniMicronHandler([]native.Value{native.NewTypeLiteral(native.TString)}, nil, nil, r); err == nil {
		t.Error("micron handler: a non-concrete src should error")
	}
}
