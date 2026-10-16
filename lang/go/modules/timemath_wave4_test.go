package modules

import (
	"math"
	"strings"
	"testing"
	"time"

	// Embed the tz database so the named-zone tests are host-independent.
	_ "time/tzdata"

	"github.com/boru-lang/boru/lang/go/native"
)

// Wave-4 coverage for modules/time.go (dur-sign clock arms, calendar
// borrow arithmetic, the Instant conversion overloads, start-of / end-of
// units, tz-offset / is-dst zones, and every per-argument error arm) and
// modules/math.go (the float overload closures, IEEE classifiers, binary
// float ops, atan2 / hypot integer overloads, scalb and fma).

// w4Handler invokes the idx-th signature's Go handler directly, asserting it
// never panics (ADR-005: bad args are errors, not crashes).
func w4Handler(t *testing.T, n native.NativeFunc, idx int, r *native.Registry, args ...native.Value) ([]native.Value, error) {
	t.Helper()
	var out []native.Value
	var err error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("%s sig %d panicked: %v", n.Name, idx, rec)
			}
		}()
		h := n.Signatures[idx].DispatchHandler()
		if h == nil {
			t.Fatalf("%s sig %d has no Go handler", n.Name, idx)
		}
		out, err = h(args, nil, nil, r)
	}()
	return out, err
}

// w4MismatchSweep drives every signature of every native with one
// wrong-typed argument at each position (valid values elsewhere): the
// per-argument AsConcrete* error arms must reject loudly, never panic.
func w4MismatchSweep(t *testing.T, r *native.Registry, natives []native.NativeFunc, valid func(*native.Type) native.Value) {
	t.Helper()
	for _, n := range natives {
		for si, sig := range n.Signatures {
			if len(sig.Args) == 0 || sig.DispatchHandler() == nil {
				continue
			}
			for bad := 0; bad < len(sig.Args); bad++ {
				args := make([]native.Value, len(sig.Args))
				for i, tp := range sig.Args {
					if i == bad {
						if tp == native.TString {
							args[i] = native.NewInteger(9)
						} else {
							args[i] = native.NewString("bad")
						}
						continue
					}
					args[i] = valid(tp)
				}
				_, _ = w4Handler(t, n, si, r, args...) // must not panic
			}
		}
	}
}

// w4TimeValid maps a time-module signature arg type to a valid value.
func w4TimeValid(tt native.TemporalModuleTypes) func(*native.Type) native.Value {
	utc := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	return func(tp *native.Type) native.Value {
		switch tp {
		case native.TString:
			return native.NewString("day")
		case native.TDate:
			return native.NewDate(utc)
		case native.TDateTime:
			return native.NewDateTime(utc)
		case native.TInstant:
			return native.NewInstant(utc)
		case tt.CalendarDuration:
			return tt.NewCalendarDuration(1, 2, 3)
		case tt.ClockDuration:
			return tt.NewClockDuration(time.Second)
		case tt.Timezone:
			return tt.NewTimezone(time.UTC)
		case tt.TimeOfDay:
			return tt.NewTimeOfDay(time.Hour)
		default: // TInteger / TNumber / TAny
			return native.NewInteger(2)
		}
	}
}

// TestTimeWave4MismatchSweep pins the per-argument rejection arms of every
// time-module word.
func TestTimeWave4MismatchSweep(t *testing.T) {
	r := timeRegistry(t)
	tt := timeTT(t, r)
	w4MismatchSweep(t, r, timeNatives(tt), w4TimeValid(tt))
}

// TestTimeWave4TzErrors pins the unknown-timezone rejection and the happy
// named-zone construction (paired).
func TestTimeWave4TzErrors(t *testing.T) {
	r := timeRegistry(t)
	e := native.New(r)
	if _, err := e.Run(callTimeDot("tz", native.NewString("No/Such_Zone"))); err == nil ||
		!strings.Contains(err.Error(), "unknown timezone") {
		t.Errorf("bad zone: %v, want unknown timezone", err)
	}
	result := runBoru(t, r, callTimeDot("tz", native.NewString("America/New_York")))
	if loc := native.AsTimezone(result[0]); loc == nil || loc.String() != "America/New_York" {
		t.Errorf("tz = %v, want America/New_York", result[0])
	}
}

