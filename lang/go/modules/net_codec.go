package modules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// Tier-2 high-level networking for boru:net — the executable slice of
// design/NETWORK-SERVERS.0.md §6 and design/NETWORK-CLIENTS.0.md §6:
//
//	listen {tcp: <port> codec: <codec>} <service> -> Listener
//	connect {tcp: "host:port" codec: <codec>}     -> Endpoint (a Service)
//
// A Codec is a plain Map of functions over Bytes — the RFC's whole
// extension point, so custom protocols are ordinary boru values:
//
//	{ decode:      [buf:Bytes]  -> {msg: Value rest: Bytes} | {need: Integer}
//	  encode:      [reply:Any]  -> Bytes
//	  encode-req:  [req:Any]    -> Bytes   (client side; defaults to encode)
//	  decode-resp: [buf:Bytes]  -> {msg,rest}|{need}  (client; defaults to decode) }
//
// Built-in codecs (`Net.lines`, `Net.json-lines`, `Net.http`) are such
// maps whose functions carry Go handlers — same value shape, native
// speed. The `http` codec additionally cooperates with a small router:
// service patterns whose `path` carries `:param` / `*rest` segments are
// matched against the request path when the exact patrun route misses,
// and the bound segments surface as `req.params`
// (NETWORK-SERVERS.0.md §12 Q5's "small router in boru:net").
//
// `connect` returns an Endpoint — a Service whose call/send forward over
// the socket, one in-flight request at a time (synchronous correlation;
// the RFC's pending-call pump and peer-push handlers are deferred, see
// design/legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore §1). `call {req} ep
// {timeout: <ms>}` bounds the reply wait, raising `timeout`.

// ---- Go-backed Function values ----

// goFn builds a boru Function value whose implementation is a Go
// handler — the same shape as a lambda, dispatchable via CallBoru /
// direct application. Params are authored (not legacy Args) because a
// constructed Function value compiles through compileFnDef, which
// derives the forward barrier from len(Params).
func goFn(name string, arity int, handler core.Handler) native.Value {
	params := make([]core.FnParam, arity)
	for i := range params {
		params[i] = core.FnParam{Type: native.TAny}
	}
	sig := native.Signature{
		Params:     params,
		Impl:       native.Go(handler),
		Returns:    []*native.Type{native.TAny},
		BarrierPos: arity,
	}
	return native.NewFunction(native.FnDefInfo{Name: name, Signatures: []native.Signature{sig}})
}

// invokeFn applies a Function value to args and returns its last result
// (None when the body produced nothing). A Go-backed function (a goFn
// built-in codec) dispatches its handler directly — CallBoru only splices
// boru bodies; a boru-authored function (a user codec lambda) takes the
// MatchFnSig + CallBoru path.
func invokeFn(r *native.Registry, fn native.Value, args ...native.Value) (native.Value, error) {
	fnInfo, ok := native.FnDefFromValue(fn)
	if !ok {
		return native.Value{}, fmt.Errorf("codec entry is not a function")
	}
	for i := range fnInfo.Signatures {
		s := &fnInfo.Signatures[i]
		if gi, isGo := s.Impl.(*core.GoImpl); isGo && gi.Handler != nil && len(s.Params) == len(args) {
			res, err := gi.Handler(args, nil, nil, r)
			if err != nil {
				return native.Value{}, err
			}
			if len(res) == 0 {
				return native.NewTypeLiteral(native.TNone), nil
			}
			return res[len(res)-1], nil
		}
	}
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		return native.Value{}, fmt.Errorf("codec function does not accept %d argument(s)", len(args))
	}
	// InvokeCallback runs a compiled codec body on the VM (nested in the live run,
	// or fresh on an idle connection fork) and falls back to CallBoru otherwise.
	res, err := core.InvokeCallbackFn(r, fnInfo, sig, args)
	if err != nil {
		return native.Value{}, err
	}
	if len(res) == 0 {
		return native.NewTypeLiteral(native.TNone), nil
	}
	return res[len(res)-1], nil
}

// codecFuncs resolves a codec Map's four directions, defaulting the
// client-side pair to the server-side pair for symmetric protocols.
type codecFuncs struct {
	decode, encode, encodeReq, decodeResp native.Value
}

