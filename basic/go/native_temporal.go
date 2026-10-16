package basic

import (
	"fmt"
	"time"

	core "github.com/boru-lang/boru/core/go"
)

// The Scalar/Time type family splits across two owners:
//
//   - CORE (global external builtins, wire-stable FixedIDs 1000-1003):
//     the family root Scalar/Time and the three instant-bearing
//     leaves Date / DateTime / Instant. These stay global because
//     cross-module producers construct them (IO mtime is an Instant),
//     they order chronologically through the Scalar/Time Comparer,
//     and their identities are baked into serialised Value IDs.
//
//   - boru:time-util (per-import module mints, no FixedIDs): TimeOfDay,
//     Duration and its CalendarDuration / ClockDuration leaves, and Timezone.
//     BuildTimeModule mints them per import via
//     MintTemporalModuleTypes below — the StreamKind pattern
//     (io_stream.go) — so a program that never imports the module
//     doesn't see the types at all. They are reachable as
//     `TimeUtil.CalendarDuration` etc. The temporal add/sub overloads ride
//     the module's word-extension exports (TemporalArithmeticExtensions,
//     native_math.go), and the minted types are what satisfy the
//     module-scope user-type rule for those extensions.
//
// Registration uses var-initialiser form so any other var that
// references the core types — notably signatures built at
// package-init time — sees a non-nil pointer at slice-init time.
// Go's init order resolves dependencies before declaration order.
var (
	TTime     = RegisterTemporalType("Scalar/Time", 1000, TimeCompareBehavior{})
	TDate     = RegisterTemporalType("Scalar/Time/Date", 1001, DateFormatBehavior{})
	TDateTime = RegisterTemporalType("Scalar/Time/DateTime", 1002, DateTimeFormatBehavior{})
	TInstant  = RegisterTemporalType("Scalar/Time/Instant", 1003, InstantFormatBehavior{})
)

// TemporalModuleTypes are the Scalar/Time leaves owned by
// boru:time-util, minted per import into the module's sub-registry by
// MintTemporalModuleTypes. Their values escape to the importer (a
// `TimeUtil.days 3` result is a CalendarDuration), so the mints draw IDs
// from the importing tree's counter — BuildTimeModule adopts the
// parent's sequence before minting (see eng TypeTable.mintID and the
// BuildIOModule / StreamKind precedent).
type TemporalModuleTypes struct {
	TimeOfDay        *Type
	Duration         *Type
	CalendarDuration *Type
	ClockDuration    *Type
	Timezone         *Type
	// The timer handle types (former global FixedIDs 4000-4001) —
	// their words (timeout / interval / await / cancel) are module
	// words already; the handles they return carry these mints.
	Timeout  *Type
	Interval *Type
}

// MintTemporalModuleTypes mints the module-owned temporal types into
// r's type table (r is boru:time-util's sub-registry) and returns the
// nodes. Parents mint first so the children's Parent chains resolve;
// the whole set hangs under the global Scalar/Time root, so the
// family Comparer and lattice ordering see them exactly where the
// former global registrations sat.
func MintTemporalModuleTypes(r *Registry) TemporalModuleTypes {
	timeOfDay := r.Types.MintTypeWithBehavior("TimeOfDay", TTime, TimeOfDayFormatBehavior{})
	duration := r.Types.MintType("Duration", TTime)
	return TemporalModuleTypes{
		TimeOfDay:        timeOfDay,
		Duration:         duration,
		CalendarDuration: r.Types.MintTypeWithBehavior("CalendarDuration", duration, CalDurationFormatBehavior{}),
		ClockDuration:    r.Types.MintTypeWithBehavior("ClockDuration", duration, ClkDurationFormatBehavior{}),
		Timezone:         r.Types.MintTypeWithBehavior("Timezone", TTime, TimezoneFormatBehavior{}),
		Timeout:          r.Types.MintTypeWithBehavior("Timeout", core.TIdeal, TimeoutFormatBehavior{}),
		Interval:         r.Types.MintTypeWithBehavior("Interval", core.TIdeal, IntervalFormatBehavior{}),
	}
}

func RegisterTemporalType(path string, fixedID int, behavior core.TypeBehavior) *core.Type {
	t, err := core.Builtin.RegisterType(path, fixedID, "boru:time-util", behavior)
	if err != nil {
		// Init-time registration error — recorded, not panicked.
		// See ADR-005 and typeinit.go.
		recordTypeInitErr(fmt.Errorf("native_temporal: register %s: %w", path, err))
	}
	return t
}

