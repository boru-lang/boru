package native

import (
	"bytes"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/policy"
)

// Wave-4 coverage for the boru:log machinery: log_module.go (levels,
// records, sinks, render), log_logger.go (contextual loggers),
// log_span.go (traces), log_metrics.go (instruments), and log_sinks.go
// (host / boru sink registration). The natives are registered BARE
// (log-info, log-set-level, …) on a DefaultRegistry — the same
// NativeFuncs the boru:log module wraps — so the handlers run in-package
// and count toward native coverage.

// w4LogEnv builds a registry with the log natives bound to a fresh
// LogSinkRegistry (also installed as the host capability), routing the
// console sink into a buffer.
func w4LogEnv(t *testing.T) (*Registry, *LogSinkRegistry, *bytes.Buffer) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	buf := &bytes.Buffer{}
	r.ErrOutput = buf
	lsr := NewLogSinkRegistry()
	SetHostLogSinks(r, lsr)
	for _, n := range LogModuleNativeFuncs(lsr) {
		r.RegisterNativeFunc(n)
	}
	return r, lsr, buf
}

func w4LogWant(t *testing.T, r *Registry, src, want string) {
	t.Helper()
	got, err := w3Run(t, r, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if got != want {
		t.Errorf("%q = %q, want %q", src, got, want)
	}
}

func w4LogErr(t *testing.T, r *Registry, src, substr string) {
	t.Helper()
	_, err := w3Run(t, r, src)
	if err == nil {
		t.Fatalf("%q: expected error containing %q, got none", src, substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("%q: error %q does not contain %q", src, err.Error(), substr)
	}
}

// --- level names / config words ---------------------------------------------

func TestW4LogLevelNames(t *testing.T) {
	for lvl, name := range map[LogLevel]string{
		LevelTrace: "trace", LevelDebug: "debug", LevelInfo: "info",
		LevelWarn: "warn", LevelError: "error", LevelFatal: "fatal",
	} {
		if lvl.Name() != name {
			t.Errorf("%d.Name() = %q, want %q", lvl, lvl.Name(), name)
		}
		if lvl.Text() != strings.ToUpper(name) {
			t.Errorf("%d.Text() = %q, want %q", lvl, lvl.Text(), strings.ToUpper(name))
		}
		if got, ok := parseLevel(name); !ok || got != lvl {
			t.Errorf("parseLevel(%q) = %v, %v", name, got, ok)
		}
	}
	// Out-of-band severities fall back to info.
	if LogLevel(99).Name() != "info" || LogLevel(99).Text() != "INFO" {
		t.Errorf("unknown level should default to info")
	}
	if _, ok := parseLevel("loud"); ok {
		t.Error("parseLevel(loud) should fail")
	}
}

func TestW4LogDefaultsAndConfig(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r, `log-get-level`, `info/q`)
	w4LogWant(t, r, `log-get-format`, `text/q`)
	w4LogWant(t, r, `log-sinks`, `[console/q]`)
	w4LogWant(t, r, `log-set-level warn/q ; log-get-level`, `warn/q`)
	w4LogWant(t, r, `log-set-format json/q ; log-get-format`, `json/q`)
	// Negative: unknown level / format, non-atom arguments.
	w4LogErr(t, r, `log-set-level loud/q`, "unknown level")
	w4LogErr(t, r, `log-set-format xml/q`, "unknown format")
}

func TestW4LogEnabled(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r, `log-enabled info/q`, `true`)
	w4LogWant(t, r, `log-enabled debug/q`, `false`)
	w4LogWant(t, r, `log-set-level error/q ; log-enabled warn/q`, `false`)
	w4LogWant(t, r, `log-set-level trace/q ; log-enabled trace/q`, `true`)
	w4LogErr(t, r, `log-enabled loud/q`, "unknown level")
	// With no sinks attached at all, nothing can be emitted.
	w4LogWant(t, r, `log-remove-sink console/q ; log-enabled fatal/q`, `false`)
}