// validateCodecShape is resolveCodec's PURE PREFIX: the codec value must
// be a readable map carrying decode: and encode:.
//
// Split out because only this half may run during analysis — what
// follows STAMPS the codec's fn values for compilation, a side effect
// that has no business firing on the check pass. The mirror therefore
// gets the refusals without the stamping, and cannot drift from the
// handler's wording.
func validateCodecShape(r *native.Registry, v native.Value, word string) (native.ReadMap, error) {
	mp, err := native.RequireConcreteMap(v, word)
	if err != nil {
		return nil, r.BoruErrorHint("net_error", word+": codec: must be a {decode encode} map", word,
			"use a built-in codec (Net.lines, Net.json-lines, Net.http) or supply your own functions")
	}
	_, okD := mp.Get("decode")
	_, okE := mp.Get("encode")
	if !okD || !okE {
		return nil, r.BoruErrorHint("net_error", word+": codec must carry decode: and encode: functions", word,
			"decode: [buf:Bytes] -> {msg rest}|{need n};  encode: [reply] -> Bytes")
	}
	return mp, nil
}

func resolveCodec(r *native.Registry, v native.Value, word string) (codecFuncs, error) {
	mp, err := validateCodecShape(r, v, word)
	if err != nil {
		return codecFuncs{}, err
	}
	var cf codecFuncs
	get := func(key string) (native.Value, bool) { return mp.Get(key) }
	dec, _ := get("decode")
	enc, _ := get("encode")
	// Detached-stamp each custom boru codec fn so the per-request invokeFn →
	// InvokeCallback dispatch runs it on the VM instead of CallBoru. The codec
	// fns live INSIDE this map, where the compile-time store-fn bake never
	// reaches (recordCallOperands stamps only direct fn operands), so the
	// resolve site is their stamping seam. StampFnValue declines silently —
	// Go-backed built-ins, policy off (-no-compile), capturing or refusing
	// bodies — returning the value unchanged, so behaviour is byte-identical
	// on every decline. Stamp BEFORE the client-side defaulting below so a
	// symmetric codec's aliases share one stamped clone (one compile, not two).
	dec, _ = compiler.StampFnValue(r, dec)
	enc, _ = compiler.StampFnValue(r, enc)
	cf.decode, cf.encode = dec, enc
	if v, ok := get("encode-req"); ok {
		cf.encodeReq, _ = compiler.StampFnValue(r, v)
	} else {
		cf.encodeReq = enc
	}
	if v, ok := get("decode-resp"); ok {
		cf.decodeResp, _ = compiler.StampFnValue(r, v)
	} else {
		cf.decodeResp = dec
	}
	return cf, nil
}

// decodeStep interprets a decode result: {msg rest} or {need n}.
type decodeStep struct {
	msg     native.Value
	rest    []byte
	need    bool
	invalid string
}

func readDecodeStep(res native.Value) decodeStep {
	mp, _ := native.AsMap(res)
	if mp == nil {
		return decodeStep{invalid: "decode must return a {msg rest} or {need} map"}
	}
	if nv, ok := mp.Get("need"); ok {
		if !native.IsConcrete(nv) {
			return decodeStep{invalid: "decode {need:} must be an Integer"}
		}
		return decodeStep{need: true}
	}
	msg, okM := mp.Get("msg")
	if !okM {
		return decodeStep{invalid: "decode result missing msg:"}
	}
	rest := []byte{}
	if rv, ok := mp.Get("rest"); ok {
		if b, okB := native.AsBytesValue(rv); okB {
			rest = b
		}
	}
	return decodeStep{msg: msg, rest: rest}
}

// ---- listen {codec} service ----

func listenSvcHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	svc := args[1]
	if !svc.Parent.ConformsTo(native.TService) {
		return nil, r.BoruError("net_error", "listen: second argument must be a Service", "listen")
	}
	optsMap, err := native.RequireConcreteMap(args[0], "listen")
	if err != nil {
		return nil, err
	}
	cv, ok := optsMap.Get("codec")
	if !ok {
		// A wire protocol is never implicit (NETWORK-SERVERS.0.md §12 Q4).
		return nil, r.BoruErrorHint("net_error", "listen: a codec: is required to expose a service", "listen",
			"pass codec: Net.lines / Net.json-lines / Net.http, or a custom {decode encode} map")
	}
	cf, cErr := resolveCodec(r, cv, "listen")
	if cErr != nil {
		return nil, cErr
	}
	lv, lErr := listenHandler(args[:1], nil, nil, r)
	if lErr != nil {
		return nil, lErr
	}
	nl, _ := asListener(lv[0])

	acceptorFork := r.ForkConcurrent()
	if rt := r.Procs; rt != nil {
		acceptorFork.Output = rt.SyncOutput(r.Output)
		acceptorFork.ErrOutput = rt.SyncOutput(r.ErrOutput)
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil { //covergate:allow acceptor-goroutine recover body: the loop calls only Accept (errors, never panics) and ForkConcurrent on a listener listenHandler mints valid; per-connection panics land in runConnSession's own covered recover (§native)
				fmt.Fprintf(acceptorFork.ErrOutput, "[boru/net] listen acceptor crashed: %v\n", rec)
			}
		}()
		for {
			conn, aErr := nl.ln.Accept()
			if aErr != nil {
				return
			}
			connFork := acceptorFork.ForkConcurrent()
			go runConnSession(connFork, conn, cf, svc)
		}
	}()
	return lv, nil
}

