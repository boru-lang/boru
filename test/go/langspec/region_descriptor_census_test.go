// Region descriptors, measured against real programs — and against an
// INDEPENDENT oracle (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// WHY THIS FILE WAS REWRITTEN, because the first version is the lesson. It
// walked each corpus row and reported 100%, and the number meant nothing:
//
//	TAUTOLOGY      it compared CaptureRegionSlots' length against
//	               `RegionEnd(w,at) - at`. CaptureRegionSlots takes its
//	               length FROM RegionSlotCount, which IS that expression, so
//	               both sides were the same call. Reversed or duplicated slot
//	               contents of the same length passed. The coverage sum was
//	               just the cursor advances RegionEnd itself had chosen.
//	WRONG INPUT    lang.Parse leaves a paren group as ONE ParenExpr value and
//	               executable bodies inside List values; the engine expands
//	               those into OpenParen…CloseParen markers and runs bodies on
//	               fresh tapes. Measured over the corpus: 0 of 7586 parsed
//	               rows contained a paren MARKER at the level being walked, so
//	               RegionEnd's paren-depth logic — depth++/depth--, and "a
//	               close paren at depth 0 ends the region", which is the whole
//	               subtlety of the function — never executed once. 89% of rows
//	               were a single region where the walk asserts nothing.
//
// Both were caught in review, both were right. The first version also HAD a
// per-slot content check, and it was deleted when it failed — the comparator
// was wrong (core.DeepEqual is not reflexive for an unevaluated
// ParenExprPayload), but deleting the only non-tautological assertion was the
// wrong repair. That is why the oracle below is written by hand.
//
// SO THE CORRECTNESS CLAIM LIVES IN TestRegionDescriptorOracle, whose expected
// boundaries are typed out rather than derived, and which fails if a boundary
// moves. The corpus walk that follows it is a REACH check — it says the walk
// terminates and covers a much richer input — and it no longer claims to
// validate boundaries, because it cannot.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// expandMarkers mirrors the engine's own expandParenExpr: a ParenExpr becomes
// the OpenParen…CloseParen span the tape actually carries, recursively. Without
// this the walk never sees a paren marker at all.
func expandMarkers(vals []core.Value) []core.Value {
	out := make([]core.Value, 0, len(vals))
	for _, v := range vals {
		items, err := core.AsParenExpr(v)
		if core.IsParenExpr(v) && err == nil {
			out = append(out, core.NewOpenParen())
			out = append(out, expandMarkers(items)...)
			out = append(out, core.NewCloseParen())
			continue
		}
		out = append(out, v)
	}
	return out
}

// nestedBodies collects the executable LIST bodies inside a token stream. The
// engine runs each on its own tape, so each is a separate region workload the
// outer walk would otherwise never visit.
func nestedBodies(vals []core.Value, depth int) [][]core.Value {
	if depth > 8 {
		return nil
	}
	var out [][]core.Value
	for _, v := range vals {
		if !core.IsConcrete(v) || !v.Parent.Equal(core.TList) {
			continue
		}
		lst, err := core.AsList(v)
		if err != nil || lst.IsNil() || lst.Len() == 0 {
			continue
		}
		body := lst.Slice()
		out = append(out, expandMarkers(body))
		out = append(out, nestedBodies(body, depth+1)...)
	}
	return out
}

// regionSpans returns the [start,end) span of every region in w, plus the
// indices of the hard delimiters between them. It is the walk under test; the
// oracle compares its output against hand-written expectations.
func regionSpans(w core.CollectWindow) (spans [][2]int, delims []int, ok bool) {
	n := w.Len()
	at, guard := 0, 0
	for at < n {
		guard++
		if guard > n+1 {
			return spans, delims, false
		}
		end := core.RegionEnd(w, at)
		if end < at || end > n {
			return spans, delims, false
		}
		if end == at {
			v := w.At(at)
			if !core.IsEnd(v) && !core.IsCloseParen(v) {
				return spans, delims, false
			}
			delims = append(delims, at)
			at++
			continue
		}
		spans = append(spans, [2]int{at, end})
		at = end
	}
	return spans, delims, true
}

