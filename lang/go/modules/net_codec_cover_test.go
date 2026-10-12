package modules

import (
	"errors"
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// Direct unit coverage for net_codec.go's package-level pieces: the
// codec plumbing (invokeFn / resolveCodec / readDecodeStep), the
// listen/connect option validation, the HTTP router matchers, the
// built-in codec edge branches, and the JSON <-> Value converters.
// Positive shapes are paired with the rejected ones per lang/go/CLAUDE.md;
// the session-loop and endpoint-transport arms live in
// net_codec_cover2_test.go.

// newCodecTestRegistry builds the same registry shape runNetSteps uses,
// so package-level net_codec functions can be driven directly.
func newCodecTestRegistry(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	return reg
}

// runNetStepsReg is runNetSteps returning the registry too, so a test can
// keep driving package-level functions against values the steps produced.
func runNetStepsReg(t *testing.T, steps []string) (*native.Registry, []native.Value, error) {
	t.Helper()
	reg := newCodecTestRegistry(t)
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return reg, nil, pErr
		}
		var err error
		result, err = engine.Run(vals)
		if err != nil {
			return reg, nil, err
		}
	}
	return reg, result, nil
}

// codecTestMap builds a concrete Map value from key/value pairs.
func codecTestMap(pairs ...any) native.Value {
	m := native.NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(native.Value))
	}
	return native.NewMap(m)
}

// goFnErr builds a 1-arg Go-backed codec function that always errors.
func goFnErr(msg string) native.Value {
	return goFn("err-fn", 1, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, errors.New(msg)
	})
}

// goFnConst builds a 1-arg Go-backed codec function returning a fixed value.
func goFnConst(v native.Value) native.Value {
	return goFn("const-fn", 1, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return []native.Value{v}, nil
	})
}

// ---- invokeFn ----

func TestInvokeFnDirectArms(t *testing.T) {
	reg := newCodecTestRegistry(t)

	// Not a function at all.
	if _, err := invokeFn(reg, native.NewInteger(1), native.NewInteger(2)); err == nil ||
		!strings.Contains(err.Error(), "not a function") {
		t.Errorf("non-function codec entry must be rejected, got %v", err)
	}

	// A Go handler error propagates.
	if _, err := invokeFn(reg, goFnErr("boom"), native.NewInteger(1)); err == nil ||
		!strings.Contains(err.Error(), "boom") {
		t.Errorf("Go handler error must propagate, got %v", err)
	}

	// A Go handler with no results yields None.
	empty := goFn("empty-fn", 1, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	res, err := invokeFn(reg, empty, native.NewInteger(1))
	if err != nil {
		t.Fatalf("empty Go handler: %v", err)
	}
	if native.IsConcrete(&res) || res.String() != "None" {
		t.Errorf("empty Go handler must yield None, got %v", res)
	}

	// Arity mismatch: neither the Go fast path nor MatchFnSig accepts it.
	two := goFn("two-fn", 2, func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
		return nil, nil
	})
	if _, err := invokeFn(reg, two, native.NewInteger(1)); err == nil ||
		!strings.Contains(err.Error(), "does not accept 1 argument") {
		t.Errorf("arity mismatch must be rejected, got %v", err)
	}

	// Positive: the Go fast path returns the handler's last result.
	res, err = invokeFn(reg, goFnConst(native.NewString("ok")), native.NewInteger(1))
	if err != nil {
		t.Fatalf("const Go handler: %v", err)
	}
	if s, sErr := core.AsString(res); sErr != nil || s != "ok" {
		t.Errorf("const Go handler = %v, want ok", res)
	}
}