// runConnSession is the per-connection loop: read → decode → dispatch →
// encode → write (the RFC §6.1 conn-session, in Go). One goroutine +
// fork per connection; a crash or protocol error kills only this
// connection.
func runConnSession(r *native.Registry, conn net.Conn, cf codecFuncs, svc native.Value) {
	sc := &socketConn{id: native.GenerateID("SK_"), conn: conn}
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Fprintf(r.ErrOutput, "[boru/net] connection session crashed: %v\n", rec)
		}
		_ = sc.close()
	}()

	peerVal := peerMapOf(conn)
	var buf []byte
	chunk := make([]byte, 4096)
	for {
		// Decode as many complete messages as the buffer holds.
		for len(buf) > 0 {
			res, dErr := invokeFn(r, cf.decode, native.NewBytesValue(append([]byte(nil), buf...)))
			if dErr != nil {
				fmt.Fprintf(r.ErrOutput, "[boru/net] codec decode error: %v\n", dErr)
				return
			}
			step := readDecodeStep(res)
			if step.invalid != "" {
				fmt.Fprintf(r.ErrOutput, "[boru/net] %s\n", step.invalid)
				return
			}
			if step.need {
				break
			}
			buf = append([]byte(nil), step.rest...)
			reply := dispatchDecoded(r, svc, step.msg, peerVal)
			out, eErr := invokeFn(r, cf.encode, reply)
			if eErr != nil {
				fmt.Fprintf(r.ErrOutput, "[boru/net] codec encode error: %v\n", eErr)
				return
			}
			data, okB := native.AsBytesValue(out)
			if !okB {
				fmt.Fprintf(r.ErrOutput, "[boru/net] codec encode must return Bytes\n")
				return
			}
			if _, wErr := conn.Write(data); wErr != nil {
				return
			}
		}
		n, rErr := conn.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if rErr != nil {
			return // peer closed (or transport error) — session over
		}
	}
}

// dispatchDecoded routes one decoded message into the service, applying
// the HTTP path-template router on a patrun miss, and converting
// dispatch errors into an {error message} reply value the codec encodes
// (no_match → the protocol's not-found shape).
func dispatchDecoded(r *native.Registry, svc native.Value, msg native.Value, peer native.Value) native.Value {
	req := withMeta(msg, peer, nil)
	res, err := native.DispatchServiceValue(r, svc, req)
	if err != nil && isNoMatch(err) {
		// HTTP router: retry against `:param` / `*rest` path templates.
		if method, path, ok := httpShape(msg); ok {
			if tmpl, params, found := matchRouteTemplate(svc, method, path); found {
				req = withMeta(msg, peer, map[string]native.Value{
					"path":   native.NewString(tmpl),
					"params": params,
				})
				res, err = native.DispatchServiceValue(r, svc, req)
			}
		}
	}
	if err != nil {
		var ae *core.BoruError
		code, detail := "error", err.Error()
		if errors.As(err, &ae) {
			code, detail = ae.Code, ae.Detail
		}
		m := native.NewOrderedMap()
		m.Set("error", native.NewString(code))
		m.Set("message", native.NewString(detail))
		return native.NewMap(m)
	}
	if len(res) == 0 {
		return native.NewTypeLiteral(native.TNone)
	}
	return res[len(res)-1]
}

func isNoMatch(err error) bool {
	var ae *core.BoruError
	return errors.As(err, &ae) && ae.Code == "no_match"
}

// withMeta copies a decoded message map, adding @peer and any overrides.
func withMeta(msg native.Value, peer native.Value, overrides map[string]native.Value) native.Value {
	out := native.NewOrderedMap()
	if mp, _ := native.AsMap(msg); mp != nil {
		for _, k := range mp.Keys() {
			v, _ := mp.Get(k)
			out.Set(k, v)
		}
	} else {
		// A non-map message is wrapped so handlers can still route on it.
		out.Set("msg", msg)
	}
	for k, v := range overrides {
		out.Set(k, v)
	}
	if native.IsConcrete(peer) {
		out.Set("@peer", peer)
	}
	return native.NewMap(out)
}