// TestRegionDescriptorOracle pins region boundaries against expectations
// written out BY HAND. Nothing here is derived from RegionEnd, so a wrong
// boundary fails — which is precisely what the tautological version could not
// do. The cases are chosen to exercise the paren-depth logic the corpus walk
// never reached.
func TestRegionDescriptorOracle(t *testing.T) {
	t.Parallel()
	open, closep, end := core.NewOpenParen(), core.NewCloseParen(), core.NewEnd()
	w := func(s string) core.Value { return core.NewWord(s) }
	i := func(n int64) core.Value { return core.NewInteger(n) }

	cases := []struct {
		name       string
		toks       []core.Value
		from       int
		wantSpans  [][2]int
		wantDelims []int
	}{{
		name:      "a plain statement is one region",
		toks:      []core.Value{w("add"), i(1), i(2)},
		wantSpans: [][2]int{{0, 3}},
	}, {
		name:       "`end` is a hard delimiter and belongs to no region",
		toks:       []core.Value{w("def"), w("x"), i(1), end, w("x")},
		wantSpans:  [][2]int{{0, 3}, {4, 5}},
		wantDelims: []int{3},
	}, {
		name:      "a CONTAINED group does not end the region",
		toks:      []core.Value{open, w("add"), i(1), i(2), closep, i(5)},
		wantSpans: [][2]int{{0, 6}},
	}, {
		name:      "starting INSIDE a group, the close paren ends the region",
		toks:      []core.Value{open, w("add"), i(1), i(2), closep, i(5)},
		from:      1,
		wantSpans: [][2]int{{1, 4}},
	}, {
		name:      "`end` INSIDE a group does not end the OUTER region",
		toks:      []core.Value{open, w("do"), end, closep, i(9)},
		wantSpans: [][2]int{{0, 5}},
	}, {
		name:      "nested groups unwind to the right depth",
		toks:      []core.Value{open, open, i(1), closep, i(2), closep, i(3)},
		wantSpans: [][2]int{{0, 7}},
	}, {
		name:       "two statements either side of a delimiter, group in the first",
		toks:       []core.Value{open, i(1), closep, end, w("z")},
		wantSpans:  [][2]int{{0, 3}, {4, 5}},
		wantDelims: []int{3},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tape := core.NewTape(c.toks, 0)
			if c.from != 0 {
				// A walk that begins inside a group: assert the single extent
				// directly, since the partition helper always starts at 0.
				got := core.RegionEnd(tape, c.from)
				if got != c.wantSpans[0][1] {
					t.Errorf("RegionEnd(from=%d) = %d, want %d", c.from, got, c.wantSpans[0][1])
				}
				return
			}
			spans, delims, ok := regionSpans(tape)
			if !ok {
				t.Fatal("the walk did not terminate cleanly")
			}
			if !sameSpans(spans, c.wantSpans) {
				t.Errorf("spans = %v, want %v", spans, c.wantSpans)
			}
			if !sameInts(delims, c.wantDelims) {
				t.Errorf("delimiters = %v, want %v", delims, c.wantDelims)
			}
			// The slots a region captures must be ITS OWN tokens, in written
			// order. Compared by rendered identity rather than DeepEqual, which
			// is not reflexive for an unevaluated paren payload.
			for _, sp := range spans {
				slots := compiler.CaptureRegionSlots(tape, sp[0])
				for k := range slots {
					want := core.CanonValue(tape.At(sp[0] + k))
					if got := core.CanonValue(slots[k].Token); got != want {
						t.Errorf("region %v slot %d = %s, want %s", sp, k, got, want)
					}
				}
			}
		})
	}
}

