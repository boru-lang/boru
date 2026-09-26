package core

import (
	"strings"
	"testing"
)

// TestStepBudget pins Engine.StepBudget / StepsTaken (Codex P2 on PR #512):
// the budget caps the Run loop, an exhausted run reports the configured
// bound in place of the cap, the restore puts both back, and StepsTaken
// counts every step across runs.
func TestStepBudget(t *testing.T) {
	e := New(covRegistry(t, nil))
	lits := func(n int) []Value {
		out := make([]Value, n)
		for i := range out {
			out[i] = NewInteger(int64(i))
		}
		return out
	}
	before := e.StepsTaken()
	if _, err := e.Run(lits(3)); err != nil {
		t.Fatalf("unbudgeted run: %v", err)
	}
	if e.StepsTaken() <= before {
		t.Fatalf("StepsTaken must grow over a run: %d -> %d", before, e.StepsTaken())
	}
	full := e.stepLimit
	restore := e.StepBudget(2, 99)
	_, err := e.Run(lits(10))
	if err == nil || !strings.Contains(err.Error(), "step limit of 99") {
		t.Errorf("a 10-literal run under a 2-step budget must report the configured 99: %v", err)
	}
	restore()
	if e.stepLimit != full || e.limitReport != 0 {
		t.Errorf("restore must put the cap and the report back: %d/%d", e.stepLimit, e.limitReport)
	}
	restore = e.StepBudget(0, 0)
	if e.stepLimit != 1 {
		t.Errorf("a non-positive budget is at least one step: %d", e.stepLimit)
	}
	restore()
	if _, err := e.Run(lits(10)); err != nil {
		t.Errorf("the restored engine runs freely: %v", err)
	}
}

// TestIsGenMemoName pins the instantiation memo's name predicate: the key
// genMemoKey builds is a memo name, a program's own binding is not.
func TestIsGenMemoName(t *testing.T) {
	if !IsGenMemoName(genMemoKey("T_1", "Integer")) {
		t.Error("an instantiation memo key is a memo name")
	}
	if IsGenMemoName("x") || IsGenMemoName("Box") {
		t.Error("a program binding is not a memo name")
	}
}
