package modules

import (
	"fmt"
	"time"

	"github.com/boru-lang/boru/lang/go/native"
)

// BuildTimeModule creates the "boru:time-util" native module.
func BuildTimeModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	// The module-owned temporal types (TimeOfDay, Duration, CalendarDuration,
	// ClockDuration, Timezone) escape to the importer — as exported type
	// literals, as the Parent tag on every duration/timezone value the
	// words produce, and inside the add/sub word-extension signatures —
	// so they must draw IDs from the importing tree's counter (see eng
	// TypeTable.mintID and the BuildIOModule / StreamKind precedent).
	subReg.Types.AdoptSeqFrom(parent.Types)
	subReg.Types.MintOwner = "boru:time-util"
	tt := native.MintTemporalModuleTypes(subReg)

	for _, n := range timeNatives(tt) {
		subReg.RegisterNativeFunc(n)
	}
	// now / sleep / timeout / interval / await / cancel moved here from core.
	asyncNatives := native.TimeAsyncModuleNatives(tt)
	for _, n := range asyncNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := delegatingExports(asyncNatives, subReg)

	// Construction — numeric and IANA-zone only. ISO 8601 date /
	// datetime / instant / time-of-day / duration string parsing
	// was removed as a feature; construct dates from `now-local`,
	// `today`, `today-utc`, `unix`, `unix-ms`, `unix-ns`, the
	// numeric duration constructors (`years`, `months`, `days`,
	// `hours`, `minutes`, `seconds`, …), or `cal-dur` directly.
	exports.Set("tz", makeTypedFnDef("time-tz", subReg, tt.Timezone, native.TString))
	exports.Set("unix", makeTypedFnDef("time-unix", subReg, native.TInstant, native.TInteger))
	exports.Set("unix-ms", makeTypedFnDef("time-unix-ms", subReg, native.TInstant, native.TInteger))
	exports.Set("unix-ns", makeTypedFnDef("time-unix-ns", subReg, native.TInstant, native.TInteger))

	// Current time
	exports.Set("now-local", makeTypedFnDef("time-now-local", subReg, native.TDateTime))
	exports.Set("today", makeTypedFnDef("time-today", subReg, native.TDate))
	exports.Set("today-utc", makeTypedFnDef("time-today-utc", subReg, native.TDate))

	// Extraction (Date -> Integer)
	for _, name := range []string{"year", "month", "day", "weekday", "year-day", "iso-week", "quarter", "days-in-month", "days-in-year"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TInteger, native.TDate))
	}
	for _, name := range []string{"weekday-name", "month-name"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TString, native.TDate))
	}
	exports.Set("is-leap-year", makeTypedFnDef("is-leap-year", subReg, native.TBoolean, native.TDate))

	// Extraction from Instant
	exports.Set("to-unix", makeTypedFnDef("to-unix", subReg, native.TInteger, native.TInstant))
	exports.Set("to-unix-ms", makeTypedFnDef("to-unix-ms", subReg, native.TInteger, native.TInstant))

	// Comparison (Date Date -> Boolean)
	for _, name := range []string{"is-before", "is-after", "is-equal"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TBoolean, native.TDate, native.TDate))
	}

	// Formatting
	exports.Set("to-string", makeTypedFnDef("to-string", subReg, native.TString, native.TDate))
	// Params match the inner native's Args (sig order: top-first).
	// stack `date layout`: Params[0]=layout (top, String), Params[1]=date (deeper, Date).
	exports.Set("format", makeTypedFnDef("format", subReg, native.TString, native.TString, native.TDate))
	exports.Set("to-iso", makeTypedFnDef("to-iso", subReg, native.TString, native.TDate))

	// Duration construction
	for _, name := range []string{"years", "months", "weeks", "days"} {
		exports.Set(name, makeTypedFnDef(name, subReg, tt.CalendarDuration, native.TInteger))
	}
	for _, name := range []string{"hours", "minutes", "seconds", "ms", "us", "ns"} {
		exports.Set(name, makeTypedFnDef(name, subReg, tt.ClockDuration, native.TNumber))
	}
	exports.Set("cal-dur", makeTypedFnDef("cal-dur", subReg, tt.CalendarDuration, native.TInteger, native.TInteger, native.TInteger))
	// ISO 8601 duration string parsing (`"P1Y2M3D" duration`)
	// removed as a feature. Use cal-dur or the years/months/days
	// constructors directly.

	// Duration extraction
	for _, name := range []string{"total-hours", "total-minutes", "total-seconds", "total-ms"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TFloat, tt.ClockDuration))
	}
	for _, name := range []string{"dur-years", "dur-months", "dur-days"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TInteger, tt.CalendarDuration))
	}
	exports.Set("dur-sign", makeTypedFnDef("dur-sign", subReg, native.TInteger, tt.CalendarDuration))

	// Arithmetic
	exports.Set("until", makeTypedFnDef("until", subReg, tt.CalendarDuration, native.TDate, native.TDate))
	exports.Set("since", makeTypedFnDef("since", subReg, tt.CalendarDuration, native.TDate, native.TDate))
	exports.Set("diff", makeTypedFnDef("diff", subReg, tt.ClockDuration, native.TInstant, native.TInstant))
	exports.Set("elapsed", makeTypedFnDef("elapsed", subReg, tt.ClockDuration, native.TInstant))

	// Comparison extended
	exports.Set("compare", makeTypedFnDef("time-compare", subReg, native.TInteger, native.TDate, native.TDate))
	exports.Set("is-between", makeTypedFnDef("is-between", subReg, native.TBoolean, native.TDate, native.TDate, native.TDate))
	exports.Set("earliest", makeTypedFnDef("earliest", subReg, native.TDate, native.TDate, native.TDate))
	exports.Set("latest", makeTypedFnDef("latest", subReg, native.TDate, native.TDate, native.TDate))

	// Conversion
	exports.Set("to-date", makeTypedFnDef("to-date", subReg, native.TDate, native.TDateTime))
	exports.Set("to-time-of-day", makeTypedFnDef("to-time-of-day", subReg, tt.TimeOfDay, native.TDateTime))
	exports.Set("to-datetime", makeTypedFnDef("to-datetime", subReg, native.TDateTime, native.TDate))
	// Wrapper Params match the inner native Args (sig order, top-first).
	exports.Set("to-instant", makeTypedFnDef("to-instant", subReg, native.TInstant, tt.Timezone, native.TDateTime))
	exports.Set("to-local", makeTypedFnDef("to-local", subReg, native.TDateTime, tt.Timezone, native.TInstant))
	exports.Set("to-utc", makeTypedFnDef("to-utc", subReg, native.TDateTime, native.TInstant))

	// Rounding
	exports.Set("start-of", makeTypedFnDef("start-of", subReg, native.TDate, native.TString, native.TDate))
	exports.Set("end-of", makeTypedFnDef("end-of", subReg, native.TDate, native.TString, native.TDate))

	// Timezone
	exports.Set("tz-utc", makeTypedFnDef("tz-utc", subReg, tt.Timezone))
	exports.Set("tz-local", makeTypedFnDef("tz-local", subReg, tt.Timezone))
	exports.Set("tz-name", makeTypedFnDef("tz-name", subReg, native.TString, tt.Timezone))
	exports.Set("tz-offset", makeTypedFnDef("tz-offset", subReg, native.TString, tt.Timezone, native.TInstant))
	exports.Set("is-dst", makeTypedFnDef("is-dst", subReg, native.TBoolean, tt.Timezone, native.TInstant))

	// Parsing — removed as a feature. parse-date / parse-datetime
	// (layout-based) and auto-date (auto-format-detecting) are gone.

	// Wrapper Params match inner native Args (sig order, top-first):
	// stack `date n add-days` → top=n (Integer)=sig[0], deeper=date (Date)=sig[1].
	for _, name := range []string{"add-days", "add-months", "add-years"} {
		exports.Set(name, makeTypedFnDef(name, subReg, native.TDate, native.TInteger, native.TDate))
	}

	// The module-owned temporal types, exported as type literals so the
	// importer can reference them — `x is TimeUtil.CalendarDuration`,
	// `def d:(TimeUtil.ClockDuration) …` — mirroring IO.StreamKind.
	exports.Set("TimeOfDay", native.NewTypeLiteral(tt.TimeOfDay))
	exports.Set("Duration", native.NewTypeLiteral(tt.Duration))
	exports.Set("CalendarDuration", native.NewTypeLiteral(tt.CalendarDuration))
	exports.Set("ClockDuration", native.NewTypeLiteral(tt.ClockDuration))
	exports.Set("Timezone", native.NewTypeLiteral(tt.Timezone))
	exports.Set("Timeout", native.NewTypeLiteral(tt.Timeout))
	exports.Set("Interval", native.NewTypeLiteral(tt.Interval))

	// The temporal add / sub overloads, exported as WORD EXTENSIONS
	// (design/OPEN-WORDS.0.md): import transplants them onto the
	// importer's bare add / sub, and TimeUtil.add / TimeUtil.sub work
	// namespaced. The duration operand types are this import's mints,
	// which is what satisfies the module-scope user-type rule.
	for _, ext := range native.TemporalArithmeticExtensions(tt) {
		exports.Set(ext.Extends, native.NewFunction(ext))
	}

	return moduleDesc(parent, "TimeUtil", subReg, exports), nil
}

