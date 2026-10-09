# Saved work: interpreter performance and the Value no-copy review

Everything a later session needs to reproduce, verify or continue the work
handed over in [../../INTERPRETER-PERF-HANDOVER.0.md](../../INTERPRETER-PERF-HANDOVER.0.md).
Nothing here is built or run by `make`; the Go sources below are embedded
as text so that no tooling (gofmt, lint, the coverage gates, the knowledge
graph's module walk) picks them up. Every output and patch was produced on
main at `6780a6030`.

## Contents

| path | what it is |
|---|---|
| `inventory/core-engine.md` | engine-side copy inventory for `core/go` (tape, step loop, loops, sealed regions, registry, forks, captures) |
| `inventory/core-value.md` | value-side copy inventory for `core/go` (`Value`, payloads, clone, unify, carriers, type literals) |
| `inventory/eng-check-compiler.md` | copy inventory for the VM, the checker and the compiler |
| `inventory/basic-lang-parser-cmd.md` | copy inventory for the word libraries, the parser and the CLI, with the language's aliasing semantics answered |
| `patches/vet-nocopy-marker.patch` | part of phase 0: a zero-size `noCopy` field that makes `go vet -copylocks` report every copy of a single `Value`; bulk copies through `copy` and variadic `append` over `[]Value` stay invisible to it |
| `patches/pointer-receivers.patch` | the first step of phase 1: pointer receivers on 20 of `Value`'s 23 value-receiver methods |
| `probes/*.txt` | two-lane probe corpora, one program per line |
| `probes/*.out` | each corpus's output on `6780a6030`, from the harness below |

The inventories were written by four parallel read-only review passes and
reconciled into [../../VALUE-NO-COPY.0.md](../../VALUE-NO-COPY.0.md); their
line numbers are at `6780a6030`. Spot-check a row before acting on it.

## The patches

Both apply with `git apply` to `6780a6030` and leave every module's
non-test code building. Verified on 2026-10-09:

| patch | applies to `6780a6030` | non-test build, every module | tests |
|---|---|---|---|
| `vet-nocopy-marker.patch` | yes | yes; `Value` stays 104 bytes | every module's tests compile; `go vet -copylocks` on `core/go` then reports 10,627 copies, as in the measurements note |
| `pointer-receivers.patch` | yes | yes | 105 call sites in 30 test files call a switched method on a non-addressable value and stop compiling; `pointer-receivers.test-sites.txt` lists every one. The patch fixes the 7 such sites in non-test code with a local variable; the test sites need the same |

Applying the marker and running `go vet -copylocks ./...` per module is
how the copy counts in
[../../VALUE-NO-COPY-MEASUREMENTS.0.md](../../VALUE-NO-COPY-MEASUREMENTS.0.md)
§3 were taken. The receiver patch is the experiment in §5 of that note
(Stage6 geomean 0.888 of the baseline). Neither is meant to land as is:
phase 0 also needs the `make vet-copies` ratchet, and phase 1 continues
into the step loop, the matcher and the VM run loop.

## The probe corpora

Each corpus was a before/after oracle for one change: the change was
accepted only when the corpus's output was byte-identical on both lanes
before and after, apart from the differences the change intended.

| corpus | rows | covers | used for |
|---|---|---|---|
| `loops.txt` | 106 | `break`/`continue` through bodies and lambdas, lambda no-match and zero-result forms, nested fault attribution, `do` trapping, context containment, def leaks, residual containers, inert fn-value elements, every ported word's empty, error and escape cases | the collection words' port to the tape (`Loop`, `LoopDriver`) |
| `loops-empty-bodies.txt` | 12 | empty bodies and lambda-valued bodies for `each`, `for-each`, `fold`, `scan`, `filter`, `eachrank` | the same port |
| `literals.txt` | 76 | list and map literals, members, computed keys, interpolation holes, nesting, escapes from inside a literal (NUR358), literals in fn bodies | sealed literals |
| `pattern-members.txt` | 9 | a dispatch whose pattern evaluates a map member that itself dispatches (NUR235) | the `sealedHold` hazard |
| `case-arms.txt` | 53 | `case` blocks, defaults and predicates, the computed `if` arm, `break` escaping a block, `args` inside an arm | call regions |
| `call-region-errors.txt` | 2 | a `case` inside a fn, a computed `if` arm | the call-loop pool |
| `debug-stack.txt` | 11 | `Debug.stack` and `Debug.disasm` inside loop bodies, call regions and literals | the Codex review of #533 (the stack view and the recorder) |
| `sift-differential.txt` | 4 | `import "boru:sift"` inside `for` | the `TestVariationDifferential` timeout seen only under heavy box load |
| `fn-frames.txt` | 22 | fn frames: params named like words (`len`, `add`, `Foo`), lambdas, nested calls, a 100,000-deep tail recursion and a 2,000-deep non-tail one, body-local defs, `args` after a loop, `do` trapping a raise, `break` in a body, `undef` of a param | the structural frame tail (#532, NUR385) |

### The harness

Save this as `lang/go/zz_loop_probe_test.go`. The `**/zz*probe*` rule in
`.gitignore` keeps such files out of commits by design.

```go
package lang

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestZZLoopProbe(t *testing.T) {
	in := os.Getenv("BORU_PROBES")
	if in == "" {
		t.Skip("no BORU_PROBES")
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out, err := os.Create(os.Getenv("BORU_PROBES_OUT"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		src := sc.Text()
		if strings.TrimSpace(src) == "" {
			continue
		}
		b, _ := New()
		gotI, errI := b.RunInterp(src)
		c, _ := New()
		gotC, compiled, errC := c.RunCompiled(src)
		fmt.Fprintf(out, "%s\n  I: %v | %v\n  C(%v): %v | %v\n", src, gotI, errI, compiled, gotC, errC)
	}
}
```

Run a corpus and compare with its saved output:

```bash
cd lang/go
P=$PWD/../../design/handover/interpreter-perf/probes
BORU_PROBES=$P/loops.txt BORU_PROBES_OUT=/tmp/loops.out \
  go test -count=1 -run '^TestZZLoopProbe$' .
diff $P/loops.out /tmp/loops.out
```

Each row prints the program, then `I:` with the interpreter's results and
error, then `C(compiled):` with the compiled lane's. The saved outputs were
regenerated on `6780a6030` for this folder and are byte-identical to the
outputs recorded at the end of each change, so the harness is
deterministic run to run. A lane divergence is
an `I` line and a `C` line that disagree; a regression is a `diff` against
the saved output.

## Timing harnesses

The repo's benchmarks (`BenchmarkStage6`, `BenchmarkFnKinds`,
`BenchmarkPerfWords`) cover most shapes. These two scratch harnesses
covered the literal, `case` and collection-word shapes they do not. Save
each under a name the ignore rule matches.

`lang/go/zz_perf_probe_test.go`, run with `BORU_PERF_BENCH=1 go test
-count=1 -run '^TestZZPerfProbe$' .` (interpreter lane, min of 7):

```go
package lang

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TEMPORARY timing harness (not committed): interpreter-lane wall time,
// min of 7 runs each.
func TestZZPerfProbe(t *testing.T) {
	if os.Getenv("BORU_PERF_BENCH") == "" {
		t.Skip("set BORU_PERF_BENCH")
	}
	cases := []struct{ name, src string }{
		{"list3", `for 3000 [(size [1 2 3]) drop]`},
		{"list_groups", `for 3000 [(size [(1 add 2) (3 add 4)]) drop]`},
		{"map3", `for 3000 [(size {a:1 b:2 c:3}) drop]`},
		{"map_groups", `for 3000 [(size {a:(1 add 2) b:(3 add 4)}) drop]`},
		{"nested", `for 3000 [(size [[1 2] [3 (1 add 2)]]) drop]`},
		{"interp", "for 3000 [(size `x${1 add 2}y`) drop]"},
		{"list_word", `def x 5 end for 3000 [(size [x (x add 1)]) drop]`},
		{"consumed_each", `for 300 [(each [mul 2] [1 2 3 4 5 6 7 8 9 10]) drop]`},
		{"case_lit", `for 3000 [(case 1 [1 ['one'] ['many']]) drop]`},
		{"case_pred", `for 3000 [(case 5 [[gt 3] ['big'] ['small']]) drop]`},
		{"case_body", `for 3000 [(case 2 [2 [dup add]]) drop]`},
		{"case_default", `for 3000 [(case 9 [1 ['one'] ['other']]) drop]`},
		{"case_in_fn", `def f fn [[x:Integer] [Any] [case x [1 [drop 'one'] [drop 'many']]]] end for 3000 [(f 2) drop]`},
		{"if_computed", `def a (quote [10 add 1]) end for 3000 [(if true a [2]) drop]`},
	}
	for _, c := range cases {
		best := time.Hour
		for i := 0; i < 7; i++ {
			b, err := New()
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			if _, err := b.RunInterp(c.src); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		fmt.Printf("%-14s %8.2f ms\n", c.name, float64(best.Microseconds())/1000)
	}
}
```

`lang/go/zz_loop_bench_probe_test.go`, run with `BORU_LOOP_BENCH=1 go test
-count=1 -run '^TestZZLoopBench$' .` (20k-element inputs, min of 5):

```go
package lang

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TEMPORARY timing harness (not committed): interpreter-lane wall time of the
// collection words over 20k-element inputs, min of 5 runs each.
func TestZZLoopBench(t *testing.T) {
	if os.Getenv("BORU_LOOP_BENCH") == "" {
		t.Skip("set BORU_LOOP_BENCH")
	}
	cases := []struct{ name, setup, src string }{
		{"each_mul", `def xs (iota 20000)`, `each [mul 2] xs`},
		{"fold_add", `def xs (iota 20000)`, `fold [add] xs 0`},
		{"for_each", `def xs (iota 20000)`, `for-each [drop] xs`},
		{"filter_gt", `def xs (iota 20000)`, `filter [gt 10] xs`},
		{"scan_add", `def xs (iota 20000)`, `scan [add] xs`},
		{"each_fnvalue", `def xs (iota 20000) def dbl fn [[x:Integer] [Integer] [x mul 2]]`, `each dbl/v xs`},
		{"nested_each", `def xs (iota 200)`, `each [drop (each [mul 2] xs)] xs`},
		{"each_map", `def m (fold [[k v] def k (convert String v) end set k v] (iota 5000) {})`, `each [mul 2] m`},
	}
	for _, c := range cases {
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.RunInterp(c.setup); err != nil {
			t.Fatalf("%s setup: %v", c.name, err)
		}
		best := time.Hour
		for i := 0; i < 5; i++ {
			start := time.Now()
			if _, err := b.RunInterp(c.src); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		fmt.Printf("%-14s %8.2f ms\n", c.name, float64(best.Microseconds())/1000)
	}
}
```

## The leaf-rule probes behind PR #532's open review thread

These seven programs are the shapes the Codex P1 thread on #532 is about
(a binding word reached through an alias or a `Function` parameter inside
a leaf body). The fifth, `def h fn [[b:Function n:Integer][Integer][b y 2
n]]  h def/v 1  y`, is the hole left for the maintainer's decision, recorded
as U16 in `design/COMPILABLE-SUBSET.md` §5. Save as
`lang/go/zz_alias_probe_test.go`; it prints each program's answer on both
lanes and asserts nothing:

```go
package lang

import (
	"fmt"
	"testing"
)

var aliasProbes = []string{
	`def mydef def/v  def f fn [[n:Integer][Integer][mydef y 2 n]]  f 1  y`,
	`def mydef def/v  def g ([n:Integer] => [mydef y 2 n])  each g/v [1]  y`,
	`def mydef def/v  def g ([n:Integer] => [mydef y 2 n])  1 g/v apply  y`,
	`import module [def mydef def/v def f fn [[n:Integer][Integer][mydef y 2 n]] def g fn [[][Integer][y]] export "M" {f: f/v g: g/v}] end M.f 1 M.g`,
	`def h fn [[b:Function n:Integer][Integer][b y 2 n]]  h def/v 1  y`,
	`def myfn fn/v  def mk fn [[n:Integer][Function][myfn [[][Integer][n]]]]  (mk 5)`,
	`def mk fn [[n:Integer][Function][fn [[][Integer][n]]]]  (mk 5)`,
}

func TestScratchAliasProbes(t *testing.T) {
	for _, src := range aliasProbes {
		a, _ := New()
		iv, ierr := a.RunInterp(src)
		b, _ := New()
		cv, cerr := b.Run(src)
		ie, ce := fmt.Sprint(ierr), fmt.Sprint(cerr)
		if len(ie) > 80 { ie = ie[:80] }
		if len(ce) > 80 { ce = ce[:80] }
		fmt.Printf("SRC    %s\nINTERP %v err=%s\nCOMP   %v err=%s\n\n", src, iv, ie, cv, ce)
	}
}
```

## Benchmark comparison

The before/after tables came from `go test -bench … -count 3` output
compared by this script (min and median per benchmark, geomean of the
min ratios):

```python
import re, sys, statistics
def load(p):
    d = {}
    for line in open(p):
        m = re.match(r'^(\S+)\s+\d+\s+([\d.]+) ns/op', line)
        if m:
            d.setdefault(m.group(1), []).append(float(m.group(2)))
    return d
a, b = load(sys.argv[1]), load(sys.argv[2])
print(f"{'benchmark':48s} {'before(min)':>12s} {'after(min)':>12s} {'ratio':>7s} {'before(med)':>12s} {'after(med)':>12s} {'ratio':>7s}")
rs = []
for k in a:
    if k not in b: continue
    mn = min(b[k]) / min(a[k]); md = statistics.median(b[k]) / statistics.median(a[k]); rs.append(mn)
    print(f"{k:48s} {min(a[k]):12.0f} {min(b[k]):12.0f} {mn:7.3f} {statistics.median(a[k]):12.0f} {statistics.median(b[k]):12.0f} {md:7.3f}")
if rs:
    g = statistics.geometric_mean(rs)
    print(f"geomean ratio (min) over {len(rs)} benchmarks: {g:.3f}")
```

Typical use:

```bash
cd lang/go
go test -run '^$' -bench 'BenchmarkStage6/(arith_chain64|compare_loop|if_scalar|for_tight|map_get|recursion_nontail|recursion_tail|each_list)/' \
  -benchtime 1s -count 3 . | grep '^Benchmark' > before.txt
# apply the change, then the same into after.txt
python3 -I compare.py before.txt after.txt
```

Run benchmarks with nothing else on the box: the gates' own suites running
alongside moved timings by 10% or more on the 4-core machine.

## Profiling the copy share

This script produced the profile table in the measurements note §2:

```bash
#!/bin/bash
# CPU-profile the end-to-end benchmarks and attribute flat time to copy/zero/alloc runtime functions.
set -u
OUT=${OUT:-/tmp/boru-prof}; mkdir -p $OUT   # profiles, the test binary and the top tables land here
cd "$(git rev-parse --show-toplevel)/lang/go"
go test -c -o $OUT/lang.test . 2>&1 | tail -3
for spec in "fnkinds-interp:BenchmarkFnKinds/.*/interp" "stage6-interp:BenchmarkStage6/.*/interp" "stage6-compiled:BenchmarkStage6/.*/compiled" "perfwords:BenchmarkPerfWords"; do
  name=${spec%%:*}; pat=${spec#*:}
  echo "=== $name ($pat) ==="
  $OUT/lang.test -test.run '^$' -test.bench "$pat" -test.benchtime 1s -test.benchmem -test.cpuprofile $OUT/$name.prof > $OUT/$name.txt 2>&1
  tail -3 $OUT/$name.txt
  go tool pprof -top -nodecount=400 $OUT/lang.test $OUT/$name.prof > $OUT/$name.top 2>/dev/null
  echo "--- copy / zero / alloc / GC flat shares ---"
  grep -E "runtime\.(duffcopy|duffzero|memmove|typedmemmove|memclrNoHeapPointers|mallocgc|newobject|growslice|makeslice|gcDrain|scanobject|gcBgMarkWorker|wbBufFlush|gcWriteBarrier|bulkBarrierPreWrite)\b|core\.CloneValue|core\.\(\*cloner\)|ReadList\)\.Slice|core\.StripAscribed|core\.WithPos|ReparentValue" $OUT/$name.top | head -30
done
echo DONE
```

To list who calls the struct copy routine in a profile:

```bash
go tool pprof -peek '^runtime\.duffcopy$' lang.test stage6-interp.prof
```

## Counting every Value copy with go vet

Every module `go.work` lists counts; the first census missed five of
them. `wpg/wasm` builds only for js/wasm and needs `GOOS=js GOARCH=wasm
go vet -copylocks ./wasm/` from `wpg` (it has none). With the marker
applied, the ordinary `go vet ./...` lanes fail on these findings, so
revert the patch when done.

```bash
git apply design/handover/interpreter-perf/patches/vet-nocopy-marker.patch
for m in $(awk '/^use \(/{f=1; next} /^\)/{f=0} f {sub(/^[ \t]*\.\//, ""); print}' go.work); do
  (cd $m && go vet -copylocks ./... > /tmp/vet-$(echo $m | tr / -).txt 2>&1)
  echo "$m: $(grep -c 'lock' /tmp/vet-$(echo $m | tr / -).txt) all, $(grep 'lock' /tmp/vet-$(echo $m | tr / -).txt | grep -vc '_test.go') non-test"
done
cat /tmp/vet-*.txt | grep -v '_test.go' | grep lock | cut -d: -f1 | sort | uniq -c | sort -rn | head -40
git apply -R design/handover/interpreter-perf/patches/vet-nocopy-marker.patch
```

## The synthetic representation benchmark

A scratch module outside the repo, so its `go.mod` never joins the
workspace. Point the `replace` at the checkout's `core/go`.

`go.mod`:

```
module rep

go 1.24.7

require github.com/boru-lang/boru/core/go v0.0.0

require github.com/cockroachdb/apd/v3 v3.2.3 // indirect

replace github.com/boru-lang/boru/core/go => /path/to/boru/core/go
```

`rep_test.go` (the first comparison, through the accessor functions):

```go
// Synthetic representation benchmark: the same interpreter-shaped workload
// over (a) a slice of 104-byte core.Value structs copied by value, as today,
// (b) a slice of *core.Value with one heap allocation per minted value, and
// (c) a slice of *core.Value minted from a slab (chunked arena, no per-value
// allocation). The workload per op: splice a body of 8 tokens onto a tape,
// push each as an operand, "dispatch" an add every two operands (read two
// IntPayloads, mint a result, write it back), and drop the results.
package rep

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

const bodyLen = 8

var sinkInt int64

// ---------- (a) by-value, as today ----------

func mintA(n int64) core.Value { return core.NewInteger(n) }

type tapeA struct{ cells []core.Value }

func (t *tapeA) splice(at int, body []core.Value) {
	t.cells = append(t.cells, body...) // grow
	copy(t.cells[at+len(body):], t.cells[at:len(t.cells)-len(body)])
	copy(t.cells[at:], body)
}

func runA(b *testing.B, body []core.Value) {
	t := &tapeA{cells: make([]core.Value, 0, 64)}
	stack := make([]core.Value, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.cells = t.cells[:0]
		t.splice(0, body)
		for p := 0; p < len(t.cells); p++ {
			stack = append(stack, t.cells[p]) // push: 104-byte copy
			if len(stack) >= 2 {
				x, _ := core.AsInteger(stack[len(stack)-1])
				y, _ := core.AsInteger(stack[len(stack)-2])
				stack = stack[:len(stack)-2]
				r := mintA(x + y)
				stack = append(stack, r) // result: 104-byte copy
				t.cells[p] = r           // write back to the tape: 104-byte copy
			}
		}
		for _, v := range stack {
			n, _ := core.AsInteger(v)
			sinkInt += n
		}
		stack = stack[:0]
	}
}

// ---------- (b) pointers, heap per mint ----------

func mintB(n int64) *core.Value { v := core.NewInteger(n); return &v }

type tapeB struct{ cells []*core.Value }

func (t *tapeB) splice(at int, body []*core.Value) {
	t.cells = append(t.cells, body...)
	copy(t.cells[at+len(body):], t.cells[at:len(t.cells)-len(body)])
	copy(t.cells[at:], body)
}

func runB(b *testing.B, body []*core.Value, mint func(int64) *core.Value) {
	t := &tapeB{cells: make([]*core.Value, 0, 64)}
	stack := make([]*core.Value, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.cells = t.cells[:0]
		t.splice(0, body)
		for p := 0; p < len(t.cells); p++ {
			stack = append(stack, t.cells[p]) // push: 8-byte copy
			if len(stack) >= 2 {
				x, _ := core.AsInteger(*stack[len(stack)-1])
				y, _ := core.AsInteger(*stack[len(stack)-2])
				stack = stack[:len(stack)-2]
				r := mint(x + y)
				stack = append(stack, r)
				t.cells[p] = r
			}
		}
		for _, v := range stack {
			n, _ := core.AsInteger(*v)
			sinkInt += n
		}
		stack = stack[:0]
	}
}

// ---------- (c) pointers from a slab ----------

type slab struct {
	chunk []core.Value
	next  int
}

func (s *slab) mint(n int64) *core.Value {
	if s.next == len(s.chunk) {
		s.chunk = make([]core.Value, 1024)
		s.next = 0
	}
	v := &s.chunk[s.next]
	s.next++
	*v = core.NewInteger(n) // one 104-byte store into the slot
	return v
}

func bodyA() []core.Value {
	out := make([]core.Value, bodyLen)
	for i := range out {
		out[i] = core.NewInteger(int64(i))
	}
	return out
}

func bodyB() []*core.Value {
	out := make([]*core.Value, bodyLen)
	for i := range out {
		out[i] = mintB(int64(i))
	}
	return out
}

func BenchmarkByValue(b *testing.B)     { runA(b, bodyA()) }
func BenchmarkPointerHeap(b *testing.B) { runB(b, bodyB(), mintB) }
func BenchmarkPointerSlab(b *testing.B) {
	s := &slab{}
	runB(b, bodyB(), s.mint)
}

// ---------- the raw copy cost in isolation ----------

func BenchmarkCopyValue(b *testing.B) {
	src := bodyA()
	dst := make([]core.Value, bodyLen)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(dst, src)
	}
}

func BenchmarkCopyPointer(b *testing.B) {
	src := bodyB()
	dst := make([]*core.Value, bodyLen)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(dst, src)
	}
}

func BenchmarkMintValue(b *testing.B) {
	var v core.Value
	for i := 0; i < b.N; i++ {
		v = core.NewInteger(int64(i))
	}
	sinkInt += func() int64 { n, _ := core.AsInteger(v); return n }()
}

func BenchmarkMintHeap(b *testing.B) {
	var v *core.Value
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v = mintB(int64(i))
	}
	sinkInt += func() int64 { n, _ := core.AsInteger(*v); return n }()
}
```

`rep2_test.go` (the fair comparison: direct reads, plus the arena and the
32-byte header variants):

```go
package rep

// Fair variants: every representation reads payloads directly (no by-value
// accessor call), so only the representation differs. (d) is a 32-byte
// header, the "shrink instead of pointerise" alternative.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func intOf(v *core.Value) int64 { return v.Data.(core.IntPayload).N }

// (a') by value, direct reads
func BenchmarkByValueDirect(b *testing.B) {
	body := bodyA()
	t := &tapeA{cells: make([]core.Value, 0, 64)}
	stack := make([]core.Value, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.cells = t.cells[:0]
		t.splice(0, body)
		for p := 0; p < len(t.cells); p++ {
			stack = append(stack, t.cells[p])
			if len(stack) >= 2 {
				x := stack[len(stack)-1].Data.(core.IntPayload).N
				y := stack[len(stack)-2].Data.(core.IntPayload).N
				stack = stack[:len(stack)-2]
				r := core.NewInteger(x + y)
				stack = append(stack, r)
				t.cells[p] = r
			}
		}
		for i := range stack {
			sinkInt += stack[i].Data.(core.IntPayload).N
		}
		stack = stack[:0]
	}
}

func runBDirect(b *testing.B, body []*core.Value, mint func(int64) *core.Value) {
	t := &tapeB{cells: make([]*core.Value, 0, 64)}
	stack := make([]*core.Value, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.cells = t.cells[:0]
		t.splice(0, body)
		for p := 0; p < len(t.cells); p++ {
			stack = append(stack, t.cells[p])
			if len(stack) >= 2 {
				x := intOf(stack[len(stack)-1])
				y := intOf(stack[len(stack)-2])
				stack = stack[:len(stack)-2]
				r := mint(x + y)
				stack = append(stack, r)
				t.cells[p] = r
			}
		}
		for _, v := range stack {
			sinkInt += intOf(v)
		}
		stack = stack[:0]
	}
}

// (b') pointers, heap per mint, direct reads
func BenchmarkPointerHeapDirect(b *testing.B) { runBDirect(b, bodyB(), mintB) }

// (c') pointers from a slab, direct reads
func BenchmarkPointerSlabDirect(b *testing.B) {
	s := &slab{}
	runBDirect(b, bodyB(), s.mint)
}

// (c'') pointers from a slab that is RESET per op (an arena freed with the
// frame: the best case for a region allocator, no zeroing of a fresh chunk).
type arena struct {
	chunk []core.Value
	next  int
}

func (a *arena) mint(n int64) *core.Value {
	v := &a.chunk[a.next]
	a.next++
	v.Parent, v.Data = core.TInteger, core.IntPayload{N: n}
	return v
}

func BenchmarkPointerArenaDirect(b *testing.B) {
	a := &arena{chunk: make([]core.Value, 64)}
	body := bodyB()
	t := &tapeB{cells: make([]*core.Value, 0, 64)}
	stack := make([]*core.Value, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.next = 0
		t.cells = t.cells[:0]
		t.splice(0, body)
		for p := 0; p < len(t.cells); p++ {
			stack = append(stack, t.cells[p])
			if len(stack) >= 2 {
				x := intOf(stack[len(stack)-1])
				y := intOf(stack[len(stack)-2])
				stack = stack[:len(stack)-2]
				r := a.mint(x + y)
				stack = append(stack, r)
				t.cells[p] = r
			}
		}
		for _, v := range stack {
			sinkInt += intOf(v)
		}
		stack = stack[:0]
	}
}

// (d) a 32-byte header by value: Parent, Data, one pointer for the rest.
type small struct {
	parent *core.Type
	data   core.Payload
	rest   *core.SrcPos
}

func mintS(n int64) small { return small{parent: core.TInteger, data: core.IntPayload{N: n}} }

func BenchmarkSmallByValueDirect(b *testing.B) {
	body := make([]small, bodyLen)
	for i := range body {
		body[i] = mintS(int64(i))
	}
	cells := make([]small, 0, 64)
	stack := make([]small, 0, 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cells = cells[:0]
		cells = append(cells, body...)
		for p := 0; p < len(cells); p++ {
			stack = append(stack, cells[p])
			if len(stack) >= 2 {
				x := stack[len(stack)-1].data.(core.IntPayload).N
				y := stack[len(stack)-2].data.(core.IntPayload).N
				stack = stack[:len(stack)-2]
				r := mintS(x + y)
				stack = append(stack, r)
				cells[p] = r
			}
		}
		for i := range stack {
			sinkInt += stack[i].data.(core.IntPayload).N
		}
		stack = stack[:0]
	}
}
```

Run with `GOWORK=off GOFLAGS=-mod=mod go test -run '^$' -bench . -benchmem
-count=5 .`. The sizes table came from a `main.go` in a module of the same
shape:

```go
package main

import (
	"fmt"
	"unsafe"

	core "github.com/boru-lang/boru/core/go"
)

func main() {
	fmt.Printf("Value           %4d bytes\n", unsafe.Sizeof(core.Value{}))
	fmt.Printf("ListPayload     %4d bytes (boxed into Data)\n", unsafe.Sizeof(core.ListPayload{}))
	fmt.Printf("MapPayload      %4d bytes\n", unsafe.Sizeof(core.MapPayload{}))
	fmt.Printf("IntPayload      %4d bytes\n", unsafe.Sizeof(core.IntPayload{}))
	fmt.Printf("StrPayload      %4d bytes\n", unsafe.Sizeof(core.StrPayload{}))
	fmt.Printf("FnDefInfo       %4d bytes\n", unsafe.Sizeof(core.FnDefInfo{}))
	fmt.Printf("MarkInfo        %4d bytes\n", unsafe.Sizeof(core.MarkInfo{}))
	fmt.Printf("MoveInfo        %4d bytes\n", unsafe.Sizeof(core.MoveInfo{}))
	fmt.Printf("Loop            %4d bytes\n", unsafe.Sizeof(core.Loop{}))
	fmt.Printf("SrcPos          %4d bytes\n", unsafe.Sizeof(core.SrcPos{}))
	fmt.Printf("CapturedBinding %4d bytes\n", unsafe.Sizeof(core.CapturedBinding{}))
	fmt.Printf("pointer         %4d bytes\n", unsafe.Sizeof(uintptr(0)))
}
```

## Where the other numbers live

- PR #532's description: the twelve fn kinds before and after on the
  interpreter lane (geomean −41%).
- PR #533's description: the literal, `case` and `each` timings and the
  allocation drops.
- Issue #530: the cost breakdown of a native `add` on the compiled lane.
- Issue #531: the first account of the `Value` copy cost.
