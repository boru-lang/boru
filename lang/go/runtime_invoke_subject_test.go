package lang

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// unattributedEntries runs src compiled with the interp-entry hook armed and
// returns the per-seam count of UNATTRIBUTED interpreter entries — the
// interp-entry census's rule (test/go/langspec/interp_entry_census_test.go),
// applied to one program.
func unattributedEntries(t *testing.T, src string) (map[string]int, []any) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetOutput(&bytes.Buffer{})
	var mu sync.Mutex
	seen := map[string]int{}
	disarm := a.ArmInterpEntryHook(func(ev InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" {
			return
		}
		mu.Lock()
		seen[ev.Seam]++
		mu.Unlock()
	})
	got, compiled, err := a.RunCompiled(src)
	disarm()
	if err != nil || !compiled {
		t.Fatalf("%q: compiled %v / %v (compiled=%v)", src, got, err, compiled)
	}
	mu.Lock()
	defer mu.Unlock()
	return seen, got
}

// TestInvokeSubjectHostsOnVM: `Test.invoke` runs its subject through the
// InvokeBody seam — the word's tokens as the body, the inputs as resolved
// stack data — so on a compiled run the VM's token-body host dispatches the
// subject as a run-time-stamped unit and the invoke adds NO interpreter entry
// (it used to run the subject on a fresh engine of its own: the interp-entry
// census's module-test.tsv L37/L38). Both lanes answer alike, the dotted
// subject (`"M.sq"`) and the `run-spec` cases included, and a subject that
// raises is the invoke's Error VALUE on both.
func TestInvokeSubjectHostsOnVM(t *testing.T) {
	const double = `import "boru:test"  def double fn [[n:Integer] [Integer] [n 2 mul]] end `
	for _, c := range []struct{ src, want string }{
		{double + `[3] Test.invoke double/q`, "[6]"},
		{double + `def s {name: "doubling" subject: double/q cases: [{name: "d3" in: [3] out: 6} {name: "d0" in: [0] out: 0}] subs: []} end s Test.run-spec end Test.summary`, "[{total:2 passed:2 failed:0}]"},
		{`import "boru:test" import module [def sq fn [[n:Integer][Integer][n mul n]] export "M" {sq: sq/v}] end [4] Test.invoke "M.sq"`, "[16]"},
		{`import "boru:test" def boom fn [[n:Integer][Integer][raise bad_input "boom"]] end [4] Test.invoke boom/q`, "[error(boom)]"},
		{`import "boru:test" def eat fn [[n:Integer] [] []] end [4] Test.invoke eat/q`, "[none]"},
	} {
		seen, gotC := unattributedEntries(t, c.src)
		if len(seen) != 0 {
			t.Errorf("%q: unattributed interpreter entries %v — the subject is not hosted", c.src, seen)
		}
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: compiled %v, want %s", c.src, gotC, c.want)
		}
		a := mustNew(t)
		a.SetOutput(&bytes.Buffer{})
		gotI, err := a.RunInterp(c.src)
		if err != nil || fmt.Sprint(gotI) != c.want {
			t.Errorf("%q: interp %v / %v, want %s", c.src, gotI, err, c.want)
		}
	}
}

// TestInvokeSubjectUndefinedStaysInterpreted: a subject the stamp declines —
// here an undefined word, which the check pass stops at — takes the seam's
// interpreter path and answers the Error value the fresh engine used to
// (corpus-modules.tsv L165/L166 stay on the census; the row is a bare read of
// the interpreter's undefined-word raise, which no pre-check reproduces).
func TestInvokeSubjectUndefinedStaysInterpreted(t *testing.T) {
	src := `import "boru:test" end Test.invoke "a" ["a","b"]`
	seen, gotC := unattributedEntries(t, src)
	if fmt.Sprint(gotC) != "[error(undefined word: a)]" {
		t.Errorf("compiled %v, want the undefined-word Error value", gotC)
	}
	if seen["RunResolved"] != 1 || seen["Engine.Run"] != 1 {
		t.Errorf("the declined subject must take the seam's interpreter path once: %v", seen)
	}
	a := mustNew(t)
	gotI, err := a.RunInterp(src)
	if err != nil || fmt.Sprint(gotI) != "[error(undefined word: a)]" {
		t.Errorf("interp %v / %v", gotI, err)
	}
}
