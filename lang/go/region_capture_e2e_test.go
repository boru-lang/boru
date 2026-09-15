package lang

import (
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// TestRegionCaptureFiresOnRealPrograms is the end-to-end pin for Stage 4's
// region capture. The unit tests in compiler/go drive tryRecordRegion and
// completeRegion directly over hand-built windows, which proves the functions
// but not the WIRING — that core's RegionRecorder seam is installed, fires
// from the real dispatch path, reaches the live EmitState, is CLAIMED by the
// dispatch that walked it, and rides out on the Program.
//
// It lives in lang because that is the only layer where all three exist at
// once: the interpreter that fires the seam, the compiler that seats on it,
// and a registry a program actually ran on.
//
// The subject is the finished table rather than the pending map, and that is
// the wiring change Phase B made: a capture is an OFFER, claimed at
// RecordCall and gone from the map afterwards, so an assertion on what is
// still pending would now be asserting that the join FAILED.
func TestRegionCaptureFiresOnRealPrograms(t *testing.T) {
	compile := func(t *testing.T, src string) *compiler.Program {
		t.Helper()
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, _, _, cerr := b.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("compile %q: %v", src, cerr)
		}
		if prog == nil {
			t.Fatalf("%q did not compile — the pin needs a Program to read", src)
		}
		return prog
	}

	t.Run("a forward-collecting dispatch is captured and claimed", func(t *testing.T) {
		prog := compile(t, `add 1 2`)
		if len(prog.Regions) == 0 {
			t.Fatal("`add 1 2` forward-collects, so it must produce a region descriptor")
		}
		var d *compiler.RegionDesc
		for i := range prog.Regions {
			if prog.Regions[i].Word == "add" {
				d = &prog.Regions[i]
				break
			}
		}
		if d == nil {
			t.Fatal("no descriptor for `add` — the (word, pos) join Phase B looks up by")
		}
		if d.Lead != compiler.LeadWord {
			t.Errorf("lead = %v, want LeadWord", d.Lead)
		}
		if len(d.Slots) != 2 {
			t.Fatalf("captured %d slots for `add 1 2`, want 2", len(d.Slots))
		}
		// Both operands were written forward, so the claim covers the span.
		if d.NFwd != 2 {
			t.Errorf("NFwd = %d, want 2 — both slots were the dispatch's operands", d.NFwd)
		}
		for i := 0; i < d.NFwd; i++ {
			if d.Slots[i].Source != compiler.SlotConst {
				t.Errorf("slot %d source = %v, want SlotConst", i, d.Slots[i].Source)
			}
		}
		if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
			t.Errorf("the emitted descriptor must validate against the program: %v", err)
		}
	})

	// A fn body's own dispatch is described too, and its word slots are the
	// case the model would get WRONG if it kept them live. Inside the body the
	// analysis binds each param into the def stack, so `a` and `b` resolve
	// during the pass — but the emitted body reads them from the FRAME, and at
	// run time the def stack holds no such binding. The completion takes the
	// operand's frame slot instead, which is the design's "a word slot that
	// resolves to a param lowers as SlotLocal rather than SlotWordRef".
	t.Run("a fn body's param slots describe the frame, not the def stack", func(t *testing.T) {
		prog := compile(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
		var d *compiler.RegionDesc
		for i := range prog.Regions {
			if prog.Regions[i].Word == "add" {
				d = &prog.Regions[i]
				break
			}
		}
		if d == nil {
			t.Fatal("the body's `add a b` must produce a descriptor")
		}
		if d.NFwd != 2 {
			t.Fatalf("NFwd = %d, want 2", d.NFwd)
		}
		for i, want := range []int{0, 1} {
			if d.Slots[i].Source != compiler.SlotLocal || d.Slots[i].Idx != want {
				t.Errorf("slot %d = %v/%d, want SlotLocal/%d — a param lives in the frame",
					i, d.Slots[i].Source, d.Slots[i].Idx, want)
			}
		}
		for i := range prog.Regions {
			if err := prog.Regions[i].Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
				t.Errorf("descriptor %d does not validate: %v", i, err)
			}
		}
	})

	// The seat is RecordCall's AND RecordUserCall's (the user-call family
	// joined 2026-09-14, the first slice of the generic lane's line), and
	// since the sixty-first increment the two POLY families' as well
	// (RecordUserPolyCall and RecordPolyCall, pinned below). A user fn call
	// is offered a capture exactly as a native dispatch is — Phase A fires in
	// resolveForwardArgs for every forward-collecting word — and now claims
	// it: `f 1 2` carries its own descriptor, keyed by the WORD's position
	// (CheckState.CurCallWord/CurCallPos, read at the ReturnsFn's entry),
	// not by args[0]'s, which is the event's blame position and would miss
	// every offer. The dyn-apply and dyn-method families still have their
	// own entry points and claim nothing — that is the stated bound now, and
	// this pin fails if a seat lands without updating it.
	t.Run("a user-fn call claims its capture", func(t *testing.T) {
		prog := compile(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
		d := findRegion(prog, "f")
		if d == nil {
			t.Fatal("`f 1 2` forward-collects, so the user-fn call must claim its region descriptor")
		}
		if d.Lead != compiler.LeadWord || d.Word != "f" {
			t.Errorf("lead = %v/%q, want LeadWord/f", d.Lead, d.Word)
		}
		if len(d.Slots) != 2 || d.NFwd != 2 {
			t.Fatalf("slots %d, NFwd %d — want 2 and 2: both operands were written forward", len(d.Slots), d.NFwd)
		}
		for i := 0; i < d.NFwd; i++ {
			if d.Slots[i].Source != compiler.SlotConst {
				t.Errorf("slot %d source = %v, want SlotConst", i, d.Slots[i].Source)
			}
		}
		if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
			t.Errorf("the user call's descriptor must validate against the program: %v", err)
		}
	})

	// The claim is keyed by the dispatching word token AS DISPATCHED, name
	// and position. A namespaced call `M.m 5` dispatches the member word `m`
	// at the `M.m` token's own position (column 81 here), so the offer and the
	// claim meet there — and NOT at args[0]'s position (column 85), which is
	// what the event's blame position carries and what a claim keyed by it
	// would have missed. Both facts are pinned: the name and the column.
	t.Run("a namespaced user-fn call claims its capture at the dispatching token", func(t *testing.T) {
		src := `import module [def m fn [[n:Integer][Integer][n 1 add]] export "M" {m:m/v}] end M.m 5`
		prog := compile(t, src)
		d := findRegion(prog, "m")
		if d == nil {
			t.Fatal("`M.m 5` must claim its region under the dispatched member word `m`")
		}
		if want := strings.Index(src, "M.m 5") + 1; d.Pos.Col != want {
			t.Errorf("descriptor at column %d, want %d — the claim must be keyed by the WORD token's position, not the first argument's", d.Pos.Col, want)
		}
		if d.NFwd != 1 || d.Slots[0].Source != compiler.SlotConst {
			t.Errorf("NFwd %d, slot 0 %v — want 1 and SlotConst", d.NFwd, d.Slots[0].Source)
		}
	})

	// A user fn whose operands come from the VALUE STACK claims nothing
	// forward — the same NFwd 0 a stack-fed native dispatch records — and a
	// call with no capture at all (a stack-only dispatch never reaches
	// forward collection) records its event and no descriptor.
	t.Run("a stack-fed user-fn call claims nothing forward", func(t *testing.T) {
		prog := compile(t, `def f fn [[a:Integer b:Integer][Integer][add a b]] end 1 2 f`)
		if d := findRegion(prog, "f"); d != nil && d.NFwd != 0 {
			t.Errorf("NFwd = %d, want 0 — `1 2 f` filled every position from the stack", d.NFwd)
		}
	})

	// The offer pool is keyed by (word, row, col) and not by source — SrcPos
	// carries the token's TEXT, not a file — so a module's recursive `f 0`
	// at 2:1 of ITS source and the main program's `Ns.f 1` at 2:1 of the
	// main source share one key. The user call's record runs AFTER the
	// callee's body is analysed, and that analysis dispatches the inner
	// `f 0`, whose capture used to overwrite the outer's offer and whose
	// record consumed it: the outer call ended with no descriptor, the miss
	// that looks like "no region here". The ReturnsFn now HOLDS its offer at
	// entry (HoldRegion), so both calls are described — the outer by the
	// token it walked (`1`), the inner by its own (`0`).
	t.Run("a user-fn call keeps its offer through a same-position dispatch in another source", func(t *testing.T) {
		lib := "def f fn [[n:Integer][Integer][if (n lte 0) [0] [\nf 0]]]\nexport \"Ns\" { f: f/v }"
		src := "import \"/lib.boru\"\nNs.f 1"
		mem := capabilities.NewMem()
		mem.Files["/lib.boru"] = []byte(lib)
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		b.SetFileOps(mem)
		prog, reason, _, cerr := b.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("the two-source program must compile: reason=%q err=%v", reason, cerr)
		}
		var tokens []string
		for i := range prog.Regions {
			d := &prog.Regions[i]
			if d.Word != "f" || d.Pos.Row != 2 || d.Pos.Col != 1 {
				continue
			}
			if len(d.Slots) != 1 || d.NFwd != 1 || d.Slots[0].Source != compiler.SlotConst {
				t.Errorf("descriptor for f at 2:1: slots %d NFwd %d source %v, want 1/1/SlotConst", len(d.Slots), d.NFwd, d.Slots[0].Source)
			}
			tokens = append(tokens, fmt.Sprint(d.Slots[0].Token))
		}
		if len(tokens) != 2 || (tokens[0] != "1" && tokens[1] != "1") || (tokens[0] != "0" && tokens[1] != "0") {
			t.Fatalf("both calls at 2:1 must be described, the outer by its `1` and the inner by its `0`; got tokens %v", tokens)
		}
	})

	// The recovered user call — a single-overload fn dispatched over an
	// operand the checker could not match statically (`y` is Integer|String
	// from the two `if` arms) reaches its ReturnsFn through the recovery
	// hook (TryRecordRecoveredUserFn) rather than declaredReturnCarriers,
	// which is where the word cursor is published. checkModeAssumeSig now
	// publishes it at entry, so the recovered `h y` claims its region under
	// `h` at the word's column: one slot, the module-scope `y`, kept live.
	// Without the publish the ReturnsFn read the PREVIOUS dispatch's cursor
	// and the claim missed (measured: the cursor still named `gt`).
	t.Run("a recovered user-fn call claims its capture", func(t *testing.T) {
		src := `def h fn [[a:Integer] [Integer] [a]] def y (if (1 gt 0) [1] ['s']) h y`
		prog := compile(t, src)
		if !strings.Contains(prog.Disassemble(), "CALL_USER") {
			t.Fatalf("the pin needs the recovered guarded CALL_USER:\n%s", prog.Disassemble())
		}
		d := findRegion(prog, "h")
		if d == nil {
			t.Fatal("the recovered `h y` must claim its region — the recovery publishes the word cursor before invoking the ReturnsFn")
		}
		if want := strings.Index(src, "h y") + 1; d.Pos.Col != want {
			t.Errorf("descriptor at column %d, want %d — the `h` token's own position", d.Pos.Col, want)
		}
		if len(d.Slots) != 1 || d.NFwd != 1 || d.Slots[0].Source != compiler.SlotWordRef {
			t.Errorf("slots %d, NFwd %d, slot 0 %v — want 1, 1 and SlotWordRef (the live module-scope y)", len(d.Slots), d.NFwd, d.Slots[0].Source)
		}
	})
}

