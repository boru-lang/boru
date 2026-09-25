package langspec

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/boru-lang/boru/test/go/vary"
)

// TestVariationDifferential — the standing variation gate (plan WS3): a
// deterministic sample of passing corpus rows is re-embedded in every
// compile context of the shared transform table (test/go/vary) and each
// variant is classified through the dual pipeline AT TEST TIME (no checked-in
// generated corpus — the interpreter is the oracle, like the property
// fuzzer). The gate asserts:
//
//   - NO unpinned variant diverges (a divergence is a miscompile — a NEW one
//     always fails; a KNOWN one is pinned in varyKnownMiscompiles with a
//     stale arm forcing graduation on fix — empty since the 2026-09-02 flip);
//   - every compile failure / island falls in a KNOWN reason bucket
//     (varyBucket) pinned in varyCompileFailureLedger — a new bucket is a new
//     frontier class that must be triaged (fix, or pin here + add a
//     representative row to lang/spec/frontier/);
//   - ledgered buckets are still observed (stale arm → delete the entry) —
//     enforced only at >= default breadth, since Sample() nests samples
//     (a bucket observed at the default stays observed at any larger one,
//     while a smaller sample legitimately misses some).
//
// Breadth is env-crankable for triage sweeps: BORU_VARY_SEEDS=<n> (0 = the
// whole corpus). The default is modest for CI wall-clock; the full sweep
// lives in `specgen -vary`.
func TestVariationDifferential(t *testing.T) {
	t.Parallel()
	seeds, err := vary.LoadSeeds(filepath.Join("..", "..", "..", "lang", "spec"))
	if err != nil {
		t.Fatalf("seeds: %v", err)
	}
	n := defaultVarySeeds
	if s := os.Getenv("BORU_VARY_SEEDS"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("BORU_VARY_SEEDS=%q: %v", s, err)
		}
		n = v
	}
	sample := vary.Sample(seeds, n)

	// NUR092: the sample is a hash-ordered PREFIX of the corpus, so adding an
	// unrelated spec row can displace the one seed that exercised a ledgered
	// bucket or a pinned variant. A stale arm therefore never reports on the
	// sample alone: it re-checks the UNSAMPLED rest of the corpus first
	// (lazily, once — the full sweep is slow and an emptied bucket is rare)
	// and calls the entry stale only when the class is gone at full breadth.
	var rest map[string]int
	var restDiverged map[string]bool
	recheck := func() {
		if rest != nil {
			return
		}
		rest, restDiverged = map[string]int{}, map[string]bool{}
		all := vary.Sample(seeds, 0) // priority order; the sample is its prefix
		if len(all) <= len(sample) {
			return
		}
		for _, v := range vary.SweepSeeds(all[len(sample):], nil) {
			if v.Transform == "seed" {
				continue
			}
			switch v.Res.Outcome {
			case vary.Diverged:
				restDiverged[v.Src] = true
			case vary.Declined, vary.Islanded:
				rest[varyBucket(v.Res.Detail)]++
			}
		}
	}

	variants := vary.SweepSeeds(sample, nil)
	observed := map[string]int{}
	divergedSeen := map[string]bool{}
	counts := map[vary.Outcome]int{}
	skipped := 0
	for _, v := range variants {
		if v.Transform == "seed" {
			if v.Res.Outcome != vary.Pass {
				skipped++ // base not vary-eligible (fixture rows, corpus compile failures — the ratchets own those)
			}
			continue
		}
		counts[v.Res.Outcome]++
		switch v.Res.Outcome {
		case vary.Diverged:
			if _, known := varyKnownMiscompiles[v.Src]; known {
				divergedSeen[v.Src] = true
				continue
			}
			t.Errorf("MISCOMPILE — variant diverges from the interpreter:\n  seed:      %s (%s:%d)\n  transform: %s\n  variant:   %s\n  %s\ntriage: shrink + fix, or pin in varyKnownMiscompiles AND add a frontier row (never leave a divergence unpinned)",
				v.Seed.Input, v.Seed.File, v.Seed.Line, v.Transform, v.Src, v.Res.Detail)
		case vary.Panicked, vary.Hung:
			t.Errorf("ENGINE CRASH OR HANG on a variant — no answer:\n  seed:      %s (%s:%d)\n  transform: %s\n  variant:   %s\n  %s",
				v.Seed.Input, v.Seed.File, v.Seed.Line, v.Transform, v.Src, v.Res.Detail)
		case vary.Declined, vary.Islanded:
			bucket := varyBucket(v.Res.Detail)
			observed[bucket]++
			if _, ok := varyCompileFailureLedger[bucket]; !ok {
				t.Errorf("NEW compile failure class from variation:\n  bucket:    %q\n  seed:      %s (%s:%d)\n  transform: %s\n  variant:   %s\n  detail:    %s\ntriage: widen the compiler, or pin the bucket in varyCompileFailureLedger AND add a representative row to lang/spec/frontier/",
					bucket, v.Seed.Input, v.Seed.File, v.Seed.Line, v.Transform, v.Src, v.Res.Detail)
			}
		}
	}
	if n >= defaultVarySeeds || n <= 0 {
		for bucket, why := range varyCompileFailureLedger {
			if ledgerStale(observed[bucket], func() int { recheck(); return rest[bucket] }) {
				t.Errorf("stale varyCompileFailureLedger bucket %q — no variant declines in it any more, at the default breadth or the full corpus; graduate it (delete the entry).\n  was red because: %s", bucket, why)
			} else if observed[bucket] == 0 {
				t.Logf("varyCompileFailureLedger bucket %q is unsampled at breadth %d but live at full breadth (%d variant(s)) — not stale (NUR092)", bucket, len(sample), rest[bucket])
			}
		}
		for src, why := range varyKnownMiscompiles {
			if !divergedSeen[src] {
				recheck()
			}
			if !divergedSeen[src] && restDiverged[src] {
				t.Logf("varyKnownMiscompiles pin unsampled at breadth %d but still diverging at full breadth — not stale (NUR092):\n  variant: %.160s", len(sample), src)
				continue
			}
			if !divergedSeen[src] {
				t.Errorf("stale varyKnownMiscompiles pin — this variant no longer diverges; graduate it (delete the pin; also graduate the frontier-do-registry-replay rows if the class is fixed).\n  variant: %.120s…\n  was red because: %s", src, why)
			}
		}
	}
	t.Logf("variation census: %d seeds (%d skipped) → pass=%d declined=%d islanded=%d diverged-known=%d interp-reject=%d check-reject=%d; compile failure buckets: %v",
		len(sample), skipped, counts[vary.Pass], counts[vary.Declined], counts[vary.Islanded],
		len(divergedSeen), counts[vary.InterpReject], counts[vary.CheckReject], observed)
}