// New* constructors for Scalar/Time/* — moved from eng/value.go at
// Step 8 since the kernel no longer carries constructors for types
// it doesn't own.

func NewDate(t time.Time) Value {
	return core.NewValueRaw(TDate, core.TimePayload{T: t})
}

func NewDateTime(t time.Time) Value {
	return core.NewValueRaw(TDateTime, core.TimePayload{T: t})
}

func NewInstant(t time.Time) Value {
	return core.NewValueRaw(TInstant, core.TimePayload{T: t.UTC()})
}

// The module-owned constructors are methods on TemporalModuleTypes:
// the Parent tag comes from that import's mints, so every value a
// module instance produces carries the instance's own type identity.

func (tt TemporalModuleTypes) NewTimeOfDay(d time.Duration) Value {
	return core.NewValueRaw(tt.TimeOfDay, core.DurationPayload{D: d})
}

func (tt TemporalModuleTypes) NewCalendarDuration(years, months, days int) Value {
	return core.NewValueRaw(tt.CalendarDuration, core.CalDurationData{Years: years, Months: months, Days: days})
}

func (tt TemporalModuleTypes) NewClockDuration(d time.Duration) Value {
	return core.NewValueRaw(tt.ClockDuration, core.DurationPayload{D: d})
}

func (tt TemporalModuleTypes) NewTimezone(loc *time.Location) Value {
	return core.NewValueRaw(tt.Timezone, core.TimezonePayload{Loc: loc})
}

// As* accessors for the time-family types. Moved from
// eng/value.go at Step 6/7 — the kernel no longer carries methods
// for types it doesn't own. The implementations are identical to
// the previous methods: assert the kernel-owned payload variant
// (TimePayload / DurationPayload / TimezonePayload / CalDurationData)
// and return the inner Go value, or the zero value on mismatch.

