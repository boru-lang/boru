package lang

import (
	"fmt"
	"strings"
	"testing"
)

// EXPLANATION.md — "Mutable side effects within a branch are local to that
// branch's sub-engine." HOWTO.md — "Each branch runs in a sub-engine, so
// writes to mutable objects inside one branch do not bleed into the
// others." Both were false for exactly the containers they are about.
//
// ForkConcurrent isolates execution SCOPES, and its header says so
// precisely: it clones Defs, Types, builtinWords, Contexts and Args, and
// "its later writes stay private" means writes to those scopes — `def`,
// `context set`. But cloning a binding table copies Values, and a FlexMap
// / FlexList / Store / Table / class-instance Value holds a POINTER to
// shared state. Nothing deep-copied the payload, so an in-place mutation
// was visible to every sibling branch and to the parent. The Go-level
// comment was accurate; the user-facing prose generalised from it.
//
// This is undefined behaviour, not merely a surprise: OrderedMap.Set is an
// unsynchronised map assign plus a slice append, and `make test-race` runs
// this whole package under the detector.
//
// The resolution is REFUSAL, not a silent deep copy: sharing a stateful
// container across concurrent branches is rejected at the boundary, the
// same answer `send` gives at a process boundary. Cloning would make these
// programs work silently, but it hides the sharing the author wrote and
// pays a per-branch copy for every container in scope. Refusing states the
// rule once, at the point where it is violated.

const awaitPrelude = "import \"boru:time-util\"\n"

// TestAwaitRefusesSharedMutableContainer covers every way a branch reaches
// a stateful container, in BOTH engine modes. The compiled shape matters
// on its own: a branch body arrives as a synthetic fn-value carrier rather
// than a token list, and an earlier version of this check walked only the
// list — leaving the boundary unguarded on the default path, which is the
// only path most programs take.
func TestAwaitRefusesSharedMutableContainer(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Named directly in the branch body.
		{"FlexMap by def", awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] end`, "m"},
		// Reached through a lambda. `m` is module-level, so nothing is
		// CAPTURED — the reference is dynamic and lives only in the
		// lambda's body, which is why the walk has to follow into it.
		// The refusal names `m`, not `w1`: the container is what the
		// author has to change, and the lambda is only how it was reached.
		{"FlexMap through a lambda", awaitPrelude + `def m (make FlexMap {})
def w1 ([] => [m set a 1])
def w2 ([] => [m set b 2])
TimeUtil.await [[w1] [w2]] end`, "m"},
		// Nested inside an immutable Map: the container is still shared,
		// so the enclosing binding is refused.
		{"FlexMap nested in a plain Map", awaitPrelude + `def box (make FlexMap {})
def holder {nested: box}
TimeUtil.await [[(holder dot nested) set a 1] [(holder dot nested) set b 2]] end`, "holder"},
		// FlexList is the same hazard with a different payload.
		{"FlexList by def", awaitPrelude + `def l (flex [0])
TimeUtil.await [[l set 0 1] [l set 0 2]] end`, "l"},
	}
	for _, c := range cases {
		for _, mode := range []string{"compiled", "interpreted"} {
			t.Run(c.name+"/"+mode, func(t *testing.T) {
				a, _ := New()
				var err error
				if mode == "compiled" {
					_, err = a.Run(c.src)
				} else {
					_, err = a.RunInterp(c.src)
				}
				if err == nil {
					t.Fatalf("sharing a stateful container across branches must "+
						"be refused, got no error for:\n%s", c.src)
				}
				msg := fmt.Sprint(err)
				if !strings.Contains(msg, "not_sendable") {
					t.Errorf("want a not_sendable refusal, got: %s", msg)
				}
				if !strings.Contains(msg, "`"+c.want+"`") {
					t.Errorf("the refusal must NAME the offending binding (%q) so "+
						"the author knows which one to change, got: %s", c.want, msg)
				}
			})
		}
	}
}

// TestAwaitAcceptsImmutableAndScalarSharing is the negative half, and it
// carries most of the weight: a refusal rule that over-fires is worse than
// the bug, because it breaks working programs. An immutable Map's `set`
// returns a copy, so it was never the hazard and must stay legal.
func TestAwaitAcceptsImmutableAndScalarSharing(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"immutable Map", awaitPrelude + `def m {}
TimeUtil.await [[m set a 1] [m set b 2]] end
m`, "[[{a:1} {b:2}] {}]"},
		{"scalars and strings", awaitPrelude + `def n 40
def s "x"
TimeUtil.await [[n add 1] [n add 2] [s]] end`, "[[41 42 'x']]"},
		{"plain List", awaitPrelude + `def xs [1 2 3]
TimeUtil.await [[xs size] [xs size]] end`, "[[3 3]]"},
		// A RECURSIVE fn must not send the reachability walk into a loop.
		{"recursive fn reference", awaitPrelude +
			`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]
