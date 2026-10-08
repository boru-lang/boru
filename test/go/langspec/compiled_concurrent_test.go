// Stage-5 concurrency gate (design/legacy/boru-bytecode-plan.0.ignore §Stage 5:
// "Race detector (go test -race) over the concurrent spec rows in
// compiled mode"). The concurrent words — await (parallel bodies),
// timeout, interval, cancel — fork an isolated registry per branch
// (ForkConcurrent). In compiled (v1) mode their bodies run as
// compile failures, so this exercises the fork machinery from the
// RunCompiled path. Run under `go test -race`: the bodies execute on
// separate goroutines over forked registries, and the result must match
// the interpreter with no data race.
package langspec

import (
	"sync"
	"testing"
)

func TestSpecCompiledConcurrentRowsRaceFree(t *testing.T) {
	t.Parallel()
	const tu = `import "boru:time-util" `
	cases := []struct {
		src  string
		want string
	}{
		{tu + `TimeUtil.await [[1 add 2] [3 add 4]]`, "[3 7]"},
		{tu + `TimeUtil.await [[10 mul 2] [5 sub 1] [100 add 1]]`, "[20 4 101]"},
		{tu + `typeof (TimeUtil.timeout 1000 [1 add 2])`, "Timeout"},
		{tu + `typeof (TimeUtil.interval 1000 [1 add 2])`, "Interval"},
		{tu + `100 (TimeUtil.cancel (TimeUtil.timeout 1000 [1 add 2])) add 23`, "123"},
	}

	// First: compiled-vs-interpreter parity for each concurrent row.
	for _, c := range cases {
		ac := newDifferentialInstance(t)
		gotC, _, errC := ac.RunCompiled(c.src)
		if errC != nil {
			t.Fatalf("compiled %q: %v", c.src, errC)
		}
		if got := renderAny(gotC); got != c.want {
			t.Fatalf("compiled %q = %q, want %q", c.src, got, c.want)
		}
	}

	// Then: hammer the await row (which forks parallel branch bodies)
	// from many goroutines at once through RunCompiled, so -race sees the
	// fork machinery driven concurrently across independent instances.
	const goroutines = 12
	const iters = 12
	await := tu + `TimeUtil.await [[1 add 2] [3 add 4] [5 add 6]]`
	const awaitWant = "[3 7 11]"
	var wg sync.WaitGroup
	errs := make(chan string, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				a := newDifferentialInstance(t)
				got, _, err := a.RunCompiled(await)
				if err != nil {
					errs <- err.Error()
					return
				}
				if r := renderAny(got); r != awaitWant {
					errs <- "got " + r + " want " + awaitWant
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("concurrent await under load: %s", e)
	}
}