func peerMapOf(conn net.Conn) native.Value {
	m := native.NewOrderedMap()
	host, portStr, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		m.Set("host", native.NewString(conn.RemoteAddr().String()))
		m.Set("port", native.NewInteger(0))
	} else {
		port, _ := strconv.Atoi(portStr)
		m.Set("host", native.NewString(host))
		m.Set("port", native.NewInteger(int64(port)))
	}
	return native.NewMap(m)
}

// httpShape reports whether a decoded message looks like an HTTP request
// (string method + path), returning both.
func httpShape(msg native.Value) (string, string, bool) {
	mp, _ := native.AsMap(msg)
	if mp == nil {
		return "", "", false
	}
	mv, okM := mp.Get("method")
	pv, okP := mp.Get("path")
	if !okM || !okP {
		return "", "", false
	}
	method, mErr := mv.AsConcreteString()
	path, pErr := pv.AsConcreteString()
	if mErr != nil || pErr != nil {
		return "", "", false
	}
	return method, path, true
}

// matchRouteTemplate scans the service's registered patterns for a
// `path` template with `:param` / `*rest` segments matching the request
// path (same method). First registered match wins; exact patrun routes
// have already had their chance.
func matchRouteTemplate(svc native.Value, method, path string) (string, native.Value, bool) {
	for _, pat := range native.ServiceRoutePatterns(svc) {
		mp, _ := native.AsMap(pat)
		if mp == nil { //covergate:allow route patterns are stored only via serviceAddHandler, which gates them through RequireConcreteMap — a nil-AsMap pattern cannot be stored (§native)
			continue
		}
		if mv, ok := mp.Get("method"); ok {
			if pm, err := mv.AsConcreteString(); err != nil || pm != method {
				continue
			}
		}
		pv, ok := mp.Get("path")
		if !ok {
			continue
		}
		tmpl, err := pv.AsConcreteString()
		if err != nil || !strings.ContainsAny(tmpl, ":*") {
			continue
		}
		if params, matched := matchPathTemplate(tmpl, path); matched {
			return tmpl, params, true
		}
	}
	return "", native.Value{}, false
}

// matchPathTemplate matches "/todos/:id" / "/files/*rest" style
// templates, binding `:name` to one segment and `*name` to the joined
// remainder.
func matchPathTemplate(tmpl, path string) (native.Value, bool) {
	tSegs := splitPath(tmpl)
	pSegs := splitPath(path)
	params := native.NewOrderedMap()
	for i, t := range tSegs {
		switch {
		case strings.HasPrefix(t, "*"):
			name := t[1:]
			if name == "" {
				name = "rest"
			}
			if i > len(pSegs) { //covergate:allow star-case bounds guard: iteration i is reached only after i-1 passed the i>=len(pSegs) return, so i<=len(pSegs) always; the guard defends the pSegs[i:] slice (§native)
				return native.Value{}, false
			}
			params.Set(name, native.NewString(strings.Join(pSegs[i:], "/")))
			return native.NewMap(params), true
		case i >= len(pSegs):
			return native.Value{}, false
		case strings.HasPrefix(t, ":"):
			params.Set(t[1:], native.NewString(pSegs[i]))
		case t != pSegs[i]:
			return native.Value{}, false
		}
	}
	if len(pSegs) != len(tSegs) {
		return native.Value{}, false
	}
	return native.NewMap(params), true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// ---- connect {codec} → Endpoint ----

// endpointTransport serializes request/reply exchanges over one dialed
// socket: one in-flight call at a time, leftover bytes kept across calls.
type endpointTransport struct {
	mu  sync.Mutex
	sc  *socketConn
	cf  codecFuncs
	buf []byte
}

// connectCodecMirror is connect's check-mode guaranteed-error mirror. It
// runs the handler's own first two decidable steps — the codec-presence
// requirement and the address parse — over options that are concrete all
// the way down, so a call that provably refuses before any dial is
// reported at check time with the byte-identical code, detail and hint.
// It stops there: resolveCodec may read registry state and the dial
// itself is an effect, so neither runs during analysis.
func connectCodecMirror() native.ReturnsFunc {
	return native.MirrorReturns("connect", native.DeepConcreteOptions,
		func(args []native.Value, r *native.Registry) error {
			optsMap, err := native.RequireConcreteMap(args[0], "connect")
			if err != nil {
				return nil // not a readable map: dispatch owns the refusal
			}
			cv, ok := optsMap.Get("codec")
			if !ok {
				return r.BoruErrorHint("net_error", "connect: a codec: is required (use connect-raw for a raw Socket)", "connect",
					"pass codec: Net.lines / Net.json-lines / Net.http, or a custom {decode encode} map")
			}
			// connectHandler's order: it RESOLVES the codec before handing
			// off to connectRawHandler, so a malformed codec refuses before
			// the address is ever parsed. Validating the address first
			// would name the wrong error when both are wrong.
			if _, cErr := validateCodecShape(r, cv, "connect"); cErr != nil {
				return cErr
			}
			_, aErr := parseNetAddr(r, args[0], "connect", false)
			return aErr
		},
		native.ReturnsStatic(native.TService))
}

func connectHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	optsMap, err := native.RequireConcreteMap(args[0], "connect")
	if err != nil {
		return nil, err
	}
	cv, ok := optsMap.Get("codec")
	if !ok {
		return nil, r.BoruErrorHint("net_error", "connect: a codec: is required (use connect-raw for a raw Socket)", "connect",
			"pass codec: Net.lines / Net.json-lines / Net.http, or a custom {decode encode} map")
	}
	cf, cErr := resolveCodec(r, cv, "connect")
	if cErr != nil {
		return nil, cErr
	}
	sv, dErr := connectRawHandler(args[:1], nil, nil, r)
	if dErr != nil {
		return nil, dErr
	}
	sc, _ := asSocket(sv[0])
	tr := &endpointTransport{sc: sc, cf: cf}
	ep := native.NewRemoteServiceValue(tr.dispatch, func() error { return sc.close() })
	return []native.Value{ep}, nil
}