// ledgerStale is the stale verdict for one ledger entry: unobserved in the
// sample AND, consulted only then, unobserved in the rest of the corpus. An
// entry the default breadth merely failed to sample is not stale — the rule
// NUR092 pins (a corpus row unrelated to a refusal class cannot instruct the
// author to delete the class's ledger entry).
func ledgerStale(inSample int, inRest func() int) bool {
	return inSample == 0 && inRest() == 0
}

// varyBucket normalises a compile failure/island detail into a stable ledger key:
// the vary-specific families first (runtime-bail details embed full rendered
// errors; the check-diagnostics sentinel and the do/for reasons are stable
// prefixes), then the census's normaliseReason for everything else.
func varyBucket(detail string) string {
	switch {
	case strings.HasPrefix(detail, "runtime bail:"):
		return "runtime bail (VM defer, compile failure)"
	case strings.HasPrefix(detail, "check diagnostics"):
		return "check diagnostics (wrapped-context false positive)"
	case strings.HasPrefix(detail, "do: fallible multi-value body"):
		return "do: fallible multi-value body under a catch"
	case strings.HasPrefix(detail, "for: body nets multiple values"):
		return "for: body nets multiple values per iteration"
	case strings.Contains(detail, "variadic result promoted to frame slots"):
		// Contains, not HasPrefix: the detail leads with the declining word
		// (`do:`, `__splicedyn:`) — one bucket regardless.
		return "variadic result promoted (exact-arity frame seat)"
	case strings.HasPrefix(detail, "program embeds an OpFallback island"):
		return "islanded"
	case strings.HasPrefix(detail, "twin regime:"):
		// The rollback-and-replay regime's placement gate (Finalize): a bind
		// transition the check pass performed that no op replays or installs.
		return "twin regime (unplaced bind transition)"
	default:
		return normaliseReason(detail)
	}
}

// defaultVarySeeds is the CI breadth: large enough to hold every ledgered
// bucket observable, small enough to keep the langspec wall-clock sane.
const defaultVarySeeds = 32