// --- helpers ---

// extractTime returns the time.Time from a Date, DateTime, or Instant value.
func extractTime(v native.Value) time.Time {
	if tp, ok := v.Data.(native.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// dateDiffCalendarDuration computes the CalendarDuration between two dates (from → to).
func dateDiffCalendarDuration(from, to time.Time) native.CalDurationData {
	years := to.Year() - from.Year()
	months := int(to.Month()) - int(from.Month())
	days := to.Day() - from.Day()
	if days < 0 {
		months--
		// Days in the previous month of 'to'
		prev := time.Date(to.Year(), to.Month(), 0, 0, 0, 0, 0, time.UTC)
		days += prev.Day()
	}
	if months < 0 {
		years--
		months += 12
	}
	return native.CalDurationData{Years: years, Months: months, Days: days}
}

// ISO 8601 duration parsing (parseISO8601Duration) and auto-date
// layout tables (autoDateLayouts) removed at the parser-hand-off
// step. boru no longer parses dates / times / durations from
// strings; construct via numeric helpers (cal-dur, years, months,
// days, hours, …) or via current-time sources (today, now-local,
// unix, unix-ms, unix-ns).

// --- builders for repeating shapes ---

// dateToIntNative builds a NativeFunc with a single [TDate] -> [TInteger]
// signature whose handler applies fn to the extracted time.Time.
func dateToIntNative(name string, fn func(time.Time) int64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TDate},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewInteger(fn(extractTime(args[0])))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	}
}

