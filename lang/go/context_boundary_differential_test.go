package lang_test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestContextBoundaryAccumulates covers the property a single end-of-body
// check misses: the interpreter's boundary is per INVOCATION, so a compiled
// path that pushed one frame for the whole loop instead of one per element
// would pass every row above and still be wrong.
//
// The note's own repro: each element reads the counter, adds one, writes it
// back. Contained per element, every element sees 0 and the result is [1 1];
// leaking, the writes accumulate and it is [1 2].
func TestContextBoundaryAccumulates(t *testing.T) {
	const src = `context set 'z' 0
each [ drop context set 'z' ((context get 'z') add 1) end (context get 'z') ] [1 2]`
	a, _ := lang.New()
	compiled, cErr := a.Run(src)
	b, _ := lang.New()
	interp, iErr := b.RunInterp(src)
	if cErr != nil || iErr != nil {
		t.Fatalf("compiled: %v / interpreted: %v", cErr, iErr)
	}
	if fmt.Sprint(compiled) != fmt.Sprint(interp) {
		t.Errorf("per-element accumulation diverges: compiled %v != interpreted %v — "+
			"the frame is being pushed once for the loop rather than once per "+
			"element invocation", compiled, interp)
	}
	// And the frame must actually contain the write, not merely agree: two
	// engines that both leaked would pass the comparison above.
	if strings.Contains(fmt.Sprint(compiled), "2") {
		t.Errorf("got %v — the second element saw the first element's write, so the "+
			"body is not a boundary on either engine", compiled)
	}
}