TimeUtil.await [[fact 5] [fact 6]] end`, "[[120 720]]"},
		// A container built INSIDE a branch is private to it by
		// construction — the documented way to use one concurrently.
		{"FlexMap built inside each branch", awaitPrelude +
			`TimeUtil.await [[def a (make FlexMap {}) a set k 1] [def b (make FlexMap {}) b set k 2]] end`,
			"[[{k:1} {k:2}]]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.Run(c.src)
			if err != nil {
				t.Fatalf("must remain legal: %v", err)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}

// TestAwaitDoesNotLowerFnLocalLambdaInBothEngines closes what was this check's
// worst hole, and the shape of the hole is why it is worth a long comment.
//
// The check resolves branch-body words through `r.Defs`. Compiled, a
// lambda defined INSIDE a fn body (`def w ([] => …)`) is placed in a VM
// frame slot instead, and a native handler has no path to frame locals —
// so the walk dead-ended at `w` and never reached what `w` touches. It was
// not a uniform miss, which is what made it dangerous:
//
//	lambda body     compiled (before)   interpreted
//	[m]             refused             refused
//	[m get a]       refused             refused
//	[m size]        ALLOWED             refused
//	[m set a 1]     ALLOWED             refused   <-- real DATA RACE
//
// The compiled path refused the harmless shapes and permitted the mutating
// one — under `-race`, two branch goroutines in `OrderedMap.Set` at once,
// on the path that ships by default.
//
// The fix does NOT resolve `w` (it cannot: `m` and `w` are runtime values
// in a frame this layer cannot see). It observes instead that `m` was in
// `r.Defs` the whole time, one step out of reach, and that an unresolvable
// reference means the check can no longer BOUND what a branch touches. So
// an opaque reference falls back to asking whether anything mutable is in
// scope at all. See branchRefIsOpaque / scopeSharedMutable.
//
// All four shapes now refuse in both engines. If any row here starts
// passing, the walk has regressed to the state that raced.
func TestAwaitDoesNotLowerFnLocalLambdaInBothEngines(t *testing.T) {
	for _, body := range []string{"[m]", "[m get a]", "[m size]", "[m set a 1]"} {
		t.Run(body, func(t *testing.T) {
			src := awaitPrelude + `def outer fn [[] [Any] [
  def m (make FlexMap {a:0})
  def w ([] => ` + body + `)
  TimeUtil.await [[w] [w]]
]]
outer`
			for _, mode := range []string{"compiled", "interpreted"} {
				a, _ := New()
				var err error
				if mode == "compiled" {
					_, err = a.Run(src)
				} else {
					_, err = a.RunInterp(src)
				}
				if err == nil {
					t.Errorf("%s: a fn-local lambda reaching a shared FlexMap must "+
						"be stopped; %s ALLOWED it, which is the data race this "+
						"check exists to stop", mode, mode)
				} else if mode == "compiled" && noteCompileDefect(t, src, nil, err) {
					// The program does not compile, so the race check never
					// runs on this lane. The interpreter arm still pins it.
				} else if !strings.Contains(fmt.Sprint(err), "not_sendable") {
					t.Errorf("%s: want not_sendable, got %v", mode, err)
				}
			}
		})
	}
}

// TestAwaitOpaqueRefIsNotRefusedWithoutAMutable is the negative half of the
// fix above, and it carries the weight: the fallback fires on an
// UNRESOLVABLE reference, and unresolvable references are everywhere.
//
// A map key walks as a bare Word (`m set a 1` yields `a`), and a
// branch-local `def` is unresolvable by construction. Refusing on either
// would reject the two idioms the docs actively promote. What makes the
// rule safe is that it only refuses when something mutable is actually in
// scope for the opaque reference to have reached.
func TestAwaitOpaqueRefIsNotRefusedWithoutAMutable(t *testing.T) {
	cases := []struct{ name, src string }{
		// `a` and `b` are map KEYS, not bindings — opaque, and harmless
		// because `m` is an immutable Map.
		{"immutable map keys", awaitPrelude + `def m {}