// TestRegionDescriptorValidateRejects is the negative control: a descriptor
// that is malformed must be DECLINED, not quietly executed. Without this the
// suite only ever proves that well-formed input is accepted, which is the
// half that cannot catch a regression (AGENTS.md, test discipline).
func TestRegionDescriptorValidateRejects(t *testing.T) {
	t.Parallel()
	pos := core.SrcPos{Row: 1, Col: 1}
	// The invalid zero: a slot nobody gave a source to. NFwd 1 puts it INSIDE
	// the recorded claim, which is what makes the missing source a defect —
	// beyond the claim a slot is not the dispatch's operand and is owed none
	// (the relaxation is pinned just below, so this fixture is not silently
	// asserting the wrong half).
	d := &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		NFwd: 1, Slots: []compiler.SlotDesc{{}}}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("a slot left at SlotNone must be declined — it is the invalid zero, not Consts[0]")
	}
	// …and the relaxation's own boundary: unsourced BEYOND the claim is legal,
	// unsourced-but-indexed is a lowerer writing past its own claim, and the
	// claim bound may not point outside the slots.
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		NFwd: 0, Slots: []compiler.SlotDesc{{}}}
	if err := d.Validate(1, 0, 0); err != nil {
		t.Errorf("an unsourced slot beyond the claim is legal, got %v", err)
	}
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		NFwd: 0, Slots: []compiler.SlotDesc{{Idx: 3}}}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("an unsourced slot carrying an index must be declined even beyond the claim")
	}
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		NFwd: 2, Slots: []compiler.SlotDesc{{Source: compiler.SlotConst}}}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("a claim bound past the slot count must be declined")
	}
	// An index that is in the struct but out of the table it addresses.
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		Slots: []compiler.SlotDesc{{Source: compiler.SlotConst, Idx: 7}}}
	if err := d.Validate(1, 0, 0); err == nil {
		t.Error("a const index past the const table must be declined")
	}
	// A word lead with no name, and a name on a non-word lead: both malformed.
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Pos: pos}
	if err := d.Validate(0, 0, 0); err == nil {
		t.Error("LeadWord with no word name must be declined")
	}
	d = &compiler.RegionDesc{Lead: compiler.LeadApply, Word: "add", Pos: pos}
	if err := d.Validate(0, 0, 0); err == nil {
		t.Error("a word name on a non-word lead must be declined")
	}
	// The well-formed case still passes, so the rejections above are not
	// vacuous — a Validate that declined everything would satisfy them all.
	d = &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "add", Pos: pos,
		Slots: []compiler.SlotDesc{{Source: compiler.SlotConst, Idx: 0}}}
	if err := d.Validate(1, 0, 0); err != nil {
		t.Errorf("a well-formed descriptor must pass: %v", err)
	}
}

func sameSpans(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRegionDescriptorReach walks every corpus program — the EXPANDED token
// stream, plus each nested executable body on its own tape, as the engine runs
// them — and reports how far the region walk reaches.
//
// Read it as REACH, not as correctness. What it can fail on is a walk that
// spins, runs past its window, or stops on a token that is not a delimiter.
// What it deliberately does NOT claim is that the boundaries are right: the
// only available cross-check for that would be RegionEnd against itself, which
// is the tautology this file exists to have stopped making. Boundaries are
// pinned by the oracle above.
func TestRegionDescriptorReach(t *testing.T) {
	t.Parallel()
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatal(err)
	}
	var rows, parsed, tapes, clean, withParen, multiRegion int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			rows++
			vals, err := lang.Parse(strings.TrimSpace(parts[0]))
			if err != nil {
				continue // a parse-error row is the parser's business
			}
			parsed++
			streams := append([][]core.Value{expandMarkers(vals)}, nestedBodies(vals, 0)...)
			for _, s := range streams {
				if len(s) == 0 {
					continue
				}
				tapes++
				for _, v := range s {
					if core.IsOpenParen(v) {
						withParen++
						break
					}
				}
				tape := core.NewTape(s, 0)
				spans, delims, ok := regionSpans(tape)
				if !ok {
					t.Errorf("%s: the region walk did not terminate cleanly on %q", e.Name(), strings.TrimSpace(parts[0]))
					continue
				}
				if len(spans)+len(delims) > 1 {
					multiRegion++
				}
				covered := len(delims)
				for _, sp := range spans {
					covered += sp[1] - sp[0]
				}
				if covered != tape.Len() {
					t.Errorf("%s: partition covers %d of %d tokens on %q",
						e.Name(), covered, tape.Len(), strings.TrimSpace(parts[0]))
					continue
				}
				clean++
			}
		}
		f.Close()
	}
	t.Logf("region-descriptor reach: %d corpus rows, %d parsed, %d tapes walked "+
		"(outer + nested bodies), %d clean", rows, parsed, tapes, clean)
	t.Logf("   tapes containing a paren MARKER: %d", withParen)
	t.Logf("   tapes splitting into >1 region : %d", multiRegion)
	if withParen == 0 {
		t.Error("no tape carried a paren marker — the walk is not reaching the nesting " +
			"it claims to cover, which is exactly how the first version of this census " +
			"reported 100% while never running RegionEnd's paren-depth logic")
	}
}

