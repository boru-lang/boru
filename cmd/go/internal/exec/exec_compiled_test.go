package exec

import (
	"net/http/httptest"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// The Phase 2 entry-point pin (plan p2 case): an exec request runs
// COMPILED-BY-DEFAULT — no unattributed interpreter entry fires for a
// compilable program — and a refused program still returns the
// interpreter's exact result (the refusal the interpreter absorbs, never an error).
func TestExecRunsCompiled(t *testing.T) {
	var entries []string
	prev := langNew
	t.Cleanup(func() { langNew = prev })
	langNew = func(opts ...lang.Options) (*lang.Boru, error) {
		a, err := prev(opts...)
		if err != nil {
			return nil, err
		}
		a.ArmInterpEntryHook(func(e core.InterpEntry) {
			if e.Attribution == "" {
				entries = append(entries, e.Seam)
			}
		})
		return a, nil
	}

	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", map[string]any{"code": "1 add 2"}, &got)
	if got.Error != "" || got.Result != float64(3) {
		t.Fatalf("compiled exec: %+v", got)
	}
	if len(entries) != 0 {
		t.Errorf("unattributed interpreter entries on a compiled exec request: %v", entries)
	}

	// A refused program falls back to the interpreter's result.
	entries = nil
	var ref execResponse
	post(t, srv, "/v1/exec", map[string]any{"code": "for 3 [1 2]"}, &ref)
	if ref.Error != "" || len(ref.Stack) != 6 {
		t.Fatalf("refused exec must return the interpreter result: %+v", ref)
	}
}
