package native

import (
	golog "log"
	"sync"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/policy"
)

// log_module.go implements the Go-side machinery for the `boru:log`
// native module (namespace `Log`). The module builder and FnDef
// wrappers live in modules/log.go; this file owns the value types
// (LogLevel, LogRecord), the fan-out sink registry, the built-in
// sinks (console / memory / null), and the word handlers.
//
// Phase 1 (this file) covers the traditional-logging layer: six
// severity levels, structured fields, a global level threshold, a
// console sink built on Go's standard `log` package writing to
// Registry.ErrOutput, plus `memory` (capture-for-tests) and `null`
// sinks. Provider hooks (RegisterHostLogSink / Log.register) and the
// trace/span surface arrive in later phases — see
// design/LOG-MODULE.10.md.
//
// Per-import state (the level, format, attached sinks, capture buffer)
// lives in ONE *LogSinkRegistry captured by the word closures at
// module-build time — the same pattern boru:rand uses for its PRNG
// (modules/rand.go). The registry passed to a handler at dispatch is
// used only for the live ErrOutput / Clock / Policy seams, exactly as
// `print` reaches them.

// LogLevel is a severity, valued at the OpenTelemetry severity number
// for its band so the mapping to the OTel logs data model is lossless.
type LogLevel int

// The six severity bands, on OTel severity numbers.
const (
	LevelTrace LogLevel = 1
	LevelDebug LogLevel = 5
	LevelInfo  LogLevel = 9
	LevelWarn  LogLevel = 13
	LevelError LogLevel = 17
	LevelFatal LogLevel = 21
)

// Name returns the lowercase level atom name (the surface spelling:
// `trace`, `debug`, …) used by Log.get-level and accepted by
// Log.set-level / Log.enabled.
func (l LogLevel) Name() string {
	switch l {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "info"
	}
}

// Text returns the uppercase OTel severity text ("INFO", …).
func (l LogLevel) Text() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "INFO"
	}
}

// parseLevel maps a level atom name to its LogLevel. The second result
// is false for an unrecognised name (the caller raises log_error).
func parseLevel(name string) (LogLevel, bool) {
	switch name {
	case "trace":
		return LevelTrace, true
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn":
		return LevelWarn, true
	case "error":
		return LevelError, true
	case "fatal":
		return LevelFatal, true
	default:
		return 0, false
	}
}

// LogRecord is the neutral OTel-logs shape: the in-memory record every
// sink consumes. A host OTel bridge (phase 3) translates it to an OTel
// LogRecord; the built-in console/memory sinks consume it directly.
// TraceID/SpanID are populated once the span surface lands (phase 4);
// in phase 1 they are always empty so the shape stays stable.
type LogRecord struct {
	Timestamp  time.Time
	Severity   LogLevel
	Body       Value // the message (a String) or any logged Value
	Attributes Value // a concrete Map of structured fields (never a type literal)
	Logger     string
	TraceID    string
	SpanID     string
}

// toMap renders a LogRecord as a Record-shaped Map, the form Log.dump
// returns so boru code (and spec rows) can inspect captured records.
// Field order matches design/LOG-MODULE.10.md §3.2.
func (rec LogRecord) toMap() Value {
	m := NewOrderedMap()
	m.Set("timestamp", NewDateTime(rec.Timestamp))
	m.Set("severity", NewInteger(int64(rec.Severity)))
	m.Set("severity-text", NewString(rec.Severity.Text()))
	m.Set("body", rec.Body)
	m.Set("attributes", rec.attributesOrEmpty())
	m.Set("logger", NewString(rec.Logger))
	m.Set("trace-id", NewString(rec.TraceID))
	m.Set("span-id", NewString(rec.SpanID))
	return NewMap(m)
}

// attributesOrEmpty returns the record's attribute map, or a fresh
// empty Map when no structured fields were supplied (so the record
// shape is uniform and never carries a bare type literal).
func (rec LogRecord) attributesOrEmpty() Value {
	if IsConcrete(rec.Attributes) && rec.Attributes.Parent != nil && rec.Attributes.Parent.Equal(TMap) {
		return rec.Attributes
	}
	return NewMap(NewOrderedMap())
}

