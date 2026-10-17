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

	lang "github.com/boru-lang/boru/lang/go"
)

// The real-program gate: the repo's own boru test suites run end to end and
// report a clean Test.summary. A corpus row pins a word; a suite exercises a
// whole program (the knowledge-graph pipeline, the utils library), which is
// where a step budget, a routed dispatch or a nested callback meets real
// data.
//
// Each suite runs in a SUBPROCESS of this test binary with the program's own
// working directory: the suites read their fixtures relative to it (kg's
// "fixtures/…", "../go.work"), and a process-wide chdir cannot run programs
// in parallel. Programs run in parallel.
//
// Only the SUITES run here: their side effects are confined to their own
// fixtures. kg/main.boru writes the committed graph (kg/out) and the apps
// serve sockets, so neither is run by a test.
var realProgramSuites = []string{"utils/tests", "kg/tests"}

// realProgramSlow names the suites too slow for the shard
// (kg/tests/codegraph_test.boru walks the repository's Go tree: about 2m15s
// against the shard's three-minute ceiling). They are skipped here; `boru
// test` runs them on demand.
var realProgramSlow = map[string]bool{
	"kg/tests/codegraph_test.boru": true,
}

// realProgramRunLedger is the inventory of suites that do not run clean
// today — each an OPEN DEFECT, ratcheting down. Empty since the gate landed.
var realProgramRunLedger = map[string]string{}

// realRunOutcome is one suite's answer, as the helper subprocess reports it.
type realRunOutcome struct {
	Result  string `json:"result"`
	Err     string `json:"err"`
	Summary string `json:"summary"`
}

const realRunHelperEnv = "BORU_REAL_RUN_FILE"

func TestRealProgramsRun(t *testing.T) {
	if file := os.Getenv(realRunHelperEnv); file != "" {
		realRunHelper(file)
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
		for _, f := range m {
			if rel, _ := filepath.Rel(repo, f); !realProgramSlow[filepath.ToSlash(rel)] {
				files = append(files, f)
			}
		}
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
			out, err := realRunRun(f, dir)
			if err != nil {
				verdicts[i] = verdict{rel, "run: " + err.Error()}
				return
			}
			verdicts[i] = verdict{rel, realRunJudge(out)}
			t.Logf("%s: %s", rel, firstN(out.Summary, 60))
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
				t.Errorf("%s: %s\n    A real program no longer runs clean. Fix it, or —\n"+
					"    if this is an argued step backwards — ledger it in realProgramRunLedger with the reason.", v.rel, v.failure)
			} else if !strings.Contains(v.failure, want) {
				t.Errorf("%s fails differently than ledgered:\n  ledgered: %s\n  actual:   %s", v.rel, want, v.failure)
			}
		case ledgered:
			t.Errorf("%s is ledgered as failing but now runs clean — delete its realProgramRunLedger entry", v.rel)
		}
	}
	t.Logf("real programs run: %d suites, %d run clean", len(files), len(files)-len(failing))
}

// realRunJudge reads a suite's outcome against its own assertions and
// returns the failure, or "".
func realRunJudge(out realRunOutcome) string {
	if out.Err != "" {
		return "run failed: " + firstN(out.Err, 300)
	}
	if !strings.Contains(out.Summary, "failed:0") || strings.Contains(out.Summary, "total:0") {
		return "summary is not clean: " + out.Summary
	}
	return ""
}

// firstN truncates s to its first n bytes for a report line.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// realRunRun runs file in a subprocess whose working directory is dir, and
// decodes the outcome it prints.
func realRunRun(file, dir string) (realRunOutcome, error) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestRealProgramsRun$", "-test.count=1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), realRunHelperEnv+"="+file)
	out, err := cmd.Output()
	var o realRunOutcome
	line := ""
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "REALRUN ") {
			line = strings.TrimPrefix(l, "REALRUN ")
		}
	}
	if line == "" {
		return o, fmt.Errorf("the helper reported nothing (%v): %s", err, firstN(string(out), 300))
	}
	if jerr := json.Unmarshal([]byte(line), &o); jerr != nil {
		return o, jerr
	}
	return o, nil
}

// realRunHelper is the subprocess half: run file in the working directory
// the parent set, and print the outcome as one JSON line.
func realRunHelper(file string) {
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
	res, err := a.RunInterpValues(string(src))
	o.Result = fmt.Sprint(res)
	if err != nil {
		o.Err = err.Error()
	}
	if v, e := a.RunInterpValues("Test.summary"); e == nil {
		o.Summary = fmt.Sprint(v)
	}
}