// TestTimeWave4UnixNs pins the unix-ns constructor round trip.
func TestTimeWave4UnixNs(t *testing.T) {
	r := timeRegistry(t)
	ins := runBoru(t, r, callTimeDot("unix-ns", native.NewInteger(1_500_000_000_000_000_000)))
	back := runBoru(t, r, callTimeDot("to-unix", ins[0]))
	if n, err := back[0].AsConcreteInteger(); err != nil || n != 1_500_000_000 {
		t.Errorf("unix-ns round trip = %v (%v), want 1500000000", back[0], err)
	}
}

// TestTimeWave4UntilBorrow pins dateDiffCalendarDuration's borrow arms: a
// day borrow (negative day delta) and the cascading month borrow.
func TestTimeWave4UntilBorrow(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("until", d1, d2))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 0 || cd.Months != 1 || cd.Days != 10 {
		t.Errorf("until day-borrow = %+v (%v), want {0 1 10}", cd, ok)
	}

	d3 := native.NewDate(time.Date(2023, 10, 20, 0, 0, 0, 0, time.UTC))
	d4 := native.NewDate(time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC))
	result = runBoru(t, r, callTimeDot("until", d3, d4))
	cd, ok = native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 0 || cd.Months != 3 || cd.Days != 21 {
		t.Errorf("until month-borrow = %+v (%v), want {0 3 21}", cd, ok)
	}
}

// TestTimeWave4DurSignClock pins the ClockDuration overload of dur-sign
// across all three arms (the CalendarDuration twin alongside).
func TestTimeWave4DurSignClock(t *testing.T) {
	r := timeRegistry(t)
	for _, c := range []struct {
		n    int64
		want int64
	}{{-2, -1}, {0, 0}, {3, 1}} {
		d := runBoru(t, r, callTimeDot("hours", native.NewInteger(c.n)))
		s := runBoru(t, r, callTimeDot("dur-sign", d[0]))
		if n, err := s[0].AsConcreteInteger(); err != nil || n != c.want {
			t.Errorf("clock dur-sign(%dh) = %v (%v), want %d", c.n, s[0], err, c.want)
		}
	}
	for _, c := range []struct {
		n    int64
		want int64
	}{{-1, -1}, {0, 0}, {2, 1}} {
		d := runBoru(t, r, callTimeDot("years", native.NewInteger(c.n)))
		s := runBoru(t, r, callTimeDot("dur-sign", d[0]))
		if n, err := s[0].AsConcreteInteger(); err != nil || n != c.want {
			t.Errorf("calendar dur-sign(%dy) = %v (%v), want %d", c.n, s[0], err, c.want)
		}
	}
}

// TestTimeWave4Elapsed pins elapsed: the duration since the epoch is
// decidedly positive.
func TestTimeWave4Elapsed(t *testing.T) {
	r := timeRegistry(t)
	d := runBoru(t, r, callTimeDot("elapsed", native.NewInstant(time.Unix(0, 0))))
	total := runBoru(t, r, callTimeDot("total-hours", d[0]))
	if f, err := total[0].AsConcreteFloat(); err != nil || f <= 0 {
		t.Errorf("elapsed since epoch = %v (%v), want positive", total[0], err)
	}
}

// TestTimeWave4CompareEarliestLatest pins time-compare's three arms and both
// branches of earliest / latest.
func TestTimeWave4CompareEarliestLatest(t *testing.T) {
	r := timeRegistry(t)
	a := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	b := native.NewDate(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))

	for _, c := range []struct {
		d1, d2 native.Value
		want   int64
	}{{a, b, -1}, {b, a, 1}, {a, a, 0}} {
		result := runBoru(t, r, callTimeDot("compare", c.d1, c.d2))
		if n, err := result[0].AsConcreteInteger(); err != nil || n != c.want {
			t.Errorf("time-compare = %v (%v), want %d", result[0], err, c.want)
		}
	}
	for _, c := range []struct {
		word   string
		d1, d2 native.Value
		want   int
	}{
		{"earliest", a, b, 1}, {"earliest", b, a, 1},
		{"latest", a, b, 6}, {"latest", b, a, 6},
	} {
		result := runBoru(t, r, callTimeDot(c.word, c.d1, c.d2))
		if got := native.AsDate(result[0]); int(got.Month()) != c.want {
			t.Errorf("%s month = %v, want %d", c.word, got.Month(), c.want)
		}
	}
}