// spanEvent is a timestamped event recorded on a span (phase 4).
type spanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes Value
}

// spanState is the mutable state of one span (phase 4). A Span instance
// value (an OrderedMap of method closures) shares the pointer, so
// set-attr / add-event / record-error mutate the live span.
type spanState struct {
	traceID   string
	spanID    string
	parentID  string
	name      string
	attrs     *OrderedMap
	events    []spanEvent
	start     time.Time
	end       time.Time
	status    string // "" (unset/ok) | "error"
	statusMsg string
	ended     bool
}

// Measurement is the neutral OTel-metrics shape (phase 5): one recorded
// value against a named instrument.
type Measurement struct {
	Instrument string
	Kind       string // "counter" | "gauge" | "histogram"
	Value      Value
	Attributes Value
	Timestamp  time.Time
}

// logSink is one named destination. A sink may consume records (emit),
// span lifecycle events (spanStart/spanEnd), and measurements (measure)
// — the latter three are optional (nil) so a logs-only sink is the
// common case. Built-in sinks never error; the error in each signature
// is for the host-registered sinks of phase 3.
type logSink struct {
	name string
	min  LogLevel
	emit func(r *Registry, rec LogRecord) error
	// spanStart may return a host-owned SpanContext to restamp the span's
	// ids; a zero value keeps the local ids.
	spanStart func(r *Registry, sp *spanState) SpanContext
	spanEnd   func(r *Registry, sp *spanState)
	measure   func(r *Registry, m Measurement)
}

// LogSinkRegistry holds the per-import logging state: the global level
// threshold, the console render format, the set of available
// (attachable) sinks, the attachment order, and the memory sink's
// capture buffer. All access is mutex-guarded — a logger may be shared
// across goroutines.
type LogSinkRegistry struct {
	mu        sync.Mutex
	level     LogLevel
	format    string // "text" | "json"
	available map[string]*logSink
	attached  []string
	captured  []LogRecord
	spanStack []*spanState  // active spans (phase 4), innermost last
	spans     []*spanState  // ended spans captured by the memory sink (phase 4)
	measures  []Measurement // measurements captured by the memory sink (phase 5)
	spanSeq   uint64        // per-registry counter for deterministic span ids
}

// NewLogSinkRegistry returns a default sink registry (level INFO, text
// format, console attached, memory/null available). Exposed so a host
// can construct one, pre-attach an OTel/Datadog sink, and install it
// with SetHostLogSinks before the boru program runs `import "boru:log"`.
func NewLogSinkRegistry() *LogSinkRegistry { return newLogSinkRegistry() }

// newLogSinkRegistry builds the default registry: level INFO, text
// format, the three built-in sinks registered, and `console` attached
// — so `Log.info` works the moment the module is imported, with no
// host wiring, exactly like `print`.
func newLogSinkRegistry() *LogSinkRegistry {
	lsr := &LogSinkRegistry{
		level:     LevelInfo,
		format:    "text",
		available: map[string]*logSink{},
		attached:  []string{"console"},
	}
	// console — traditional logging via Go's standard `log` package,
	// writing one line per record to the registry's ErrOutput.
	lsr.available["console"] = &logSink{
		name: "console",
		min:  LevelTrace,
		emit: func(r *Registry, rec LogRecord) error {
			if r == nil || r.ErrOutput == nil {
				return nil
			}
			golog.New(r.ErrOutput, "", 0).Println(lsr.render(rec))
			return nil
		},
		measure: func(r *Registry, m Measurement) {
			if r == nil || r.ErrOutput == nil {
				return
			}
			golog.New(r.ErrOutput, "", 0).Println(renderMeasurement(m))
		},
	}
	// memory — capture records (Log.dump), ended spans (Log.traces), and
	// measurements (Log.measurements) for inspection / tests.
	lsr.available["memory"] = &logSink{
		name: "memory",
		min:  LevelTrace,
		emit: func(_ *Registry, rec LogRecord) error {
			lsr.mu.Lock()
			lsr.captured = append(lsr.captured, rec)
			lsr.mu.Unlock()
			return nil
		},
		spanEnd: func(_ *Registry, sp *spanState) {
			lsr.mu.Lock()
			lsr.spans = append(lsr.spans, sp)
			lsr.mu.Unlock()
		},
		measure: func(_ *Registry, m Measurement) {
			lsr.mu.Lock()
			lsr.measures = append(lsr.measures, m)
			lsr.mu.Unlock()
		},
	}
	// null — discard (benchmarking / silencing).
	lsr.available["null"] = &logSink{
		name: "null",
		min:  LevelTrace,
		emit: func(_ *Registry, _ LogRecord) error { return nil },
	}
	return lsr
}

