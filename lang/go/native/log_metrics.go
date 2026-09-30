package native

import (
	"time"

	"github.com/boru-lang/boru/lang/go/policy"
)

// log_metrics.go — phase 5 of boru:log: the OpenTelemetry *metrics*
// signal. `Log.counter NAME`, `Log.gauge NAME`, and `Log.histogram NAME`
// return instrument handles (the loggers/spans instance shape); a
// counter's `add` and a gauge/histogram's `record` method produce a
// neutral Measurement that fans out to every attached sink's measure
// hook. The memory sink captures them for `Log.measurements`; a host
// OTel sink's OnMeasure forwards them to a Meter. Metrics carry no
// severity, so the global level threshold does not apply — only the
// `log` policy scope gates them (telemetry egress).

// instrumentState is the per-instrument binding captured by its method
// closures: the instrument name, its kind, and the shared registry.
type instrumentState struct {
	name string
	kind string // "counter" | "gauge" | "histogram"
	lsr  *LogSinkRegistry
}

// recordMeasurement fans a measurement out to every attached sink's
// measure hook, gated on the log policy scope (dropped silently when
// denied — telemetry must never abort the program).
func (lsr *LogSinkRegistry) recordMeasurement(r *Registry, m Measurement) {
	if pol := HostPolicy(r); pol != nil {
		if pol.Check("log", "emit", policy.Args{"metric": m.Instrument}) != nil {
			return
		}
	}
	lsr.mu.Lock()
	sinks := lsr.attachedSinksLocked()
	lsr.mu.Unlock()
	for _, s := range sinks {
		if s.measure != nil {
			s.measure(r, m)
		}
	}
}

// Measurements returns the measurements captured by the memory sink.
func (lsr *LogSinkRegistry) Measurements() []Measurement {
	lsr.mu.Lock()
	defer lsr.mu.Unlock()
	return append([]Measurement(nil), lsr.measures...)
}

// attrsOrEmpty returns v if it is a concrete Map, else a fresh empty Map.
func attrsOrEmpty(v Value) Value {
	if IsConcrete(v) && v.Parent != nil && v.Parent.Equal(TMap) {
		return v
	}
	return NewMap(NewOrderedMap())
}

// measurementToMap renders a Measurement as a Record-shaped Map for
// Log.measurements.
func measurementToMap(m Measurement) Value {
	om := NewOrderedMap()
	om.Set("instrument", NewString(m.Instrument))
	om.Set("kind", NewString(m.Kind))
	om.Set("value", m.Value)
	om.Set("attributes", attrsOrEmpty(m.Attributes))
	om.Set("timestamp", NewDateTime(m.Timestamp))
	return NewMap(om)
}

// renderMeasurement formats a measurement for the console sink:
//
//	2021-01-01T00:00:00Z METRIC counter requests=1 region=eu
func renderMeasurement(m Measurement) string {
	out := m.Timestamp.UTC().Format(time.RFC3339) + " METRIC " + m.Kind + " " +
		m.Instrument + "=" + FormatForPrint(m.Value)
	if am, err := AsMap(attrsOrEmpty(m.Attributes)); err == nil && am != nil {
		for _, k := range am.Keys() {
			v, _ := am.Get(k)
			out += " " + k + "=" + FormatForPrint(v)
		}
	}
	return out
}

// buildInstrumentInstance constructs an instrument handle: an
// OrderedMap with a "name" / "kind" field and a single method — `add`
// for a counter, `record` for a gauge or histogram.
func buildInstrumentInstance(st *instrumentState) (*OrderedMap, error) {
	subReg, err := newSubRegistry()
	if err != nil {
		return nil, err
	}
	method := instrumentMethodName(st.kind)
	n := instrumentNative("instrument-"+method, st)
	n.CompileEffect |= CompileSideEffect
	subReg.RegisterNativeFunc(n)
	inst := NewOrderedMap()
	inst.Set("name", NewString(st.name))
	inst.Set("kind", NewString(st.kind))
	inst.Set(method, wrapLoggerFnDef(n, subReg))
	return inst, nil
}

// instrumentMethodName picks the recording verb for a kind.
func instrumentMethodName(kind string) string {
	if kind == "counter" {
		return "add"
	}
	return "record"
}

// instrumentNative builds the recording method (add / record) closing
// over the instrument state.
func instrumentNative(word string, st *instrumentState) NativeFunc {
	record := func(r *Registry, value, attrs Value) {
		st.lsr.recordMeasurement(r, Measurement{
			Instrument: st.name,
			Kind:       st.kind,
			Value:      value,
			Attributes: attrs,
			Timestamp:  EffectiveClock(r).Now(),
		})
	}
	return NativeFunc{
		Name: word,
		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					record(r, args[0], args[1])
					return nil, nil
				}),
			},
			{
				Args:       []*Type{TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					record(r, args[0], NewMap(NewOrderedMap()))
					return nil, nil
				}),
			},
		},
	}
}

// instrumentCtor builds a top-level instrument constructor (Log.counter
// / Log.gauge / Log.histogram).
func instrumentCtor(word, kind string, lsr *LogSinkRegistry) NativeFunc {
	shape := func(_ []Value, _ *Registry) []Value {
		inst, err := buildInstrumentInstance(&instrumentState{kind: kind, lsr: lsr})
		if err != nil {
			return []Value{NewCarrier(TMap)}
		}
		return []Value{NewMap(inst)}
	}
	return NativeFunc{
		Name: word,
		Signatures: []Signature{{
			Args:       []*Type{TString},
			Returns:    []*Type{TMap},
			BarrierPos: -1,
			ReturnsFn:  shape,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, r.BoruError("log_error", "instrument name must be a string", word)
				}
				inst, err := buildInstrumentInstance(&instrumentState{name: name, kind: kind, lsr: lsr})
				if err != nil {
					return nil, r.BoruError("log_error", err.Error(), word)
				}
				return []Value{NewMap(inst)}, nil
			}),
		}},
	}
}

func logCounterNative(lsr *LogSinkRegistry) NativeFunc {
	return instrumentCtor("log-counter", "counter", lsr)
}

func logGaugeNative(lsr *LogSinkRegistry) NativeFunc {
	return instrumentCtor("log-gauge", "gauge", lsr)
}

func logHistogramNative(lsr *LogSinkRegistry) NativeFunc {
	return instrumentCtor("log-histogram", "histogram", lsr)
}

// logMeasurementsNative is `Log.measurements` — the measurements
// captured by the memory sink.
func logMeasurementsNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-measurements",
		Signatures: []Signature{{
			Args:       []*Type{},
			Returns:    []*Type{TList},
			BarrierPos: -1,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				ms := lsr.Measurements()
				out := make([]Value, len(ms))
				for i, m := range ms {
					out[i] = measurementToMap(m)
				}
				return []Value{NewList(out)}, nil
			}),
		}},
	}
}