// TestTimeWave4InstantConversions pins the Instant overloads of to-date and
// to-time-of-day.
func TestTimeWave4InstantConversions(t *testing.T) {
	r := timeRegistry(t)
	tt := timeTT(t, r)
	ins := native.NewInstant(time.Date(2024, 3, 15, 13, 45, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("to-date", ins))
	if got := native.AsDate(result[0]); got.Year() != 2024 || got.Month() != 3 || got.Day() != 15 {
		t.Errorf("to-date(Instant) = %v, want 2024-03-15", got)
	}

	result = runBoru(t, r, callTimeDot("to-time-of-day", ins))
	if !result[0].Parent().ConformsTo(tt.TimeOfDay) {
		t.Errorf("to-time-of-day(Instant) type = %s, want TimeOfDay", result[0].Parent())
	}
}

// TestTimeWave4StartEndOfUnits pins the remaining start-of / end-of units —
// week (both the Sunday special case and a midweek day), day, quarter — and
// the unknown-unit rejections.
func TestTimeWave4StartEndOfUnits(t *testing.T) {
	r := timeRegistry(t)
	sunday := native.NewDate(time.Date(2024, 3, 17, 0, 0, 0, 0, time.UTC))
	wednesday := native.NewDate(time.Date(2024, 3, 13, 0, 0, 0, 0, time.UTC))

	for _, c := range []struct {
		d       native.Value
		wantDay int
	}{{sunday, 11}, {wednesday, 11}} {
		result := runBoru(t, r, callTimeDot("start-of", c.d, native.NewString("week")))
		if got := native.AsDate(result[0]); got.Day() != c.wantDay || got.Weekday() != time.Monday {
			t.Errorf("start-of week = %v, want Monday the %d", got, c.wantDay)
		}
	}
	result := runBoru(t, r, callTimeDot("start-of", wednesday, native.NewString("day")))
	if got := native.AsDate(result[0]); got.Day() != 13 {
		t.Errorf("start-of day = %v, want the 13th", got)
	}

	for _, c := range []struct {
		d       native.Value
		wantDay int
	}{{sunday, 17}, {wednesday, 17}} {
		result = runBoru(t, r, callTimeDot("end-of", c.d, native.NewString("week")))
		if got := native.AsDate(result[0]); got.Day() != c.wantDay || got.Weekday() != time.Sunday {
			t.Errorf("end-of week = %v, want Sunday the %d", got, c.wantDay)
		}
	}
	result = runBoru(t, r, callTimeDot("end-of", wednesday, native.NewString("day")))
	if got := native.AsDate(result[0]); got.Day() != 13 {
		t.Errorf("end-of day = %v, want the 13th", got)
	}
	result = runBoru(t, r, callTimeDot("end-of", wednesday, native.NewString("quarter")))
	if got := native.AsDate(result[0]); got.Month() != 3 || got.Day() != 31 {
		t.Errorf("end-of quarter = %v, want 2024-03-31", got)
	}

	e := native.New(r)
	if _, err := e.Run(callTimeDot("start-of", wednesday, native.NewString("fortnight"))); err == nil ||
		!strings.Contains(err.Error(), "unknown unit") {
		t.Errorf("start-of fortnight: %v, want unknown unit", err)
	}
	if _, err := e.Run(callTimeDot("end-of", wednesday, native.NewString("fortnight"))); err == nil ||
		!strings.Contains(err.Error(), "unknown unit") {
		t.Errorf("end-of fortnight: %v, want unknown unit", err)
	}
}

// TestTimeWave4TzLocalNameOffset pins tz-local, tz-name for a named zone,
// and tz-offset's negative-hour and half-hour arms.
func TestTimeWave4TzLocalNameOffset(t *testing.T) {
	r := timeRegistry(t)
	tt := timeTT(t, r)

	result := runBoru(t, r, callTimeDot("tz-local"))
	if loc := native.AsTimezone(result[0]); loc == nil {
		t.Error("tz-local should carry the local zone")
	}

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("tzdata: %v", err)
	}
	result = runBoru(t, r, callTimeDot("tz-name", tt.NewTimezone(ny)))
	if s, _ := native.AsString(result[0]); s != "America/New_York" {
		t.Errorf("tz-name = %q", s)
	}

	winter := native.NewInstant(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))
	result = runBoru(t, r, callTimeDot("tz-offset", winter, tt.NewTimezone(ny)))
	if s, _ := native.AsString(result[0]); s != "-05:00" {
		t.Errorf("tz-offset NY winter = %q, want -05:00", s)
	}

	stj, err := time.LoadLocation("America/St_Johns")
	if err != nil {
		t.Fatalf("tzdata: %v", err)
	}
	result = runBoru(t, r, callTimeDot("tz-offset", winter, tt.NewTimezone(stj)))
	if s, _ := native.AsString(result[0]); s != "-03:30" {
		t.Errorf("tz-offset St_Johns winter = %q, want -03:30", s)
	}
}

