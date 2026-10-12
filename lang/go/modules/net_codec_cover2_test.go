package modules

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// Deterministic coverage for net_codec.go's connection-session loop
// (runConnSession) and the Endpoint transport (endpointTransport.dispatch)
// using a scripted in-memory net.Conn — every read, write and deadline
// outcome is fixed up front, so the error arms fire without real sockets
// or timing races. dispatchDecoded / matchRouteTemplate are driven against
// real Services built by boru steps.

// scriptAddr is a net.Addr whose String carries no host:port split —
// exercising peerMapOf's SplitHostPort fallback.
type scriptAddr string

func (a scriptAddr) Network() string { return "script" }
func (a scriptAddr) String() string  { return string(a) }

// scriptConn is a scripted net.Conn: Read pops pre-loaded chunks (then
// returns readErr / io.EOF), Write and the deadline setters fail on demand.
type scriptConn struct {
	reads       [][]byte
	readErr     error
	writeErr    error
	deadlineErr error
	written     bytes.Buffer
}

func (c *scriptConn) Read(p []byte) (int, error) {
	if len(c.reads) == 0 {
		if c.readErr != nil {
			return 0, c.readErr
		}
		return 0, errEOFScript
	}
	b := c.reads[0]
	c.reads = c.reads[1:]
	return copy(p, b), nil
}

func (c *scriptConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return c.written.Write(p)
}

func (c *scriptConn) Close() error                       { return nil }
func (c *scriptConn) LocalAddr() net.Addr                { return scriptAddr("script-local") }
func (c *scriptConn) RemoteAddr() net.Addr               { return scriptAddr("script-peer") }
func (c *scriptConn) SetDeadline(_ time.Time) error      { return c.deadlineErr }
func (c *scriptConn) SetReadDeadline(_ time.Time) error  { return c.deadlineErr }
func (c *scriptConn) SetWriteDeadline(_ time.Time) error { return c.deadlineErr }

var errEOFScript = errors.New("script: EOF")

// goFnPanic builds a codec function whose Go handler panics — the session
// loop's recover guard must contain it to this connection.
func goFnPanic() native.Value {
	return goFn("panic-fn", 1, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		panic("codec-test-panic")
	})
}

// runScriptSession runs one connection session over a scripted conn and
// returns what the session logged to ErrOutput.
func runScriptSession(t *testing.T, conn net.Conn, cf codecFuncs) string {
	t.Helper()
	reg := newCodecTestRegistry(t)
	var errBuf bytes.Buffer
	reg.ErrOutput = &errBuf
	runConnSession(reg, conn, cf, native.NewServiceValue(nil))
	return errBuf.String()
}