// Level / SetLevel / Format / SetFormat are the config accessors used
// by the Log.get-level / set-level / get-format / set-format words.
func (lsr *LogSinkRegistry) Level() LogLevel {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return lsr.level
}

func (lsr *LogSinkRegistry) SetLevel(l LogLevel) {
	lsr.mu.Lock()
	lsr.level = l
	lsr.mu.Unlock()
}

func (lsr *LogSinkRegistry) Format() string {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return lsr.format
}

func (lsr *LogSinkRegistry) SetFormat(f string) {
	lsr.mu.Lock()
	lsr.format = f
	lsr.mu.Unlock()
}

// Available reports whether a sink name is registered (attachable).
func (lsr *LogSinkRegistry) Available(name string) bool {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	_, ok := lsr.available[name]
	return ok
}

// Attach adds a registered sink to the active pipeline, idempotently
// (re-attaching an already-attached sink is a no-op, preserving its
// position).
func (lsr *LogSinkRegistry) Attach(name string) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	for _, n := range lsr.attached {
		if n == name {
			return
		}
	}
	lsr.attached = append(lsr.attached, name)
}

// Detach removes a sink from the active pipeline; absent is a no-op.
func (lsr *LogSinkRegistry) Detach(name string) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	out := lsr.attached[:0:0]
	for _, n := range lsr.attached {
		if n != name {
			out = append(out, n)
		}
	}
	lsr.attached = out
}

// Attached returns the attachment order (a copy).
func (lsr *LogSinkRegistry) Attached() []string {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]string(nil), lsr.attached...)
}

// Captured returns the memory sink's buffer (a copy).
func (lsr *LogSinkRegistry) Captured() []LogRecord {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]LogRecord(nil), lsr.captured...)
}

// ClearCaptured empties every memory-sink buffer — records, ended
// spans, and measurements — so `Log.clear` restores full test
// isolation rather than leaving Log.traces / Log.measurements stale.
func (lsr *LogSinkRegistry) ClearCaptured() {
	lsr.mu.Lock()
	lsr.captured = nil
	lsr.spans = nil
	lsr.measures = nil
	lsr.mu.Unlock()
}

// anyAttachedAdmits reports whether at least one attached sink would
// accept a record at the given level (its per-sink minimum is met).
func (lsr *LogSinkRegistry) anyAttachedAdmits(level LogLevel) bool {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	for _, n := range lsr.attached {
		if s := lsr.available[n]; s != nil && level >= s.min {
			return true
		}
	}
	return false
}

// policyAllowsEmit reports whether the log policy scope permits
// telemetry egress for r. A nil policy (the default posture) allows
// everything. Shared by record emission, span lifecycle notification,
// and measurement recording so all three telemetry paths gate
// identically (a sandbox that denies log:emit blocks records, spans,
// AND metrics — not just records).
func policyAllowsEmit(r *Registry, args policy.Args) bool {
	pol := HostPolicy(r)
	if pol == nil {
		return true
	}
	return pol.Check("log", "emit", args) == nil
}

// Emit fans a record out to every attached sink whose threshold admits
// it. Emission is gated on the "log" policy scope: when a policy
// uninstalls (or denies emit on) the scope the record is dropped
// silently — logging must never abort the program. A record below the
// global level is dropped before fan-out.
func (lsr *LogSinkRegistry) Emit(r *Registry, rec LogRecord) {
	if !policyAllowsEmit(r, policy.Args{"level": rec.Severity.Name()}) {
		return
	}
	lsr.mu.Lock()
	if rec.Severity < lsr.level {
		lsr.mu.Unlock()
		return
	}
	order := append([]string(nil), lsr.attached...)
	sinks := make([]*logSink, 0, len(order))
	for _, n := range order {
		sinks = append(sinks, lsr.available[n])
	}
	lsr.mu.Unlock()
	for _, s := range sinks {
		if s == nil || rec.Severity < s.min {
			continue
		}
		_ = s.emit(r, rec) // a sink error must never crash the caller
	}
}