// slotKind names what a captured region slot's TOKEN is, syntactically. It is
// deliberately a property of the token alone: no binding is consulted, nothing
// is evaluated, and the classification is therefore stable across executions —
// which is the whole question TestRegionSlotTokenKinds exists to answer.
func slotKind(v core.Value) string {
	switch {
	case core.IsOpenParen(v) || core.IsCloseParen(v) || core.IsParenExpr(v):
		return "group"
	case core.IsDispatchMod(v):
		return "mod"
	case core.IsAtom(v):
		return "atom"
	case core.IsBareTypeNode(v):
		return "type"
	case core.IsWord(v):
		return "word"
	case core.BearsActiveTokens(v):
		// A compound literal whose MEMBERS are active tokens is not its own
		// value: `[true true true]` is a list of three WORDS until they run,
		// and an interpolated string is a template until its holes do. This
		// bucket was folded into `const` in this census's first version, and
		// the live probe that caught it is recorded in §6.2 — a token that
		// merely LOOKS concrete is the same frozen-class mistake one level
		// down, inside the literal.
		return "active"
	case core.IsConcrete(v):
		return "const"
	}
	if v.Parent == nil {
		return "OTHER:<no parent>"
	}
	return "OTHER:" + core.CanonValue(*v.Parent)
}