func TestInvokeFnBoruArms(t *testing.T) {
	// A boru-authored codec function whose body fails at run time.
	reg, vals, err := runNetStepsReg(t, []string{`([b:Any] => [zzz-undefined-codec-word])`})
	if err != nil {
		t.Fatalf("build failing lambda: %v", err)
	}
	fn := vals[len(vals)-1]
	if _, iErr := invokeFn(reg, fn, native.NewInteger(1)); iErr == nil {
		t.Error("failing boru codec body must surface its error")
	}

	// A boru-authored codec function producing no values yields None.
	reg2, vals2, err := runNetStepsReg(t, []string{`([b:Any] => [b drop])`})
	if err != nil {
		t.Fatalf("build empty lambda: %v", err)
	}
	fn2 := vals2[len(vals2)-1]
	res, iErr := invokeFn(reg2, fn2, native.NewInteger(1))
	if iErr != nil {
		t.Fatalf("empty boru codec body: %v", iErr)
	}
	if native.IsConcrete(&res) || res.String() != "None" {
		t.Errorf("empty boru codec body must yield None, got %v", res)
	}
}

// ---- resolveCodec ----

func TestResolveCodecArms(t *testing.T) {
	reg := newCodecTestRegistry(t)

	// Not a map at all.
	if _, err := resolveCodec(reg, native.NewInteger(3), "listen"); err == nil ||
		!strings.Contains(err.Error(), "codec") {
		t.Errorf("non-map codec must be rejected, got %v", err)
	}

	// Missing encode: (decode alone is not enough).
	if _, err := resolveCodec(reg, codecTestMap("decode", goFnConst(needMap(1))), "listen"); err == nil ||
		!strings.Contains(err.Error(), "decode: and encode:") {
		t.Errorf("codec without encode must be rejected, got %v", err)
	}

	// Positive: a {decode encode} pair resolves, defaulting the client side.
	cf, err := resolveCodec(reg, codecTestMap(
		"decode", goFnConst(needMap(1)),
		"encode", goFnConst(native.NewBytesValue([]byte("x")))), "listen")
	if err != nil {
		t.Fatalf("minimal codec must resolve: %v", err)
	}
	out, iErr := invokeFn(reg, cf.encodeReq, native.NewInteger(1))
	if iErr != nil {
		t.Fatalf("defaulted encode-req: %v", iErr)
	}
	if b, ok := native.AsBytesValue(out); !ok || string(b) != "x" {
		t.Errorf("encode-req must default to encode, got %v", out)
	}
}

// ---- readDecodeStep ----

func TestReadDecodeStepShapes(t *testing.T) {
	// Not a map.
	if step := readDecodeStep(native.NewInteger(1)); step.invalid == "" {
		t.Error("non-map decode result must be invalid")
	}
	// {need: n} — a concrete need is a valid continuation.
	if step := readDecodeStep(codecTestMap("need", native.NewInteger(4))); !step.need || step.invalid != "" {
		t.Errorf("{need: 4} must read as need, got %+v", step)
	}
	// {need: Integer} — a type literal is not a byte count.
	if step := readDecodeStep(codecTestMap("need", native.NewTypeLiteral(native.TInteger))); step.invalid == "" {
		t.Error("non-concrete need: must be invalid")
	}
	// A map with neither need: nor msg:.
	if step := readDecodeStep(codecTestMap("foo", native.NewInteger(1))); step.invalid == "" ||
		!strings.Contains(step.invalid, "msg") {
		t.Errorf("decode result without msg: must be invalid, got %+v", step)
	}
	// Positive: {msg rest}.
	step := readDecodeStep(msgRest(native.NewString("m"), []byte("tail")))
	if step.invalid != "" || step.need || string(step.rest) != "tail" {
		t.Errorf("{msg rest} must read cleanly, got %+v", step)
	}
}

// ---- listen {codec} svc / connect argument validation ----

