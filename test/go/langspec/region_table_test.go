// region_table_test.go gates §6.2/§6.5's INERT region-descriptor stage: every
// compiled program carries the descriptors Phase B completed, and every one of
// them is well-formed against the program's own tables.
//
// This is the region table's turn at the discipline the bind twins were held
// to before their flip — emit the table while nothing reads it, and assert it
// against the corpus, so the first thing OpCollect executes is a descriptor
// the whole corpus has already exercised. `Validate` is the assertion: it is
// the check that a slot inside the recorded claim was actually given a source
// and that the source's index addresses the table it claims to.
//
// NO ALLOWANCE. A malformed descriptor is a recorder defect, not a number to
// baseline: the sentinel SlotNone exists precisely so a missed initialisation
// cannot masquerade as Consts[0], and a sentinel nothing checks moves the
// silent wrong answer one step later.
//
// WHAT THE TABLE COVERS, stated because a census that does not say what it
// omits reads as covering everything. Phase B is seated on RecordCall — the
// mono native dispatch — and, since 2026-09-14 (the first slice of the
// generic lane's line), on RecordUserCall — the committed user-fn call,
// claimed under the dispatching word's own name and position, which the
// check pass publishes as CheckState.CurCallWord/CurCallPos — and, since the
// sixty-first increment, on the two POLY records as well: RecordUserPolyCall
// (keyed by the same published pair) and RecordPolyCall (whose pos is the
// word's at every call site). The dyn-apply and dyn-method families still
// have their own entry points and claim nothing, so their regions are
// captured by Phase A and never completed. Widening those seats is
// follow-on work, and
// lang/go/region_capture_e2e_test.go fails if a seat lands without this note
// being updated.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// regionTally is the census's accumulator. The three NFwd buckets are the
// shape of the model's central claim — a region's extent is its STATEMENT
// while a dispatch's claim is usually far shorter — so they are counted
// rather than assumed.
// routedFloor is the ratchet on the generic lane's executing arm: how many
// corpus dispatches lower THROUGH their descriptor (OpDispatchGeneric). UP
// only — a fall means a seat stopped routing or the drivability rule
// narrowed without a measurement saying so.
const routedFloor = 600 // 676 measured at the head (2026-09-15, the sixty-fifth increment: the user seat's 2 and the native seat's 674; 677 as first built, before the review of #461's declines kept one corpus site on its committed call)

type regionTally struct {
	rows, withRegions, descs int
	// routed counts the dispatches lowered THROUGH their descriptor
	// (Program.Generics, OpDispatchGeneric) — the generic lane's executing
	// arm, measured beside the table it reads.
	routed                  int
	claimedSlots, spanSlots int
	nfwdZero, nfwdPartial   int
	nfwdWhole               int
	dupSites                int
	sources                 map[string]int
	bad                     []string
}

func sourceName(s compiler.SlotSource) string {
	switch s {
	case compiler.SlotNone:
		return "none"
	case compiler.SlotConst:
		return "const"
	case compiler.SlotLocal:
		return "local"
	case compiler.SlotEvent:
		return "event"
	case compiler.SlotGroup:
		return "group"
	case compiler.SlotType:
		return "type"
	case compiler.SlotWordRef:
		return "wordRef"
	}
	return fmt.Sprintf("OTHER:%d", s)
}

// tallyRow compiles one row and folds its region table into the tally.
func (tl *regionTally) tallyRow(src, where string) {
	a, err := lang.New()
	if err != nil {
		return
	}
	prog, _, _, _ := a.CompileCheck(src)
	if prog == nil {
		return
	}
	tl.rows++
	if len(prog.Regions) == 0 {
		return
	}
	tl.withRegions++
	tl.routed += len(prog.Generics)
	// A (word, position) appearing twice in ONE program's table is the shape a
	// recorder-side table produced when a discarded loop round left its
	// descriptors behind. The table is built at lowering now, so it shares the
	// event rollback; this counts what remains, which is the genuinely
	// repeated lowering site (a chained read that synthesises one position for
	// each step of the chain).
	seen := map[string]int{}
	for i := range prog.Regions {
		d := &prog.Regions[i]
		tl.descs++
		key := fmt.Sprintf("%s@%d:%d", d.Word, d.Pos.Row, d.Pos.Col)
		seen[key]++
		if seen[key] == 2 {
			tl.dupSites++
		}
		tl.claimedSlots += d.NFwd
		tl.spanSlots += len(d.Slots)
		switch {
		case d.NFwd == 0:
			tl.nfwdZero++
		case d.NFwd == len(d.Slots):
			tl.nfwdWhole++
		default:
			tl.nfwdPartial++
		}
		for k := 0; k < d.NFwd; k++ {
			tl.sources[sourceName(d.Slots[k].Source)]++
		}
		if verr := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); verr != nil {
			if len(tl.bad) < 15 {
				tl.bad = append(tl.bad, fmt.Sprintf("%s: %v", where, verr))
			}
		}
	}
}