// TestRegionSlotTokenKinds censuses what region slots ARE, and it is here to
// settle a design question rather than to guard a behaviour.
//
// WHAT IT WALKS, stated precisely because the previous censuses in this file
// were wrong about exactly this. Production captures from at+1, where `at` is
// the DISPATCHING WORD — a fact that needs the live binding set to find. This
// walk has no binding set, so it captures from the region START and reports
// the LEAD token separately. Every token of every region is therefore visited
// (which is what the kind-coverage assertion needs), and the reader can
// subtract the leads to get production's window (which is what the
// percentages describe). Neither number is quietly standing in for the other.
//
// THE QUESTION. A slot is a WRITTEN-ORDER token; a dispatch's operands are
// SIG-ORDER args. Filling SlotDesc.Source by matching one against the other
// needs a join, and three attempts at that join have now been built and
// reverted (task #15). This census asks whether the join is needed at all —
// i.e. for how many slots the source is derivable from the TOKEN, with no
// reference to the operand model:
//
//	const / atom  -> intern the token; it IS the value
//	type          -> intern the canonical registry node
//	word          -> LIVE derivation, which the descriptor model already
//	                 mandates (a word's class is contextual, region_desc.go)
//	group         -> a compiled fragment, run conditionally (SlotGroup)
//	active        -> a compound literal whose MEMBERS are active tokens, so
//	                 it is a fragment in disguise rather than a const
//	mod           -> read as the slot's Quote, already captured
//
// None of those five is an operand lookup. If they account for the corpus,
// the join is not a hard problem the next attempt must solve — it is a
// problem the descriptor model does not have.
//
// THE CONSISTENCY CHECK, built in rather than bolted on (the lesson from two
// measurements that were wrong in ways their headline number could not show):
// the buckets must SUM to the slot total, and an unclassified token must name
// its own type rather than land in a silent remainder. A residue that hides
// is exactly how the previous numbers passed.
func TestRegionSlotTokenKinds(t *testing.T) {
	t.Parallel()
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := specEntries(specDir)
	if err != nil {
		t.Fatal(err)
	}
	kinds, leads := map[string]int{}, map[string]int{}
	var regions, slots, regionsWithGroup, regionsAllSimple, slotsAllSimple int
	var regionsCompletable, slotsCompletable int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(specDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			vals, err := lang.Parse(strings.TrimSpace(parts[0]))
			if err != nil {
				continue
			}
			streams := append([][]core.Value{expandMarkers(vals)}, nestedBodies(vals, 0)...)
			for _, s := range streams {
				if len(s) == 0 {
					continue
				}
				tape := core.NewTape(s, 0)
				spans, _, ok := regionSpans(tape)
				if !ok {
					continue
				}
				for _, sp := range spans {
					got := compiler.CaptureRegionSlots(tape, sp[0])
					if got == nil {
						continue
					}
					regions++
					slots += len(got)
					leads[slotKind(got[0].Token)]++
					hasGroup, allSimple, completable := false, len(got) > 1, len(got) > 1
					for k := range got {
						kind := slotKind(got[k].Token)
						kinds[kind]++
						if k == 0 {
							continue
						}
						if kind == "group" {
							hasGroup = true
						}
						// SIMPLE means the slot's value IS its own token, so
						// the source needs nothing but interning: no live
						// binding lookup, no compiled fragment, no marker to
						// consume. That is the subset a first OpCollect can
						// execute end to end.
						if kind != "const" && kind != "atom" && kind != "type" {
							allSimple = false
						}
						// COMPLETABLE is the wider claim SlotWordRef opens up:
						// every slot has a source the model can express today.
						// A word joins the simple kinds here because its source
						// is "resolve it live", which needs no table and no
						// lowering.
						//
						// Three kinds fall outside. A group has no compiled
						// fragment yet. A mod is not an operand at all — the
						// collection walk consumes it. And an ACTIVE compound
						// is a fragment in disguise: `[true true true]` is a
						// list of three WORDS whose members are evaluated, so
						// freezing it as a const is the frozen-class mistake
						// one level down, inside the literal.
						if kind == "group" || kind == "mod" || kind == "active" {
							completable = false
						}
					}
					if hasGroup {
						regionsWithGroup++
					}
					if allSimple {
						regionsAllSimple++
						slotsAllSimple += len(got) - 1
					}
					if completable {
						regionsCompletable++
						slotsCompletable += len(got) - 1
					}
				}
			}
		}
		f.Close()
	}
	if regions == 0 {
		t.Fatal("no region captured a slot — the census has no subject")
	}
	sum := 0
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
		sum += kinds[k]
	}
	sort.Strings(names)
	t.Logf("region tokens: %d regions, %d tokens (leads included)", regions, slots)
	for _, n := range names {
		t.Logf("   %-10s %7d  (%.1f%%)   of which LEAD: %d",
			n, kinds[n], 100*float64(kinds[n])/float64(slots), leads[n])
	}
	t.Logf("   regions with a group slot after the lead: %d (%.1f%%)",
		regionsWithGroup, 100*float64(regionsWithGroup)/float64(regions))
	t.Logf("   regions ENTIRELY simple (every slot after the lead is its own value): "+
		"%d (%.1f%%), %d slots", regionsAllSimple,
		100*float64(regionsAllSimple)/float64(regions), slotsAllSimple)
	t.Logf("   regions COMPLETABLE (adding wordRef: no group, mod or active slot): "+
		"%d (%.1f%%), %d slots", regionsCompletable,
		100*float64(regionsCompletable)/float64(regions), slotsCompletable)
	// The buckets must account for every slot. A census whose parts do not
	// sum to its whole is measuring something other than what it reports.
	if sum != slots {
		t.Errorf("buckets sum to %d but %d slots were captured", sum, slots)
	}
	// Nothing may hide in an unnamed remainder: an OTHER bucket names the
	// token's own type, so a class the model has not considered surfaces as
	// a failure here rather than as a wrong Source later.
	for _, n := range names {
		if strings.HasPrefix(n, "OTHER:") {
			t.Errorf("%d slots carry an unclassified token kind %s — the descriptor "+
				"model has no source for it", kinds[n], n)
		}
	}
}
