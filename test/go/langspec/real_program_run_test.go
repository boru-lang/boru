package langspec

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// The RUN half of the real-program gate.
//
// TestRealProgramsCompile proves the repo's own programs COMPILE; it never
// runs them. That left a class no gate could see: a program that compiles
// and then fails inside the compiled runtime, or answers differently from the
// interpreter. On 2026-09-27 four of the knowledge-graph pipeline's programs
// did exactly that — every corpus gate green — for three unrelated reasons (a
// routed dispatch whose interpolation slot the VM could not evaluate, a
// tail-calling unit that never sized its spill temps, and a nested callback
// charged to the whole program's step budget). This gate runs each suite on
// BOTH lanes and requires the same answer: the same error, the same result,
// the same Test.summary, and a summary with no failures.
//
// Each lane runs in a SUBPROCESS of this test binary with the program's own
// working directory: the suites read their fixtures relative to it (kg's
// "fixtures/…", "../go.work"), and a process-wide chdir cannot run programs
// in parallel. A program's two lanes run one after the other (they share
// fixture files under /tmp); programs run in parallel.
//
// Only the SUITES run here: their side effects are confined to their own
// fixtures. kg/main.boru writes the committed graph (kg/out) and the apps
// serve sockets, so neither is run by a test.
var realProgramSuites = []string{"utils/tests", "kg/tests"}

// realProgramInterpSlow names the suites whose INTERPRETED run is too slow for
// the shard (kg/tests/codegraph_test.boru walks the repository's Go tree:
// about 2m15s interpreted, 14s compiled). For those the suite's own
// assertions are the oracle: the compiled run must finish and report a clean
// summary.
var realProgramInterpSlow = map[string]bool{
	"kg/tests/codegraph_test.boru": true,
}

// realProgramRunLedger is the inventory of suites whose compiled run does not
// match the interpreter today — each an OPEN DEFECT, ratcheting down like
// realProgramLedger. Empty since the gate landed.
var realProgramRunLedger = map[string]string{}

// realRunOutcome is one lane's answer, as the helper subprocess reports it.
type realRunOutcome struct {
	Result  string `json:"result"`
	Err     string `json:"err"`
	Summary string `json:"summary"`
}

const realRunHelperEnv = "BORU_REAL_RUN_FILE"

func TestRealProgramsRun(t *testing.T) {
	if file := os.Getenv(realRunHelperEnv); file != "" {
		realRunHelper(file, os.Getenv("BORU_REAL_RUN_LANE") == "compiled")
		return
	}
	t.Parallel()
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, dir := range realProgramSuites {
		m, _ := filepath.Glob(filepath.Join(repo, dir, "*_test.boru"))
		files = append(files, m...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no real-program suites discovered — the roots are wrong, and a gate that runs nothing passes vacuously")
	}
	type verdict struct{ rel, failure string }
	verdicts := make([]verdict, len(files))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	// Progress on stderr (the shard runs without -v, where t.Logf is silent
	// until the end): a line per finished suite with the COMPLETED count, and
	// a heartbeat while the slow ones run (AGENTS.md: transient tasks report
	// progress at least every 30 seconds).
	var done atomic.Int32
	start := time.Now()
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				n := int(done.Load())
				fmt.Fprintf(os.Stderr, "real programs run: %d/%d suites (%d%%), %s\n", n, len(files), 100*n/len(files), time.Since(start).Round(time.Second))
			}
		}
	}()
	for i, f := range files {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer func() {
				n := int(done.Add(1))
				fmt.Fprintf(os.Stderr, "real programs run: [%d/%d] %s\n", n, len(files), verdicts[i].rel)
			}()
			rel, _ := filepath.Rel(repo, f)
			rel = filepath.ToSlash(rel)
			// Every suite is written to run from its tests/ folder's parent
			// (`boru test tests/x_test.boru` from utils/ or kg/, as each
			// header says): imports and fixture reads both resolve there.
			dir := filepath.Dir(filepath.Dir(f))
			comp, err := realRunLane(f, dir, true)
			if err != nil {
				verdicts[i] = verdict{rel, "compiled lane: " + err.Error()}
				return
			}
			verdicts[i] = verdict{rel, realRunJudge(rel, comp, dir, f)}
			t.Logf("%s: %s", rel, firstN(comp.Summary, 60))
		}(i, f)
	}
	wg.Wait()
	close(stop)
	failing := map[string]bool{}
	for _, v := range verdicts {
		want, ledgered := realProgramRunLedger[v.rel]
		switch {
		case v.failure != "":
			failing[v.rel] = true
			if !ledgered {
				t.Errorf("%s: %s\n    A real program no longer runs compiled as it runs interpreted. Fix it, or —\n"+
					"    if this is an argued step backwards — ledger it in realProgramRunLedger with the reason.", v.rel, v.failure)
			} else if !strings.Contains(v.failure, want) {
				t.Errorf("%s fails differently than ledgered:\n  ledgered: %s\n  actual:   %s", v.rel, want, v.failure)
			}
		case ledgered:
			t.Errorf("%s is ledgered as failing but now RUNS compiled as it runs interpreted — delete its realProgramRunLedger entry", v.rel)
		}
	}
	t.Logf("real programs run: %d suites, %d match the interpreter", len(files), len(files)-len(failing))
}