func TestRunConnSessionErrorArms(t *testing.T) {
	linesCF := func(t *testing.T) codecFuncs {
		t.Helper()
		cf, err := resolveCodec(newCodecTestRegistry(t), makeLinesCodec(), "t")
		if err != nil {
			t.Fatal(err)
		}
		return cf
	}

	t.Run("decode panic is recovered per-connection", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnPanic()
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x\n")}}, cf)
		if !strings.Contains(out, "connection session crashed") {
			t.Errorf("panicking codec must be recovered and logged, got %q", out)
		}
	})

	t.Run("decode error ends the session", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnErr("bad-frame")
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x\n")}}, cf)
		if !strings.Contains(out, "codec decode error") {
			t.Errorf("decode error must be logged, got %q", out)
		}
	})

	t.Run("invalid decode shape ends the session", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnConst(native.NewInteger(1))
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x\n")}}, cf)
		if !strings.Contains(out, "decode must return") {
			t.Errorf("invalid decode shape must be logged, got %q", out)
		}
	})

	t.Run("need waits for more bytes then peer close ends quietly", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnConst(needMap(2))
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x")}}, cf)
		if out != "" {
			t.Errorf("a need continuation is not an error, got %q", out)
		}
	})

	t.Run("encode error ends the session", func(t *testing.T) {
		cf := linesCF(t)
		// The scalar msg also walks the non-map withMeta/httpShape arms,
		// and the empty service turns it into an {error} reply.
		cf.decode = goFnConst(msgRest(native.NewString("m"), nil))
		cf.encode = goFnErr("no-encode")
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x")}}, cf)
		if !strings.Contains(out, "codec encode error") {
			t.Errorf("encode error must be logged, got %q", out)
		}
	})

	t.Run("non-Bytes encode ends the session", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnConst(msgRest(native.NewString("m"), nil))
		cf.encode = goFnConst(native.NewInteger(7))
		out := runScriptSession(t, &scriptConn{reads: [][]byte{[]byte("x")}}, cf)
		if !strings.Contains(out, "encode must return Bytes") {
			t.Errorf("non-Bytes encode must be logged, got %q", out)
		}
	})

	t.Run("write failure ends the session quietly", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnConst(msgRest(native.NewString("m"), nil))
		cf.encode = goFnConst(native.NewBytesValue([]byte("r\n")))
		conn := &scriptConn{reads: [][]byte{[]byte("x")}, writeErr: errors.New("wfail")}
		out := runScriptSession(t, conn, cf)
		if out != "" {
			t.Errorf("a write failure is the peer's problem, got %q", out)
		}
		if conn.written.Len() != 0 {
			t.Errorf("nothing must have been written, got %q", conn.written.String())
		}
	})

	t.Run("positive: decode-dispatch-encode-write round trip", func(t *testing.T) {
		cf := linesCF(t)
		cf.decode = goFnConst(msgRest(native.NewString("m"), nil))
		cf.encode = goFnConst(native.NewBytesValue([]byte("reply\n")))
		conn := &scriptConn{reads: [][]byte{[]byte("x")}}
		out := runScriptSession(t, conn, cf)
		if out != "" {
			t.Errorf("clean session must log nothing, got %q", out)
		}
		if conn.written.String() != "reply\n" {
			t.Errorf("session wrote %q, want reply\\n", conn.written.String())
		}
	})
}

// ---- endpointTransport.dispatch ----

func newScriptTransport(conn net.Conn, cf codecFuncs) *endpointTransport {
	return &endpointTransport{sc: &socketConn{id: "SK_test", conn: conn}, cf: cf}
}

func TestEndpointDispatchErrorArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	req := codecTestMap("line", native.NewString("q"))
	okBytes := goFnConst(native.NewBytesValue([]byte("q\n")))

	t.Run("timeout option must be a non-negative Integer", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{}, codecFuncs{encodeReq: okBytes})
		for _, opts := range []native.Value{
			codecTestMap("timeout", native.NewString("soon")),
			codecTestMap("timeout", native.NewInteger(-5)),
		} {
			if _, err := tr.dispatch(reg, req, opts); err == nil ||
				!strings.Contains(err.Error(), "timeout: must be a non-negative Integer") {
				t.Errorf("bad timeout %v must be rejected, got %v", opts, err)
			}
		}
	})

	t.Run("encode-req failure", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{}, codecFuncs{encodeReq: goFnErr("enc-req-fail")})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "codec encode failed") {
			t.Errorf("encode-req failure must surface, got %v", err)
		}
	})

	t.Run("encode-req must return Bytes", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{}, codecFuncs{encodeReq: goFnConst(native.NewInteger(1))})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "must return Bytes") {
			t.Errorf("non-Bytes encode-req must surface, got %v", err)
		}
	})

	t.Run("write failure maps to a net error", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{writeErr: errors.New("wfail")},
			codecFuncs{encodeReq: okBytes})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "wfail") {
			t.Errorf("write failure must surface, got %v", err)
		}
	})

	t.Run("decode-resp failure", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{reads: [][]byte{[]byte("r\n")}},
			codecFuncs{encodeReq: okBytes, decodeResp: goFnErr("dec-resp-fail")})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "codec decode failed") {
			t.Errorf("decode-resp failure must surface, got %v", err)
		}
	})

	t.Run("invalid decode-resp shape", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{reads: [][]byte{[]byte("r\n")}},
			codecFuncs{encodeReq: okBytes, decodeResp: goFnConst(native.NewInteger(1))})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "decode must return") {
			t.Errorf("invalid decode-resp shape must surface, got %v", err)
		}
	})

	t.Run("deadline set failure with timeout", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{deadlineErr: errors.New("dlfail")},
			codecFuncs{encodeReq: okBytes})
		if _, err := tr.dispatch(reg, req, codecTestMap("timeout", native.NewInteger(50))); err == nil ||
			!strings.Contains(err.Error(), "dlfail") {
			t.Errorf("deadline failure must surface, got %v", err)
		}
	})

	t.Run("deadline clear failure without timeout", func(t *testing.T) {
		tr := newScriptTransport(&scriptConn{deadlineErr: errors.New("dlfail")},
			codecFuncs{encodeReq: okBytes})
		if _, err := tr.dispatch(reg, req, native.Value{}); err == nil ||
			!strings.Contains(err.Error(), "dlfail") {
			t.Errorf("deadline-clear failure must surface, got %v", err)
		}
	})

	t.Run("positive: reply decoded, leftover kept", func(t *testing.T) {
		conn := &scriptConn{reads: [][]byte{[]byte("ok\nleft")}}
		tr := newScriptTransport(conn, codecFuncs{
			encodeReq:  okBytes,
			decodeResp: goFnConst(msgRest(native.NewString("ok"), []byte("left"))),
		})
		res, err := tr.dispatch(reg, req, native.Value{})
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("dispatch results = %v", res)
		}
		if s, sErr := core.AsString(res[0]); sErr != nil || s != "ok" {
			t.Errorf("reply = %v, want ok", res[0])
		}
		if string(tr.buf) != "left" {
			t.Errorf("leftover buf = %q, want left", tr.buf)
		}
		if conn.written.String() != "q\n" {
			t.Errorf("request bytes = %q, want q\\n", conn.written.String())
		}
	})
}

// ---- dispatchDecoded / matchRouteTemplate over real Services ----

func TestDispatchDecodedEmptyReplyIsNone(t *testing.T) {
	reg, vals, err := runNetStepsReg(t, []string{
		`def svc (service {})`,
		`add {} ([req:Map state:Any] => [req drop]) svc`,
		`svc`,
	})
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	svc := vals[len(vals)-1]
	reply := dispatchDecoded(reg, svc, codecTestMap("op", native.NewString("x")), native.Value{})
	if native.IsConcrete(&reply) || reply.String() != "None" {
		t.Errorf("a handler with no reply must yield None, got %v", reply)
	}
}

func TestMatchRouteTemplateMethodFilter(t *testing.T) {
	_, vals, err := runNetStepsReg(t, []string{
		`def svc (service {})`,
		// A pathless pattern first: the router must skip it entirely.
		`add {op:"ping"} ([req:Map state:Any] => [ {pong: 1} ]) svc`,
		`add {method:"POST" path:"/x/:id"} ([req:Map state:Any] => [ {ok: 1} ]) svc`,
		`svc`,
	})
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	svc := vals[len(vals)-1]

	// A different method must not match the template route.
	if _, _, found := matchRouteTemplate(svc, "GET", "/x/9"); found {
		t.Error("GET must not match a POST-only template")
	}
	// The declared method binds the :id segment.
	tmpl, params, found := matchRouteTemplate(svc, "POST", "/x/9")
	if !found || tmpl != "/x/:id" {
		t.Fatalf("POST /x/9 must match /x/:id, got %q %v", tmpl, found)
	}
	mp, _ := native.AsMap(params)
	if v, _ := mp.Get("id"); native.ValToString(v) != "9" {
		t.Errorf(":id = %v, want 9", v)
	}
	// A path that fits no template misses entirely.
	if _, _, found := matchRouteTemplate(svc, "POST", "/y/9/z"); found {
		t.Error("/y/9/z must not match /x/:id")
	}
}
