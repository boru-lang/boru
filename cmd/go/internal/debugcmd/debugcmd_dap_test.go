package debugcmd

import (
	"fmt"
	"strings"
	"testing"
)

// The DAP end-to-end tests: a fixed byte stream of framed requests
// drives `boru debug --dap` exactly as an editor would, and the framed
// responses/events are asserted by substring (Go marshals map bodies
// with sorted keys, so the JSON is deterministic).

func dapFrame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func dapReq(seq int, command, args string) string {
	if args == "" {
		return dapFrame(fmt.Sprintf(`{"seq":%d,"type":"request","command":%q}`, seq, command))
	}
	return dapFrame(fmt.Sprintf(`{"seq":%d,"type":"request","command":%q,"arguments":%s}`, seq, command, args))
}

func TestDAPFullSession(t *testing.T) {
	// def n 41 / n add 1 / add 1 / add 1 — breakpoints on lines 2 and 4.
	path := writeProgram(t, "def n 41\nn add 1\nadd 1\nadd 1\n")
	bps := `{"source":{"path":"` + path + `"},"breakpoints":[{"line":2},{"line":4}]}`
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "setBreakpoints", bps),
		dapReq(4, "evaluate", `{"expression":"1"}`),        // before the run: declined
		dapReq(5, "variables", `{"variablesReference":1}`), // ditto
		dapReq(6, "stackTrace", "{}"),                      // ditto
		dapReq(7, "configurationDone", ""),                 // → entry stop (step)
		dapReq(8, "next", ""),                              // → breakpoint line 2
		dapReq(9, "threads", ""),
		dapReq(10, "stackTrace", "{}"),
		dapReq(11, "scopes", `{"frameId":0}`),
		dapReq(12, "variables", `{"variablesReference":1}`),
		dapReq(13, "variables", `{"variablesReference":2}`),
		dapReq(14, "evaluate", `{"expression":"n add 9"}`),
		dapReq(15, "setBreakpoints", bps),                   // while paused: the direct-mutate arm
		dapReq(16, "pause", ""),                             // honest compile failure
		dapReq(17, "customFoo", ""),                         // unsupported
		dapReq(18, "stepIn", ""),                            // → step stop line 3
		dapReq(19, "stepOut", ""),                           // top level: runs to the line-4 breakpoint
		dapReq(20, "variables", `{"variablesReference":2}`), // the stack holds 43 at line 4
		dapReq(21, "continue", ""),                          // → completion: exited 0 + terminated
		dapReq(22, "stepIn", ""),                            // after the run: respond, nothing to resume
		dapReq(23, "disconnect", ""),
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out,
		`"supportsConfigurationDoneRequest":true`,
		`"event":"initialized"`,
		`"verified":true`,
		`"command":"evaluate","request_seq":4,"success":false`,
		`"reason":"step"`, // the entry stop
		`"description":"breakpoint: line 2"`, `"reason":"breakpoint"`,
		`"threads":[{"id":1,"name":"main"}]`,
		`"name":"main"`, `"path":"`+path+`"`, // stackTrace top frame
		`"name":"Defs"`, `"name":"Stack"`,
		`"name":"n","value":"41"`,                                     // variables: the program's def
		`"variables":[]`,                                              // the data stack is empty at the line-2 boundary
		`"result":"50"`,                                               // evaluate n add 9
		`"reason":"step"`,                                             // stepIn → line 3
		`"description":"breakpoint: line 4"`, `"reason":"breakpoint"`, // stepOut caught by BP
		`"name":"#0","value":"43"`, // the stack at the line-4 boundary
		`"exitCode":0`,
		`"event":"terminated"`,
		`"command":"disconnect"`)
	// Negative: the pause request is declined, not silently dropped.
	if !strings.Contains(out, `"command":"pause","request_seq":16,"success":false`) {
		t.Errorf("pause must be declined:\n%s", out)
	}
	if !strings.Contains(out, `"command":"customFoo","request_seq":17,"success":false`) {
		t.Errorf("unknown requests must be declined:\n%s", out)
	}
}

func TestDAPFunctionBreakpoints(t *testing.T) {
	// DAP "function" breakpoints are boru word breakpoints: the stop
	// fires when the named word is about to dispatch. The second
	// setFunctionBreakpoints (while paused: the direct-mutate arm)
	// REPLACES the set with nothing, so the program runs out.
	path := writeProgram(t, "def n 1\nn add 2\nadd 3\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "setFunctionBreakpoints", `{"breakpoints":[{"name":"add"}]}`),
		dapReq(4, "configurationDone", ""), // entry stop
		dapReq(5, "continue", ""),          // → word add on line 2
		dapReq(6, "setFunctionBreakpoints", `{"breakpoints":[]}`),
		dapReq(7, "continue", ""), // no words left: runs to completion
		dapReq(8, "disconnect", ""),
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out,
		`"supportsFunctionBreakpoints":true`,
		`"reason":"step"`, // entry
		`"description":"breakpoint: word add"`, `"reason":"breakpoint"`,
		`"exitCode":0`)
}