// realRunJudge compares the compiled lane with the interpreted one (or, for a
// suite too slow to interpret, with its own assertions) and returns the
// failure, or "".
func realRunJudge(rel string, comp realRunOutcome, dir, file string) string {
	if comp.Err != "" {
		return "compiled run failed: " + firstN(comp.Err, 300)
	}
	if !strings.Contains(comp.Summary, "failed:0") || strings.Contains(comp.Summary, "total:0") {
		return "compiled summary is not clean: " + comp.Summary
	}
	if realProgramInterpSlow[rel] {
		return ""
	}
	interp, err := realRunLane(file, dir, false)
	if err != nil {
		return "interpreted lane: " + err.Error()
	}
	if comp != interp {
		return fmt.Sprintf("compiled and interpreted differ:\n    compiled: %+v\n    interp:   %+v", comp, interp)
	}
	return ""
}

// realRunLane runs one lane of file in a subprocess whose working directory is
// dir, and decodes the outcome it prints.
func realRunLane(file, dir string, compiled bool) (realRunOutcome, error) {
	lane := "interp"
	if compiled {
		lane = "compiled"
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRealProgramsRun$", "-test.count=1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), realRunHelperEnv+"="+file, "BORU_REAL_RUN_LANE="+lane)
	out, err := cmd.Output()
	var o realRunOutcome
	line := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "REALRUN ") {
			line = strings.TrimPrefix(l, "REALRUN ")
		}
	}
	if line == "" {
		return o, fmt.Errorf("the %s helper reported nothing (%v): %s", lane, err, firstN(string(out), 300))
	}
	if jerr := json.Unmarshal([]byte(line), &o); jerr != nil {
		return o, jerr
	}
	return o, nil
}

// realRunHelper is the subprocess half: run file on one lane in the working
// directory the parent set, and print the outcome as one JSON line.
func realRunHelper(file string, compiled bool) {
	o := realRunOutcome{}
	defer func() {
		b, _ := json.Marshal(o)
		fmt.Println("REALRUN " + string(b))
	}()
	src, err := os.ReadFile(file)
	if err != nil {
		o.Err = err.Error()
		return
	}
	a, err := lang.New()
	if err != nil {
		o.Err = err.Error()
		return
	}
	a.SetClock(specClock)
	a.SetOutput(io.Discard)
	a.NativeRegistry().ErrOutput = io.Discard
	if wd, err := os.Getwd(); err == nil {
		a.NativeRegistry().BaseDir = wd
	}
	var res []core.Value
	if compiled {
		prog, reason, _, cerr := a.CompileCheck(string(src))
		if cerr != nil || prog == nil {
			o.Err = fmt.Sprintf("did not compile: %s %v", reason, cerr)
			return
		}
		res, err = eng.RunProgram(prog, a.NativeRegistry())
	} else {
		res, err = a.RunInterpValues(string(src))
	}
	o.Result = fmt.Sprint(res)
	if err != nil {
		o.Err = err.Error()
	}
	if v, e := a.RunInterpValues("Test.summary"); e == nil {
		o.Summary = fmt.Sprint(v)
	}
}