// TestRegionTableIsInLoweringOrder pins where an index comes from. The table
// is appended in lowerCall, so a descriptor's index is its position in the
// LOWERED code — the same discipline Dispatches follows, and the reason
// neither needs a rollback of its own. An index assigned at record time would
// shift under a discarded loop-analysis round; one assigned here cannot,
// because a round that is not lowered contributes nothing.
//
// This matters before OpCollect exists, not after: the op's Arg will be that
// index, and an off-by-one there is a descriptor describing another dispatch.
func TestRegionTableIsInLoweringOrder(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := b.CompileCheck(`add 1 2  sub 4 3  mul 5 6`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	var got []string
	for i := range prog.Regions {
		got = append(got, prog.Regions[i].Word)
	}
	want := []string{"add", "sub", "mul"}
	if len(got) != len(want) {
		t.Fatalf("descriptors %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descriptors %v, want %v — the table is not in lowering order", got, want)
		}
	}
}

// findRegion returns the descriptor for word, or nil.
// TestRegionCapturePolySeats is the e2e pin for the two POLY seats (the
// sixty-first increment): a runtime-re-matched user call and a
// runtime-re-matched native call each claim the capture Phase A offered for
// their dispatch. A separate test from the one above for gocyclo's sake only;
// the subject and the helpers are the same.
func TestRegionCapturePolySeats(t *testing.T) {
	// The POLY user-call seat. `g 7 (id 5)` cannot commit to one overload
	// (the paren result is Any) and lowers to CALL_USER_POLY; the record is
	// RecordUserPolyCall's, whose event pos is args[0]'s exactly as the mono
	// record's is, so the claim is keyed by the (callWord, wordPos) pair the
	// check pass published. What it claims is the prefix rule at work: slot 0
	// is the written `7`, a const operand; slot 1 is the paren's OPEN token,
	// and the dispatch's second operand is the paren's RESULT, not that
	// token, so the claim stops there — NFwd 1 over five raw slots.
	t.Run("a poly user-fn call claims its capture", func(t *testing.T) {
		src := `def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer b:Integer] [Integer] [1] [a:Integer b:String] [Integer] [2]] g 7 (id 5)`
		prog := compileRegionProgram(t, src)
		if !strings.Contains(prog.Disassemble(), "CALL_USER_POLY") {
			t.Fatalf("the pin needs a runtime-re-matched user call:\n%s", prog.Disassemble())
		}
		d := findRegion(prog, "g")
		if d == nil {
			t.Fatal("`g 7 (id 5)` forward-collects, so the poly user call must claim its region descriptor")
		}
		if want := strings.Index(src, "g 7") + 1; d.Pos.Col != want {
			t.Errorf("descriptor at column %d, want %d — keyed by the WORD token's position, not args[0]'s", d.Pos.Col, want)
		}
		if len(d.Slots) != 5 || d.NFwd != 1 || d.Slots[0].Source != compiler.SlotConst {
			t.Errorf("slots %d, NFwd %d, slot 0 %v — want 5, 1 and SlotConst", len(d.Slots), d.NFwd, d.Slots[0].Source)
		}
		if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
			t.Errorf("the poly user call's descriptor must validate against the program: %v", err)
		}
	})

	// The same seat over the generic lane's own shape: a module-scope name
	// read forward at a poly user call. `k` stays a LIVE word reference in
	// the descriptor (region_desc.go's `k` pair), which is exactly what
	// OpCollect exists to re-derive; the paren token stops the claim as above.
	t.Run("a poly user-fn call keeps a module-scope read live", func(t *testing.T) {
		prog := compileRegionProgram(t, `def id fn [[x:Any] [Any] [x]] def g fn [[a:Integer b:Integer] [Integer] [1] [a:Integer b:String] [Integer] [2]] def k 7 g k (id 5)`)
		d := findRegion(prog, "g")
		if d == nil {
			t.Fatal("`g k (id 5)` must claim its region under the poly user-call seat")
		}
		if len(d.Slots) != 2 || d.NFwd != 1 || d.Slots[0].Source != compiler.SlotWordRef {
			t.Errorf("slots %d, NFwd %d, slot 0 %v — want 2, 1 and SlotWordRef (a live module-scope read)", len(d.Slots), d.NFwd, d.Slots[0].Source)
		}
	})

	// The POLY native seat. `is y Integer` straddles two `is` overloads (`y`
	// is Integer|String from the two `if` arms) and lowers to
	// CALL_NATIVE_POLY; RecordPolyCall's pos is the word's at every call
	// site, so the mono seat's key serves and the descriptor rides emitCall.
	// Slot 0 is `y`, a live module-scope read; slot 1 is the type name
	// `Integer`, a word slot whose binding is not on the def stack (a builtin
	// type name is stepped to a literal, not bound), so the word-slot
	// comparison cannot resolve it and the claim stops — NFwd 1, the
	// under-claim the prefix rule is designed to fall to.
	t.Run("a poly native call claims its capture", func(t *testing.T) {
		src := `def y (if (1 gt 0) [1] ['s']) is y Integer`
		prog := compileRegionProgram(t, src)
		if !strings.Contains(prog.Disassemble(), "CALL_NATIVE_POLY") {
			t.Fatalf("the pin needs a runtime-re-matched native call:\n%s", prog.Disassemble())
		}
		d := findRegion(prog, "is")
		if d == nil {
			t.Fatal("`is y Integer` forward-collects, so the poly native call must claim its region descriptor")
		}
		if want := strings.Index(src, "is y") + 1; d.Pos.Col != want {
			t.Errorf("descriptor at column %d, want %d", d.Pos.Col, want)
		}
		if len(d.Slots) != 2 || d.NFwd != 1 || d.Slots[0].Source != compiler.SlotWordRef {
			t.Errorf("slots %d, NFwd %d, slot 0 %v — want 2, 1 and SlotWordRef", len(d.Slots), d.NFwd, d.Slots[0].Source)
		}
	})

	// A poly native call fed from the stack claims nothing forward: `y is
	// Integer` takes `y` from the value stack and only the type name is
	// written forward, which the word-slot comparison declines as above.
	t.Run("a stack-fed poly native call claims nothing forward", func(t *testing.T) {
		prog := compileRegionProgram(t, `def y (if (1 gt 0) [1] ['s']) y is Integer`)
		if d := findRegion(prog, "is"); d == nil || d.NFwd != 0 {
			t.Errorf("descriptor %v — want one with NFwd 0", d)
		}
	})

	// A held offer belongs to its HOLDER. The poly user call `Lib.min 1
	// (id 5)` at 2:1 of the main source holds its offer across its arms'
	// compilation; the Integer arm's body dispatches the NATIVE
	// `MathUtil.min a b` at 2:1 of the module source, the same
	// (word, row, col). That native record completes from the pool, where
	// its own offer is, and never the outer call's (the review finding on
	// #457, where it took the outer capture and left the poly call with
	// nothing): two descriptors for `min` at 2:1, the outer's over the
	// written `1` and the inner's over the frame locals `a b`.
	t.Run("a nested native record cannot take a poly user call's held offer", func(t *testing.T) {
		lib := "import \"boru:math-util\" end def min fn [[a:Integer b:Integer][Integer][\nMathUtil.min a b] [a:Integer b:String][Integer][a]]\nexport \"Lib\" { min: min/v }"
		src := "import \"/lib.boru\" end def id fn [[x:Any][Any][x]]\nLib.min 1 (id 5)"
		mem := capabilities.NewMem()
		mem.Files["/lib.boru"] = []byte(lib)
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		b.SetFileOps(mem)
		prog, reason, _, cerr := b.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("the two-source program must compile: reason=%q err=%v", reason, cerr)
		}
		if !strings.Contains(prog.Disassemble(), "CALL_USER_POLY") {
			t.Fatalf("the pin needs the outer call to be a poly user call:\n%s", prog.Disassemble())
		}
		var outer, inner int
		for i := range prog.Regions {
			d := &prog.Regions[i]
			if d.Word != "min" || d.Pos.Row != 2 || d.Pos.Col != 1 {
				continue
			}
			switch {
			case d.NFwd == 1 && d.Slots[0].Source == compiler.SlotConst:
				outer++
			case d.NFwd == 2 && d.Slots[0].Source == compiler.SlotLocal && d.Slots[1].Source == compiler.SlotLocal:
				inner++
			default:
				t.Errorf("a descriptor for min at 2:1 with neither call's shape: NFwd %d %+v", d.NFwd, d.Slots)
			}
		}
		if outer != 1 || inner != 1 {
			t.Fatalf("want the outer poly call's descriptor (1) and the inner native's (1), got %d and %d", outer, inner)
		}
	})
}