// dateToStringNative builds a NativeFunc with a single [TDate] -> [TString]
// signature whose handler applies fn to the extracted time.Time.
func dateToStringNative(name string, fn func(time.Time) string) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TDate},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewString(fn(extractTime(args[0])))}, nil
			}),
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	}
}

// intToCalendarDurationNative builds a NativeFunc with [TInteger] -> [TCalendarDuration]
// where the handler turns the integer into a CalDurationData via fn.
func intToCalendarDurationNative(tt native.TemporalModuleTypes, name string, fn func(int) (int, int, int)) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				y, m, d := fn(int(n))
				return []native.Value{tt.NewCalendarDuration(y, m, d)}, nil
			}),
			Returns: []*native.Type{tt.CalendarDuration}, BarrierPos: -1,
		}},
	}
}

// numToClockDurationNative builds a NativeFunc with [TNumber] -> [TClockDuration]
// where the handler scales the numeric input by `unit`.
func numToClockDurationNative(tt native.TemporalModuleTypes, name string, unit time.Duration) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TNumber},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := native.AsNumber(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{tt.NewClockDuration(time.Duration(n * float64(unit)))}, nil
			}),
			Returns: []*native.Type{tt.ClockDuration}, BarrierPos: -1,
		}},
	}
}

// clkDurationToFloatNative builds a NativeFunc with
// [TClockDuration] -> [TFloat] for a total-* extraction. `returnType`
// is exposed because total-ms historically declared TInteger even
// though the value pushed is Float.
func clkDurationToFloatNative(tt native.TemporalModuleTypes, name string, returnType *native.Type, fn func(time.Duration) float64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{tt.ClockDuration},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				d, _ := native.AsClockDuration(args[0])
				return []native.Value{native.NewFloat(fn(d))}, nil
			}),
			Returns: []*native.Type{returnType}, BarrierPos: -1,
		}},
	}
}

