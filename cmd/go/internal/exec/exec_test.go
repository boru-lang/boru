package exec

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// post sends a JSON body to path on srv, decodes the response into
// out (if non-nil), and returns the response status code. The body is
// always closed before return so the caller doesn't have to.
func post(t *testing.T, srv *httptest.Server, path string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %s", err)
		}
	}
	resp, err := http.Post(srv.URL+path, "application/json", &buf)
	if err != nil {
		t.Fatalf("POST %s: %s", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode response (%s): %s", string(raw), err)
		}
	}
	return resp.StatusCode
}

func TestExecHealthz(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestExecHealthzRejectsWrongMethod(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/healthz", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestExecAdd(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	status := post(t, srv, "/v1/exec", execRequest{Code: "1 add 2"}, &got)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	// JSON numbers come back as float64.
	n, ok := got.Result.(float64)
	if !ok {
		t.Fatalf("result type = %T, want float64", got.Result)
	}
	if n != 3 {
		t.Errorf("result = %v, want 3", n)
	}
	if len(got.Stack) != 1 {
		t.Errorf("stack len = %d, want 1", len(got.Stack))
	}
}

func TestExecStringResult(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: `import "boru:string-util" "hello" StringUtil.upper`}, &got)
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	s, ok := got.Result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", got.Result)
	}
	if s != "HELLO" {
		t.Errorf("result = %q, want %q", s, "HELLO")
	}
}

func TestExecLastValueWins(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: "10 20 30"}, &got)
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	// Three values left on the stack — result is the deepest-pushed
	// (= top of stack = last value, per the spec).
	n, ok := got.Result.(float64)
	if !ok {
		t.Fatalf("result type = %T, want float64", got.Result)
	}
	if n != 30 {
		t.Errorf("result = %v, want 30", n)
	}
	if len(got.Stack) != 3 {
		t.Errorf("stack len = %d, want 3", len(got.Stack))
	}
}

func TestExecEmptyStack(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	// `1 drop` consumes the only value, leaving an empty stack.
	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: "1 drop"}, &got)
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.Result != nil {
		t.Errorf("result = %v, want nil for empty stack", got.Result)
	}
	if len(got.Stack) != 0 {
		t.Errorf("stack len = %d, want 0", len(got.Stack))
	}
}

func TestExecBoruError(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	// `add` with no args is a runtime error.
	var got execResponse
	status := post(t, srv, "/v1/exec", execRequest{Code: "add"}, &got)
	// boru errors are returned at HTTP 200 with the error field set.
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got.Error == "" {
		t.Errorf("expected non-empty error, got %+v", got)
	}
}

func TestExecRejectsMissingCode(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	status := post(t, srv, "/v1/exec", execRequest{Code: ""}, &got)
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(got.Error, "code is required") {
		t.Errorf("error = %q, want to contain 'code is required'", got.Error)
	}
}

func TestExecRejectsWrongMethod(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/exec")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestExecRejectsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/exec", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestExecCapturesOutput(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: `"hi" print`}, &got)
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if !strings.Contains(got.Output, "hi") {
		t.Errorf("output = %q, want to contain 'hi'", got.Output)
	}
}

func TestExecRequestsAreIndependent(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	// A `def` in one request must not leak into the next request,
	// since each request runs in a fresh boru instance.
	var got1 execResponse
	post(t, srv, "/v1/exec", execRequest{Code: "def x 42"}, &got1)
	if got1.Error != "" {
		t.Fatalf("first request error: %s", got1.Error)
	}

	var got2 execResponse
	post(t, srv, "/v1/exec", execRequest{Code: "x"}, &got2)
	// x is undefined in the second request, so we expect an error.
	if got2.Error == "" {
		t.Errorf("expected undefined-word error in second request, got %+v", got2)
	}
}

// --- Policy enforcement (Phase 5) ---

func TestExecHonoursPolicy(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler("", pol))
	defer srv.Close()

	// sandbox allows engine.add but not network/process/disk.write.
	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: "1 add 2"}, &got)
	if got.Error != "" {
		t.Errorf("add should be allowed under sandbox: %s", got.Error)
	}
}

// A program that does not compile (the mid-expression fn-value apply, whose
// handler captures the factory's parameter) is REPORTED. The server used to
// re-run it on the interpreter and answer 7, so a posted expression could hit
// a compiler defect and nothing in the response said so — a served surface is
// the last place that silence belongs, because the person who posted the code
// is the one who can report it.
func TestExecProgramThatDoesNotCompileIsReported(t *testing.T) {
	srv := httptest.NewServer(Handler("", nil))
	defer srv.Close()

	var got execResponse
	post(t, srv, "/v1/exec", execRequest{Code: `def mk (fn [[n:Integer] [Any] [ def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ n ]) svc svc ]]) def s (mk 7) (call {cmd:"X"} s)`}, &got)
	if !strings.Contains(got.Error, "bytecode compilation FAILED") {
		t.Fatalf("error = %q, want the compile failure named (result=%v)", got.Error, got.Result)
	}
}

func TestExecRequestCannotOverridePolicy(t *testing.T) {
	// Even though the request body carries a "policy" field with a
	// permissive value, the server's bound policy is the only one
	// consulted. Adding an unknown field to execRequest is ignored
	// by the JSON decoder, so we just confirm the bound policy
	// still applies.
	pol, err := policy.LoadInline(`{
		name: "no-add",
		scopes: { engine: { words: { default: "allow", rules: [{ deny: ["add"] }] } } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler("", pol))
	defer srv.Close()

	// Client tries to override by sending a "policy" field that
	// (if honoured) would be more permissive. The execRequest
	// struct has only Code, so the policy field is silently dropped.
	body := map[string]any{
		"code":   "1 add 2",
		"policy": "trusted",
	}
	var got execResponse
	post(t, srv, "/v1/exec", body, &got)
	if got.Error == "" {
		t.Error("server-bound policy must reject add regardless of request payload")
	}
}