// varyCompileFailureLedger pins the compile failure reason-buckets (varyBucket) that
// variation currently produces, with the same graduation contract as every
// expected-red ledger in the suite: a NEW bucket fails until triaged; a
// bucket no longer observed at default breadth fails as stale. The buckets
// are calibrated against the DEFAULT sample — editing the corpus can shift
// which seeds are sampled and legitimately graduate a bucket. Bootstrap
// census (2026-07-13, 32 seeds): pass=384 declined=42 islanded=0.
var varyCompileFailureLedger = map[string]string{
	"check diagnostics (wrapped-context false positive)": "checker emits model-undermining diagnostics for a program the interpreter runs clean once re-embedded (for/fn wrapping of typed defs) — sound-but-lossy compile failure",
	"code-body word (NoEvalArgs)":                        "RE-ENTERED 2026-07-30 (project rename resampled the corpus): a NoEvalArgs code-body word (do/each) wrapping a mount-handler seed still declines at Stage 2 — the 2026-07-14 graduation held only for the do-def replay shapes then in the default sample, not for this one",
	"conditional fn shadow (branch/loop)":                "a user fn REDEFINED inside a conditionally-reached body (if/case arm, for/each loop) overlap-removes the enclosing overload in place, so the branch/loop def rollback cannot restore it and compiled resolution bakes the shadow while the interpreter keeps the outer fn when the branch is not taken (or the loop runs zero times) — compile failure (design/frontier: frontier-conditional-fn-shadow.tsv)",
	"for: body nets multiple values per iteration":       "NARROWED (net drivers landed): only Function-bearing multi-value loop regions keep this compile failure — the parked-fn cross-iteration auto-apply hazard; const/computed regions compile",
	"operand provenance":                                 "residual operand loses provenance across a wrapped context (plan Phase 4/5 accounting classes)",
	"residual lowering (Stage 1 limit)":                  "scheduling — the wrapped residual shape exceeds Stage 1's lowering (prefix-stack transform)",
	"stack discipline (lowering)":                        "scheduling — dirty-stack prefixes the lowerer cannot arrange (prefix-stack transform)",
	"variadic result promoted (exact-arity frame seat)":  "a MULTI-OUT variadic call result (a fallible or branch-variant do region) promoted to frame slots has no fixed arity to store — the raise/short path delivers fewer values than the static seat popped (PR #280 review; lowerCall's store-prologue gate, representative row in frontier-do-catch.tsv)",
	"twin regime (unplaced bind transition)":             "ENTERED 2026-09-02 with the §6.5 default flip (rollback-and-replay is the only regime): a bind transition the check pass performs inside a wrapped body that no compiled op replays or installs — an `import` inside a multi-run body (a module bind is not a resident install), a TYPE def inside a multi-run body (the arm-residency bridge pairs BindDef twins only), and a `do` body whose closure compile declines (a Stage-3 residual shape), so its twins are never adopted — declines at Finalize's full-placement gate instead of compiling on the check pass's kept installs, which the old default did. Sound: the interpreter owns every one, and a replay the rollback would lose is exactly what the gate exists to decline. Graduation, shape by shape: resident module binds, resident type twins, and a closure lowering that admits the declined do bodies. Representative rows: frontier-twin-placement.tsv",
}

// Graduated 2026-07-14 (do-def leak fidelity): "code-body word
// (NoEvalArgs)" — the replay-hazard do bodies compile as closure units
// once the check pass keeps do-body defs (the parts conflict was the
// rollback's artifact).
// Graduated 2026-07-13 (the do-unit replay fix's collateral): "dynamic/
// opaque output" (module-fn dot results now compile inside units once
// ensureExportsBound re-binds a real ModuleExport), "dynamic input", and
// "runtime bail (VM defer, compile failure)" (module-body wraps now
// compile without bailing). Graduated 2026-07-14: "do: fallible
// multi-value body under a catch" (L-DO part 1 — the SetCatchVariadic
// latch records fallible do results variadic instead of declining) and
// "for: body nets multiple values per iteration" (net drivers part 1 —
// computed multi-value loop bodies ride the residualN>1 reconciliation).

// varyKnownMiscompiles pins KNOWN divergences by EXACT variant source, with
// a stale arm forcing graduation when a pinned variant stops diverging. A
// divergence NOT in this map always fails — new miscompiles are never
// silently ledgerable.
//
// The do-unit registry-replay class (10 pins, 5 seeds × do-body/do-catch —
// minimal repro preserved in lang/spec/frontier/frontier-do-registry-replay.tsv)
// graduated 2026-07-13 when the replay-hazard bake compile failure (eng
// bodyHasReplayHazard) + the ensureExportsBound ModuleExport re-bind fix
// landed: the typed-def half declines, the import half compiles
// natively as a closure unit.
// varyKnownMiscompiles pins variants that diverge from the interpreter, so a
// divergence is never left unrecorded.
var varyKnownMiscompiles = map[string]string{
	// EMPTY since 2026-09-02, and the graduation is worth its record. The one
	// pin — the mount-handler loop seed (module-io.tsv:241 under `for 2`,
	// pinned 2026-07-30; briefly masked by stampFnConst's container descent on
	// 2026-08-28 and un-masked deliberately, because a stamp is an
	// optimisation and a wrong answer that is right only while one applies
	// comes back silently) — stopped diverging with the §6.5 DEFAULT FLIP:
	// every compiled request now rolls the check pass's binding installs back
	// before the run and replays them from the placed twins, so `def files
	// (flex {})` binds ONE runtime instance that the loop body and the mount
	// handlers capturing it agree on, where the old keep-installs default left
	// the check pass's own flex map behind for the handlers' dep to see while
	// the loop re-bound the name (`expected a FlexMap, got FlexMap` — same
	// type name, different instance). Measured with -force-compile: the
	// variant now answers `hello mounted hello mounted` on both engines. The
	// stale arm did its job: the pin failed the day the divergence went, and
	// this map stays so the next one is never left unrecorded.
}