// TestRegionTableWellFormed walks the corpus, compiles every row, and
// validates every descriptor the program carries.
func TestRegionTableWellFormed(t *testing.T) {
	tl := &regionTally{sources: map[string]int{}}

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatal(ferr)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			tl.tallyRow(strings.TrimSpace(parts[0]), fmt.Sprintf("%s:%d", e.Name(), lineNo))
		}
		_ = f.Close()
	}

	names := make([]string, 0, len(tl.sources))
	for k := range tl.sources {
		names = append(names, k)
	}
	sort.Strings(names)
	t.Logf("region table: %d compiled rows, %d carrying regions, %d descriptors, %d dispatches routed through one", tl.rows, tl.withRegions, tl.descs, tl.routed)
	if tl.routed < routedFloor {
		t.Errorf("only %d dispatches route through their descriptor (floor %d) — a seat stopped routing", tl.routed, routedFloor)
	}
	t.Logf("   slots: %d claimed of %d in span", tl.claimedSlots, tl.spanSlots)
	t.Logf("   claim: %d claimed nothing forward, %d a prefix, %d the whole span",
		tl.nfwdZero, tl.nfwdPartial, tl.nfwdWhole)
	t.Logf("   sites described more than once in one program: %d", tl.dupSites)
	for _, n := range names {
		t.Logf("   source %-8s %6d", n, tl.sources[n])
	}
	for _, b := range tl.bad {
		t.Logf("    %s", b)
	}

	if tl.rows == 0 || tl.descs == 0 {
		t.Fatal("no compiled row carried a region descriptor — the gate is measuring its own wiring")
	}
	// A floor, not a pin. The exact counts move with the corpus and pinning
	// them would only force regeneration churn; what must never happen is the
	// wiring going quietly dead — a seam that stops firing, a join key that
	// stops matching — and leaving a green test measuring nothing. The floor
	// is an order of magnitude below the live figure for that reason.
	const descFloor = 4000
	if tl.descs < descFloor {
		t.Errorf("only %d descriptors emitted (floor %d) — Phase A or the (word, pos) join "+
			"has stopped firing; find the seam, do not lower the floor", tl.descs, descFloor)
	}
	// The claim is a PREFIX of the span, never the other way round: a region
	// runs to the next hard delimiter while a dispatch claims what it took
	// forward. If these were equal the model would have collapsed into
	// "the region IS the claim", which is the reading NFwd exists to refuse.
	if tl.claimedSlots >= tl.spanSlots {
		t.Errorf("claimed %d of %d span slots — the claim can never cover the whole corpus span",
			tl.claimedSlots, tl.spanSlots)
	}
	// No claimed slot may be sourced from a compiled fragment: SlotGroup's
	// filler is the recorder change that stops evaluating groups eagerly, and
	// that has not landed. One appearing here means a fragment source arrived
	// without the conditional-evaluation order it exists to preserve.
	if n := tl.sources["group"]; n > 0 {
		t.Errorf("%d claimed slots carry SlotGroup — no compiled fragment exists to run", n)
	}
	// A claimed slot that kept the invalid zero is the failure the sentinel
	// exists to make visible: Phase B decided the slot WAS this dispatch's
	// operand and then gave it no source.
	if n := tl.sources["none"]; n > 0 {
		t.Errorf("%d claimed slots carry SlotNone — Phase B claimed a slot it did not source", n)
	}
	if len(tl.bad) > 0 {
		t.Errorf("%d malformed region descriptors (first %d shown) — fix the recorder, never the number",
			len(tl.bad), len(tl.bad))
	}
}