// dispatch is the Endpoint's RemoteDispatch: encode the request, write,
// then read/decode exactly one reply (synchronous correlation).
func (tr *endpointTransport) dispatch(r *native.Registry, req native.Value, opts native.Value) ([]native.Value, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	var deadline time.Time
	if mp, _ := native.AsMap(opts); mp != nil {
		if tv, ok := mp.Get("timeout"); ok {
			ms, err := tv.AsConcreteInteger()
			if err != nil || ms < 0 {
				return nil, r.BoruError("net_error", "call: timeout: must be a non-negative Integer (milliseconds)", "call")
			}
			deadline = time.Now().Add(time.Duration(ms) * time.Millisecond)
		}
	}

	out, err := invokeFn(r, tr.cf.encodeReq, req)
	if err != nil {
		return nil, r.BoruError("net_error", "call: codec encode failed: "+err.Error(), "call")
	}
	data, okB := native.AsBytesValue(out)
	if !okB {
		return nil, r.BoruError("net_error", "call: codec encode must return Bytes", "call")
	}
	tr.sc.wmu.Lock()
	_, wErr := tr.sc.conn.Write(data)
	tr.sc.wmu.Unlock()
	if wErr != nil {
		return nil, mapNetErr(r, "call", wErr)
	}

	chunk := make([]byte, 4096)
	for {
		if len(tr.buf) > 0 {
			res, dErr := invokeFn(r, tr.cf.decodeResp, native.NewBytesValue(append([]byte(nil), tr.buf...)))
			if dErr != nil {
				return nil, r.BoruError("net_error", "call: codec decode failed: "+dErr.Error(), "call")
			}
			step := readDecodeStep(res)
			if step.invalid != "" {
				return nil, r.BoruError("net_error", "call: "+step.invalid, "call")
			}
			if !step.need {
				tr.buf = append([]byte(nil), step.rest...)
				return []native.Value{step.msg}, nil
			}
		}
		if !deadline.IsZero() {
			if err := tr.sc.conn.SetReadDeadline(deadline); err != nil {
				return nil, mapNetErr(r, "call", err)
			}
		} else if err := tr.sc.conn.SetReadDeadline(time.Time{}); err != nil {
			return nil, mapNetErr(r, "call", err)
		}
		n, rErr := tr.sc.conn.Read(chunk)
		if n > 0 {
			tr.buf = append(tr.buf, chunk[:n]...)
		}
		if rErr != nil {
			return nil, mapNetErr(r, "call", rErr)
		}
	}
}

// ---- built-in codecs ----