// calDurationToIntNative builds a NativeFunc with
// [TCalendarDuration] -> [TInteger] for dur-years/dur-months/dur-days.
func calDurationToIntNative(tt native.TemporalModuleTypes, name string, fn func(native.CalDurationData) int64) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{tt.CalendarDuration},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				cd, _ := native.AsCalendarDuration(args[0])
				return []native.Value{native.NewInteger(fn(cd))}, nil
			}),
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	}
}

// addDateNative builds the add-days/add-months/add-years words. The
// fn signature mirrors time.Time.AddDate's parameters so each word
// just supplies (years, months, days) shifts driven by the integer arg.
func addDateNative(name string, build func(n int) (years, months, days int)) native.NativeFunc {
	return native.NativeFunc{
		Name: name,

		Signatures: []native.Signature{{
			Args: []*native.Type{native.TInteger, native.TDate},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := args[0].AsConcreteInteger()
				if err != nil {
					return nil, err
				}
				t := extractTime(args[1])
				y, m, d := build(int(n))
				return []native.Value{native.NewDate(t.AddDate(y, m, d))}, nil
			}),
			// Every path builds a Date — the shift changes the value, never
			// the type.
			Returns: []*native.Type{native.TDate}, BarrierPos: -1,
		}},
	}
}

// --- Word definitions ---