// --- emission, capture, thresholds --------------------------------------------

func TestW4LogEmitAndDump(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ; log-info "a" ; log-dump size`,
		`1`)
	w4LogWant(t, r, `log-clear ; log-dump size`, `0`)
	// A record with fields; captured attributes round-trip.
	w4LogWant(t, r,
		`log-info "b" {x:1} ; log-dump 0 get "attributes" get`,
		`{x:1}`)
	w4LogWant(t, r, `log-dump 0 get "severity-text" get`, `'INFO'`)
	w4LogWant(t, r, `log-dump 0 get "logger" get`, `''`)
	// Below-threshold records are dropped before fan-out.
	w4LogWant(t, r,
		`log-clear ; log-set-level warn/q ; log-info "quiet" ; log-dump size`,
		`0`)
	w4LogWant(t, r, `log-error "e" ; log-fatal "f" ; log-dump size`, `2`)
	w4LogWant(t, r,
		`log-clear ; log-set-level trace/q ; log-trace "t" ; log-debug "d" ; log-warn "w" ; log-dump size`,
		`3`)
}

func TestW4LogGenericWord(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ; log-log error/q "boom" ; log-dump size`,
		`1`)
	w4LogWant(t, r,
		`log-clear ; log-log warn/q "w" {k:2} ; log-dump 0 get "attributes" get`,
		`{k:2}`)
	w4LogErr(t, r, `log-log loud/q "m"`, "unknown level")
	w4LogErr(t, r, `log-log loud/q "m" {a:1}`, "unknown level")
}

func TestW4LogSinkManagement(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r, `log-add-sink memory/q ; log-sinks`, `[console/q memory/q]`)
	// Idempotent re-attach.
	w4LogWant(t, r, `log-add-sink memory/q ; log-sinks`, `[console/q memory/q]`)
	w4LogWant(t, r, `log-remove-sink console/q ; log-sinks`, `[memory/q]`)
	// Detaching an absent sink is a no-op.
	w4LogWant(t, r, `log-remove-sink console/q ; log-sinks`, `[memory/q]`)
	w4LogErr(t, r, `log-add-sink nosuch/q`, "no registered sink")
}

// --- console renderers --------------------------------------------------------

func TestW4LogConsoleTextRender(t *testing.T) {
	r, _, buf := w4LogEnv(t)
	if _, err := w3Run(t, r, `log-info "hello" {k:1}`); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"INFO", "hello", "k=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("console text missing %q: %s", want, out)
		}
	}
	// Correlation fields: a named logger emitting inside a span carries
	// logger= / trace-id= / span-id= on the text line.
	buf.Reset()
	src := `def s (log-span "op") ; def l (log-logger "api") ; l.warn "corr" {c:3} ; s.finish`
	if _, err := w3Run(t, r, src); err != nil {
		t.Fatal(err)
	}
	out = buf.String()
	for _, want := range []string{"WARN", "corr", "c=3", "logger=api", "trace-id=", "span-id="} {
		if !strings.Contains(out, want) {
			t.Errorf("correlated text missing %q: %s", want, out)
		}
	}
}