// AsDate extracts the time.Time from a Date value.
func AsDate(v Value) time.Time {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsDateTime extracts the time.Time from a DateTime value.
func AsDateTime(v Value) time.Time {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsInstant extracts the time.Time from an Instant value.
func AsInstant(v Value) time.Time {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// AsTimeOfDay extracts the time.Duration offset for a TimeOfDay value.
func AsTimeOfDay(v Value) time.Duration {
	if dp, ok := v.Data().(core.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d
		}
	}
	return 0
}

// AsCalendarDuration extracts the CalDurationData payload.
func AsCalendarDuration(v Value) (core.CalDurationData, bool) {
	if d, ok := v.Data().(core.CalDurationData); ok {
		return d, true
	}
	return core.CalDurationData{}, false
}

// AsClockDuration extracts the time.Duration payload for a ClockDuration value.
func AsClockDuration(v Value) (time.Duration, bool) {
	if dp, ok := v.Data().(core.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d, true
		}
	}
	return 0, false
}

// AsTimezone extracts the *time.Location for a Timezone value.
func AsTimezone(v Value) *time.Location {
	if tp, ok := v.Data().(core.TimezonePayload); ok {
		if loc, ok := tp.Loc.(*time.Location); ok {
			return loc
		}
	}
	return nil
}

// Format Behaviors for the time-family types. Moved from
// eng/coretype_format_behaviors.go at Step 8.

type DateFormatBehavior struct{}

func (DateFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (DateFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (DateFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	}
	return "Date(nil)"
}

// Compare orders Date values chronologically (earlier < later).
// Implements core.Comparer so `lt`/`gt`/`sort` work on Dates via the
// canonical CompareValues lattice dispatch.
func (DateFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type DateTimeFormatBehavior struct{}

func (DateTimeFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (DateTimeFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (DateTimeFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format("2006-01-02T15:04:05.999999999")
		}
	}
	return "DateTime(nil)"
}

func (DateTimeFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type InstantFormatBehavior struct{}

func (InstantFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (InstantFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (InstantFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data().(core.TimePayload); ok {
		if t, ok := tp.T.(time.Time); ok {
			return t.Format(time.RFC3339Nano)
		}
	}
	return "Instant(nil)"
}

func (InstantFormatBehavior) Compare(a, b Value) (int, error) {
	return compareTimePayloads(a, b), nil
}

type TimeOfDayFormatBehavior struct{}

func (TimeOfDayFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (TimeOfDayFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (TimeOfDayFormatBehavior) Format(v Value) string {
	dp, ok := v.Data().(core.DurationPayload)
	if !ok {
		return "TimeOfDay(nil)"
	}
	d, ok := dp.D.(time.Duration)
	if !ok {
		return "TimeOfDay(nil)"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ns := d.Nanoseconds() % 1e9
	if ns != 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%09d", h, m, s, ns)
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (TimeOfDayFormatBehavior) Compare(a, b Value) (int, error) {
	return compareDurationPayloads(a, b), nil
}

// compareTimePayloads returns -1/0/1 for two values whose Data is an
// core.TimePayload wrapping a time.Time. Non-Time payloads compare as
// equal — the matching dispatch already filters by Parent, so this
// only fires on well-formed temporal values.
func compareTimePayloads(a, b Value) int {
	ta, _ := timeFromValue(a)
	tb, _ := timeFromValue(b)
	switch {
	case ta.Before(tb):
		return -1
	case ta.After(tb):
		return 1
	default:
		return 0
	}
}

func compareDurationPayloads(a, b Value) int {
	da := DurationFromValue(a)
	db := DurationFromValue(b)
	switch {
	case da < db:
		return -1
	case da > db:
		return 1
	default:
		return 0
	}
}

// TimeCompareBehavior is the Comparer installed on the abstract
// Scalar/Time node. It is reached by the CompareValues LCA walk only
// for CROSS-leaf Time pairs (same-leaf pairs hit their own per-leaf
// Comparer first). It unifies the instant-bearing leaves — Date,
// DateTime, Instant — so they order chronologically against one another
// (a date and a datetime are both moments in time). Pairs it can't
// place chronologically (TimeOfDay and the Duration leaves, type
// literals — none of which carry a time.Time) decline with
// ErrNoComparer, so the cascade falls through to the lattice Rank, and
// the family-restricted ordering words reject them in favour of tcmp.
type TimeCompareBehavior struct{}

func (TimeCompareBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (TimeCompareBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (TimeCompareBehavior) Format(v Value) string       { return core.DefaultBehavior.Format(v) }

func (TimeCompareBehavior) Compare(a, b Value) (int, error) {
	ta, aok := timeFromValue(a)
	tb, bok := timeFromValue(b)
	if !aok || !bok {
		return 0, core.ErrNoComparer
	}
	switch {
	case ta.Before(tb):
		return -1, nil
	case ta.After(tb):
		return 1, nil
	default:
		return 0, nil
	}
}

func timeFromValue(v Value) (time.Time, bool) {
	tp, ok := v.Data().(core.TimePayload)
	if !ok {
		return time.Time{}, false
	}
	t, ok := tp.T.(time.Time)
	return t, ok
}

func DurationFromValue(v Value) time.Duration {
	dp, ok := v.Data().(core.DurationPayload)
	if !ok {
		return 0
	}
	d, ok := dp.D.(time.Duration)
	if !ok {
		return 0
	}
	return d
}

type CalDurationFormatBehavior struct{}

func (CalDurationFormatBehavior) Match(v Value, t *Type) bool {
	return core.DefaultBehavior.Match(v, t)
}
func (CalDurationFormatBehavior) Equal(a, b Value) bool { return core.DefaultBehavior.Equal(a, b) }
func (CalDurationFormatBehavior) Format(v Value) string {
	if cd, ok := v.Data().(core.CalDurationData); ok {
		return fmt.Sprintf("P%dY%dM%dD", cd.Years, cd.Months, cd.Days)
	}
	return "CalendarDuration(nil)"
}

type ClkDurationFormatBehavior struct{}

func (ClkDurationFormatBehavior) Match(v Value, t *Type) bool {
	return core.DefaultBehavior.Match(v, t)
}
func (ClkDurationFormatBehavior) Equal(a, b Value) bool { return core.DefaultBehavior.Equal(a, b) }
func (ClkDurationFormatBehavior) Format(v Value) string {
	if dp, ok := v.Data().(core.DurationPayload); ok {
		if d, ok := dp.D.(time.Duration); ok {
			return d.String()
		}
	}
	return "ClockDuration(nil)"
}

func (ClkDurationFormatBehavior) Compare(a, b Value) (int, error) {
	return compareDurationPayloads(a, b), nil
}

type TimezoneFormatBehavior struct{}

func (TimezoneFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (TimezoneFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (TimezoneFormatBehavior) Format(v Value) string {
	if tp, ok := v.Data().(core.TimezonePayload); ok {
		if loc, ok := tp.Loc.(*time.Location); ok {
			return loc.String()
		}
	}
	return "Timezone(nil)"
}
