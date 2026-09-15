package lang

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
)

// The COLLECT oracle from real programs: under compiler.RegionOracle the
// lowerer places an OpCollect before every dispatch Phase B described — at
// the mono native, user, poly user and poly native seats — and the VM
// executes each one, reporting through the region-oracle hook without
// changing the run. The corpus lane (test/go/langspec) is the measurement;
// these pin the wiring seat by seat, and that the default lane is untouched.
func TestRegionOracleWiring(t *testing.T) {
	compiler.RegionOracle = true
	t.Cleanup(func() { compiler.RegionOracle = false })

	run := func(t *testing.T, src string) (string, []RegionOracleEvent, []any) {
		t.Helper()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := b.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q must compile: reason=%q err=%v", src, reason, cerr)
		}
		var evs []RegionOracleEvent
		disarm := b.ArmRegionOracleHook(func(ev RegionOracleEvent) { evs = append(evs, ev) })
		defer disarm()
		got, compiled, rerr := b.RunCompiled(src)
		if rerr != nil || !compiled {
			t.Fatalf("%q must run compiled: compiled=%v err=%v", src, compiled, rerr)
		}
		return prog.Disassemble(), evs, got
	}
	find := func(evs []RegionOracleEvent, word string) *RegionOracleEvent {
		for i := range evs {
			if evs[i].Word == word {
				return &evs[i]
			}
		}
		return nil
	}

	t.Run("the mono native seat reproduces a forward dispatch", func(t *testing.T) {
		dis, evs, got := run(t, `add 1 2`)
		if !strings.Contains(dis, "COLLECT") || !strings.Contains(dis, "collect oracle over add") {
			t.Fatalf("no oracle op before the native call:\n%s", dis)
		}
		if strings.Index(dis, "COLLECT") > strings.Index(dis, "CALL_NATIVE") {
			t.Errorf("the oracle must sit BEFORE the call it checks:\n%s", dis)
		}
		if ev := find(evs, "add"); ev == nil || ev.Outcome != "reproduced" || ev.NFwd != 2 {
			t.Errorf("`add 1 2` walked live must reproduce the two-slot claim: %+v", evs)
		}
		if len(got) != 1 || got[0] != int64(3) {
			t.Errorf("the run is unchanged by the oracle: %v", got)
		}
	})

	t.Run("the user seat reproduces, and region_desc.go's k pair reproduces", func(t *testing.T) {
		dis, evs, got := run(t, `def w fn [[a:Any b:Any][Any][a]] def k 5 def go fn [[][Any][w k 1]] go`)
		if !strings.Contains(dis, "collect oracle over w") {
			t.Fatalf("no oracle op for the user call inside go's unit:\n%s", dis)
		}
		if ev := find(evs, "w"); ev == nil || ev.Outcome != "reproduced" || ev.NFwd != 2 {
			t.Errorf("`w k 1` with k a module value: the live walk collects k and reproduces the claim: %+v", evs)
		}
		if len(got) != 1 || got[0] != int64(5) {
			t.Errorf("go answers 5: %v", got)
		}
	})

	t.Run("the poly seats execute: a group declines, a type name reproduces", func(t *testing.T) {
		dis, evs, _ := run(t, `def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer b:Integer] [Integer] [1] [a:Integer b:String] [Integer] [2]] g 7 (id 5)`)
		if strings.Index(dis, "collect oracle over g") > strings.Index(dis, "CALL_USER_POLY") {
			t.Fatalf("the oracle must precede the poly user call:\n%s", dis)
		}
		// The claim is NFwd 1 (the written 7). The candidates are the poly's
		// stored arm table, both arms consuming position 1, so the plan walk
		// asks the host to evaluate the paren group there — and this host
		// declines every evaluation: nothing is known, nothing is claimed.
		if ev := find(evs, "g"); ev == nil || ev.Outcome != "declined" || ev.NFwd != 1 {
			t.Errorf("a group a viable arm consumes is an evaluation the host declines: %+v", evs)
		}
		dis, evs, _ = run(t, `def y (if (1 gt 0) [1] ['s']) is y Integer`)
		if strings.Index(dis, "collect oracle over is") > strings.Index(dis, "CALL_NATIVE_POLY") {
			t.Fatalf("the oracle must precede the poly native call:\n%s", dis)
		}
		// Phase B stopped at the type name (a word slot with no def-stack
		// binding), and so does the live scan: a builtin type name is
		// stepped to its literal at arrival, not claimed by the scan.
		if ev := find(evs, "is"); ev == nil || ev.Outcome != "reproduced" || ev.NFwd != 1 {
			t.Errorf("the type-name slot: the live scan stops where the record did: %+v", evs)
		}
	})

	t.Run("the default lane carries no oracle op", func(t *testing.T) {
		compiler.RegionOracle = false
		defer func() { compiler.RegionOracle = true }()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, _, _, cerr := b.CompileCheck(`add 1 2`)
		if cerr != nil || prog == nil {
			t.Fatal("must compile")
		}
		if strings.Contains(prog.Disassemble(), "COLLECT") {
			t.Errorf("with the lane off the bytecode must be byte-identical:\n%s", prog.Disassemble())
		}
	})
}