// makeLinesCodec — newline-delimited text: decode → {line: String},
// encode a String/Scalar reply as text + "\n".
func makeLinesCodec() native.Value {
	decode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		buf, _ := native.AsBytesValue(args[0])
		idx := bytes.IndexByte(buf, '\n')
		if idx < 0 {
			return []native.Value{needMap(1)}, nil
		}
		line := strings.TrimSuffix(string(buf[:idx]), "\r")
		m := native.NewOrderedMap()
		m.Set("line", native.NewString(line))
		return []native.Value{msgRest(native.NewMap(m), buf[idx+1:])}, nil
	}
	encode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		var text string
		v := args[0]
		if s, err := core.AsString(v); err == nil {
			text = s
		} else if mp, _ := native.AsMap(v); mp != nil {
			if lv, ok := mp.Get("line"); ok {
				text, _ = lv.AsConcreteString()
			} else {
				text = native.ValToString(v)
			}
		} else {
			text = native.ValToString(v)
		}
		return []native.Value{native.NewBytesValue(append([]byte(text), '\n'))}, nil
	}
	return codecMap("lines",
		goFn("lines-decode", 1, decode),
		goFn("lines-encode", 1, encode), nil, nil)
}

// makeJSONLinesCodec — NDJSON: one JSON value per line.
func makeJSONLinesCodec() native.Value {
	decode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		buf, _ := native.AsBytesValue(args[0])
		idx := bytes.IndexByte(buf, '\n')
		if idx < 0 {
			return []native.Value{needMap(1)}, nil
		}
		line := bytes.TrimSuffix(buf[:idx], []byte("\r"))
		msg, err := jsonToValue(line)
		if err != nil {
			return nil, r.BoruError("codec_error", "json-lines: malformed JSON frame: "+err.Error(), "decode")
		}
		return []native.Value{msgRest(msg, buf[idx+1:])}, nil
	}
	encode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		data, err := valueToJSON(args[0])
		if err != nil {
			return nil, r.BoruError("codec_error", "json-lines: unencodable reply: "+err.Error(), "encode")
		}
		return []native.Value{native.NewBytesValue(append(data, '\n'))}, nil
	}
	return codecMap("json-lines",
		goFn("json-lines-decode", 1, decode),
		goFn("json-lines-encode", 1, encode), nil, nil)
}