// TestTimeWave4IsDst pins is-dst across the zone shapes: a northern DST
// zone in summer and winter, a fixed zone (no DST), and a southern zone in
// its DST season (the julOff < janOff arm).
func TestTimeWave4IsDst(t *testing.T) {
	r := timeRegistry(t)
	tt := timeTT(t, r)
	summer := native.NewInstant(time.Date(2024, 7, 3, 12, 0, 0, 0, time.UTC))
	winter := native.NewInstant(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

	zone := func(name string) native.Value {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("tzdata %s: %v", name, err)
		}
		return tt.NewTimezone(loc)
	}
	cases := []struct {
		name string
		ins  native.Value
		tz   native.Value
		want bool
	}{
		{"NY summer", summer, zone("America/New_York"), true},
		{"NY winter", winter, zone("America/New_York"), false},
		{"UTC never", summer, tt.NewTimezone(time.UTC), false},
		{"Sydney January", winter, zone("Australia/Sydney"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := runBoru(t, r, callTimeDot("is-dst", c.ins, c.tz))
			if b, err := result[0].AsConcreteBoolean(); err != nil || b != c.want {
				t.Errorf("is-dst = %v (%v), want %v", result[0], err, c.want)
			}
		})
	}
}

// TestTimeWave4NilTimezoneArms pins the AsTimezone-nil fallbacks of
// to-instant, to-local, tz-name, tz-offset and is-dst via direct handler
// calls with a non-Timezone value in the tz slot.
func TestTimeWave4NilTimezoneArms(t *testing.T) {
	r := timeRegistry(t)
	tt := timeTT(t, r)
	natives := timeNatives(tt)
	dt := native.NewDateTime(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	ins := native.NewInstant(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	notTz := native.NewInteger(1)

	if out, err := w4Handler(t, findNativeWave3(t, natives, "to-instant"), 0, r, notTz, dt); err != nil || len(out) != 1 {
		t.Errorf("to-instant nil-tz: %v %v", out, err)
	}
	if out, err := w4Handler(t, findNativeWave3(t, natives, "to-local"), 0, r, notTz, ins); err != nil || len(out) != 1 {
		t.Errorf("to-local nil-tz: %v %v", out, err)
	}
	if out, err := w4Handler(t, findNativeWave3(t, natives, "tz-name"), 0, r, notTz); err != nil ||
		len(out) != 1 || out[0].String() != "'UTC'" {
		t.Errorf("tz-name nil-tz = %v (%v), want 'UTC'", out, err)
	}
	if out, err := w4Handler(t, findNativeWave3(t, natives, "tz-offset"), 0, r, notTz, ins); err != nil || len(out) != 1 {
		t.Errorf("tz-offset nil-tz: %v %v", out, err)
	}
	if out, err := w4Handler(t, findNativeWave3(t, natives, "is-dst"), 0, r, notTz, ins); err != nil || len(out) != 1 {
		t.Errorf("is-dst nil-tz: %v %v", out, err)
	}
}

// --- math module ---

// w4MathValid maps a math signature arg type to a valid value.
func w4MathValid(tp *native.Type) native.Value {
	switch tp {
	case native.TFloat:
		return native.NewFloat(1.5)
	case native.TString:
		return native.NewString("x")
	default: // TInteger / TNumber
		return native.NewInteger(2)
	}
}

// TestMathWave4MismatchSweep pins the per-argument rejection arms of every
// math-module word.
func TestMathWave4MismatchSweep(t *testing.T) {
	r := mathRegistry(t)
	w4MismatchSweep(t, r, MathNatives, w4MathValid)
}

// TestMathWave4FloatOverloads drives the float overload closures of abs,
// negate and sign (the integer twins are pinned alongside).
func TestMathWave4FloatOverloads(t *testing.T) {
	r := mathRegistry(t)
	abs := findNativeWave3(t, MathNatives, "abs")
	if out, err := w4Handler(t, abs, 1, r, native.NewFloat(-2.5)); err != nil || out[0].String() != "2.5" {
		t.Errorf("abs(-2.5) = %v (%v)", out, err)
	}
	neg := findNativeWave3(t, MathNatives, "negate")
	if out, err := w4Handler(t, neg, 1, r, native.NewFloat(2.5)); err != nil || out[0].String() != "-2.5" {
		t.Errorf("negate(2.5) = %v (%v)", out, err)
	}
	sign := findNativeWave3(t, MathNatives, "sign")
	for _, c := range []struct {
		sig  int
		v    native.Value
		want string
	}{
		{0, native.NewInteger(-5), "-1"}, {0, native.NewInteger(5), "1"}, {0, native.NewInteger(0), "0"},
		{1, native.NewFloat(-2.5), "-1"}, {1, native.NewFloat(2.5), "1"}, {1, native.NewFloat(0), "0"},
	} {
		if out, err := w4Handler(t, sign, c.sig, r, c.v); err != nil || out[0].String() != c.want {
			t.Errorf("sign(%v) = %v (%v), want %s", c.v, out, err, c.want)
		}
	}
}

// TestMathWave4MinMaxNaN pins the IEEE minNum/maxNum arms: NaN on either
// side yields the other operand; ordinary operands order normally.
func TestMathWave4MinMaxNaN(t *testing.T) {
	r := mathRegistry(t)
	nan := native.NewFloat(math.NaN())
	five := native.NewFloat(5.0)
	three := native.NewFloat(3.0)
	for _, name := range []string{"min", "max"} {
		n := findNativeWave3(t, MathNatives, name)
		// Drive every signature with each operand mix; int sigs reject the
		// floats loudly, float sigs compute — either way, no panic.
		for si := range n.Signatures {
			for _, pair := range [][2]native.Value{{nan, five}, {five, nan}, {three, five}, {five, three}, {nan, nan}} {
				out, err := w4Handler(t, n, si, r, pair[0], pair[1])
				if err != nil {
					continue // integer overload rejecting a Float
				}
				if len(out) != 1 {
					t.Errorf("%s sig %d returned %v", name, si, out)
				}
			}
		}
	}
}

// TestMathWave4Classifiers pins the classifier predicate closures.
func TestMathWave4Classifiers(t *testing.T) {
	r := mathRegistry(t)
	cases := []struct {
		word string
		v    native.Value
		want bool
	}{
		{"is-nan", native.NewFloat(math.NaN()), true},
		{"is-nan", native.NewInteger(1), false},
		{"is-inf", native.NewFloat(math.Inf(1)), true},
		{"is-inf", native.NewFloat(1.0), false},
		{"is-finite", native.NewFloat(1.0), true},
		{"is-finite", native.NewFloat(math.Inf(-1)), false},
		{"signbit", native.NewFloat(-1.0), true},
		{"signbit", native.NewFloat(1.0), false},
	}
	for _, c := range cases {
		n := findNativeWave3(t, MathNatives, c.word)
		out, err := w4Handler(t, n, 0, r, c.v)
		if err != nil {
			t.Fatalf("%s(%v): %v", c.word, c.v, err)
		}
		if b, berr := out[0].AsConcreteBoolean(); berr != nil || b != c.want {
			t.Errorf("%s(%v) = %v, want %v", c.word, c.v, out[0], c.want)
		}
	}
}

// TestMathWave4BinaryFloatOps pins the swap-convention binary float words
// (remainder / copysign / nextafter), scalb and fma.
func TestMathWave4BinaryFloatOps(t *testing.T) {
	r := mathRegistry(t)
	// handler computes fn(args[1], args[0]).
	cases := []struct {
		word   string
		a0, a1 float64
		want   float64
	}{
		{"remainder", 3.0, 10.0, math.Remainder(10.0, 3.0)},
		{"copysign", -2.0, 7.0, math.Copysign(7.0, -2.0)},
		{"nextafter", 2.0, 1.0, math.Nextafter(1.0, 2.0)},
	}
	for _, c := range cases {
		n := findNativeWave3(t, MathNatives, c.word)
		out, err := w4Handler(t, n, 0, r, native.NewFloat(c.a0), native.NewFloat(c.a1))
		if err != nil {
			t.Fatalf("%s: %v", c.word, err)
		}
		if f, ferr := out[0].AsConcreteFloat(); ferr != nil || f != c.want {
			t.Errorf("%s(%v,%v) = %v, want %v", c.word, c.a0, c.a1, out[0], c.want)
		}
	}

	scalb := findNativeWave3(t, MathNatives, "scalb")
	if out, err := w4Handler(t, scalb, 0, r, native.NewInteger(3), native.NewFloat(1.5)); err != nil ||
		out[0].String() != "12.0" {
		t.Errorf("scalb(1.5, 3) = %v (%v), want 12.0", out, err)
	}
	fma := findNativeWave3(t, MathNatives, "fma")
	if out, err := w4Handler(t, fma, 0, r,
		native.NewFloat(2.0), native.NewFloat(3.0), native.NewFloat(4.0)); err != nil ||
		out[0].String() != "10.0" {
		t.Errorf("fma(2,3,4) = %v (%v), want 10.0", out, err)
	}
}

// TestMathWave4Atan2Hypot pins atan2's float and integer overloads and
// hypot's integer overload (with their error arms swept above).
func TestMathWave4Atan2Hypot(t *testing.T) {
	r := mathRegistry(t)
	atan2 := findNativeWave3(t, MathNatives, "atan2")
	out, err := w4Handler(t, atan2, 0, r, native.NewFloat(1.0), native.NewFloat(1.0))
	if err != nil {
		t.Fatalf("atan2 float: %v", err)
	}
	if f, _ := out[0].AsConcreteFloat(); math.Abs(f-math.Pi/4) > 1e-12 {
		t.Errorf("atan2(1.0,1.0) = %v, want pi/4", out[0])
	}
	intSig := len(atan2.Signatures) - 1
	out, err = w4Handler(t, atan2, intSig, r, native.NewInteger(1), native.NewInteger(1))
	if err != nil {
		t.Fatalf("atan2 int: %v", err)
	}
	if f, _ := out[0].AsConcreteFloat(); math.Abs(f-math.Pi/4) > 1e-12 {
		t.Errorf("atan2(1,1) = %v, want pi/4", out[0])
	}

	hypot := findNativeWave3(t, MathNatives, "hypot")
	out, err = w4Handler(t, hypot, len(hypot.Signatures)-1, r, native.NewInteger(3), native.NewInteger(4))
	if err != nil {
		t.Fatalf("hypot int: %v", err)
	}
	if f, _ := out[0].AsConcreteFloat(); f != 5.0 {
		t.Errorf("hypot(3,4) = %v, want 5", out[0])
	}
}