// stampSpan populates a record's TraceID/SpanID from the currently
// active span, if any. Implemented in phase 4 (spans); a no-op until a
// span has been started.
func (lsr *LogSinkRegistry) stampSpan(rec *LogRecord) {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	if n := len(lsr.spanStack); n > 0 {
		sp := lsr.spanStack[n-1]
		rec.TraceID = sp.traceID
		rec.SpanID = sp.spanID
	}
}

// render formats a record for the console sink per the active format.
func (lsr *LogSinkRegistry) render(rec LogRecord) string {
	if lsr.Format() == "json" {
		return renderJSON(rec)
	}
	return renderText(rec)
}

// renderText produces a human-readable line:
//
//	2021-01-01T00:00:00Z INFO  message key=val key2=val2
func renderText(rec LogRecord) string {
	out := rec.Timestamp.UTC().Format(time.RFC3339) + " " +
		PadRight(rec.Severity.Text(), 5) + " " + FormatForPrint(rec.Body)
	if m, err := AsMap(rec.attributesOrEmpty()); err == nil && m != nil {
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			out += " " + k + "=" + FormatForPrint(v)
		}
	}
	// Correlation context — logger name and trace/span ids — so a text
	// line can be tied back to its logger and trace.
	if rec.Logger != "" {
		out += " logger=" + rec.Logger
	}
	if rec.TraceID != "" {
		out += " trace-id=" + rec.TraceID
	}
	if rec.SpanID != "" {
		out += " span-id=" + rec.SpanID
	}
	return out
}

// renderJSON produces a one-line JSON object for log shippers. Falls
// back to the text form if marshalling ever fails.
func renderJSON(rec LogRecord) string {
	obj := map[string]any{
		"time":     rec.Timestamp.UTC().Format(time.RFC3339),
		"level":    rec.Severity.Text(),
		"severity": int(rec.Severity),
		"msg":      core.ToNative(rec.Body),
	}
	if attrs, ok := core.ToNative(rec.attributesOrEmpty()).(map[string]any); ok && len(attrs) > 0 {
		obj["attributes"] = attrs
	}
	// Correlation fields log shippers need to join a record to its logger
	// and trace; included only when present so root/loggerless records
	// stay clean.
	if rec.Logger != "" {
		obj["logger"] = rec.Logger
	}
	if rec.TraceID != "" {
		obj["trace-id"] = rec.TraceID
	}
	if rec.SpanID != "" {
		obj["span-id"] = rec.SpanID
	}
	bs, err := jsonMarshal(obj)
	if err != nil {
		return renderText(rec)
	}
	return string(bs)
}

// LogModuleNativeFuncs builds the Go-implemented words for the boru:log
// module, all closing over the single per-import sink registry `lsr`.
// They are registered ONLY into the module's sub-registry by
// modules.BuildLogModule — deliberately absent from the global
// registry, like the boru:io words.
func LogModuleNativeFuncs(lsr *LogSinkRegistry) []NativeFunc {
	funcs := []NativeFunc{
		logSetLevelNative(lsr),
		logGetLevelNative(lsr),
		logEnabledNative(lsr),
		logSetFormatNative(lsr),
		logGetFormatNative(lsr),
		logAddSinkNative(lsr),
		logRemoveSinkNative(lsr),
		logSinksNative(lsr),
		logDumpNative(lsr),
		logClearNative(lsr),
		logGenericNative(lsr),
		logLoggerNative(lsr),
		logWithNative(lsr),
		logRegisterNative(lsr),
		logSpanNative(lsr),
		logWithSpanNative(lsr),
		logEndNative(lsr),
		logCurrentSpanNative(lsr),
		logTracesNative(lsr),
		logCounterNative(lsr),
		logGaugeNative(lsr),
		logHistogramNative(lsr),
		logMeasurementsNative(lsr),
	}
	for _, lvl := range []struct {
		word  string
		level LogLevel
	}{
		{"log-trace", LevelTrace},
		{"log-debug", LevelDebug},
		{"log-info", LevelInfo},
		{"log-warn", LevelWarn},
		{"log-error", LevelError},
		{"log-fatal", LevelFatal},
	} {
		funcs = append(funcs, levelNative(lsr, lvl.word, lvl.level))
	}
	return SideEffecting(funcs)
}

