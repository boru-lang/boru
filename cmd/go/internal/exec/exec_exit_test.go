package exec

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A served request must NEVER terminate the server: this process is not
// that program's, it is every request's. `IO.exit` posted to /v1/exec is
// reported as the REQUEST's outcome and the server keeps serving — proved
// by a second request succeeding after it.
func TestExecServedExitDoesNotKillTheServer(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", map[string]any{"code": `import "boru:io" IO.exit 3`}, &got)
	if got.Error == "" {
		t.Fatal("a served exit request was silently honoured or ignored")
	}
	if !strings.Contains(got.Error, "code 3") || !strings.Contains(got.Error, "cannot exit the server") {
		t.Errorf("exit report = %q, want the code and the compile failure", got.Error)
	}

	var after execResponse
	post(t, srv, "/v1/exec", map[string]any{"code": "1 add 2"}, &after)
	if after.Error != "" || after.Result != float64(3) {
		t.Fatalf("the server did not survive a served exit request: %+v", after)
	}
}
