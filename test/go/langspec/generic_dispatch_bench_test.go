package langspec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	eng "github.com/boru-lang/boru/eng/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// The generic-dispatch benchmark: where compiled programs still re-match
// overloads at run time (CALL_NATIVE_POLY) or read opaque values, and what
// the guarded fast paths buy. Off by default — set BORU_BENCH=1:
//
//	BORU_BENCH=1 go test ./test/go/langspec -run TestGenericDispatchBench -v
//
// BORU_BENCH_REPS (default 3) sets the repetitions (the minimum is
// reported); BORU_BENCH_ONLY=substr restricts the cases. Every micro
// program is also run on the interpreter and must answer the same; every
// suite must report a clean Test.summary. The table is the measurement,
// not a gate: nothing asserts a time.

// benchMicro is one micro program, one per shape the review of discarded
// type information ranked.
type benchMicro struct{ name, src string }

var benchMicros = []benchMicro{
	{"map-param-field", `def f fn [[m:Map][Integer][m.a add m.b]]  def v {a:1 b:2}  0 fold [drop (f v) add] (range 0 20000)`},
	{"list-param-get", `def f fn [[xs:List][Integer][(xs get 0) add (xs get 1)]]  def l [1 2 3]  0 fold [drop (f l) add] (range 0 20000)`},
	{"record-param-field", `def R {a:Integer b:Integer}  def f fn [[m:R][Integer][m.a add m.b]]  def v {a:1 b:2}  0 fold [drop (f v) add] (range 0 20000)`},
	{"is-not-chain", `def f fn [[m:Map][Integer][if ((m.a is None) not) [1] [0]]]  def v {a:1}  0 fold [drop (f v) add] (range 0 20000)`},
	{"any-return", `def g fn [[x:Integer][Any][x add 1]]  0 fold [drop ((g 3) mul 2) add] (range 0 20000)`},
	{"list-return", `def mk fn [[n:Integer][List][[n (n add 1)]]]  0 fold [drop ((mk 3) get 0) add] (range 0 20000)`},
	{"each-lambda", `def xs [1 2 3]  0 fold [drop ((each (x:Integer => [x mul 2]) xs) get 0) add] (range 0 5000)`},
	{"lambda-recursion", `def s ([n:Integer] => [if (n lte 0) [0] [(s (n sub 1)) add n]])  0 fold [drop (s 50) add] (range 0 400)`},
	{"fn-param", `def inc fn [[x:Integer][Integer][x add 1]]  def h fn [[g:Function n:Integer][Integer][0 fold [drop g] (range 0 n)]]  h inc/v 20000`},
}

// benchSuites are the real test suites the census measured (utils).
var benchSuites = []string{
	"utils/tests/cat_test.boru", "utils/tests/cut_test.boru", "utils/tests/grep_test.boru",
	"utils/tests/head_test.boru", "utils/tests/seq_test.boru", "utils/tests/sort_test.boru",
	"utils/tests/uniq_test.boru",
}

func benchInstance(base string) *lang.Boru {
	a, err := lang.New()
	if err != nil {
		panic(err)
	}
	a.SetClock(specClock)
	a.SetOutput(io.Discard)
	a.NativeRegistry().ErrOutput = io.Discard
	if base != "" {
		a.NativeRegistry().BaseDir = base
	}
	return a
}

// benchRun compiles src on a fresh instance and times the RUN alone
// (eng.RunProgram — compilation is reported separately, it is not what the
// guarded fast paths change); it returns the result, the error and, for a
// suite, its Test.summary.
func benchRun(src, base string) (time.Duration, time.Duration, string, error, string) {
	a := benchInstance(base)
	defer a.ArmRuntimeStamping()()
	c0 := time.Now()
	prog, reason, _, err := a.CompileCheck(src)
	compile := time.Since(c0)
	if err != nil || prog == nil {
		return 0, compile, "", fmt.Errorf("not compiled: %s %v", reason, err), ""
	}
	t0 := time.Now()
	res, err := eng.RunProgram(prog, a.NativeRegistry())
	d := time.Since(t0)
	sum := ""
	if base != "" {
		if v, e := a.RunInterpValues("Test.summary"); e == nil {
			sum = fmt.Sprint(v)
		}
	}
	return d, compile, fmt.Sprint(res), err, sum
}

func TestGenericDispatchBench(t *testing.T) {
	if os.Getenv("BORU_BENCH") == "" {
		t.Skip("set BORU_BENCH=1")
	}
	reps := 3
	if n, err := strconv.Atoi(os.Getenv("BORU_BENCH_REPS")); err == nil && n > 0 {
		reps = n
	}
	only := os.Getenv("BORU_BENCH_ONLY")
	type row struct {
		name          string
		best, compile time.Duration
		note          string
	}
	var rows []row
	measure := func(name, src, base string, check func(res string, err error, sum string) string) {
		if only != "" && !strings.Contains(name, only) {
			return
		}
		var best, compile time.Duration
		note := ""
		for i := 0; i < reps; i++ {
			d, c, res, err, sum := benchRun(src, base)
			if i == 0 {
				note = check(res, err, sum)
			}
			if i == 0 || d < best {
				best = d
			}
			if i == 0 || c < compile {
				compile = c
			}
		}
		rows = append(rows, row{name, best, compile, note})
	}
	for _, m := range benchMicros {
		want, werr := benchInstance("").RunInterp(m.src)
		measure(m.name, m.src, "", func(res string, err error, _ string) string {
			if fmt.Sprint(err) != fmt.Sprint(werr) || res != fmt.Sprint(want) {
				t.Errorf("%s: compiled %s [%v], interpreted %v [%v]", m.name, res, err, want, werr)
				return "DIVERGES"
			}
			return res
		})
	}
	repo := filepath.Join("..", "..", "..")
	for _, rel := range benchSuites {
		f := filepath.Join(repo, rel)
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		measure(rel, src, importBaseDir(repo, f, src), func(_ string, err error, sum string) string {
			if err != nil || !strings.Contains(sum, "failed:0") {
				t.Errorf("%s: err=%v summary=%s", rel, err, sum)
			}
			return sum
		})
	}
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%-30s %11s %11s  %s\n", "case", "run", "compile", "result")
	var total, totalC time.Duration
	for _, r := range rows {
		total += r.best
		totalC += r.compile
		fmt.Fprintf(&sb, "%-30s %9.2fms %9.2fms  %s\n", r.name, ms(r.best), ms(r.compile), firstN(r.note, 44))
	}
	fmt.Fprintf(&sb, "%-30s %9.2fms %9.2fms\n", "TOTAL", ms(total), ms(totalC))
	t.Log(sb.String())
	if out := os.Getenv("BORU_BENCH_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(sb.String()), 0o644)
	}
}
