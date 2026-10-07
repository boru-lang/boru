package lang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nur347_354_355_test.go pins three closes of 2026-09-29.
//
//   - NUR354: a computed body run in a counted loop resolves the loop's index
//     on the registry, where the interpreter's `for` installs it; the
//     compiled loop kept it in a frame slot alone and the body answered
//     `undefined word: i`. Under the program's dynamic-environment mirror
//     every counted loop now publishes its index per iteration and pops it
//     with the loop (compiler publishesIndex, OpForPublish) — in the calling
//     unit or across a fn call.
//   - NUR355: a break/continue no loop takes is the interpreter's
//     `flow_error: break outside loop`, at the token its pointer rests on,
//     where the compiled runtime raised an internal_error.
//   - NUR347: a named closure read by `/v` into a runtime-built map anchors
//     its contract error at the `h/v` token, as the interpreter's read stamps
//     it (MakeMapSpec.FnPos, and PolyRef.FnArgPos for a poly call's
//     argument).

// TestNUR354ComputedBodySeesLoopIndex: the register's rows and their
// variants agree on both lanes.
func TestNUR354ComputedBodySeesLoopIndex(t *testing.T) {
	const mk = `def mk fn [[][List][quote [i]]] end `
	for _, c := range []struct{ src, want string }{
		// The register's row: [0 1] inside the arity error on both lanes.
		{`def f fn [[b:List][Any][for 2 [do b]]] end f (quote [i])`, "ERROR:got 2 — [0 1]"},
		{`def f fn [[b:List][Any][for 2 [do b] end]] end f (quote [i])`, "ERROR:got 2 — [0 1]"},
		{`def f fn [[b:List][Any][for 2 [do b] 0]] end f (quote [i])`, "ERROR:got 3 — [0 1 0]"},
		{`def f fn [[b:List][Any][for 1 [do b] end]] end f (quote [i])`, "[0]"},
		{`def f fn [[b:List n:Integer][Any][for n [do b] end]] end f (quote [i]) 2`, "ERROR:got 2 — [0 1]"},
		{`def f fn [[b:List][Any][for 1 [for 2 [do b] end] end]] end f (quote [i])`, "ERROR:got 2 — [0 1]"},
		// A body def of the index is the iteration's own binding, visible to
		// the computed body and gone at the next iteration.
		// (The body's own `def i 9` over the loop index — a block-local shadow
		// the compiled lane declines until its block scopes land — is pinned
		// below with ruleOrDecline.)
		// The root, and the loop's exits: the index goes with the loop —
		// exhausted, broken or continued — and the binding beneath shows.
		{mk + `for 2 [do (mk)]`, "[0 1]"},
		{mk + `for 2 [do (mk)] do (mk)`, "[0 1 error(undefined word: i)]"},
		{mk + `def i 5 for 2 [do (mk)] do (mk)`, "[0 1 5]"},
		{mk + `def i 5 for 3 [do (mk) break] do (mk)`, "[5]"},
		{mk + `def i 5 for 3 [do (mk) continue] do (mk)`, "[5]"},
		// Across a fn call: the computed body runs in the callee.
		{mk + `def g fn [[] [Any] [do (mk)]] end for 2 [g]`, "[0 1]"},
		{`def g fn [[b:List][Any][do b]] end def h fn [[b:List][Any][for 2 [g b] end]] end h (quote [i])`, "ERROR:got 2 — [0 1]"},
		{`def g fn [[b:List][Any][do b]] end def h fn [[b:List][Any][for 2 [(g b)] end]] end h (quote [i])`, "ERROR:got 2 — [0 1]"},
		// The register's cross-unit twin's shape, compiled: map-from in a fn a
		// loop in another fn calls.
		{`import "boru:rand"  def g fn [[m:Map][Any][Rand.map-from m]] end def h fn [[m:Map][Any][for 2 [g m] end]] end h {a:(quote [i])}`, "ERROR:got 2 — [{a:0} {a:1}]"},
		// Negative: a body that reads no index reads the binding it names,
		// and a body that reads nothing reads nothing.
		{`def f fn [[b:List][Any][for 2 [do b] end]] end f (quote [5])`, "ERROR:got 2 — [5 5]"},
		{`def f fn [[b:List][Any][each [7 8] [do b] end]] end f (quote [i])`, "[[8]]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
	const mkI = `def mk fn [[][List][quote [i]]] end `
	ruleOrDecline(t, `def f fn [[b:List][Any][for 1 [def i 9 do b] end]] end f (quote [i])`, "[9]")
	ruleOrDecline(t, mkI+`def i 5 for 2 [def i 9 do (mk)] do (mk)`, "[9 9 5]")
}

// TestRuntimeStampReadsThroughCallees: a run-time stamp's dependency
// snapshot reads the user fns its body calls, transitively — a callee's unit
// is compiled against the bindings live at the stamp, baking what it reads,
// so a rebind of a name only the callee reads stales the stamp. The shallow
// snapshot answered the first binding forever (found closing NUR354;
// pre-existing at e73568c: `[{a:9} {a:9}]` and `[9 9]`).
func TestRuntimeStampReadsThroughCallees(t *testing.T) {
	const f = `def i 9 end def f fn [[] [Integer] [i]] end `
	for _, c := range []struct{ src, want string }{
		{`import "boru:rand"  ` + f + `def s {a:(quote [f])} end Rand.map-from s def i 10 end Rand.map-from s`, "[{a:9} {a:10}]"},
		{f + `def mk fn [[][List][quote [f]]] end for 2 [do (mk)]`, "[0 1]"},
		{f + `def g fn [[x:List][Any][for 2 [do x] end]] end g (quote [f])`, "ERROR:got 2 — [0 1]"},
		// Negative: a callee reading no rebound name keeps its stamp's answer.
		{`import "boru:rand"  ` + f + `def s {a:(quote [f])} end Rand.map-from s def j 10 end Rand.map-from s`, "[{a:9} {a:9}]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// flowErrorOnBothLanes asserts src compiles and raises the interpreter's
// `outside loop` flow_error byte-identically on both lanes, pointing at at
// ("source position unknown" for none).
func flowErrorOnBothLanes(t *testing.T, src, detail, at string) {
	t.Helper()
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Errorf("%s: must compile, got %v", src, errC)
		return
	}
	if errC == nil || errI == nil || errC.Error() != errI.Error() {
		t.Errorf("%s:\n  compiled %v / %v\n  interp   %v / %v", src, gotC, errC, gotI, errI)
		return
	}
	msg := errI.Error()
	if !strings.Contains(msg, "[boru/flow_error]: "+detail) {
		t.Errorf("%s: want flow_error %q, got %v", src, detail, msg)
	}
	if !strings.Contains(msg, "--> "+at+"\n") && !strings.HasSuffix(msg, "--> "+at) {
		t.Errorf("%s: want the report at %s, got %v", src, at, msg)
	}
}

// TestNUR355FlowOutsideLoop: a break/continue no loop takes raises the
// interpreter's flow_error where its pointer rests after the step — the
// token after the `break` in its sequence (a fn body, a paren, an arm), the
// first value a body-running word left (`do`'s run: a hosted body's own
// values, then its unstepped rest), the token after that word's run — and
// nowhere a frame's tail or a group's close follows.
func TestNUR355FlowOutsideLoop(t *testing.T) {
	const unknown = "source position unknown"
	const doB = `def f fn [[b:List][Any][do b]] end `
	for _, c := range []struct{ src, detail, at string }{
		// The register's rows.
		{`do (quote [1 break]) 5`, "break outside loop", "1:12"},
		{`def f fn [[] [Any] [break]] end f 9`, "break outside loop", unknown},
		// continue, and the direct break in a fn body.
		{`do (quote [continue]) 5`, "continue outside loop", "1:23"},
		{`def f fn [[] [Any] [continue]] end f 9`, "continue outside loop", unknown},
		{`def f fn [[] [Any] [break 1]] end f 9`, "break outside loop", "1:27"},
		{`def f fn [[] [Any] [(break 3) 4]] end f`, "break outside loop", "1:28"},
		{`def f fn [[] [Any] [(break) 4]] end f`, "break outside loop", unknown},
		{`def f fn [[] [Any] [if true [break 2] [0] 5]] end f`, "break outside loop", "1:36"},
		{`def f fn [[x:Integer] [Any] [if (x gt 0) [continue 3] [0]]] end f 1`, "continue outside loop", "1:52"},
		{`def f fn [[] [Any] [break]] end def g fn [[] [Any] [f 7]] end g`, "break outside loop", unknown},
		// A computed body's escape through `do`: its run, else the token
		// after the `do`'s run.
		{`def b (quote [break]) end do b`, "break outside loop", unknown},
		{`def b (quote [continue]) end do b 7`, "continue outside loop", "1:35"},
		{doB + `f (quote [break 2])`, "break outside loop", "1:52"},
		{doB + `f (quote [1 2 break])`, "break outside loop", "1:46"},
		{doB + `f (quote [1 add 2 break])`, "break outside loop", unknown},
		{doB + `f (quote [(break 3) 4])`, "break outside loop", unknown},
		{doB + `f (quote [5 (break 3)])`, "break outside loop", "1:46"},
		{doB + `f (quote [if true [break 3] [0] 4])`, "break outside loop", unknown},
		{`def f fn [[b:List][Any][(do b)]] end f (quote [1 break])`, "break outside loop", "1:48"},
		{`def mk fn [[][List][quote [1 break]]] end do (mk)`, "break outside loop", "1:28"},
		{`def mk fn [[][List][quote [break]]] end do (mk) 4`, "break outside loop", "1:49"},
		{`def mk fn [[][List][quote [break]]] end def g fn [[] [Any] [do (mk)]] end g`, "break outside loop", unknown},
		// A fn frame the hosted body opened leads its rest; its own values
		// come first.
		{`def f fn [[] [Any] [break]] end def g fn [[b:List][Any][do b]] end g (quote [f 4])`, "break outside loop", unknown},
		{`def f fn [[] [Any] [9 break]] end def g fn [[b:List][Any][do b]] end g (quote [f 4])`, "break outside loop", unknown},
		{`def f fn [[] [Any] [break]] end def g fn [[b:List][Any][do b]] end g (quote [8 f 4])`, "break outside loop", "1:78"},
		// An iterating word ends on the escape with its own result.
		{`def f fn [[] [Any] [each [1 2] [break] 5]] end f`, "break outside loop", unknown},
		{`def f fn [[] [Any] [each [1 2] [continue]]] end f`, "continue outside loop", unknown},
		{`import "boru:rand"  def g fn [[m:Map][Any][Rand.map-from m]] end g {a:(quote [1 break])}`, "break outside loop", unknown},
		// A MODULE fn's call is a boundary: the interpreter runs the fn on an
		// engine of its own, so no loop outside the call takes the signal,
		// and a report positionless inside takes the calling word's position
		// (the compiled loop took it: `for 3 [L.g 1] 7` answered [7]).
		{`import module [def g fn [[] [Any] [break]] export "L" {g: g/v}] end L.g 5`, "break outside loop", "1:69"},
		{`import module [def g fn [[] [Any] [break 1]] export "L" {g: g/v}] end L.g 5`, "break outside loop", "1:42"},
		{`import module [def g fn [[n:Integer] [Any] [break]] export "L" {g: g/v}] end for 3 [L.g 1] 7`, "break outside loop", "1:85"},
		{`import module [def g fn [[n:Integer] [Any] [continue]] export "L" {g: g/v}] end def w fn [[] [Any] [for 3 [L.g 1] end 7]] end w`, "continue outside loop", "1:108"},
		{`import module [def h fn [[] [Any] [break]] def g fn [[] [Any] [h 2]] export "L" {g: g/v}] end L.g 5`, "break outside loop", "1:95"},
		{`import module [def g fn [[b:List] [Any] [do b]] export "L" {g: g/v}] end L.g (quote [1 break])`, "break outside loop", "1:86"},
		{`import module [def useanon fn [[Function Integer] [Integer] [(args.0 args.1)]] export "L" {useanon: useanon/v, brk: (fn [[x:Integer] [Any] [break]])}] end for 3 [L.useanon L.brk 1] 7`, "break outside loop", "1:163"},
		// A computed run's fn value the mark window's island re-steps: its
		// escape was left set and dropped, `[]` with no error (pre-existing
		// at e73568c).
		{`def f fn [[] [Any] [break]] end def mk fn [[][List][quote [f/v]]] end do (mk)`, "break outside loop", unknown},
		{`def f fn [[] [Any] [continue]] end def mk fn [[][List][quote [f/v]]] end do (mk)`, "continue outside loop", unknown},
		{`def f fn [[] [Any] [break 2]] end def mk fn [[][List][quote [f/v]]] end do (mk)`, "break outside loop", "1:27"},
	} {
		flowErrorOnBothLanes(t, c.src, c.detail, c.at)
	}
	// Negative: a loop open anywhere takes the signal on both lanes.
	for _, c := range []struct{ src, want string }{
		{`def mk fn [[][List][quote [break]]] end for 3 [do (mk)] 7`, "[7]"},
		{`def f fn [[] [Any] [break]] end for 3 [f] 7`, "[7]"},
		{`def f fn [[b:List][Any][for 3 [do b] 7]] end f (quote [break])`, "[7]"},
		{`def f fn [[b:List][Any][for 3 [do b] end 7]] end f (quote [continue])`, "[7]"},
		// Inside the module fn its own loop takes it, and a signal raised
		// outside the call is the caller's loop's.
		{`import module [def g fn [[] [Any] [for 3 [break] 1]] export "L" {g: g/v}] end L.g`, "[1]"},
		{`import module [def g fn [[n:Integer] [Any] [n]] export "L" {g: g/v}] end for 3 [L.g 1 break] 7`, "[7]"},
		// A re-stepped fn value that raises no signal leaves its value.
		{`def f fn [[] [Any] [3]] end def mk fn [[][List][quote [f/v]]] end do (mk)`, "[3]"},
	} {
		agreeOnBothLanes(t, c.src, c.want)
	}
}

// TestNUR347ClosureInRuntimeMapAnchor: a named closure stored by `/v` in a
// map built at run time answers its contract error at the `h/v` token — the
// map's assembly stamps the read's position (MakeMapSpec.FnPos) — and one
// read back into a binding and passed by `/v` to a poly-dispatched word
// answers at that read (PolyRef.FnArgPos).
func TestNUR347ClosureInRuntimeMapAnchor(t *testing.T) {
	const mk = `def mk fn [[k:Integer] [Function] [(fn [[a:Integer][String] [a add k]])]] end def h (mk 1) end `
	for _, c := range []struct{ src, name, at string }{
		// The register's row.
		{mk + `def m {f: h/v} end each m.f [5]`, "h", "1:106"},
		{mk + `def m {f: h/v} each m.f [5]`, "h", "1:106"},
		{mk + `def m {g: 1 f: h/v} end each m.f [5]`, "h", "1:111"},
		{mk + `each {f: h/v}.f [5]`, "h", "1:105"},
		{mk + `def m {f: h/v} end (m.f 5)`, "h", "1:106"},
		{mk + `def m {f: h/v} end fold m.f [5] 0`, "h", "1:106"},
		{mk + `def w fn [[] [Any] [def m {f: h/v} end each m.f [5]]] end w`, "h", "1:126"},
		{`def mk fn [[k:Integer] [Function] [(fn [[a:Integer][String] [a add k]])]] end def w fn [[] [Any] [def h (mk 1) end def m {f: h/v} end each m.f [5]]] end w`, "h", "1:126"},
		{`def h fn [[a:Integer][String] [a]] end def m {f: h/v} end each m.f [5]`, "h", "1:50"},
		// A lambda closure answers at its own anchor, a map read or not.
		{`def mk fn [[k:Integer] [Function] [([a:Integer] => [a k])]] end def h (mk 1) end def m {f: h/v} end each m.f [5]`, "h", "1:92"},
		// A binding of the member read, passed by `/v` to a poly word.
		{mk + `def m {f: h/v} end def g m.f end each g/v [5]`, "g", "1:134"},
	} {
		anchoredOnBothLanes(t, c.src, c.name, c.at)
	}
}

// TestNUR354PublishedIndexLeavesNoTrail pins the review of #524's first
// finding: a published loop's dyn-bind trail entry guards the index only
// while the loop runs. It stayed on the trail past the loop's exit, and a
// raise after the loop replayed it over the name's later binding, so the
// next run on the same Boru (a REPL's continuation) found no `i` where the
// interpreter keeps 7. The exit — exhausted or broken, a nested loop's too —
// now retires the entry and the body's re-publishes of the index; a raise
// INSIDE the loop still pops what the loop installed.
func TestNUR354PublishedIndexLeavesNoTrail(t *testing.T) {
	const mk = `def mk fn [[][List][quote [i]]] end `
	for _, c := range []struct{ first, then, want string }{
		{mk + `for 1 [do (mk)] end def i 7 end raise 'x'`, `i`, "[7]"},
		{mk + `for 1 [(do (mk))] end def i 7 end raise 'x'`, `i`, "[7]"},
		{mk + `for 2 [def j 9 do (mk)] end def i 7 end raise 'x'`, `i`, "[7]"},
		{mk + `def i 5 end for 2 [def j 9 do (mk)] end def i 7 end def i 8 end raise 'x'`, `i`, "[8]"},
		{mk + `for 2 [for 2 [do (mk)] end] end def i 7 end raise 'x'`, `i`, "[7]"},
		{mk + `for 3 [do (mk) break] end def i 7 end raise 'x'`, `i`, "[7]"},
		{mk + `def i 5 end for 2 [for 3 [do (mk) break] end] end def i 7 end raise 'x'`, `i`, "[7]"},
		// A raise inside the loop: the loop's install goes with it.
		{mk + `def i 5 end for 2 [do (mk) (raise 'x')] end`, `i`, "[5]"},
		{mk + `def i 5 end for 2 [for 2 [do (mk) (raise 'x')] end] end`, `i`, "[5]"},
		// Negative: no loop, no trail — the later binding stands.
		{`def i 7 end raise 'x'`, `i`, "[7]"},
	} {
		c1, i1 := mustNew(t), mustNew(t)
		firstC, compiled, errC := c1.RunCompiled(c.first)
		firstI, errI := i1.RunInterp(c.first)
		if !compiled || errI == nil || fmt.Sprint(firstC, errC) != fmt.Sprint(firstI, errI) {
			t.Errorf("%s: compiled %v / %v (compiled=%v), interp %v / %v", c.first, firstC, errC, compiled, firstI, errI)
			continue
		}
		gotC, compiled, errC := c1.RunCompiled(c.then)
		gotI, errI := i1.RunInterp(c.then)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != c.want || fmt.Sprint(gotI) != c.want {
			t.Errorf("%s, then %s: compiled %v / %v (compiled=%v), interp %v / %v, want %s", c.first, c.then, gotC, errC, compiled, gotI, errI, c.want)
		}
	}
}

// TestNUR355FileModuleFlowSource pins the review of #524's second finding: a
// break/continue that leaves a FILE-backed module's fn is the report of the
// module's own engine, rendered against the module's source — at its own
// token (`break 1`'s `1`), or at the calling word's position where it has
// none, as the interpreter's report stands. The compiled report took the
// importing program's source and rendered the module's position over the
// import line.
func TestNUR355FileModuleFlowSource(t *testing.T) {
	const g = "export \"Lib\" {g: g/v}\n"
	for _, c := range []struct{ lib, src, at string }{
		{"def g fn [[] [Any] [break 1]]\n" + g, `import "./lib.boru" Lib.g 5`, "1:27"},
		{"def g fn [[] [Any] [break]]\n" + g, `import "./lib.boru" Lib.g 5`, "1:21"},
		{"def g fn [[b:List] [Any] [do b]]\n" + g, `import "./lib.boru" Lib.g (quote [1 break])`, "1:35"},
		{"def mk fn [[] [List] [quote [7 break]]]\ndef g fn [[] [Any] [do (mk)]]\n" + g, `import "./lib.boru" Lib.g`, "1:30"},
		{"def h fn [[] [Any] [continue 9]]\ndef g fn [[] [Any] [h 2]]\n" + g, `import "./lib.boru" Lib.g`, "1:30"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "lib.boru"), []byte(c.lib), 0o644); err != nil {
			t.Fatal(err)
		}
		lane := func() *Boru {
			a, err := New(Options{BaseDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			return a
		}
		gotC, compiled, errC := lane().RunCompiled(c.src)
		gotI, errI := lane().RunInterp(c.src)
		if !compiled || errC == nil || errI == nil || errC.Error() != errI.Error() {
			t.Errorf("%s over %q:\n  compiled %v / %v (compiled=%v)\n  interp   %v / %v", c.src, c.lib, gotC, errC, compiled, gotI, errI)
			continue
		}
		msg := errI.Error()
		first, _, _ := strings.Cut(c.lib, "\n")
		if !strings.Contains(msg, "flow_error") || !strings.Contains(msg, "--> "+c.at+"\n") || !strings.Contains(msg, "1 | "+first) {
			t.Errorf("%s: want the report at %s over the module's first line, got %v", c.src, c.at, msg)
		}
	}
}
