package native

// log_logger.go — phase 2 of boru:log: contextual / named loggers.
//
// `Log.logger NAME` and `Log.with NAME FIELDS` return a logger instance
// (an OrderedMap of method closures, the same shape as a Rand.with-seed
// instance) bound to a name and a set of default attributes that are
// merged into every record it emits. `Logger.child FIELDS` derives a
// child logger layering more defaults. The instance methods are the six
// severity levels plus `child`; they share the module's single
// LogSinkRegistry, so an instance emit fans out through the same sinks
// and obeys the same global threshold as the top-level Log.* words.

// loggerState is the per-instance binding captured by a logger's method
// closures: its name, its default attributes, and the shared sink
// registry.
type loggerState struct {
	name  string
	attrs *OrderedMap // default attributes merged under each call's fields
	lsr   *LogSinkRegistry
}

// buildLoggerInstance constructs the OrderedMap of method FnDefs for one
// logger, each closing over st. Mirrors buildRandExportsForState: a
// fresh sub-registry holds the natives, and the exported FnDef wrappers
// carry that sub-registry so dot-access dispatch finds them.
func buildLoggerInstance(st *loggerState) (*OrderedMap, error) {
	subReg, err := newSubRegistry()
	if err != nil {
		return nil, err
	}
	natives := loggerNatives(st)
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}
	exports := NewOrderedMap()
	for _, n := range natives {
		exports.Set(loggerExportName(n.Name), wrapLoggerFnDef(n, subReg))
	}
	return exports, nil
}

// loggerExportName maps an inner native name (`logger-info`) to the
// dotted method a user types (`info`).
func loggerExportName(inner string) string {
	return inner[len("logger-"):]
}

// wrapLoggerFnDef builds the trivial-delegation FnDef wrapper for one
// logger method, carrying the instance's sub-registry.
func wrapLoggerFnDef(n NativeFunc, subReg *Registry) Value {
	sigs := make([]FnSig, len(n.Signatures))
	for i, s := range n.Signatures {
		params := make([]FnParam, len(s.Args))
		for j, t := range s.Args {
			params[j] = FnParam{Type: t}
		}
		sigs[i] = FnSig{
			Params:     params,
			Returns:    s.Returns,
			Impl:       Boru([]Value{NewWord(n.Name)}),
			BarrierPos: -1,
		}
	}
	return NewFunction(FnDefInfo{Name: n.Name, Signatures: sigs, Registry: subReg})
}

// loggerNatives builds the instance methods closing over st.
func loggerNatives(st *loggerState) []NativeFunc {
	funcs := []NativeFunc{loggerChildNative(st)}
	for _, lvl := range []struct {
		word  string
		level LogLevel
	}{
		{"logger-trace", LevelTrace},
		{"logger-debug", LevelDebug},
		{"logger-info", LevelInfo},
		{"logger-warn", LevelWarn},
		{"logger-error", LevelError},
		{"logger-fatal", LevelFatal},
	} {
		funcs = append(funcs, loggerLevelNative(st, lvl.word, lvl.level))
	}
	return SideEffecting(funcs)
}

// loggerLevelNative builds one severity method (l.info, …) that emits
// with the logger's name and its defaults merged under the call fields.
func loggerLevelNative(st *loggerState, word string, level LogLevel) NativeFunc {
	return NativeFunc{
		Name: word,
		Signatures: []Signature{
			{
				Args:       []*Type{TAny, TMap},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitWith(st.lsr, r, level, args[0], mergeAttrs(st.attrs, args[1]), st.name)
					return nil, nil
				}),
			},
			{
				Args:       []*Type{TAny},
				Returns:    []*Type{},
				BarrierPos: -1,
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					emitWith(st.lsr, r, level, args[0], mergeAttrs(st.attrs, Value{}), st.name)
					return nil, nil
				}),
			},
		},
	}
}

// loggerChildNative builds `child FIELDS` — a new logger with the same
// name whose defaults are the parent's overlaid with FIELDS. Its
// check-mode shape is a logger instance like the parent's
// (loggerShapeReturns — the method SHAPE is state-independent), so a
// derived child's own method reads resolve statically too
// (`def c (l.child {y:2}) ; c.info "m"`, module-log.tsv:55).
func loggerChildNative(st *loggerState) NativeFunc {
	return NativeFunc{
		Name: "logger-child",
		Signatures: []Signature{{
			Args:       []*Type{TMap},
			Returns:    []*Type{TMap},
			ReturnsFn:  loggerShapeReturns(st.lsr),
			BarrierPos: -1,
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				merged := asConcreteOrderedMap(mergeAttrs(st.attrs, args[0]))
				inst, err := buildLoggerInstance(&loggerState{name: st.name, attrs: merged, lsr: st.lsr})
				if err != nil {
					return nil, r.BoruError("log_error", "child logger: "+err.Error(), "Logger.child")
				}
				return []Value{NewMap(inst)}, nil
			}),
		}},
	}
}

// logLoggerNative is the top-level `Log.logger NAME` — a logger with no
// default attributes.
func logLoggerNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-logger",
		Signatures: []Signature{{
			Args:       []*Type{TString},
			Returns:    []*Type{TMap},
			BarrierPos: -1,
			ReturnsFn:  loggerShapeReturns(lsr),
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, r.BoruError("log_error", "logger name must be a string", "Log.logger")
				}
				inst, err := buildLoggerInstance(&loggerState{name: name, attrs: NewOrderedMap(), lsr: lsr})
				if err != nil {
					return nil, r.BoruError("log_error", err.Error(), "Log.logger")
				}
				return []Value{NewMap(inst)}, nil
			}),
		}},
	}
}

// logWithNative is the top-level `Log.with NAME FIELDS` — a logger
// carrying default attributes.
func logWithNative(lsr *LogSinkRegistry) NativeFunc {
	return NativeFunc{
		Name: "log-with",
		Signatures: []Signature{{
			Args:       []*Type{TString, TMap},
			Returns:    []*Type{TMap},
			BarrierPos: -1,
			ReturnsFn:  loggerShapeReturns(lsr),
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, r.BoruError("log_error", "logger name must be a string", "Log.with")
				}
				inst, err := buildLoggerInstance(&loggerState{
					name:  name,
					attrs: asConcreteOrderedMap(args[1]),
					lsr:   lsr,
				})
				if err != nil {
					return nil, r.BoruError("log_error", err.Error(), "Log.with")
				}
				return []Value{NewMap(inst)}, nil
			}),
		}},
	}
}

// loggerShapeReturns is the check-mode shape for Log.logger / Log.with.
// The handler does not run during static analysis, so without this the
// result is a shapeless Map carrier and `l.info` could not resolve the
// method wrapper. This returns a CONCRETE instance Map (built from a
// shape-only state) so the field read resolves the closure-bearing
// wrapper — identical to randWithSeedReturns. The method SHAPE is
// state-independent, so no per-call behaviour is baked.
func loggerShapeReturns(lsr *LogSinkRegistry) func([]Value, *Registry) []Value {
	return func(_ []Value, _ *Registry) []Value {
		inst, err := buildLoggerInstance(&loggerState{name: "", attrs: NewOrderedMap(), lsr: lsr})
		if err != nil {
			return []Value{NewCarrier(TMap)}
		}
		MarkShapeModel(inst) // a model named "", never the run's logger
		return []Value{NewMap(inst)}
	}
}