TimeUtil.await [[m set a 1] [m set b 2]] end`},
		// `a` is bound INSIDE the branch: private by construction, and the
		// documented way to use a container concurrently.
		{"branch-local container", awaitPrelude +
			`TimeUtil.await [[def a (make FlexMap {}) a set k 1] [def b (make FlexMap {}) b set k 2]] end`},
		// A fn-local lambda that touches nothing mutable, with nothing
		// mutable in scope: opaque, and still legal.
		{"fn-local lambda, nothing mutable in scope", awaitPrelude + `def outer fn [[] [Any] [
  def w ([] => [1 add 2])
  TimeUtil.await [[w] [w]]
]]
outer`},
		// The literal case, and the one with a mutable ACTUALLY in scope —
		// so it is the fallback firing or not firing, not the guard being
		// vacuous. `true` / `false` reach the walk as bare Words and never
		// resolve in r.Defs (measured), so without the literal exemption in
		// branchRefIsOpaque this program goes opaque, finds `m`, and refuses
		// two branches that touch nothing at all.
		{"bare literals with a FlexMap in scope", awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[true] [false]] end`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			if _, err := a.Run(c.src); err != nil {
				t.Errorf("must remain legal — an unresolvable reference is only a "+
					"hazard when there is something mutable for it to reach: %v", err)
			}
		})
	}
}

// TestAwaitRefusalAppliesToEveryMode pins the gate on all four combinator
// modes, not just the default. They are four separate entry points that
// each fork independently, so a check wired into only one of them would
// leave the other three open — the same "one seam of several" shape that
// made the compiled branch case ship inert.
func TestAwaitRefusalAppliesToEveryMode(t *testing.T) {
	for _, mode := range []string{`"all"`, `"full"`, `"first"`, `"any"`} {
		t.Run(mode, func(t *testing.T) {
			src := awaitPrelude + `def m (make FlexMap {})
TimeUtil.await {mode: ` + mode + `} [[m set a 1] [m set b 2]] end`
			a, _ := New()
			if _, err := a.Run(src); err == nil {
				t.Errorf("mode %s must refuse a shared mutable container", mode)
			} else if !strings.Contains(fmt.Sprint(err), "not_sendable") {
				t.Errorf("mode %s: want not_sendable, got %v", mode, err)
			}
		})
	}
}

// TestAwaitRefusalNamesTheContainerKind pins the message content. A
// refusal that says only "something is wrong" costs the author the
// debugging session the check was meant to save, so the kind and the
// remedy are part of the contract.
func TestAwaitRefusalNamesTheContainerKind(t *testing.T) {
	a, _ := New()
	_, err := a.Run(awaitPrelude + `def m (make FlexMap {})
TimeUtil.await [[m set a 1] [m set b 2]] end`)
	if err == nil {
		t.Fatal("want a refusal")
	}
	msg := fmt.Sprint(err)
	for _, frag := range []string{"FlexMap", "concurrent branches"} {
		if !strings.Contains(msg, frag) {
			t.Errorf("refusal should mention %q, got: %s", frag, msg)
		}
	}
}