// makeHTTPCodec — HTTP/1.1 with Content-Length bodies. Server side
// decodes requests / encodes responses; client side (encode-req /
// decode-resp) mirrors it.
func makeHTTPCodec() native.Value {
	decode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		buf, _ := native.AsBytesValue(args[0])
		head, body, rest, need, err := splitHTTPMessage(buf)
		if need > 0 {
			return []native.Value{needMap(need)}, nil
		}
		if err != nil {
			return nil, r.BoruError("codec_error", "http: "+err.Error(), "decode")
		}
		lines := strings.Split(head, "\r\n")
		parts := strings.SplitN(lines[0], " ", 3)
		if len(parts) < 2 {
			return nil, r.BoruError("codec_error", "http: malformed request line", "decode")
		}
		method, rawPath := parts[0], parts[1]
		path, query := rawPath, ""
		if q := strings.IndexByte(rawPath, '?'); q >= 0 {
			path, query = rawPath[:q], rawPath[q+1:]
		}
		m := native.NewOrderedMap()
		m.Set("method", native.NewString(method))
		m.Set("path", native.NewString(path))
		m.Set("query", parseQuery(query))
		headers := headerMap(lines[1:])
		m.Set("headers", headers)
		m.Set("body", native.NewBytesValue(body))
		// JSON convenience (the RFC §6.4 body-parsing wrap, folded into
		// the codec): a JSON content-type body also arrives decoded as
		// `req.body-json`. A malformed body just omits the key — the
		// handler still has the raw bytes.
		if hm, _ := native.AsMap(headers); hm != nil && len(body) > 0 {
			if ct, ok := hm.Get("content-type"); ok {
				if cts, err := ct.AsConcreteString(); err == nil && strings.Contains(cts, "json") {
					if bj, jErr := jsonToValue(body); jErr == nil {
						m.Set("body-json", bj)
					}
				}
			}
		}
		return []native.Value{msgRest(native.NewMap(m), rest)}, nil
	}
	encode := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		status, headers, body := 200, map[string]string{}, []byte(nil)
		contentType := ""
		v := args[0]
		if mp, _ := native.AsMap(v); mp != nil {
			if sv, ok := mp.Get("status"); ok {
				if n, err := sv.AsConcreteInteger(); err == nil {
					status = int(n)
				}
				if hv, ok := mp.Get("headers"); ok {
					if hm, _ := native.AsMap(hv); hm != nil {
						for _, k := range hm.Keys() {
							hvv, _ := hm.Get(k)
							headers[strings.ToLower(k)] = native.ValToString(hvv)
						}
					}
				}
				if bv, ok := mp.Get("body"); ok {
					body, contentType = renderHTTPBody(bv)
				}
			} else if ev, ok := mp.Get("error"); ok {
				// A dispatch error reply: map the error code to a status.
				code, _ := ev.AsConcreteString()
				if code == "no_match" {
					status = 404
				} else {
					status = 500
				}
				body, contentType = renderHTTPBody(v)
			} else {
				body, contentType = renderHTTPBody(v)
			}
		} else {
			body, contentType = renderHTTPBody(v)
		}
		if contentType != "" {
			if _, has := headers["content-type"]; !has {
				headers["content-type"] = contentType
			}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\n", status, httpStatusText(status))
		fmt.Fprintf(&b, "content-length: %d\r\n", len(body))
		for _, k := range sortedKeys(headers) {
			fmt.Fprintf(&b, "%s: %s\r\n", k, headers[k])
		}
		b.WriteString("\r\n")
		out := append([]byte(b.String()), body...)
		return []native.Value{native.NewBytesValue(out)}, nil
	}
	encodeReq := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		mp, _ := native.AsMap(args[0])
		if mp == nil {
			return nil, r.BoruError("codec_error", "http: a request must be a {method path …} map", "encode-req")
		}
		method := "GET"
		if mv, ok := mp.Get("method"); ok {
			method, _ = mv.AsConcreteString()
		}
		path := "/"
		if pv, ok := mp.Get("path"); ok {
			path, _ = pv.AsConcreteString()
		}
		headers := map[string]string{"host": "boru"}
		if hv, ok := mp.Get("headers"); ok {
			if hm, _ := native.AsMap(hv); hm != nil {
				for _, k := range hm.Keys() {
					hvv, _ := hm.Get(k)
					headers[strings.ToLower(k)] = native.ValToString(hvv)
				}
			}
		}
		var body []byte
		if bv, ok := mp.Get("body"); ok {
			ct := ""
			body, ct = renderHTTPBody(bv)
			if _, has := headers["content-type"]; !has && ct != "" {
				headers["content-type"] = ct
			}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s HTTP/1.1\r\n", method, path)
		fmt.Fprintf(&b, "content-length: %d\r\n", len(body))
		for _, k := range sortedKeys(headers) {
			fmt.Fprintf(&b, "%s: %s\r\n", k, headers[k])
		}
		b.WriteString("\r\n")
		return []native.Value{native.NewBytesValue(append([]byte(b.String()), body...))}, nil
	}
	decodeResp := func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
		buf, _ := native.AsBytesValue(args[0])
		head, body, rest, need, err := splitHTTPMessage(buf)
		if need > 0 {
			return []native.Value{needMap(need)}, nil
		}
		if err != nil {
			return nil, r.BoruError("codec_error", "http: "+err.Error(), "decode-resp")
		}
		lines := strings.Split(head, "\r\n")
		parts := strings.SplitN(lines[0], " ", 3)
		if len(parts) < 2 {
			return nil, r.BoruError("codec_error", "http: malformed status line", "decode-resp")
		}
		status, _ := strconv.Atoi(parts[1])
		m := native.NewOrderedMap()
		m.Set("status", native.NewInteger(int64(status)))
		m.Set("headers", headerMap(lines[1:]))
		m.Set("body", native.NewBytesValue(body))
		return []native.Value{msgRest(native.NewMap(m), rest)}, nil
	}
	return codecMap("http",
		goFn("http-decode", 1, decode),
		goFn("http-encode", 1, encode),
		goFnPtr(goFn("http-encode-req", 1, encodeReq)),
		goFnPtr(goFn("http-decode-resp", 1, decodeResp)))
}

func goFnPtr(v native.Value) *native.Value { return &v }

// codecMap assembles the codec value: a plain Map, so custom codecs and
// built-ins share one shape.
func codecMap(name string, decode, encode native.Value, encodeReq, decodeResp *native.Value) native.Value {
	m := native.NewOrderedMap()
	m.Set("name", native.NewString(name))
	m.Set("decode", decode)
	m.Set("encode", encode)
	if encodeReq != nil {
		m.Set("encode-req", *encodeReq)
	}
	if decodeResp != nil {
		m.Set("decode-resp", *decodeResp)
	}
	return native.NewMap(m)
}

func needMap(n int) native.Value {
	m := native.NewOrderedMap()
	m.Set("need", native.NewInteger(int64(n)))
	return native.NewMap(m)
}

func msgRest(msg native.Value, rest []byte) native.Value {
	m := native.NewOrderedMap()
	m.Set("msg", msg)
	m.Set("rest", native.NewBytesValue(append([]byte(nil), rest...)))
	return native.NewMap(m)
}