// emitRecord builds and fans out a top-level (un-named-logger) record.
// attrs is a concrete Map or an empty placeholder Value.
func emitRecord(lsr *LogSinkRegistry, r *Registry, level LogLevel, body, attrs Value) {
	emitWith(lsr, r, level, body, attrs, "")
}

// emitWith builds and fans out a record carrying a logger name and the
// current span context (if any). Used by both the top-level level words
// (logger="") and the contextual-logger instance methods (phase 2).
func emitWith(lsr *LogSinkRegistry, r *Registry, level LogLevel, body, attrs Value, logger string) {
	rec := LogRecord{
		Timestamp:  EffectiveClock(r).Now(),
		Severity:   level,
		Body:       body,
		Attributes: attrs,
		Logger:     logger,
	}
	lsr.stampSpan(&rec)
	lsr.Emit(r, rec)
}

// mergeAttrs returns a fresh Map combining default attributes (lower
// precedence) with call-supplied fields (higher precedence). Either
// side may be an empty placeholder / non-map; the result is always a
// concrete Map. Used by contextual loggers to layer their bound
// defaults under each call's own fields (last-writer-wins).
func mergeAttrs(defaults *OrderedMap, call Value) Value {
	out := NewOrderedMap()
	if defaults != nil {
		for _, k := range defaults.Keys() {
			v, _ := defaults.Get(k)
			out.Set(k, v)
		}
	}
	if IsConcrete(call) && call.Parent != nil && call.Parent.Equal(TMap) {
		if m, err := AsMap(call); err == nil && m != nil {
			for _, k := range m.Keys() {
				v, _ := m.Get(k)
				out.Set(k, v)
			}
		}
	}
	return NewMap(out)
}

// asConcreteOrderedMap returns the *OrderedMap backing a concrete Map
// value, or nil. Used to snapshot a logger's bound default attributes.
func asConcreteOrderedMap(v Value) *OrderedMap {
	if !IsConcrete(v) || v.Parent == nil || !v.Parent.Equal(TMap) {
		return nil
	}
	m, err := AsMap(v)
	if err != nil || m == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil
	}
	out := NewOrderedMap()
	for _, k := range m.Keys() {
		val, _ := m.Get(k)
		out.Set(k, val)
	}
	return out
}

// levelNative builds one severity word (Log.info, …). The 2-arg
// signature (message + fields) is listed FIRST so a call with a
// trailing Map binds it as structured fields rather than the 1-arg
// form greedily claiming only the message.
func levelNative(lsr *LogSinkRegistry, word string, level LogLevel) NativeFunc {
	return NativeFunc{
		Name: word,
		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitRecord(lsr, r, level, args[0], args[1])
					return nil, nil
				}),
			},
			{
				Args:       []*Type{TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitRecord(lsr, r, level, args[0], Value{})
					return nil, nil
				}),
			},
		},
	}
}

// logGenericNative is `Log.log LEVEL MESSAGE [FIELDS]` — emit at a
// computed level.
func logGenericNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-log",
		Signatures: []Signature{
			{
				Args:       []*Type{TAtom, TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					lvl, err := levelArg(r, args[0], "Log.log")
					if err != nil {
						return nil, err
					}
					emitRecord(lsr, r, lvl, args[1], args[2])
					return nil, nil
				}),
			},
			{
				Args:       []*Type{TAtom, TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					lvl, err := levelArg(r, args[0], "Log.log")
					if err != nil {
						return nil, err
					}
					emitRecord(lsr, r, lvl, args[1], Value{})
					return nil, nil
				}),
			},
		},
	}
}