func TestDAPBreakOnErrorStopsAsException(t *testing.T) {
	// --break-on-error under DAP: the fault pause arrives as a stopped
	// event with reason "exception" and the error text.
	path := writeProgram(t, "1 div 0\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""), // entry stop
		dapReq(4, "continue", ""),          // → the raise: stopped(exception)
		dapReq(5, "continue", ""),          // → the error ends the run
		dapReq(6, "disconnect", ""),
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--break-on-error", "--dap", path}, stdin)
	if code != 1 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out,
		"division by zero", `"reason":"exception"`,
		`"exitCode":1`)
}

func TestDAPMarkerPauseShowsFrameChain(t *testing.T) {
	// A Debug.break inside nested fns: the stopped reason is
	// "breakpoint" and stackTrace names the frame chain — the innermost
	// fn on the top frame, the outer fn as a deeper frame.
	path := writeProgram(t, `import "boru:debug"
def zzdeep fn [[x:Integer] [Integer] [
Debug.break
x add 1
]]
def zzcall fn [[y:Integer] [Integer] [
zzdeep y
]]
zzcall 5
`)
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""), // entry stop
		dapReq(4, "continue", ""),          // → the marker inside inner
		dapReq(5, "stackTrace", "{}"),
		dapReq(6, "variables", `{"variablesReference":1}`), // two defs: sorted
		dapReq(7, "continue", ""),                          // → completion (6)
		dapReq(8, "disconnect", ""),
	}, "")
	code, out, errOut := launch(t, []string{"--dap", path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d\n%s\n%s", code, out, errOut)
	}
	requireOrder(t, out,
		`"reason":"breakpoint"`, // the marker pause
		`"name":"zzdeep"`,       // top frame: the innermost fn
		`"name":"zzcall"`,       // deeper frame from the chain
		`"name":"zzcall"`,       // variables: the defs, name-sorted
		`"name":"zzdeep"`,
		`"exitCode":0`)
}

func TestDAPProgramErrorReportsExitOne(t *testing.T) {
	path := writeProgram(t, "1 div 0\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""), // entry stop
		dapReq(4, "continue", ""),          // → the error ends the run
		dapReq(5, "disconnect", ""),
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 1 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out,
		`"category":"stderr"`, "division by zero",
		`"exitCode":1`,
		`"event":"terminated"`)
}

func TestDAPClientEOFDetachesAndDrains(t *testing.T) {
	// The client hanging up mid-pause is a disconnect: the engine is
	// released with quit (detach) and DRAINED before the adapter
	// returns — the program's prints still arrive as output events.
	path := writeProgram(t, "print \"zz-drain\"\n1 add 2\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""), // entry stop, then EOF
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out, `"reason":"step"`, "zz-drain")
}

func TestDAPClientEOFWithProgramError(t *testing.T) {
	// The client hangs up at the entry stop and the DETACHED program then
	// errors: the drain at shutdown must still report exit 1.
	path := writeProgram(t, "1 div 0\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""), // entry stop, then EOF
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 1 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
}

func TestDAPScriptFlagConflict(t *testing.T) {
	path := writeProgram(t, "1 add 2\n")
	code, _, errOut := launch(t, []string{"--dap", "--script", "x", path}, "")
	if code != 1 || !strings.Contains(errOut, "mutually exclusive") {
		t.Errorf("exit = %d, stderr = %q", code, errOut)
	}
}

func TestDAPProgramOutputBecomesEvents(t *testing.T) {
	path := writeProgram(t, "print \"zz-out-event\"\n")
	stdin := strings.Join([]string{
		dapReq(1, "initialize", ""),
		dapReq(2, "launch", "{}"),
		dapReq(3, "configurationDone", ""),
		dapReq(4, "continue", ""),
		dapReq(5, "disconnect", ""),
	}, "")
	code, out, _ := launch(t, []string{"--no-check", "--dap", path}, stdin)
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	requireOrder(t, out, `"category":"stdout"`, "zz-out-event", `"exitCode":0`)
	// Negative: nothing outside DAP frames reaches stdout — no banner,
	// no "(program exited)".
	if strings.Contains(out, "type 'help' for commands") || strings.Contains(out, "(program exited)") {
		t.Errorf("DAP mode must emit frames only:\n%s", out)
	}
}
