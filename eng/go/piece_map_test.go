package eng

// TestPieceMap is the four-piece split's Stage-0 "virtual package" lint
// (design/legacy/ENG-FOUR-PIECE.0.ignore): every production file is assigned to a
// piece (core / check / compiler / eng), and files assigned to core
// must not reach UP the target dependency chain
// (eng -> compiler -> check -> core) through the known entanglement
// surfaces. The reference lists below shrink to empty as the seam
// stages land; a NEW wrong-direction reference in an unlisted file
// fails immediately, so the split only ratchets forward.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readPieceMap(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("piece_map.tsv")
	if err != nil {
		t.Fatalf("piece_map.tsv: %v", err)
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("piece_map.tsv: bad line %q", line)
		}
		m[parts[0]] = parts[1]
	}
	return m
}

// upwardRefs are the entanglement probes per pattern family; a core
// file matching one is a wrong-direction reference UNLESS listed in
// the per-family accommodation set (the recon's F1/F2 inventory —
// these sets only shrink).
var upwardRefs = []struct {
	name    string
	re      *regexp.Regexp
	allowed map[string]bool
	// pieces the probe applies to; empty means core only.
	pieces map[string]bool
}{
	{
		name: "check-state consultation (F1)",
		re:   regexp.MustCompile(`\.Check\.`),
		allowed: map[string]bool{
			// Stage 2b burned the F1 inventory down to the DESIGNATED seam
			// carrier: the one core file allowed to touch CheckState, whose
			// bodies become interface calls at the package cut.
			"analysis_hooks.go": true,
			// Stage 4 re-homed the analysis STATE to core (the state-record
			// principle): these files ARE the state's home — Registry methods
			// manipulating their own Check field, not interpreter-loop
			// consultation.
			"check_state.go": true, "compile_sandbox.go": true,
		},
	},
	{
		name: "emit recorder reach (F2)",
		re:   regexp.MustCompile(`Check\.Recorder\(\)`),
		allowed: map[string]bool{
			"analysis_hooks.go": true,
		},
	},
	{
		name:    "concrete EmitState assert (F2-hard)",
		re:      regexp.MustCompile(`\.\(\*EmitState\)`),
		allowed: map[string]bool{},
	},
	{
		// Named check-piece entry points core used to call directly
		// (install-time hooks, the Run-boundary carrier operations, the
		// dispatch-model builders) — all routed through the seam carrier
		// in Stage 2c.
		name: "check entry-point call (F7)",
		re: regexp.MustCompile(`\b(checkFnBodyAtConstruction|check.BuildFnBodyReturnsFn|` +
			`StripToCarriers|stripZeroOutResiduals|check.CarrierResults|carrierMixedConform|` +
			`valueCarriesCarrier|CheckAtUncaughtTopLevel|CheckAddUnique|` +
			`CheckAddUniqueDiagnostic|OrderingReturnsFn|planUserPolyDispatch)\(`),
		allowed: map[string]bool{
			"analysis_hooks.go": true,
		},
	},
	{
		// VM-piece unit execution — core never runs compiled units
		// directly (the CompiledRuntime seam owns that decision).
		name:    "VM unit reach (F5)",
		re:      regexp.MustCompile(`\bRunUnit\(`),
		allowed: map[string]bool{},
	},
	{
		// The check piece's dispatch-recovery braid — core's step loop
		// reaches it only through the S9 CheckBraid slot table
		// (dispatch_slots.go), never by name.
		name: "check braid call (S9)",
		re: regexp.MustCompile(`(^|[^.\w])(checkMixedFormAdvisories|checkModeAssumeSig|` +
			`checkModeFallbackPositions|checkModeParenFnCollapse|checkModeSurfaceShape|` +
			`concreteEvalOnce|drainUndefinedAtoms|exprRefsCarrier|noteSpeculativeBarrierCommit|` +
			`check.DeclineForwardStackDrift|declineStrandedMemberFn|shareCheckState|spliceAnonCheckResult|` +
			`spliceCheckResults|spliceFnCheckTail|check.SpliceFnValueCheckResult|tagCheckModeDefRead|` +
			`tryDynamicFnValueDispatch|tryMemberFnArrivalDispatch|check.TryShapedMethodDispatch|` +
			`undefinedWordCheckDiag)\(`),
		allowed: map[string]bool{},
	},
	{
		// Named compiler-piece record/fold/bake entry points — the check
		// piece routes these through the S3 dispatch-hook carrier, and
		// core does not touch them at all (the drift-window offer rides
		// the S9 slot).
		name: "compiler record/fold call (F6)",
		re: regexp.MustCompile(`\b(recordDispatchOutcome|tryFoldScalarConst|` +
			`tryRecordPoly|tryCompileUserPolyArms|planUserPolyDispatch|` +
			`NewEmitState|newIsolatedEmit|tryRecordDriftWindow)\(`),
		allowed: map[string]bool{"dispatch_hooks.go": true},
		pieces:  map[string]bool{"core": true, "check": true},
	},
}

func TestPieceMap(t *testing.T) {
	pieces := readPieceMap(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		piece, ok := pieces[f]
		if !ok {
			t.Errorf("%s: not assigned in piece_map.tsv — assign it to core/check/compiler/eng", f)
			continue
		}
		if piece != "core" && piece != "check" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, probe := range upwardRefs {
			applies := probe.pieces[piece] || (probe.pieces == nil && piece == "core")
			if applies && probe.re.Match(src) && !probe.allowed[f] {
				t.Errorf("%s (core): new wrong-direction reference [%s] — route it through a seam (design/legacy/ENG-FOUR-PIECE.0.ignore)", f, probe.name)
			}
		}
	}
}