// levelArg reads a level atom and resolves it, raising log_error on an
// unknown name.
func levelArg(r *Registry, v Value, word string) (LogLevel, error) {
	name, err := v.AsConcreteAtom()
	if err != nil {
		return 0, r.BoruError("log_error", "level must be an atom (trace/debug/info/warn/error/fatal)", word)
	}
	lvl, ok := parseLevel(name)
	if !ok {
		return 0, r.BoruError("log_error", "unknown level: "+name, word)
	}
	return lvl, nil
}

func logSetLevelNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-set-level",
		Signatures: []Signature{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				lvl, err := levelArg(r, args[0], "Log.set-level")
				if err != nil {
					return nil, err
				}
				lsr.SetLevel(lvl)
				return nil, nil
			}),
		}},
	}
}

func logGetLevelNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-get-level",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{TAtom},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewAtom(lsr.Level().Name())}, nil
			}),
		}},
	}
}

func logEnabledNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-enabled",
		Signatures: []Signature{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{TBoolean},
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				lvl, err := levelArg(r, args[0], "Log.enabled")
				if err != nil {
					return nil, err
				}
				// Mirror every gate Emit applies, so a true result is a
				// real promise that a record would be emitted: the global
				// threshold, the log policy, AND at least one attached sink
				// whose per-sink minimum admits the level. Otherwise the
				// advertised cheap guard would green-light building
				// expensive fields for records Emit then drops.
				ok := lvl >= lsr.Level() &&
					policyAllowsEmit(r, policy.Args{"level": lvl.Name()}) &&
					lsr.anyAttachedAdmits(lvl)
				return []Value{NewBoolean(ok)}, nil
			}),
		}},
	}
}

func logSetFormatNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-set-format",
		Signatures: []Signature{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.BoruError("log_error", "format must be an atom (text/json)", "Log.set-format")
				}
				if name != "text" && name != "json" {
					return nil, r.BoruError("log_error", "unknown format: "+name+" (want text or json)", "Log.set-format")
				}
				lsr.SetFormat(name)
				return nil, nil
			}),
		}},
	}
}

func logGetFormatNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-get-format",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{TAtom},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewAtom(lsr.Format())}, nil
			}),
		}},
	}
}

func logAddSinkNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-add-sink",
		Signatures: []Signature{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.BoruError("log_error", "sink name must be an atom", "Log.add-sink")
				}
				if err := checkLogInstall(r, "Log.add-sink", name); err != nil {
					return nil, err
				}
				if !lsr.Available(name) {
					return nil, r.BoruError("unknown-sink", "no registered sink named "+name, "Log.add-sink")
				}
				lsr.Attach(name)
				return nil, nil
			}),
		}},
	}
}

func logRemoveSinkNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-remove-sink",
		Signatures: []Signature{{
			Args:       []*Type{TAtom},
			Returns:    []*Type{},
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteAtom()
				if err != nil {
					return nil, r.BoruError("log_error", "sink name must be an atom", "Log.remove-sink")
				}
				if err := checkLogInstall(r, "Log.remove-sink", name); err != nil {
					return nil, err
				}
				lsr.Detach(name)
				return nil, nil
			}),
		}},
	}
}

// checkLogInstall gates a sink-management op on the "log" policy scope.
// Unlike emission (which drops silently), management ops surface the
// denial as an error so the caller knows the change did not take.
func checkLogInstall(r *Registry, word, sink string) error {
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	if err := pol.Check("log", "install", policy.Args{"sink": sink}); err != nil {
		return r.BoruError("log_error", "sink management denied by policy: "+err.Error(), word)
	}
	return nil //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
}

func logSinksNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-sinks",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				names := lsr.Attached()
				out := make([]Value, len(names))
				for i, n := range names {
					out[i] = NewAtom(n)
				}
				return []Value{NewList(out)}, nil
			}),
		}},
	}
}

func logDumpNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-dump",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				recs := lsr.Captured()
				out := make([]Value, len(recs))
				for i, rec := range recs {
					out[i] = rec.toMap()
				}
				return []Value{NewList(out)}, nil
			}),
		}},
	}
}

func logClearNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-clear",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				lsr.ClearCaptured()
				return nil, nil
			}),
		}},
	}
}