// compileRegionProgram compiles src on a fresh instance and fails the test
// unless a Program came back — a region pin needs a Program to read.
func compileRegionProgram(t *testing.T, src string) *compiler.Program {
	t.Helper()
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := b.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("compile %q: %v", src, cerr)
	}
	if prog == nil {
		t.Fatalf("%q did not compile — the pin needs a Program to read", src)
	}
	return prog
}

func findRegion(prog *compiler.Program, word string) *compiler.RegionDesc {
	for i := range prog.Regions {
		if prog.Regions[i].Word == word {
			return &prog.Regions[i]
		}
	}
	return nil
}

// findRegionLeading returns the descriptor for word whose FIRST slot is the
// named word token. One source can hold several dispatches of the same word —
// `def x (1 add 2) … add x 2` has two — so picking the first by name alone
// selects the wrong one, and the assertion then reads as a pass or a failure
// about a dispatch the test never meant.
func findRegionLeading(prog *compiler.Program, word, tok string) *compiler.RegionDesc {
	for i := range prog.Regions {
		d := &prog.Regions[i]
		if d.Word != word || len(d.Slots) == 0 {
			continue
		}
		if wi, err := core.AsWord(d.Slots[0].Token); err == nil && wi.Name == tok {
			return d
		}
	}
	return nil
}

