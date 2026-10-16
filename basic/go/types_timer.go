package basic

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// The Timeout / Interval timer types are owned by boru:time-util — the
// timeout/interval handlers live in this file, and the types are
// per-import module mints (former global FixedIDs 4000-4001, retired)
// minted alongside the temporal leaves by MintTemporalModuleTypes
// (native_temporal.go). The constructors are methods on
// TemporalModuleTypes so every timer handle carries its import's own
// type identity.

// NewTimeout constructs a Timeout value carrying the given
// TimeoutInfo payload.
func (tt TemporalModuleTypes) NewTimeout(info *TimeoutInfo) Value {
	return core.NewValueRaw(tt.Timeout, info)
}

// NewInterval constructs an Interval value carrying the given
// IntervalInfo payload. See NewTimeout.
func (tt TemporalModuleTypes) NewInterval(info *IntervalInfo) Value {
	return core.NewValueRaw(tt.Interval, info)
}

// TimeoutFormatBehavior renders a Timeout as "Timeout(id,Nms)".
// Moved from eng/coretype_format_behaviors.go at Step 8.
type TimeoutFormatBehavior struct{}

func (TimeoutFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (TimeoutFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (TimeoutFormatBehavior) Format(v Value) string {
	if ti, ok := DataOf(&v).(*TimeoutInfo); ok {
		return fmt.Sprintf("Timeout(%s,%dms)", ti.ID, ti.Ms)
	}
	return "Timeout(nil)"
}

// IntervalFormatBehavior renders an Interval as "Interval(id,Nms)".
type IntervalFormatBehavior struct{}

func (IntervalFormatBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (IntervalFormatBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (IntervalFormatBehavior) Format(v Value) string {
	if ii, ok := DataOf(&v).(*IntervalInfo); ok {
		return fmt.Sprintf("Interval(%s,%dms)", ii.ID, ii.Ms)
	}
	return "Interval(nil)"
}

// ToMap / ToList implement core.IdealConverter for Timeout / Interval:
// {id:… ms:…} and [id ms].
func (TimeoutFormatBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ti, ok := DataOf(&v).(*TimeoutInfo); ok {
		m.Set("id", NewString(ti.ID))
		m.Set("ms", NewInteger(ti.Ms))
	}
	return NewMap(m), nil
}
func (TimeoutFormatBehavior) ToList(v Value) (Value, error) {
	if ti, ok := DataOf(&v).(*TimeoutInfo); ok {
		return NewList([]Value{NewString(ti.ID), NewInteger(ti.Ms)}), nil
	}
	return NewList(nil), nil
}
func (IntervalFormatBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ii, ok := DataOf(&v).(*IntervalInfo); ok {
		m.Set("id", NewString(ii.ID))
		m.Set("ms", NewInteger(ii.Ms))
	}
	return NewMap(m), nil
}
func (IntervalFormatBehavior) ToList(v Value) (Value, error) {
	if ii, ok := DataOf(&v).(*IntervalInfo); ok {
		return NewList([]Value{NewString(ii.ID), NewInteger(ii.Ms)}), nil
	}
	return NewList(nil), nil
}