// splitHTTPMessage splits a buffered HTTP message into head, body and
// leftover, or reports how many more bytes are needed.
func splitHTTPMessage(buf []byte) (head string, body, rest []byte, need int, err error) {
	sep := bytes.Index(buf, []byte("\r\n\r\n"))
	if sep < 0 {
		return "", nil, nil, 1, nil
	}
	head = string(buf[:sep])
	contentLength := 0
	for _, line := range strings.Split(head, "\r\n")[1:] {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "content-length") {
			contentLength, err = strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return "", nil, nil, 0, fmt.Errorf("bad content-length %q", strings.TrimSpace(v))
			}
		}
	}
	total := sep + 4 + contentLength
	if len(buf) < total {
		return "", nil, nil, total - len(buf), nil
	}
	return head, buf[sep+4 : total], buf[total:], 0, nil
}

func headerMap(lines []string) native.Value {
	m := native.NewOrderedMap()
	for _, line := range lines {
		if k, v, ok := strings.Cut(line, ":"); ok {
			m.Set(strings.ToLower(strings.TrimSpace(k)), native.NewString(strings.TrimSpace(v)))
		}
	}
	return native.NewMap(m)
}

func parseQuery(q string) native.Value {
	m := native.NewOrderedMap()
	for _, pair := range strings.Split(q, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		m.Set(k, native.NewString(v))
	}
	return native.NewMap(m)
}

// renderHTTPBody turns a reply body value into wire bytes plus the
// content type it implies: Bytes → raw, String → text, other → JSON.
func renderHTTPBody(v native.Value) ([]byte, string) {
	if b, ok := native.AsBytesValue(v); ok {
		return b, "application/octet-stream"
	}
	if s, err := core.AsString(v); err == nil {
		return []byte(s), "text/plain; charset=utf-8"
	}
	if !native.IsConcrete(v) {
		return nil, ""
	}
	if data, err := valueToJSON(v); err == nil {
		return data, "application/json"
	}
	return []byte(native.ValToString(v)), "text/plain; charset=utf-8"
}

func httpStatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 204:
		return "No Content"
	case 206:
		return "Partial Content"
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 409:
		return "Conflict"
	case 416:
		return "Range Not Satisfiable"
	case 500:
		return "Internal Server Error"
	default:
		return "Status"
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- JSON ⇄ Value (deterministic, integer-preserving) ----

func jsonToValue(data []byte) (native.Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return native.Value{}, err
	}
	return anyJSONToValue(v), nil
}

func anyJSONToValue(v any) native.Value {
	switch x := v.(type) {
	case nil:
		return native.NewTypeLiteral(native.TNone)
	case bool:
		return native.NewBoolean(x)
	case json.Number:
		if i, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			return native.NewInteger(i)
		}
		f, _ := x.Float64()
		return native.NewFloat(f)
	case string:
		return native.NewString(x)
	case []any:
		elems := make([]native.Value, len(x))
		for i, e := range x {
			elems[i] = anyJSONToValue(e)
		}
		return native.NewList(elems)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := native.NewOrderedMap()
		for _, k := range keys {
			m.Set(k, anyJSONToValue(x[k]))
		}
		return native.NewMap(m)
	default:
		return native.NewString(fmt.Sprintf("%v", x))
	}
}

func valueToJSON(v native.Value) ([]byte, error) {
	if !native.IsConcrete(v) {
		return []byte("null"), nil
	}
	return json.Marshal(jsonReady(native.ValueToAny(v)))
}

// jsonReady normalises ValueToAny output for encoding/json (it already
// returns plain Go types; this guards unexpected leaves).
func jsonReady(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = jsonReady(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = jsonReady(e)
		}
		return x
	case io.Reader:
		return nil
	default:
		return x
	}
}

// codecNatives lists the Tier-2 words BuildNetModule registers.
func codecNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return []native.NativeFunc{
		{Name: "listen", Signatures: []native.Signature{
			// listen {tcp: port codec: c} <service> — expose a service on
			// the wire (appends to the Tier-1 listen registration).
			{Args: T(native.TMap, native.TService), Impl: native.Go(listenSvcHandler), Returns: T(TListener),
				BarrierPos: -1, CompileEffect: native.CompileStoresFn},
		}},
		{Name: "connect", Signatures: []native.Signature{
			// connect {tcp: "host:port" codec: c} -> Endpoint.
			{Args: T(native.TMap), Impl: native.Go(connectHandler), Returns: T(native.TService),
				ReturnsFn: connectCodecMirror(), BarrierPos: -1},
		}},
	}
}