func TestListenSvcHandlerArgErrors(t *testing.T) {
	reg := newCodecTestRegistry(t)
	svc := native.NewServiceValue(nil)

	// Second argument must be a Service.
	if _, err := listenSvcHandler([]native.Value{codecTestMap(), native.NewInteger(1)}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "Service") {
		t.Errorf("non-Service arg must be rejected, got %v", err)
	}
	// Options must be a concrete map.
	if _, err := listenSvcHandler([]native.Value{native.NewTypeLiteral(native.TMap), svc}, nil, nil, reg); err == nil {
		t.Error("type-literal options must be rejected")
	}
	// codec: present but not a codec map.
	if _, err := listenSvcHandler([]native.Value{
		codecTestMap("tcp", native.NewInteger(0), "codec", native.NewInteger(9)), svc}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "codec") {
		t.Errorf("bad codec must be rejected, got %v", err)
	}
	// A valid codec but an unbindable port: the Tier-1 listen error surfaces.
	if _, err := listenSvcHandler([]native.Value{
		codecTestMap("tcp", native.NewInteger(-1), "codec", makeLinesCodec()), svc}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "port") {
		t.Errorf("bad port must surface the listen error, got %v", err)
	}
}

func TestConnectHandlerArgErrors(t *testing.T) {
	reg := newCodecTestRegistry(t)

	// Options must be a concrete map.
	if _, err := connectHandler([]native.Value{native.NewTypeLiteral(native.TMap)}, nil, nil, reg); err == nil {
		t.Error("type-literal options must be rejected")
	}
	// A codec: is required.
	if _, err := connectHandler([]native.Value{
		codecTestMap("tcp", native.NewString("127.0.0.1:1"))}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "codec") {
		t.Errorf("missing codec must be rejected, got %v", err)
	}
	// codec: present but not a codec map.
	if _, err := connectHandler([]native.Value{
		codecTestMap("tcp", native.NewString("127.0.0.1:1"), "codec", native.NewInteger(1))}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "codec") {
		t.Errorf("bad codec must be rejected, got %v", err)
	}
	// A valid codec but a bad dial target: the Tier-1 connect-raw error surfaces.
	if _, err := connectHandler([]native.Value{
		codecTestMap("tcp", native.NewInteger(9), "codec", makeLinesCodec())}, nil, nil, reg); err == nil ||
		!strings.Contains(err.Error(), "host:port") {
		t.Errorf("integer dial target must surface the connect-raw error, got %v", err)
	}
}

// ---- HTTP router: httpShape / matchPathTemplate / withMeta ----

func TestHTTPShapeArms(t *testing.T) {
	// Not a map.
	if _, _, ok := httpShape(native.NewString("x")); ok {
		t.Error("non-map message must not look like HTTP")
	}
	// A map without method/path.
	if _, _, ok := httpShape(codecTestMap("line", native.NewString("x"))); ok {
		t.Error("map without method/path must not look like HTTP")
	}
	// method present but not a String.
	if _, _, ok := httpShape(codecTestMap(
		"method", native.NewInteger(1), "path", native.NewString("/x"))); ok {
		t.Error("non-string method must not look like HTTP")
	}
	// Positive.
	m, p, ok := httpShape(codecTestMap(
		"method", native.NewString("GET"), "path", native.NewString("/x")))
	if !ok || m != "GET" || p != "/x" {
		t.Errorf("httpShape = %q %q %v, want GET /x true", m, p, ok)
	}
}

