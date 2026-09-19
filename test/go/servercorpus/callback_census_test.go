// The server-callback census — case 1 of the server-concurrency corpus
// (design/FULL-COMPILATION.0.md section 13).
//
// The corpus's bar is that every callback case compiles FULLY: zero
// runtime interpreter entries, a transcript identical to the interpreter's,
// and race-clean. Nothing in the tree measured the first of those for a
// server, so "does a callback-architected server compile completely?" had
// no numeric answer — only the whole-program compile/refuse verdict, which
// says nothing about what the per-connection handler does at runtime.
//
// This case measures it. A boru program starts a TCP echo server, connects
// to itself, exchanges messages over the callback, and closes. It runs
// compiled with the interpreter-entry hook armed, and interpreted for the
// transcript oracle. The assertions are: identical results, and the
// unattributed-entry count at or below its ceiling.
//
// Like the other Stage-1 censuses this RECORDS a ratchet rather than
// asserting the end state: the count is what it is today, it may only
// fall, and it reaches zero when the callback path stops re-entering the
// tree-walker (Stage 9).
package servercorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// echoServerProgram starts a server whose per-connection handler echoes
// newline-framed lines, drives it over a real socket, and yields the last
// reply. The handler is the callback under measurement: it is stored by
// serve-raw and invoked by the Go accept loop after the program's own
// statements have run.
const echoServerProgram = `
import "boru:net"
def ln
(Net.serve-raw {tcp:0}
  (fn sock:Socket Any
    [def nl (convert Bytes "\n")
      for 3
        [def line (Net.recv-until sock nl)
          Net.send-bytes (convert Bytes (join "" [(convert String line) "\n"])) sock
        ]
    ]
  )
)
def port ((Net.addr ln).port)
def cli (Net.connect-raw {tcp:(join "" ["127.0.0.1:" (convert String port)])})
def nl (convert Bytes "\n")
Net.send-bytes (convert Bytes "one\n") cli
def a (convert String (Net.recv-until cli nl))
Net.send-bytes (convert Bytes "two\n") cli
def b (convert String (Net.recv-until cli nl))
Net.send-bytes (convert Bytes "three\n") cli
def c (convert String (Net.recv-until cli nl))
Net.close cli
join "" [a b c]
`

// callbackEntryCeiling is the number of unattributed interpreter entries
// the compiled echo case may produce. Monotone DOWN only; 0 when the
// callback path runs entirely on the VM (Stage 9). Measured, not chosen.
const callbackEntryCeiling = 0 // measured 2026-08-25 — see the log line