// A MODULE-scope name read from inside a fn body must stay LIVE, and a
// body-local one of the SAME NAME at the SAME POSITION must not. This pair is
// the whole discriminator: what decides a word slot is not where the dispatch
// sits but where the binding lives, which is the closure-capture rule
// (Registry.FnBaselines) asked of a descriptor instead of a capture.
//
// It matters because the live half is the shape OpCollect exists for. A rule
// that stopped every word slot inside a fn unit would pass the negative half
// alone and quietly delete the model's whole point.
func TestModuleScopeWordStaysLiveInsideAFnBody(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := b.CompileCheck(`def k 5 end def f fn [[][Integer][add k 2]] end f`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	d := findRegionLeading(prog, "add", "k")
	if d == nil {
		t.Fatal("no descriptor for `add k 2`")
	}
	if d.NFwd != 2 {
		t.Errorf("NFwd = %d, want 2 — a module binding read inside a fn body is still "+
			"reachable by the live lookup the VM performs", d.NFwd)
	}
	if d.Slots[0].Source != compiler.SlotWordRef {
		t.Errorf("slot 0 = %v, want SlotWordRef", d.Slots[0].Source)
	}

	// The same name, shadowed by a body-local def: the binding now lives in
	// the body, so the claim stops.
	prog, _, _, cerr = b.CompileCheck(`def k 5 end def f fn [[][Integer][def k 9 end add k 2]] end f`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	d = findRegionLeading(prog, "add", "k")
	if d == nil {
		t.Fatal("no descriptor for the shadowed `add k 2`")
	}
	if d.NFwd != 0 {
		t.Errorf("NFwd = %d, want 0 — the body-local shadow is not what a live lookup "+
			"would find where this body runs", d.NFwd)
	}
}

// A fn-body's own `def` is describable by NOTHING the descriptor model has.
// The name exists in neither the runtime def stack (so SlotWordRef would
// resolve to an unrelated outer binding, or to nothing) nor in a frame slot
// fixed at record time — a computed one is promoted to a local only AFTER
// completion, so any source taken here would be stale. The claim stops.
//
// This is the param rule's boundary, and the pair below is the point: a PARAM
// keeps its claim as SlotLocal because its frame slot is settled, while a
// body-local of either kind does not. Both halves are asserted, because a fix
// that stopped the claim for every word inside a fn unit would pass the
// negative half alone while silently discarding 5807 corpus slots.
func TestFnBodyLocalDefStopsTheClaim(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{
		`def f fn [[][Integer][def x 1 end add x 2]] end f`,
		`def f fn [[][Integer][def x (1 add 2) end add x 2]] end f`,
	} {
		prog, _, _, cerr := b.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("compile %q: %v", src, cerr)
		}
		d := findRegionLeading(prog, "add", "x")
		if d == nil {
			t.Fatalf("%q: no descriptor for the `add x 2` dispatch", src)
		}
		if d.NFwd != 0 {
			t.Errorf("%q: NFwd = %d, want 0 — a fn-body local is describable by neither "+
				"a live lookup nor a settled frame slot", src, d.NFwd)
		}
		if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}

	// The positive half: a PARAM still claims, as a frame slot.
	prog, _, _, cerr := b.CompileCheck(`def f fn [[a:Integer b:Integer][Integer][add a b]] end f 1 2`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	d := findRegion(prog, "add")
	if d == nil || d.NFwd != 2 {
		t.Fatalf("a param's claim must survive; got %+v", d)
	}
	for i := range []int{0, 1} {
		if d.Slots[i].Source != compiler.SlotLocal {
			t.Errorf("slot %d = %v, want SlotLocal", i, d.Slots[i].Source)
		}
	}
}

// A word slot resolves against the registry the DISPATCH collected in, not
// against whichever registry the recorder last bound. After a call into a
// boru-implemented module, es.reg points at that module's sub-registry; a
// later main-registry dispatch looked up there finds nothing, and the claim
// stops short of operands that did come forward.
//
// The visible symptom is an under-claim rather than a wrong answer, which is
// exactly why it needs a test: it would never have surfaced as a failure.
func TestWordSlotUsesTheDispatchRegistry(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, cerr := b.CompileCheck(
		`import module [def m fn [[n:Integer][Integer][n 1 add]] export "M" {m:m/v}] end ` +
			`def x 1 end M.m 5 end add x 2`)
	if cerr != nil || prog == nil {
		t.Fatalf("compile: %v", cerr)
	}
	d := findRegionLeading(prog, "add", "x")
	if d == nil {
		t.Fatal("no descriptor for the main-registry `add x 2`")
	}
	if d.NFwd != 2 {
		t.Fatalf("NFwd = %d, want 2 — both operands came forward; a module sub-registry "+
			"left in the recorder must not decide where `x` is looked up", d.NFwd)
	}
	if d.Slots[0].Source != compiler.SlotWordRef {
		t.Errorf("slot 0 = %v, want SlotWordRef — `x` is a module-scope binding", d.Slots[0].Source)
	}
	if err := d.Validate(len(prog.Consts), len(prog.Fns), len(prog.Types)); err != nil {
		t.Errorf("%v", err)
	}
}