// timeNatives builds the consolidated NativeFunc slice for the time
// module's Go-implemented words, parameterised by the per-import
// temporal type mints (tt) so every duration / timezone / time-of-day
// signature and constructed value carries that import's type identity.
func timeNatives(tt native.TemporalModuleTypes) []native.NativeFunc {
	out := []native.NativeFunc{
		// --- Construction ---
		// ISO 8601 string parsers (time-date, time-datetime,
		// time-instant, time-time-of-day) were removed as a feature.
		// Construct via numeric helpers below or via the
		// extraction/conversion words.
		{
			Name: "time-tz",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					s, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					loc, err := time.LoadLocation(s)
					if err != nil {
						return nil, fmt.Errorf("tz: unknown timezone: %q", s)
					}
					return []native.Value{tt.NewTimezone(loc)}, nil
				}),
				Returns: []*native.Type{tt.Timezone}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.Unix(n, 0))}, nil
				}),
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix-ms",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.UnixMilli(n))}, nil
				}),
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			Name: "time-unix-ns",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					n, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewInstant(time.Unix(0, n))}, nil
				}),
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		// --- Current time (stack-only zero-arg words) ---
		{
			Name:          "time-now-local",
			CompileEffect: native.CompileSideEffect,

			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewDateTime(native.EffectiveClock(r).Now())}, nil
				}),
				Returns: []*native.Type{native.TDateTime}, BarrierPos: 0,
			}},
		},
		{
			Name:          "time-today",
			CompileEffect: native.CompileSideEffect,

			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					now := native.EffectiveClock(r).Now()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
					return []native.Value{native.NewDate(d)}, nil
				}),
				Returns: []*native.Type{native.TDate}, BarrierPos: 0,
			}},
		},
		{
			Name:          "time-today-utc",
			CompileEffect: native.CompileSideEffect,

			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					now := native.EffectiveClock(r).Now().UTC()
					d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
					return []native.Value{native.NewDate(d)}, nil
				}),
				Returns: []*native.Type{native.TDate}, BarrierPos: 0,
			}},
		},
		// --- Extraction (Date -> Integer) ---
		dateToIntNative("year", func(t time.Time) int64 { return int64(t.Year()) }),
		dateToIntNative("month", func(t time.Time) int64 { return int64(t.Month()) }),
		dateToIntNative("day", func(t time.Time) int64 { return int64(t.Day()) }),
		dateToIntNative("weekday", func(t time.Time) int64 {
			wd := t.Weekday()
			iso := int64(wd)
			if wd == time.Sunday {
				iso = 7
			}
			return iso
		}),
		dateToIntNative("year-day", func(t time.Time) int64 { return int64(t.YearDay()) }),
		dateToStringNative("weekday-name", func(t time.Time) string { return t.Weekday().String() }),
		dateToStringNative("month-name", func(t time.Time) string { return t.Month().String() }),
		dateToIntNative("iso-week", func(t time.Time) int64 {
			_, week := t.ISOWeek()
			return int64(week)
		}),
		dateToIntNative("quarter", func(t time.Time) int64 {
			m := t.Month()
			return int64((int(m) + 2) / 3)
		}),
		dateToIntNative("days-in-month", func(t time.Time) int64 {
			last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
			return int64(last.Day())
		}),
		dateToIntNative("days-in-year", func(t time.Time) int64 {
			y := t.Year()
			start := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(y+1, 1, 1, 0, 0, 0, 0, time.UTC)
			return int64(end.Sub(start).Hours() / 24)
		}),
		{
			Name: "is-leap-year",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					y := extractTime(args[0]).Year()
					leap := y%4 == 0 && (y%100 != 0 || y%400 == 0)
					return []native.Value{native.NewBoolean(leap)}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// to-unix / to-unix-ms (Instant -> Integer)
		{
			Name: "to-unix",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewInteger(extractTime(args[0]).Unix())}, nil
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-unix-ms",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewInteger(extractTime(args[0]).UnixMilli())}, nil
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		// --- Comparison (Date Date -> Boolean) ---
		{
			Name: "is-before",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[1]).Before(extractTime(args[0])))}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "is-after",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[1]).After(extractTime(args[0])))}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "is-equal",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewBoolean(extractTime(args[0]).Equal(extractTime(args[1])))}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// --- Formatting ---
		{
			Name: "to-string",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "format",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					layout, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					return []native.Value{native.NewString(extractTime(args[1]).Format(layout))}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-iso",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{native.NewString(extractTime(args[0]).Format("2006-01-02"))}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		// --- Legacy arithmetic ---
		addDateNative("add-days", func(n int) (int, int, int) { return 0, 0, n }),
		addDateNative("add-months", func(n int) (int, int, int) { return 0, n, 0 }),
		addDateNative("add-years", func(n int) (int, int, int) { return n, 0, 0 }),
		// --- Duration construction (Integer -> CalendarDuration) ---
		intToCalendarDurationNative(tt, "years", func(n int) (int, int, int) { return n, 0, 0 }),
		intToCalendarDurationNative(tt, "months", func(n int) (int, int, int) { return 0, n, 0 }),
		intToCalendarDurationNative(tt, "weeks", func(n int) (int, int, int) { return 0, 0, n * 7 }),
		intToCalendarDurationNative(tt, "days", func(n int) (int, int, int) { return 0, 0, n }),
		// --- Duration construction (Number -> ClockDuration) ---
		numToClockDurationNative(tt, "hours", time.Hour),
		numToClockDurationNative(tt, "minutes", time.Minute),
		numToClockDurationNative(tt, "seconds", time.Second),
		numToClockDurationNative(tt, "ms", time.Millisecond),
		numToClockDurationNative(tt, "us", time.Microsecond),
		numToClockDurationNative(tt, "ns", time.Nanosecond),
		{
			// cal-dur 1 6 15 → args[0]=15 (nearest), args[1]=6, args[2]=1 (deepest)
			Name: "cal-dur",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					y, err := args[2].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					m, err := args[1].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					d, err := args[0].AsConcreteInteger()
					if err != nil {
						return nil, err
					}
					return []native.Value{tt.NewCalendarDuration(int(y), int(m), int(d))}, nil
				}),
				Returns: []*native.Type{tt.CalendarDuration}, BarrierPos: -1,
			}},
		},
		// time-duration (ISO 8601 P1Y2M3D parser) removed as a feature.
		// --- Duration extraction ---
		clkDurationToFloatNative(tt, "total-hours", native.TFloat, func(d time.Duration) float64 { return d.Hours() }),
		clkDurationToFloatNative(tt, "total-minutes", native.TFloat, func(d time.Duration) float64 { return d.Minutes() }),
		clkDurationToFloatNative(tt, "total-seconds", native.TFloat, func(d time.Duration) float64 { return d.Seconds() }),
		clkDurationToFloatNative(tt, "total-ms", native.TFloat, func(d time.Duration) float64 { return float64(d.Milliseconds()) }),
		calDurationToIntNative(tt, "dur-years", func(cd native.CalDurationData) int64 { return int64(cd.Years) }),
		calDurationToIntNative(tt, "dur-months", func(cd native.CalDurationData) int64 { return int64(cd.Months) }),
		calDurationToIntNative(tt, "dur-days", func(cd native.CalDurationData) int64 { return int64(cd.Days) }),
		// dur-sign — two overloads (CalendarDuration and ClockDuration), both
		// return -1/0/1 integers. Historically two separate r.Register
		// calls; here unified into one NativeFunc with two signatures.
		{
			Name: "dur-sign",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{tt.CalendarDuration},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						cd, _ := native.AsCalendarDuration(args[0])
						total := cd.Years*365 + cd.Months*30 + cd.Days
						switch {
						case total < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case total > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{tt.ClockDuration},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						d, _ := native.AsClockDuration(args[0])
						switch {
						case d < 0:
							return []native.Value{native.NewInteger(-1)}, nil
						case d > 0:
							return []native.Value{native.NewInteger(1)}, nil
						default:
							return []native.Value{native.NewInteger(0)}, nil
						}
					}),
					Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
				},
			},
		},
		// --- Arithmetic ---
		{
			// d1 d2 until → args[0]=d2 (nearest), args[1]=d1
			Name: "until",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					from := extractTime(args[1])
					to := extractTime(args[0])
					cd := dateDiffCalendarDuration(from, to)
					return []native.Value{tt.NewCalendarDuration(cd.Years, cd.Months, cd.Days)}, nil
				}),
				Returns: []*native.Type{tt.CalendarDuration}, BarrierPos: -1,
			}},
		},
		{
			// d1 d2 since → args[0]=d2 (nearest), args[1]=d1
			Name: "since",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					from := extractTime(args[0])
					to := extractTime(args[1])
					cd := dateDiffCalendarDuration(from, to)
					return []native.Value{tt.NewCalendarDuration(cd.Years, cd.Months, cd.Days)}, nil
				}),
				Returns: []*native.Type{tt.CalendarDuration}, BarrierPos: -1,
			}},
		},
		{
			// ins1 ins2 diff → args[0]=ins2 (nearest), args[1]=ins1
			Name: "diff",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInstant, native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					return []native.Value{tt.NewClockDuration(t2.Sub(t1))}, nil
				}),
				Returns: []*native.Type{tt.ClockDuration}, BarrierPos: -1,
			}},
		},
		{
			Name:          "elapsed",
			CompileEffect: native.CompileSideEffect,

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					start := extractTime(args[0])
					return []native.Value{tt.NewClockDuration(time.Since(start))}, nil
				}),
				Returns: []*native.Type{tt.ClockDuration}, BarrierPos: -1,
			}},
		},
		// --- Comparison extended ---
		{
			Name: "time-compare",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					switch {
					case t1.Before(t2):
						return []native.Value{native.NewInteger(-1)}, nil
					case t1.After(t2):
						return []native.Value{native.NewInteger(1)}, nil
					default:
						return []native.Value{native.NewInteger(0)}, nil
					}
				}),
				Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
			}},
		},
		{
			// d start end is-between → args[0]=end (nearest), args[1]=start, args[2]=d (deepest)
			Name: "is-between",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					d := extractTime(args[2])
					start := extractTime(args[1])
					end := extractTime(args[0])
					return []native.Value{native.NewBoolean(!d.Before(start) && !d.After(end))}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		{
			Name: "earliest",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.Before(t2) {
						return []native.Value{native.NewDate(t1)}, nil
					}
					return []native.Value{native.NewDate(t2)}, nil
				}),
				// Both arms return NewDate — the comparison picks which one.
				Returns: []*native.Type{native.TDate}, BarrierPos: -1,
			}},
		},
		{
			Name: "latest",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t1 := extractTime(args[1])
					t2 := extractTime(args[0])
					if t1.After(t2) {
						return []native.Value{native.NewDate(t1)}, nil
					}
					return []native.Value{native.NewDate(t2)}, nil
				}),
				// Both arms return NewDate — the comparison picks which one.
				Returns: []*native.Type{native.TDate}, BarrierPos: -1,
			}},
		},
		// --- Conversion ---
		// to-date — DateTime overload + Instant overload (UTC).
		{
			Name: "to-date",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TDateTime},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						return []native.Value{native.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()))}, nil
					}),
					Returns: []*native.Type{native.TDate}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TInstant},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						return []native.Value{native.NewDate(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC))}, nil
					}),
					Returns: []*native.Type{native.TDate}, BarrierPos: -1,
				},
			},
		},
		// to-time-of-day — DateTime + Instant overloads (identical body).
		{
			Name: "to-time-of-day",

			Signatures: []native.Signature{
				{
					Args: []*native.Type{native.TDateTime},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []native.Value{tt.NewTimeOfDay(d)}, nil
					}),
					Returns: []*native.Type{tt.TimeOfDay}, BarrierPos: -1,
				},
				{
					Args: []*native.Type{native.TInstant},
					Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						t := extractTime(args[0])
						d := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute +
							time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())
						return []native.Value{tt.NewTimeOfDay(d)}, nil
					}),
					Returns: []*native.Type{tt.TimeOfDay}, BarrierPos: -1,
				},
			},
		},
		{
			Name: "to-datetime",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[0])
					return []native.Value{native.NewDateTime(t)}, nil
				}),
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		{
			// dt tz to-instant → args[0]=tz (nearest), args[1]=dt
			Name: "to-instant",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Timezone, native.TDateTime},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					dt := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					t := time.Date(dt.Year(), dt.Month(), dt.Day(), dt.Hour(), dt.Minute(), dt.Second(), dt.Nanosecond(), loc)
					return []native.Value{native.NewInstant(t)}, nil
				}),
				Returns: []*native.Type{native.TInstant}, BarrierPos: -1,
			}},
		},
		{
			// ins tz to-local → args[0]=tz (nearest), args[1]=ins
			Name: "to-local",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Timezone, native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					return []native.Value{native.NewDateTime(t.In(loc))}, nil
				}),
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		{
			Name: "to-utc",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[0])
					return []native.Value{native.NewDateTime(t.UTC())}, nil
				}),
				Returns: []*native.Type{native.TDateTime}, BarrierPos: -1,
			}},
		},
		// --- Rounding ---
		{
			Name: "start-of",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					unit, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t := extractTime(args[1])
					var result time.Time
					switch unit {
					case "year":
						result = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
					case "quarter":
						q := (int(t.Month()) + 2) / 3
						m := time.Month((q-1)*3 + 1)
						result = time.Date(t.Year(), m, 1, 0, 0, 0, 0, t.Location())
					case "month":
						result = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
					case "week":
						wd := t.Weekday()
						if wd == time.Sunday {
							wd = 7
						}
						result = t.AddDate(0, 0, -int(wd-time.Monday))
						result = time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, t.Location())
					case "day":
						result = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
					default:
						return nil, fmt.Errorf("start-of: unknown unit %q", unit)
					}
					return []native.Value{native.NewDate(result)}, nil
				}),
				// Every unit arm builds a Date; an unknown unit raises instead.
				Returns: []*native.Type{native.TDate}, BarrierPos: -1,
			}},
		},
		{
			Name: "end-of",

			Signatures: []native.Signature{{
				Args: []*native.Type{native.TString, native.TDate},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					unit, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					t := extractTime(args[1])
					var result time.Time
					switch unit {
					case "year":
						result = time.Date(t.Year(), 12, 31, 0, 0, 0, 0, t.Location())
					case "quarter":
						q := (int(t.Month()) + 2) / 3
						endMonth := time.Month(q * 3)
						last := time.Date(t.Year(), endMonth+1, 0, 0, 0, 0, 0, t.Location())
						result = last
					case "month":
						result = time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
					case "week":
						wd := t.Weekday()
						if wd == time.Sunday {
							wd = 7
						}
						daysToSunday := 7 - int(wd)
						result = t.AddDate(0, 0, daysToSunday)
						result = time.Date(result.Year(), result.Month(), result.Day(), 0, 0, 0, 0, t.Location())
					case "day":
						result = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
					default:
						return nil, fmt.Errorf("end-of: unknown unit %q", unit)
					}
					return []native.Value{native.NewDate(result)}, nil
				}),
				// Every unit arm builds a Date; an unknown unit raises instead.
				Returns: []*native.Type{native.TDate}, BarrierPos: -1,
			}},
		},
		// --- Timezone ---
		{
			Name: "tz-utc",

			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{tt.NewTimezone(time.UTC)}, nil
				}),
				Returns: []*native.Type{tt.Timezone}, BarrierPos: 0,
			}},
		},
		{
			Name: "tz-local",

			Signatures: []native.Signature{{
				Args: []*native.Type{},
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					return []native.Value{tt.NewTimezone(time.Local)}, nil
				}),
				Returns: []*native.Type{tt.Timezone}, BarrierPos: 0,
			}},
		},
		{
			Name: "tz-name",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Timezone},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					loc := native.AsTimezone(args[0])
					if loc == nil {
						return []native.Value{native.NewString("UTC")}, nil
					}
					return []native.Value{native.NewString(loc.String())}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			// ins tz tz-offset → args[0]=tz (nearest), args[1]=ins
			Name: "tz-offset",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Timezone, native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					_, offset := t.In(loc).Zone()
					h := offset / 3600
					m := (offset % 3600) / 60
					if m < 0 {
						m = -m
					}
					sign := "+"
					if h < 0 {
						sign = "-"
						h = -h
					}
					return []native.Value{native.NewString(fmt.Sprintf("%s%02d:%02d", sign, h, m))}, nil
				}),
				Returns: []*native.Type{native.TString}, BarrierPos: -1,
			}},
		},
		{
			// ins tz is-dst → args[0]=tz (nearest), args[1]=ins
			Name: "is-dst",

			Signatures: []native.Signature{{
				Args: []*native.Type{tt.Timezone, native.TInstant},
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					t := extractTime(args[1])
					loc := native.AsTimezone(args[0])
					if loc == nil {
						loc = time.UTC
					}
					name, _ := t.In(loc).Zone()
					// Heuristic: standard zone names don't end in "DT" or "ST",
					// but DST zones typically contain "DT" (e.g. EDT, CDT, PDT).
					// A more robust check: compare January offset vs current offset.
					jan := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, loc)
					jul := time.Date(t.Year(), 7, 1, 0, 0, 0, 0, loc)
					_, janOff := jan.Zone()
					_, julOff := jul.Zone()
					_, curOff := t.In(loc).Zone()
					_ = name
					// If offsets differ between Jan and Jul, the larger offset is DST.
					if janOff == julOff {
						return []native.Value{native.NewBoolean(false)}, nil
					}
					stdOff := janOff
					if julOff < janOff {
						stdOff = julOff // Southern hemisphere
					}
					return []native.Value{native.NewBoolean(curOff != stdOff)}, nil
				}),
				Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
			}},
		},
		// --- Parsing ---
		// parse-date, parse-datetime, auto-date removed as a feature.
		// Date construction is via numeric helpers / current-time
		// sources (today, now-local, unix, unix-ms, unix-ns), not
		// string parsing.
	}

	return out
}
