package lang_test

import (
	"fmt"
	"testing"
	"time"

	lang "github.com/boru-lang/boru/lang/go"
)

// A timeout body that applies a fn MINTED ON THE PARENT registry runs on
// the timer's own fork, never on the parent the foreground keeps running.
// The seam is FnHome (NUR152): a callback is at home on any fork of its
// module, so it stays where the timer fired it. Before NUR152 the home was
// compared by pointer and a parent-minted callback was sent back to the
// shared parent from the fork — the `concurrent map iteration and map
// write` the suite died on twice (a timer body under a check pass, a
// serve-raw handler). This pins the fence under the race detector: the
// test-race lane runs this package with -race.
func TestTimeoutBodyAppliesParentFnOnItsFork(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	// Three timers, each applying a parent-minted fn (a verbose fn and a
	// lambda) and defining a name of its own — every kind of registry write
	// a body can make.
	src := `import "boru:time-util"
def f fn [[n:Integer][Integer][n add 1]] end
def g ([n:Integer] => [n mul 2])
TimeUtil.timeout 5 [def x (f 1)  def y (g 2)  x add y]
TimeUtil.timeout 10 [f 3]
TimeUtil.timeout 15 [def z (f 4)  z]
7`
	if _, err := a.Run(src); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	// The foreground keeps the parent registry busy while the timers fire.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		vals, err := a.Run("def w (f 5)  w mul 7")
		if err != nil {
			t.Fatalf("foreground run: %v", err)
		}
		if len(vals) != 1 || fmt.Sprint(vals[0]) != "42" {
			t.Fatalf("foreground corrupted while the timer bodies ran: %v, want 42", vals)
		}
	}
}