func TestW4LogConsoleJSONRender(t *testing.T) {
	r, _, buf := w4LogEnv(t)
	src := `log-set-format json/q ; def s (log-span "jop") ; def l (log-logger "japi") ; l.error "jmsg" {j:1} ; s.finish`
	if _, err := w3Run(t, r, src); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`"level":"ERROR"`, `"msg":"jmsg"`, `"attributes"`, `"logger":"japi"`,
		`"trace-id"`, `"span-id"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("console json missing %q: %s", want, out)
		}
	}
}

// --- contextual loggers -------------------------------------------------------

func TestW4Loggers(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r, `def l0 (log-logger "http") ; typeof l0`, `Map`)
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ;`+
			` def l (log-with "http" {svc:"api"}) ; l.info "req" ; log-dump 0 get "logger" get`,
		`'http'`)
	// Defaults merge UNDER call fields.
	w4LogWant(t, r,
		`log-clear ; def l (log-with "http" {svc:"api"}) ; l.info "req" {path:"/x"} ;`+
			` log-dump 0 get "attributes" get`,
		`{svc:'api' path:'/x'}`)
	// A call field overrides a same-named default.
	w4LogWant(t, r,
		`log-clear ; def l (log-with "a" {x:1}) ; l.error "boom" {x:2} ;`+
			` log-dump 0 get "attributes" get`,
		`{x:2}`)
	// Child loggers layer more defaults.
	w4LogWant(t, r,
		`log-clear ; def l (log-with "a" {x:1}) ; def c (l.child {y:2}) ; c.warn "m" ;`+
			` log-dump 0 get "attributes" get`,
		`{x:1 y:2}`)
}

// --- spans / traces ------------------------------------------------------------

func TestW4Spans(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r, `log-current-span`, `None`)
	w4LogWant(t, r, `def s0 (log-span "op") ; typeof s0`, `Map`)
	w4LogWant(t, r, `def cur (log-current-span) ; cur.name`, `'op'`)
	w4LogWant(t, r, `def s0b (log-current-span) ; s0b.finish ; log-current-span`, `None`)

	// Manual lifecycle: attrs at start, set-attr, events, error status.
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ;`+
			` def s (log-span "m" {phase:"init"}) ; s.set-attr region/q "eu" ;`+
			` s.add-event "started" ; s.add-event "checked" {ok:true} ;`+
			` s.record-error "boom" ; s.finish ; log-traces size`,
		`1`)
	w4LogWant(t, r, `log-traces 0 get "name" get`, `'m'`)
	w4LogWant(t, r, `log-traces 0 get "attributes" get`, `{phase:'init' region:'eu'}`)
	w4LogWant(t, r, `log-traces 0 get "status" get`, `'error'`)
	w4LogWant(t, r, `log-traces 0 get "status-message" get`, `'boom'`)
	w4LogWant(t, r, `log-traces 0 get "events" get size`, `2`)
	w4LogWant(t, r, `log-traces 0 get "parent-id" get`, `''`)
}

func TestW4SpanFrozenAfterFinish(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	src := `log-add-sink memory/q ; log-remove-sink console/q ;
		def s (log-span "frozen") ; s.set-attr a/q 1 ; s.finish ;
		s.finish ; s.set-attr b/q 2 ; s.add-event "late" ; s.record-error "late" ;
		log-traces size`
	w4LogWant(t, r, src, `1`)
	// The post-finish mutations must not have taken.
	w4LogWant(t, r, `log-traces 0 get "attributes" get`, `{a:1}`)
	w4LogWant(t, r, `log-traces 0 get "events" get size`, `0`)
	w4LogWant(t, r, `log-traces 0 get "status" get`, `'ok'`)
}

func TestW4NestedSpansAndStamping(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	// Nested spans: the inner inherits the trace id and records its parent.
	// Finishing the OUTER first exercises non-top removal from the stack.
	src := `log-add-sink memory/q ; log-remove-sink console/q ;
		def s1 (log-span "outer") ; def s2 (log-span "inner") ;
		log-info "stamped" ;
		s1.finish ; s2.finish ; log-traces size`
	w4LogWant(t, r, src, `2`)
	w4LogWant(t, r, `log-traces 0 get "name" get`, `'outer'`)
	w4LogWant(t, r, `log-traces 1 get "name" get`, `'inner'`)
	w4LogWant(t, r, `log-traces 1 get "parent-id" get "" eq`, `false`)
	// The record emitted inside was stamped with the live trace id.
	w4LogWant(t, r, `log-dump 0 get "trace-id" get "" eq`, `false`)
	w4LogWant(t, r, `log-dump 0 get "span-id" get "" eq`, `false`)
}

func TestW4WithSpan(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	// The body's last value is returned.
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ; log-with-span "ws" [1 add 2]`,
		`3`)
	w4LogWant(t, r, `log-traces size`, `1`)
	w4LogWant(t, r, `log-traces 0 get "status" get`, `'ok'`)
	// An empty body returns nothing.
	w4LogWant(t, r, `log-clear ; log-with-span "empty" [] ; log-traces size`, `1`)
	// A raising body records the error on the span and re-raises.
	w4LogErr(t, r, `log-clear ; log-with-span "bad" [no-such-word-w4]`, "undefined_word")
	w4LogWant(t, r, `log-traces 0 get "status" get`, `'error'`)
	w4LogWant(t, r, `log-traces 0 get "status-message" get "" eq`, `false`)
}

