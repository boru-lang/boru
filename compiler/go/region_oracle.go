package compiler

import core "github.com/boru-lang/boru/core/go"

// RegionOracle arms the COLLECT oracle (the OpCollect doc): when set, the
// lowerer emits an OpCollect before every dispatch whose descriptor Phase B
// completed, and the VM walks that descriptor live and reports whether the
// walk reproduces the recorded claim (core.RegionOracleEvent). Off by
// default, so the default lane's bytecode is byte-identical and pays
// nothing; a corpus lane (test/go/langspec) sets it for the duration of its
// walk. A package variable rather than an environment read because the lane
// is a test, and a test that flips a flag must be able to flip it back.
//
// It is read at LOWERING, so a program compiled while the flag is set carries
// its oracle ops whatever the flag says when it runs.
var RegionOracle bool

// emitRegionOracle places the oracle op for the descriptor just appended at
// Program.Regions[idx], when the lane is armed. Stack-neutral, so the
// lowerer's operand model is untouched; it sits between the operand layout
// and the call opcode, which is exactly where the operands the oracle checks
// against are on the stack.
func (lw *lowerer) emitRegionOracle(idx int, pos core.SrcPos) {
	if RegionOracle {
		lw.emit(OpCollect, idx, pos)
	}
}