func TestMatchPathTemplateArms(t *testing.T) {
	// Named splat binds the joined remainder.
	params, ok := matchPathTemplate("/files/*rest", "/files/a/b")
	if !ok {
		t.Fatal("/files/*rest must match /files/a/b")
	}
	mp, _ := native.AsMap(params)
	if v, _ := mp.Get("rest"); native.ValToString(v) != "a/b" {
		t.Errorf("*rest = %v, want a/b", v)
	}
	// Anonymous splat defaults to `rest`, and matches an exhausted path.
	params, ok = matchPathTemplate("/files/*", "/files")
	if !ok {
		t.Fatal("/files/* must match /files")
	}
	mp, _ = native.AsMap(params)
	if v, _ := mp.Get("rest"); native.ValToString(v) != "" {
		t.Errorf("anonymous * on exhausted path = %v, want empty", v)
	}
	// Template longer than the path.
	if _, ok := matchPathTemplate("/a/b", "/a"); ok {
		t.Error("/a/b must not match /a")
	}
	// Path longer than the template.
	if _, ok := matchPathTemplate("/a", "/a/b"); ok {
		t.Error("/a must not match /a/b")
	}
	// Literal segment mismatch.
	if _, ok := matchPathTemplate("/a/:id", "/b/1"); ok {
		t.Error("/a/:id must not match /b/1")
	}
	// Root template: both sides split to nothing.
	if _, ok := matchPathTemplate("/", "/"); !ok {
		t.Error("/ must match /")
	}
	// Positive :param binding.
	params, ok = matchPathTemplate("/todos/:id", "/todos/7")
	if !ok {
		t.Fatal("/todos/:id must match /todos/7")
	}
	mp, _ = native.AsMap(params)
	if v, _ := mp.Get("id"); native.ValToString(v) != "7" {
		t.Errorf(":id = %v, want 7", v)
	}
}

func TestWithMetaNonMapMessage(t *testing.T) {
	// A scalar message is wrapped as {msg: ...} so routing still works.
	out := withMeta(native.NewString("ping"), native.Value{}, nil)
	mp, _ := native.AsMap(out)
	if mp == nil {
		t.Fatal("withMeta must produce a map")
	}
	if v, ok := mp.Get("msg"); !ok || native.ValToString(v) != "ping" {
		t.Errorf("wrapped scalar = %v, want msg: ping", out)
	}
	// A map message copies fields and takes overrides + @peer.
	peer := codecTestMap("host", native.NewString("h"))
	out = withMeta(codecTestMap("a", native.NewInteger(1)), peer,
		map[string]native.Value{"path": native.NewString("/p")})
	mp, _ = native.AsMap(out)
	if _, ok := mp.Get("a"); !ok {
		t.Error("withMeta must copy message fields")
	}
	if _, ok := mp.Get("@peer"); !ok {
		t.Error("withMeta must attach @peer for a concrete peer")
	}
	if v, _ := mp.Get("path"); native.ValToString(v) != "/p" {
		t.Error("withMeta must apply overrides")
	}
}

// ---- built-in codec branch arms ----

func TestLinesCodecArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeLinesCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// No newline yet: ask for more bytes.
	res, dErr := invokeFn(reg, cf.decode, native.NewBytesValue([]byte("abc")))
	if dErr != nil {
		t.Fatal(dErr)
	}
	if step := readDecodeStep(res); !step.need {
		t.Errorf("partial line must return {need}, got %v", res)
	}
	// A map reply without line: renders via ValToString.
	out, eErr := invokeFn(reg, cf.encode, codecTestMap("a", native.NewInteger(1)))
	if eErr != nil {
		t.Fatal(eErr)
	}
	if b, ok := native.AsBytesValue(out); !ok || !strings.HasSuffix(string(b), "\n") || !strings.Contains(string(b), "a") {
		t.Errorf("map-without-line encode = %v", out)
	}
	// A non-string, non-map scalar reply renders via ValToString.
	out, eErr = invokeFn(reg, cf.encode, native.NewInteger(42))
	if eErr != nil {
		t.Fatal(eErr)
	}
	if b, ok := native.AsBytesValue(out); !ok || string(b) != "42\n" {
		t.Errorf("integer encode = %q, want 42\\n", out)
	}
}

func TestJSONLinesCodecArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeJSONLinesCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// No newline yet.
	res, dErr := invokeFn(reg, cf.decode, native.NewBytesValue([]byte(`{"a":1}`)))
	if dErr != nil {
		t.Fatal(dErr)
	}
	if step := readDecodeStep(res); !step.need {
		t.Errorf("partial NDJSON frame must return {need}, got %v", res)
	}
	// A malformed frame is a codec error.
	if _, dErr := invokeFn(reg, cf.decode, native.NewBytesValue([]byte("{oops\n"))); dErr == nil ||
		!strings.Contains(dErr.Error(), "malformed JSON") {
		t.Errorf("malformed frame must error, got %v", dErr)
	}
	// An unencodable reply (NaN) is a codec error.
	if _, eErr := invokeFn(reg, cf.encode, native.NewFloat(math.NaN())); eErr == nil ||
		!strings.Contains(eErr.Error(), "unencodable") {
		t.Errorf("NaN reply must be unencodable, got %v", eErr)
	}
	// Positive round shape.
	out, eErr := invokeFn(reg, cf.encode, codecTestMap("a", native.NewInteger(1)))
	if eErr != nil {
		t.Fatal(eErr)
	}
	if b, ok := native.AsBytesValue(out); !ok || string(b) != "{\"a\":1}\n" {
		t.Errorf("json-lines encode = %q", out)
	}
}

func TestHTTPCodecDecodeArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeHTTPCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// Incomplete head: ask for more.
	res, dErr := invokeFn(reg, cf.decode, native.NewBytesValue([]byte("GET /")))
	if dErr != nil {
		t.Fatal(dErr)
	}
	if step := readDecodeStep(res); !step.need {
		t.Errorf("partial request must return {need}, got %v", res)
	}
	// Incomplete body: content-length says more is coming.
	res, dErr = invokeFn(reg, cf.decode,
		native.NewBytesValue([]byte("GET / HTTP/1.1\r\ncontent-length: 5\r\n\r\nab")))
	if dErr != nil {
		t.Fatal(dErr)
	}
	if step := readDecodeStep(res); !step.need {
		t.Errorf("short body must return {need}, got %v", res)
	}
	// A bad content-length is a codec error.
	if _, dErr := invokeFn(reg, cf.decode,
		native.NewBytesValue([]byte("GET / HTTP/1.1\r\ncontent-length: zz\r\n\r\n"))); dErr == nil ||
		!strings.Contains(dErr.Error(), "content-length") {
		t.Errorf("bad content-length must error, got %v", dErr)
	}
	// A junk request line is a codec error.
	if _, dErr := invokeFn(reg, cf.decode, native.NewBytesValue([]byte("JUNK\r\n\r\n"))); dErr == nil ||
		!strings.Contains(dErr.Error(), "malformed request line") {
		t.Errorf("junk request line must error, got %v", dErr)
	}
	// A query string splits off the path and parses pairs (and bare keys).
	res, dErr = invokeFn(reg, cf.decode,
		native.NewBytesValue([]byte("GET /a?x=1&flag HTTP/1.1\r\n\r\n")))
	if dErr != nil {
		t.Fatal(dErr)
	}
	step := readDecodeStep(res)
	if step.invalid != "" || step.need {
		t.Fatalf("query request must decode, got %+v", step)
	}
	mp, _ := native.AsMap(step.msg)
	if pv, _ := mp.Get("path"); native.ValToString(pv) != "/a" {
		t.Errorf("path = %v, want /a", step.msg)
	}
	qv, _ := mp.Get("query")
	qm, _ := native.AsMap(qv)
	if v, _ := qm.Get("x"); native.ValToString(v) != "1" {
		t.Errorf("query.x = %v, want 1", qv)
	}
	if v, ok := qm.Get("flag"); !ok || native.ValToString(v) != "" {
		t.Errorf("bare query key must map to empty, got %v", qv)
	}
}

func TestHTTPCodecEncodeArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeHTTPCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// Explicit status + headers + body.
	out, eErr := invokeFn(reg, cf.encode, codecTestMap(
		"status", native.NewInteger(201),
		"headers", codecTestMap("x-token", native.NewString("t1")),
		"body", native.NewString("hi")))
	if eErr != nil {
		t.Fatal(eErr)
	}
	b, _ := native.AsBytesValue(out)
	for _, want := range []string{"HTTP/1.1 201 Created", "x-token: t1", "content-length: 2", "hi"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded response missing %q in %q", want, b)
		}
	}
	// A non-no_match error reply maps to 500.
	out, eErr = invokeFn(reg, cf.encode, codecTestMap(
		"error", native.NewString("boom"), "message", native.NewString("m")))
	if eErr != nil {
		t.Fatal(eErr)
	}
	b, _ = native.AsBytesValue(out)
	if !strings.Contains(string(b), "HTTP/1.1 500 Internal Server Error") {
		t.Errorf("error reply must map to 500, got %q", b)
	}
	// A Bytes body ships raw with an octet-stream content type.
	out, eErr = invokeFn(reg, cf.encode, codecTestMap(
		"status", native.NewInteger(200), "body", native.NewBytesValue([]byte{1, 2, 3})))
	if eErr != nil {
		t.Fatal(eErr)
	}
	b, _ = native.AsBytesValue(out)
	if !strings.Contains(string(b), "application/octet-stream") {
		t.Errorf("bytes body must imply octet-stream, got %q", b)
	}
}

func TestHTTPCodecEncodeReqArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeHTTPCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// A request must be a map.
	if _, eErr := invokeFn(reg, cf.encodeReq, native.NewInteger(1)); eErr == nil ||
		!strings.Contains(eErr.Error(), "must be a") {
		t.Errorf("non-map request must be rejected, got %v", eErr)
	}
	// A structured body implies a JSON content type.
	out, eErr := invokeFn(reg, cf.encodeReq, codecTestMap(
		"method", native.NewString("POST"),
		"path", native.NewString("/p"),
		"body", codecTestMap("a", native.NewInteger(1))))
	if eErr != nil {
		t.Fatal(eErr)
	}
	b, _ := native.AsBytesValue(out)
	for _, want := range []string{"POST /p HTTP/1.1", "content-type: application/json", `{"a":1}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("encoded request missing %q in %q", want, b)
		}
	}
}

func TestHTTPCodecDecodeRespArms(t *testing.T) {
	reg := newCodecTestRegistry(t)
	cf, err := resolveCodec(reg, makeHTTPCodec(), "t")
	if err != nil {
		t.Fatal(err)
	}
	// Incomplete response: ask for more.
	res, dErr := invokeFn(reg, cf.decodeResp, native.NewBytesValue([]byte("HTTP/1.1 200")))
	if dErr != nil {
		t.Fatal(dErr)
	}
	if step := readDecodeStep(res); !step.need {
		t.Errorf("partial response must return {need}, got %v", res)
	}
	// A bad content-length is a codec error.
	if _, dErr := invokeFn(reg, cf.decodeResp,
		native.NewBytesValue([]byte("HTTP/1.1 200 OK\r\ncontent-length: zz\r\n\r\n"))); dErr == nil ||
		!strings.Contains(dErr.Error(), "content-length") {
		t.Errorf("bad content-length must error, got %v", dErr)
	}
	// A junk status line is a codec error.
	if _, dErr := invokeFn(reg, cf.decodeResp, native.NewBytesValue([]byte("BAD\r\n\r\n"))); dErr == nil ||
		!strings.Contains(dErr.Error(), "malformed status line") {
		t.Errorf("junk status line must error, got %v", dErr)
	}
}

// ---- renderHTTPBody / httpStatusText ----

func TestRenderHTTPBodyArms(t *testing.T) {
	if b, ct := renderHTTPBody(native.NewBytesValue([]byte("raw"))); string(b) != "raw" ||
		ct != "application/octet-stream" {
		t.Errorf("bytes body = %q %q", b, ct)
	}
	if b, ct := renderHTTPBody(native.NewString("txt")); string(b) != "txt" ||
		!strings.HasPrefix(ct, "text/plain") {
		t.Errorf("string body = %q %q", b, ct)
	}
	if b, ct := renderHTTPBody(native.NewTypeLiteral(native.TMap)); b != nil || ct != "" {
		t.Errorf("type-literal body = %q %q, want empty", b, ct)
	}
	// NaN cannot marshal: falls back to ValToString text.
	if b, ct := renderHTTPBody(native.NewFloat(math.NaN())); len(b) == 0 ||
		!strings.HasPrefix(ct, "text/plain") {
		t.Errorf("NaN body = %q %q", b, ct)
	}
	if b, ct := renderHTTPBody(codecTestMap("a", native.NewInteger(1))); string(b) != `{"a":1}` ||
		ct != "application/json" {
		t.Errorf("map body = %q %q", b, ct)
	}
}

func TestHTTPStatusTextTable(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{200, "OK"}, {201, "Created"}, {204, "No Content"}, {206, "Partial Content"},
		{400, "Bad Request"}, {404, "Not Found"}, {409, "Conflict"},
		{416, "Range Not Satisfiable"}, {500, "Internal Server Error"},
		{418, "Status"},
	} {
		if got := httpStatusText(tc.code); got != tc.want {
			t.Errorf("httpStatusText(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// ---- JSON <-> Value ----

func TestJSONValueConversionArms(t *testing.T) {
	// null -> None.
	v, err := jsonToValue([]byte("null"))
	if err != nil || native.IsConcrete(&v) || v.String() != "None" {
		t.Errorf("null = %v (%v), want None", v, err)
	}
	// A decimal stays a Float.
	v, err = jsonToValue([]byte("1.5"))
	if err != nil {
		t.Fatal(err)
	}
	if f, fErr := core.AsFloat(v); fErr != nil || f != 1.5 {
		t.Errorf("1.5 = %v, want Float 1.5", v)
	}
	// Arrays convert element-wise.
	v, err = jsonToValue([]byte(`[1, true, "s"]`))
	if err != nil {
		t.Fatal(err)
	}
	lst, lErr := native.AsList(v)
	if lErr != nil || lst.Len() != 3 {
		t.Fatalf("array = %v", v)
	}
	// Objects sort keys deterministically.
	v, err = jsonToValue([]byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	mp, _ := native.AsMap(v)
	if keys := mp.Keys(); len(keys) != 2 || keys[0] != "a" {
		t.Errorf("object keys = %v, want sorted", mp.Keys())
	}
	// Malformed JSON errors.
	if _, err := jsonToValue([]byte("{")); err == nil {
		t.Error("malformed JSON must error")
	}
	// The defensive default leaf renders as a string.
	dv := anyJSONToValue(7)
	if s, sErr := core.AsString(dv); sErr != nil || s != "7" {
		t.Errorf("default leaf = %v, want \"7\"", dv)
	}
	// A non-concrete value serializes as null.
	data, err := valueToJSON(native.NewTypeLiteral(native.TMap))
	if err != nil || string(data) != "null" {
		t.Errorf("type literal = %q (%v), want null", data, err)
	}
	// Positive: integer-preserving round trip.
	data, err = valueToJSON(codecTestMap("n", native.NewInteger(9)))
	if err != nil || string(data) != `{"n":9}` {
		t.Errorf("map = %q (%v)", data, err)
	}
}

func TestJSONReadyGuardsReaders(t *testing.T) {
	if got := jsonReady(strings.NewReader("x")); got != nil {
		t.Errorf("io.Reader leaf = %v, want nil", got)
	}
	m := map[string]any{"r": strings.NewReader("x"), "n": 1}
	out, ok := jsonReady(m).(map[string]any)
	if !ok || out["r"] != nil || out["n"] != 1 {
		t.Errorf("nested reader = %v", out)
	}
	l := []any{strings.NewReader("x"), "s"}
	outL, ok := jsonReady(l).([]any)
	if !ok || outL[0] != nil || outL[1] != "s" {
		t.Errorf("list reader = %v", outL)
	}
}