func TestW4EndSpanWord(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ;`+
			` def s (log-span "m") ; log-end s ; log-traces size`,
		`1`)
	// Ending again: the span is no longer active.
	w4LogErr(t, r, `log-end s`, "not the active span")
	// A map without a span-id is not a span.
	w4LogErr(t, r, `log-end {a:1}`, "no span-id field")
}

// --- metrics --------------------------------------------------------------------

func TestW4Metrics(t *testing.T) {
	r, _, buf := w4LogEnv(t)
	w4LogWant(t, r, `def c0 (log-counter "x") ; typeof c0`, `Map`)
	w4LogWant(t, r, `def c0b (log-counter "x") ; c0b.name`, `'x'`)
	// Console renders measurements; memory captures them.
	src := `log-add-sink memory/q ;
		def c (log-counter "requests") ; c.add 1 ; c.add 2 {region:"eu"} ;
		def g (log-gauge "temp") ; g.record 21 ;
		def h (log-histogram "lat") ; h.record 12 {ep:"/x"} ;
		log-measurements size`
	w4LogWant(t, r, src, `4`)
	w4LogWant(t, r, `log-measurements 0 get "instrument" get`, `'requests'`)
	w4LogWant(t, r, `log-measurements 0 get "kind" get`, `'counter'`)
	w4LogWant(t, r, `log-measurements 1 get "attributes" get`, `{region:'eu'}`)
	w4LogWant(t, r, `log-measurements 2 get "kind" get`, `'gauge'`)
	w4LogWant(t, r, `log-measurements 3 get "attributes" get`, `{ep:'/x'}`)
	if !strings.Contains(buf.String(), "METRIC counter requests=1") {
		t.Errorf("console measurement render missing: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "region=eu") {
		t.Errorf("console measurement attrs missing: %s", buf.String())
	}
	// Metrics ignore the record level threshold entirely.
	w4LogWant(t, r,
		`log-clear ; log-set-level fatal/q ; def c2 (log-counter "n") ; c2.add 5 ;`+
			` log-measurements size`,
		`1`)
}

// --- boru fn sinks ----------------------------------------------------------------

func TestW4RegisterFnSink(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	w4LogWant(t, r,
		`log-register (fn [[rec:Any] [] []]) tap/q info/q ; log-sinks`,
		`[console/q tap/q]`)
	// The fn is invoked per admitted record (a below-min record skips it).
	w4LogWant(t, r,
		`log-add-sink memory/q ; log-remove-sink console/q ; log-info "seen" ; log-dump size`,
		`1`)
	// Duplicate names are rejected.
	w4LogErr(t, r, `log-register (fn [[rec:Any] [] []]) tap/q info/q`, "already registered")
	w4LogErr(t, r, `log-register (fn [[rec:Any] [] []]) tap2/q loud/q`, "unknown level")
	// A sink fn that itself errors must never abort the emitting program.
	w4LogWant(t, r,
		`log-register (fn [[rec:Any] [] [no-such-word-w4]]) bad/q trace/q ;`+
			` log-warn "survives" ; log-dump size`,
		`2`)
}

// --- host sinks (Go-level registration) -------------------------------------------

func TestW4HostSink(t *testing.T) {
	r, _, _ := w4LogEnv(t)
	var recs []LogRecord
	var ended []Span
	var measures []Measurement
	err := RegisterHostLogSink(r, LogSinkSpec{
		Name:     "hostsink",
		MinLevel: LevelWarn,
		OnRecord: func(rec LogRecord) error { recs = append(recs, rec); return nil },
		OnSpanStart: func(sp Span) SpanContext {
			return SpanContext{TraceID: "cafe0000cafe0000cafe0000cafe0000", SpanID: "beef0000beef0000"}
		},
		OnSpanEnd: func(sp Span) { ended = append(ended, sp) },
		OnMeasure: func(m Measurement) { measures = append(measures, m) },
	})
	if err != nil {
		t.Fatalf("RegisterHostLogSink: %v", err)
	}
	src := `log-remove-sink console/q ;
		log-info "below-min" ;
		def s (log-span "hspan" {a:1}) ;
		s.add-event "ev" {b:2} ; s.record-error "boom" ;
		log-error "inside" ;
		s.finish ;
		def c (log-counter "reqs") ; c.add 2 {r:"eu"}`
	if _, err := w3Run(t, r, src); err != nil {
		t.Fatal(err)
	}
	// MinLevel warn: the info record was skipped, the error captured.
	if len(recs) != 1 || FormatForPrint(recs[0].Body) != "inside" {
		t.Fatalf("host recs = %+v, want 1 record 'inside'", recs)
	}
	// The host OnSpanStart restamped the span, and records inside carry it.
	if recs[0].TraceID != "cafe0000cafe0000cafe0000cafe0000" {
		t.Errorf("record trace-id = %q, want host-owned id", recs[0].TraceID)
	}
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	sp := ended[0]
	if sp.TraceID != "cafe0000cafe0000cafe0000cafe0000" || sp.SpanID != "beef0000beef0000" {
		t.Errorf("span ids = %q/%q, want host-owned ids", sp.TraceID, sp.SpanID)
	}
	if sp.Name != "hspan" || sp.Status != "error" || sp.StatusMessage != "boom" {
		t.Errorf("span view = %+v", sp)
	}
	if v, ok := sp.Attributes["a"]; !ok || v == nil {
		t.Errorf("span attrs missing a: %+v", sp.Attributes)
	}
	if len(sp.Events) != 1 || sp.Events[0].Name != "ev" || sp.Events[0].Attributes["b"] == nil {
		t.Errorf("span events = %+v", sp.Events)
	}
	if sp.EndUnixNano == 0 || sp.StartUnixNano == 0 {
		t.Errorf("span times unset: %+v", sp)
	}
	if len(measures) != 1 || measures[0].Instrument != "reqs" || measures[0].Kind != "counter" {
		t.Errorf("measures = %+v", measures)
	}
}

func TestW4HostSinkCreatesRegistryWhenAbsent(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if HostLogSinks(r) != nil {
		t.Fatal("fresh registry should have no sink registry")
	}
	err = RegisterHostLogSink(r, LogSinkSpec{
		Name:     "late",
		OnRecord: func(LogRecord) error { return nil },
	})
	if err != nil {
		t.Fatalf("RegisterHostLogSink on fresh registry: %v", err)
	}
	lsr := HostLogSinks(r)
	if lsr == nil || !lsr.Available("late") {
		t.Error("host sink was not installed on a lazily created registry")
	}
	if got := lsr.Attached(); len(got) != 2 || got[1] != "late" {
		t.Errorf("attached = %v, want [console late]", got)
	}
}

func TestW4SinkValidation(t *testing.T) {
	lsr := newLogSinkRegistry()
	ok := func(LogRecord) error { return nil }
	if err := lsr.RegisterSink(LogSinkSpec{Name: "x"}); err == nil ||
		!strings.Contains(err.Error(), "nil OnRecord") {
		t.Errorf("nil OnRecord: %v", err)
	}
	if err := lsr.RegisterSink(LogSinkSpec{Name: "", OnRecord: ok}); err == nil ||
		!strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("empty name: %v", err)
	}
	if err := lsr.RegisterSink(LogSinkSpec{Name: "Upper", OnRecord: ok}); err == nil ||
		!strings.Contains(err.Error(), "lowercase") {
		t.Errorf("uppercase name: %v", err)
	}
	if err := lsr.RegisterSink(LogSinkSpec{Name: "a b", OnRecord: ok}); err == nil {
		t.Error("whitespace name should be rejected")
	}
	if err := lsr.RegisterSink(LogSinkSpec{Name: "memory", OnRecord: ok}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Errorf("duplicate name: %v", err)
	}
	if err := lsr.RegisterSink(LogSinkSpec{Name: "fine", OnRecord: ok}); err != nil {
		t.Errorf("valid sink rejected: %v", err)
	}
}

// --- policy gating -----------------------------------------------------------------

func TestW4LogPolicyDeniesEmit(t *testing.T) {
	r, _, buf := w4LogEnv(t)
	// Attach memory while still allowed, then install a deny-all log policy.
	w4LogWant(t, r, `log-add-sink memory/q ; log-dump size`, `0`)
	pol, err := policy.LoadInline(`{
		version: 1
		name: "deny-log"
		scopes: { log: { words: { default: "deny" } } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	SetHostPolicy(r, pol)
	buf.Reset()
	// Records are dropped silently.
	w4LogWant(t, r, `log-info "gone" ; log-dump size`, `0`)
	if buf.Len() != 0 {
		t.Errorf("console received a denied record: %s", buf.String())
	}
	// enabled mirrors the policy gate.
	w4LogWant(t, r, `log-enabled fatal/q`, `false`)
	// Sink management surfaces the denial as an error.
	w4LogErr(t, r, `log-add-sink null/q`, "denied by policy")
	w4LogErr(t, r, `log-remove-sink memory/q`, "denied by policy")
	w4LogErr(t, r, `log-register (fn [[rec:Any] [] []]) polly/q info/q`, "denied by policy")
	// Span lifecycle notifications are suppressed (tracked, not exported).
	w4LogWant(t, r, `def s (log-span "quiet") ; s.finish ; log-traces size`, `0`)
	// Measurements are suppressed too.
	w4LogWant(t, r, `def c (log-counter "m") ; c.add 1 ; log-measurements size`, `0`)
}

func TestW4LogPolicyUninstallsScope(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	pol, err := policy.LoadInline(`{
		version: 1
		name: "no-log"
		scopes: { log: { install: false } }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	SetHostPolicy(r, pol)
	// SetHostLogSinks is a no-op under install:false…
	SetHostLogSinks(r, NewLogSinkRegistry())
	if HostLogSinks(r) != nil {
		t.Fatal("sink registry must not install under install:false")
	}
	// …so host sink registration reports the disabled scope.
	err = RegisterHostLogSink(r, LogSinkSpec{
		Name:     "otel",
		OnRecord: func(LogRecord) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("RegisterHostLogSink under install:false = %v", err)
	}
}

// --- direct handler guards (type literals cannot always dispatch) -------------------

// w4Handler finds a signature of n with the given arity and returns its
// dispatch handler.
func w4Handler(t *testing.T, n NativeFunc, arity int) Handler {
	t.Helper()
	for i := range n.Signatures {
		if len(n.Signatures[i].Args) == arity {
			return n.Signatures[i].DispatchHandler()
		}
	}
	t.Fatalf("%s: no %d-arg signature", n.Name, arity)
	return nil
}

func TestW4LogHandlerGuards(t *testing.T) {
	r, lsr, _ := w4LogEnv(t)
	atomLit := NewTypeLiteral(TAtom)
	strLit := NewTypeLiteral(TString)
	mapLit := NewTypeLiteral(TMap)
	listLit := NewTypeLiteral(TList)

	guards := []struct {
		name   string
		fn     NativeFunc
		args   []Value
		substr string
	}{
		{"set-level", logSetLevelNative(lsr), []Value{atomLit}, "must be an atom"},
		{"enabled", logEnabledNative(lsr), []Value{atomLit}, "must be an atom"},
		{"set-format", logSetFormatNative(lsr), []Value{atomLit}, "must be an atom"},
		{"add-sink", logAddSinkNative(lsr), []Value{atomLit}, "must be an atom"},
		{"remove-sink", logRemoveSinkNative(lsr), []Value{atomLit}, "must be an atom"},
		{"log 2", logGenericNative(lsr), []Value{atomLit, NewString("m")}, "must be an atom"},
		{"log 3", logGenericNative(lsr), []Value{atomLit, NewString("m"), mapLit}, "must be an atom"},
		{"logger", logLoggerNative(lsr), []Value{strLit}, "must be a string"},
		{"with", logWithNative(lsr), []Value{strLit, mapLit}, "must be a string"},
		{"span 1", logSpanNative(lsr), []Value{strLit}, "must be a string"},
		{"span 2", logSpanNative(lsr), []Value{strLit, mapLit}, "must be a string"},
		{"with-span", logWithSpanNative(lsr), []Value{strLit, NewList(nil)}, "must be a string"},
		{"with-span body", logWithSpanNative(lsr), []Value{NewString("n"), listLit}, "with-span body"},
		{"end", logEndNative(lsr), []Value{mapLit}, "Log.end"},
		{"counter", logCounterNative(lsr), []Value{strLit}, "must be a string"},
		{"gauge", logGaugeNative(lsr), []Value{strLit}, "must be a string"},
		{"histogram", logHistogramNative(lsr), []Value{strLit}, "must be a string"},
		{"register", logRegisterNative(lsr), []Value{NewList(nil), atomLit, NewAtom("info")}, "must be an atom"},
	}
	for _, g := range guards {
		h := w4Handler(t, g.fn, len(g.args))
		_, err := h(g.args, nil, nil, r)
		if err == nil {
			t.Errorf("%s: expected guard error containing %q", g.name, g.substr)
			continue
		}
		if !strings.Contains(err.Error(), g.substr) {
			t.Errorf("%s: error %q does not contain %q", g.name, err.Error(), g.substr)
		}
	}

	// Span-method guards: key must be an atom, event name a string.
	st := lsr.startSpan(r, "guard", nil)
	for _, n := range spanNatives(st, lsr) {
		switch n.Name {
		case "span-set-attr":
			if _, err := w4Handler(t, n, 2)([]Value{atomLit, NewInteger(1)}, nil, nil, r); err == nil ||
				!strings.Contains(err.Error(), "must be an atom") {
				t.Errorf("set-attr guard: %v", err)
			}
		case "span-add-event":
			if _, err := w4Handler(t, n, 1)([]Value{strLit}, nil, nil, r); err == nil ||
				!strings.Contains(err.Error(), "must be a string") {
				t.Errorf("add-event guard: %v", err)
			}
		}
	}
	lsr.endSpan(r, st)
}

// --- check-mode shape functions -----------------------------------------------------

func TestW4LogShapeFns(t *testing.T) {
	r, lsr, _ := w4LogEnv(t)
	// Logger / span / instrument shape functions return concrete instance
	// maps so dot-access method reads resolve statically.
	if out := loggerShapeReturns(lsr)(nil, r); len(out) != 1 || !IsConcrete(&out[0]) {
		t.Errorf("loggerShapeReturns = %v", out)
	}
	if out := spanShapeReturns(lsr)(nil, r); len(out) != 1 || !IsConcrete(&out[0]) {
		t.Errorf("spanShapeReturns = %v", out)
	}
	// with-span's ReturnsFn walks a concrete body…
	body := NewList([]Value{NewInteger(7)})
	out := withSpanReturnsFn([]Value{NewString("n"), body}, r)
	if len(out) != 1 {
		t.Fatalf("withSpanReturnsFn = %v", out)
	}
	// …an empty body yields NOTHING, matching the handler's `nil, nil`.
	//
	// This assertion used to require one Any carrier, which pinned the
	// phantom as if it were the contract: the handler pushes no value for
	// an empty residual, so modelling one is an arity over-claim that a
	// following word consumes statically and starves on at run time
	// (-force-compile faulted with CALL_NATIVE underflow). Asserting the
	// arity, not merely `len(out) != 1`, is what makes this a real pin.
	out = withSpanReturnsFn([]Value{NewString("n"), NewList(nil)}, r)
	if len(out) != 0 {
		t.Fatalf("withSpanReturnsFn empty = %v, want no values", out)
	}
	// …and a non-list body falls to the gradual dynamic hatch.
	out = withSpanReturnsFn([]Value{NewString("n"), NewTypeLiteral(TList)}, r)
	if len(out) != 1 {
		t.Fatalf("withSpanReturnsFn dynamic = %v", out)
	}
}

// --- small pure helpers ---------------------------------------------------------------

func TestW4LogSmallHelpers(t *testing.T) {
	// mergeAttrs layers defaults under call fields.
	defaults := NewOrderedMap()
	defaults.Set("a", NewInteger(1))
	defaults.Set("b", NewInteger(2))
	call := NewOrderedMap()
	call.Set("b", NewInteger(3))
	merged, err := AsMap(mergeAttrs(defaults, NewMap(call)))
	if err != nil || merged == nil {
		t.Fatalf("mergeAttrs: %v", err)
	}
	if v, _ := merged.Get("b"); FormatForPrint(v) != "3" {
		t.Errorf("call field should win: %v", v)
	}
	if v, _ := merged.Get("a"); FormatForPrint(v) != "1" {
		t.Errorf("default should survive: %v", v)
	}
	// Nil defaults / non-map call are tolerated.
	if m, err := AsMap(mergeAttrs(nil, Value{})); err != nil || m == nil || m.Len() != 0 {
		t.Errorf("mergeAttrs(nil, empty) = %v, %v", m, err)
	}
	// asConcreteOrderedMap rejects non-maps and copies maps.
	if asConcreteOrderedMap(NewTypeLiteral(TMap)) != nil {
		t.Error("type literal is not a concrete map")
	}
	if asConcreteOrderedMap(NewInteger(1)) != nil {
		t.Error("integer is not a concrete map")
	}
	if om := asConcreteOrderedMap(NewMap(call)); om == nil || om.Len() != 1 {
		t.Errorf("asConcreteOrderedMap = %v", om)
	}
	// attributesOrEmpty falls back to a fresh empty map.
	rec := LogRecord{}
	if m, err := AsMap(rec.attributesOrEmpty()); err != nil || m == nil || m.Len() != 0 {
		t.Errorf("attributesOrEmpty zero = %v, %v", m, err)
	}
	// cloneOrderedMap of nil is a fresh empty map.
	if cm := cloneOrderedMap(nil); cm == nil || cm.Len() != 0 {
		t.Errorf("cloneOrderedMap(nil) = %v", cm)
	}
	// omToNative / valToNativeMap round the host view.
	if omToNative(nil) != nil {
		t.Error("omToNative(nil) should be nil")
	}
	if m := omToNative(call); m == nil || m["b"] == nil {
		t.Errorf("omToNative = %v", m)
	}
	if valToNativeMap(NewInteger(1)) != nil {
		t.Error("valToNativeMap(int) should be nil")
	}
}