func TestServerCallbackCensus(t *testing.T) {
	var (
		mu      sync.Mutex
		entries []lang.InterpEntry
	)

	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	disarm := ac.ArmInterpEntryHook(func(e lang.InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	gotC, wasCompiled, errC := ac.RunCompiled(echoServerProgram)
	disarm()
	if errC != nil {
		t.Fatalf("compiled run: %v", errC)
	}

	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	gotI, errI := ai.RunInterp(echoServerProgram)
	if errI != nil {
		t.Fatalf("interpreted run: %v", errI)
	}

	// Transcript parity: the callback's observable output must not depend
	// on which lane ran the program.
	if renderResult(gotC) != renderResult(gotI) {
		t.Errorf("echo transcript diverged:\n  compiled    = %q\n  interpreted = %q",
			renderResult(gotC), renderResult(gotI))
	}
	if want := "onetwothree"; !strings.Contains(strings.ReplaceAll(renderResult(gotI), "\n", ""), want) {
		t.Errorf("echo did not round-trip: interpreted result %q, want it to contain %q",
			renderResult(gotI), want)
	}

	mu.Lock()
	defer mu.Unlock()
	bySeam := map[string]int{}
	runs := 0
	for _, e := range entries {
		if e.Attribution != "" {
			continue
		}
		bySeam[e.Seam]++
		if e.Seam == "Engine.Run" {
			runs++
		}
	}
	t.Logf("server-callback census: compiled=%v, %d unattributed interpreter runs (routes: %v)",
		wasCompiled, runs, bySeam)

	if runs > callbackEntryCeiling {
		t.Errorf("server-callback census %d exceeds ceiling %d — the per-connection handler re-entered the interpreter: %v",
			runs, callbackEntryCeiling, bySeam)
	}
}

func renderResult(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		if s, ok := v.(string); ok {
			parts[i] = s
			continue
		}
		parts[i] = "?"
	}
	return strings.Join(parts, "")
}

// miniRedisProgram drives the mini-redis app — case 2 of the corpus. Where
// echo exercises ONE callback, this exercises a protocol server: the wire
// codec is a pair of boru functions (decode / encode) the Go connection
// loop invokes per message, sitting under a patrun-routed command table
// and over a per-connection handler. If the callback path re-enters the
// interpreter anywhere beyond the simplest shape, this is where it shows.
const miniRedisProgram = `
import "boru:net"
import "%s"
def ln (MiniRedis.serve {port:0})
def port ((Net.addr ln).port)
def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String port)]))
MiniRedis.cmd ep "SET k hello" drop
MiniRedis.cmd ep "GET k"
`

// redisEntryCeiling is the unattributed-entry count for the protocol-server
// case. Monotone DOWN only; 0 at Stage 9. Measured, not chosen.
//
// 51 today because the program does not compile AT ALL: it refuses on the
// check-diagnostics sentinel with `undefined_word: h2`
// (design/examples/apps/mini-redis.boru:210).
//
// DIAGNOSED 2026-08-26 (NUR103), by delta-debugging this app rather than
// writing reductions: of its fourteen registered handlers, HDEL alone
// reproduces — and HSET, the site the message NAMES, does not. Three faults
// compose. (1) `boru check` does not analyse a service-handler body AT ALL:
// a bare undefined word inside one is reported clean by check and refused by
// the compiler, so a typo in a request handler ships. The compile pass must
// read that body — it records it into a compiled callback unit — so every
// divergence follows from that asymmetry. (2) A call the checker models as
// DIVERGENT silently unbinds its `def`, so the later read is undefined; at
// run time the receiver is a real value and the handler works. (3) The
// `no_signature` that would name the failing call is suppressed while
// compiling, which is why the escaping diagnostic is at a different line,
// about a different name, than its cause.
//
// This is frontier family E — the sentinel section 6.9 deletes at Stage 8 —
// and it is the whole reason a realistic protocol server refuses while the
// plain echo server compiles to zero.
// RETIRED 2026-09-19 with the interpreter fallbacks. The 51 was measured on
// the FALLBACK run — the program never compiled, the library re-ran it whole
// on the interpreter, and this counted the entries that re-run made. There is
// no such run, so there is no such number, and keeping it would be counting
// something that no longer happens.
//
// What is left to assert is the fact underneath it, which has not changed:
// this realistic protocol server DOES NOT COMPILE, for NUR103's reasons
// above, and the interpreter runs it correctly. When NUR103 closes, this test
// starts failing at the compile assertion and is rewritten around the
// compiled run it will then have — which is the right way round.

func TestMiniRedisCallbackCensus(t *testing.T) {
	app, err := filepath.Abs(filepath.Join("..", "..", "..", "design", "examples", "apps", "mini-redis.boru"))
	if err != nil {
		t.Fatalf("resolve mini-redis: %v", err)
	}
	if _, err := os.Stat(app); err != nil {
		t.Skipf("mini-redis app not present: %v", err)
	}
	program := fmt.Sprintf(miniRedisProgram, app)

	var (
		mu      sync.Mutex
		entries []lang.InterpEntry
	)
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	disarm := ac.ArmInterpEntryHook(func(e lang.InterpEntry) {
		mu.Lock()
		defer mu.Unlock()
		entries = append(entries, e)
	})
	gotC, wasCompiled, errC := ac.RunCompiled(program)
	disarm()

	// The program does not compile (NUR103). That is the finding, and it is
	// asserted rather than worked around: a passing run here would mean the
	// defect had closed and this test needs rewriting around the compiled
	// run it would then have.
	if errC == nil || !strings.Contains(errC.Error(), "bytecode compilation FAILED") {
		t.Fatalf("mini-redis COMPILES now (compiled=%v err=%v) — NUR103 has closed. "+
			"Rewrite this test around the compiled run: assert the transcript on both "+
			"lanes and re-measure the callback census, which the fallback's removal "+
			"retired because it counted the fallback's own interpreter entries.",
			wasCompiled, errC)
	}
	if !strings.Contains(errC.Error(), "undefined_word") {
		t.Errorf("the compile failure changed shape: %v\n"+
			"NUR103 pins it to the check-diagnostics sentinel raising undefined_word; "+
			"a different failure is a new finding", errC)
	}
	if len(gotC) != 0 {
		t.Errorf("a program that does not compile returned a result: %v", gotC)
	}

	// The interpreter runs it correctly, which is what makes the compile
	// failure a defect rather than a verdict.
	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	gotI, errI := ai.RunInterp(program)
	if errI != nil {
		t.Fatalf("interpreted run: %v", errI)
	}
	if got := renderResult(gotI); !strings.Contains(got, "hello") {
		t.Errorf("GET did not round-trip: interpreted result %q, want it to contain %q", got, "hello")
	}

	mu.Lock()
	defer mu.Unlock()
	t.Logf("mini-redis: does not compile (NUR103); %d interpreter entries recorded on the attempt", len(entries))
}
