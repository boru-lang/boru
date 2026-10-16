package modules

import (
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/native"
)

// timeRegistry returns a registry with the boru:time module loaded.
func timeRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallTimeExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// timeTT recovers the module instance's minted temporal types from the
// installed TimeUtil export map — the exported type literals ARE the
// nodes by value (NewTypeLiteral returns *t), so taking a pointer to a
// copy yields a *Type that matches the module's signatures by ID. Test
// values constructed through it dispatch exactly like module-produced
// ones.
func timeTT(t *testing.T, r *native.Registry) native.TemporalModuleTypes {
	t.Helper()
	v, ok := r.Defs.Top("TimeUtil")
	if !ok {
		t.Fatal("TimeUtil exports not installed")
	}
	m, _ := native.AsMap(v)
	get := func(key string) *native.Type {
		lv, ok := m.Get(key)
		if !ok {
			t.Fatalf("missing type export %s", key)
		}
		node := lv
		return &node
	}
	return native.TemporalModuleTypes{
		TimeOfDay:        get("TimeOfDay"),
		Duration:         get("Duration"),
		CalendarDuration: get("CalendarDuration"),
		ClockDuration:    get("ClockDuration"),
		Timezone:         get("Timezone"),
	}
}

// callTimeDot is a helper to call time.<word> with values on the stack.
func callTimeDot(word string, stackVals ...native.Value) []native.Value {
	var input []native.Value
	input = append(input, stackVals...)
	input = append(input,
		native.NewWord("TimeUtil"), native.NewWord("dot"), native.NewWord(word),
	)
	return input
}

// --- Module structure ---

func TestTimeModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildTimeModule(r)
	if err != nil {
		t.Fatal(err)
	}
	timeExport, ok := desc.Exports["TimeUtil"]
	if !ok {
		t.Fatal("expected 'time' export")
	}
	expected := []string{
		"tz",
		"unix", "unix-ms", "unix-ns",
		"now-local", "today", "today-utc",
		"year", "month", "day", "weekday", "year-day",
		"weekday-name", "month-name", "iso-week", "quarter",
		"days-in-month", "days-in-year", "is-leap-year",
		"to-unix", "to-unix-ms",
		"is-before", "is-after", "is-equal",
		"to-string", "format", "to-iso",
		"add-days", "add-months", "add-years",
	}
	for _, name := range expected {
		if _, ok := timeExport.Get(name); !ok {
			t.Errorf("expected %q in time export map", name)
		}
	}
}

// --- Construction ---
// ISO 8601 date / datetime / instant / time-of-day parsing tests
// removed alongside the corresponding word handlers. Construct
// dates via current-time / numeric helpers instead.

func TestTimeTz(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("tz", native.NewString("UTC")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	loc := native.AsTimezone(result[0])
	if loc == nil || loc.String() != "UTC" {
		t.Errorf("expected UTC timezone, got %v", loc)
	}
}

func TestTimeUnix(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("unix", native.NewInteger(1710500000)))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	ins := native.AsInstant(result[0])
	if ins.Unix() != 1710500000 {
		t.Errorf("expected unix 1710500000, got %d", ins.Unix())
	}
}

func TestTimeUnixMs(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("unix-ms", native.NewInteger(1710500000000)))
	ins := native.AsInstant(result[0])
	if ins.UnixMilli() != 1710500000000 {
		t.Errorf("expected unix-ms 1710500000000, got %d", ins.UnixMilli())
	}
}

// --- Current Time ---

func TestTimeNowLocal(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("now-local"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !native.ParentOf(&result[0]).ConformsTo(native.TDateTime) {
		t.Errorf("expected DateTime type, got %s", native.ParentOf(&result[0]))
	}
}

func TestTimeToday(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("today"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := native.AsDate(result[0])
	if d.Year() < 2024 {
		t.Errorf("today year = %d, expected >= 2024", d.Year())
	}
}

func TestTimeTodayUtc(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("today-utc"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := native.AsDate(result[0])
	if d.Year() < 2024 {
		t.Errorf("today-utc year = %d, expected >= 2024", d.Year())
	}
}

// --- Extraction ---

func TestTimeYear(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("year", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 2024 {
		t.Errorf("year = %d, want 2024", v)
	}
}

func TestTimeMonth(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("month", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 3 {
		t.Errorf("month = %d, want 3", v)
	}
}

func TestTimeDay(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("day", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 15 {
		t.Errorf("day = %d, want 15", v)
	}
}

func TestTimeWeekday(t *testing.T) {
	r := timeRegistry(t)
	// 2024-03-15 is Friday → ISO weekday 5
	result := runBoru(t, r, callTimeDot("weekday", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 5 {
		t.Errorf("weekday = %d, want 5 (Friday)", v)
	}
}

func TestTimeYearDay(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("year-day", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 75 {
		t.Errorf("year-day = %d, want 75", v)
	}
}

func TestTimeWeekdayName(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("weekday-name", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	s, _ := native.AsString(result[0])
	if s != "Friday" {
		t.Errorf("weekday-name = %q, want %q", s, "Friday")
	}
}

func TestTimeMonthName(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("month-name", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	s, _ := native.AsString(result[0])
	if s != "March" {
		t.Errorf("month-name = %q, want %q", s, "March")
	}
}

func TestTimeIsoWeek(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("iso-week", native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 11 {
		t.Errorf("iso-week = %d, want 11", v)
	}
}

func TestTimeQuarter(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		month time.Month
		want  int64
	}{
		{1, 1}, {3, 1}, {4, 2}, {6, 2}, {7, 3}, {9, 3}, {10, 4}, {12, 4},
	}
	for _, tt := range tests {
		result := runBoru(t, r, callTimeDot("quarter", native.NewDate(time.Date(2024, tt.month, 1, 0, 0, 0, 0, time.UTC))))
		v, _ := native.AsInteger(result[0])
		if v != tt.want {
			t.Errorf("quarter(month=%d) = %d, want %d", tt.month, v, tt.want)
		}
	}
}

func TestTimeDaysInMonth(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		date string
		want int64
	}{
		{"2024-02-01", 29}, // leap year
		{"2023-02-01", 28},
		{"2024-01-15", 31},
		{"2024-04-10", 30},
	}
	for _, tt := range tests {
		d := native.NewDate(parseDate(t, tt.date))
		result := runBoru(t, r, callTimeDot("days-in-month", d))
		v, _ := native.AsInteger(result[0])
		if v != tt.want {
			t.Errorf("days-in-month(%s) = %d, want %d", tt.date, v, tt.want)
		}
	}
}

func TestTimeDaysInYear(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("days-in-year", native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))))
	v, _ := native.AsInteger(result[0])
	if v != 366 {
		t.Errorf("days-in-year(2024) = %d, want 366", v)
	}
}

func TestTimeLeapYear(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		date string
		want bool
	}{
		{"2024-01-01", true},
		{"2023-01-01", false},
		{"2000-01-01", true},
		{"1900-01-01", false},
	}
	for _, tt := range tests {
		d := native.NewDate(parseDate(t, tt.date))
		result := runBoru(t, r, callTimeDot("is-leap-year", d))
		b, _ := native.AsBoolean(result[0])
		if b != tt.want {
			t.Errorf("is-leap-year(%s) = %v, want %v", tt.date, b, tt.want)
		}
	}
}

func TestTimeToUnix(t *testing.T) {
	r := timeRegistry(t)
	ins := native.NewInstant(time.Unix(1710500000, 0))
	result := runBoru(t, r, callTimeDot("to-unix", ins))
	v, _ := native.AsInteger(result[0])
	if v != 1710500000 {
		t.Errorf("to-unix = %d, want 1710500000", v)
	}
}

func TestTimeToUnixMs(t *testing.T) {
	r := timeRegistry(t)
	ins := native.NewInstant(time.UnixMilli(1710500000123))
	result := runBoru(t, r, callTimeDot("to-unix-ms", ins))
	v, _ := native.AsInteger(result[0])
	if v != 1710500000123 {
		t.Errorf("to-unix-ms = %d, want 1710500000123", v)
	}
}

// --- Comparison ---

func TestTimeBeforeAfter(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("is-before", d1, d2))
	b, _ := native.AsBoolean(result[0])
	if !b {
		t.Error("expected 2024-01-01 is-before 2024-12-31 = true")
	}

	result = runBoru(t, r, callTimeDot("is-after", d1, d2))
	b, _ = native.AsBoolean(result[0])
	if b {
		t.Error("expected 2024-01-01 is-after 2024-12-31 = false")
	}
}

func TestTimeEqual(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("is-equal", d, d))
	b, _ := native.AsBoolean(result[0])
	if !b {
		t.Error("expected same date is-equal = true")
	}
}

// --- Formatting ---

func TestTimeToString(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-string", d))
	s, _ := native.AsString(result[0])
	if s != "2024-03-15" {
		t.Errorf("to-string = %q, want %q", s, "2024-03-15")
	}
}

func TestTimeFormat(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("format", d, native.NewString("02 Jan 2006")))
	s, _ := native.AsString(result[0])
	if s != "15 Mar 2024" {
		t.Errorf("format = %q, want %q", s, "15 Mar 2024")
	}
}

func TestTimeToIso(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-iso", d))
	s, _ := native.AsString(result[0])
	if s != "2024-03-15" {
		t.Errorf("to-iso = %q, want %q", s, "2024-03-15")
	}
}

// --- Legacy Arithmetic ---

func TestTimeAddDays(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("add-days", d, native.NewInteger(10)))
	got := native.AsDate(result[0])
	want := time.Date(2024, 3, 25, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-days(2024-03-15, 10) = %v, want %v", got, want)
	}
}

func TestTimeAddMonths(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("add-months", d, native.NewInteger(1)))
	got := native.AsDate(result[0])
	want := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-months(2024-01-31, 1) = %v, want %v", got, want)
	}
}

func TestTimeAddYears(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("add-years", d, native.NewInteger(2)))
	got := native.AsDate(result[0])
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-years(2024-03-15, 2) = %v, want %v", got, want)
	}
}

// --- Standard now word (non-module) ---

func TestNowStandardWord(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// `now` moved to boru:time-util; register the time/async words bare to
	// exercise the unchanged handler (production uses TimeUtil.now).
	for _, n := range native.TimeAsyncModuleNatives(native.MintTemporalModuleTypes(r)) {
		r.RegisterNativeFunc(n)
	}
	e := native.New(r)
	result, err := e.Run([]native.Value{native.NewWord("now")})
	if err != nil {
		t.Fatalf("now: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !native.ParentOf(&result[0]).ConformsTo(native.TInstant) {
		t.Errorf("now should return Instant, got %s", native.ParentOf(&result[0]))
	}
}

// --- Duration Construction ---

func TestTimeDurYears(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("years", native.NewInteger(2)))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 2 || cd.Months != 0 || cd.Days != 0 {
		t.Errorf("2 years = %+v, want {2 0 0}", cd)
	}
}

func TestTimeDurMonths(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("months", native.NewInteger(6)))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Months != 6 {
		t.Errorf("6 months = %+v, want {0 6 0}", cd)
	}
}

func TestTimeDurWeeks(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("weeks", native.NewInteger(2)))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Days != 14 {
		t.Errorf("2 weeks = %+v, want {0 0 14}", cd)
	}
}

func TestTimeDurDays(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("days", native.NewInteger(30)))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Days != 30 {
		t.Errorf("30 days = %+v, want {0 0 30}", cd)
	}
}

func TestTimeDurHours(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("hours", native.NewInteger(3)))
	d, ok := native.AsClockDuration(result[0])
	if !ok || d != 3*time.Hour {
		t.Errorf("3 hours = %v, want %v", d, 3*time.Hour)
	}
}

func TestTimeDurMinutes(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("minutes", native.NewInteger(90)))
	d, ok := native.AsClockDuration(result[0])
	if !ok || d != 90*time.Minute {
		t.Errorf("90 minutes = %v, want %v", d, 90*time.Minute)
	}
}

func TestTimeDurSeconds(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("seconds", native.NewInteger(30)))
	d, ok := native.AsClockDuration(result[0])
	if !ok || d != 30*time.Second {
		t.Errorf("30 seconds = %v, want %v", d, 30*time.Second)
	}
}

func TestTimeDurMs(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("ms", native.NewInteger(500)))
	d, ok := native.AsClockDuration(result[0])
	if !ok || d != 500*time.Millisecond {
		t.Errorf("500 ms = %v, want %v", d, 500*time.Millisecond)
	}
}

func TestTimeCalDur(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("cal-dur", native.NewInteger(1), native.NewInteger(6), native.NewInteger(15)))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 1 || cd.Months != 6 || cd.Days != 15 {
		t.Errorf("cal-dur 1 6 15 = %+v, want {1 6 15}", cd)
	}
}

// ISO 8601 duration parsing tests removed alongside the
// time-duration word.

// --- Duration Extraction ---

func TestTimeTotalHours(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("total-hours", timeTT(t, r).NewClockDuration(90*time.Minute)))
	v, _ := native.AsNumber(result[0])
	if v != 1.5 {
		t.Errorf("total-hours(90min) = %v, want 1.5", v)
	}
}

func TestTimeTotalMinutes(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("total-minutes", timeTT(t, r).NewClockDuration(2*time.Hour)))
	v, _ := native.AsNumber(result[0])
	if v != 120.0 {
		t.Errorf("total-minutes(2h) = %v, want 120", v)
	}
}

func TestTimeTotalSeconds(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("total-seconds", timeTT(t, r).NewClockDuration(90*time.Second)))
	v, _ := native.AsNumber(result[0])
	if v != 90.0 {
		t.Errorf("total-seconds(90s) = %v, want 90", v)
	}
}

func TestTimeTotalMs(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("total-ms", timeTT(t, r).NewClockDuration(2*time.Second+500*time.Millisecond)))
	v, _ := native.AsNumber(result[0])
	if v != 2500.0 {
		t.Errorf("total-ms(2.5s) = %v, want 2500", v)
	}
}

func TestTimeDurYearsExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("dur-years", timeTT(t, r).NewCalendarDuration(1, 6, 15)))
	v, _ := native.AsInteger(result[0])
	if v != 1 {
		t.Errorf("dur-years = %d, want 1", v)
	}
}

func TestTimeDurMonthsExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("dur-months", timeTT(t, r).NewCalendarDuration(1, 6, 15)))
	v, _ := native.AsInteger(result[0])
	if v != 6 {
		t.Errorf("dur-months = %d, want 6", v)
	}
}

func TestTimeDurDaysExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("dur-days", timeTT(t, r).NewCalendarDuration(1, 6, 15)))
	v, _ := native.AsInteger(result[0])
	if v != 15 {
		t.Errorf("dur-days = %d, want 15", v)
	}
}

func TestTimeDurSign(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("dur-sign", timeTT(t, r).NewCalendarDuration(1, 0, 0)))
	v, _ := native.AsInteger(result[0])
	if v != 1 {
		t.Errorf("dur-sign(+) = %d, want 1", v)
	}
	result = runBoru(t, r, callTimeDot("dur-sign", timeTT(t, r).NewCalendarDuration(-1, 0, 0)))
	v, _ = native.AsInteger(result[0])
	if v != -1 {
		t.Errorf("dur-sign(-) = %d, want -1", v)
	}
	result = runBoru(t, r, callTimeDot("dur-sign", timeTT(t, r).NewCalendarDuration(0, 0, 0)))
	v, _ = native.AsInteger(result[0])
	if v != 0 {
		t.Errorf("dur-sign(0) = %d, want 0", v)
	}
}

// --- Arithmetic ---

func TestTimeUntil(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("until", d1, d2))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 0 || cd.Months != 2 || cd.Days != 14 {
		t.Errorf("until = %+v, want {0 2 14}", cd)
	}
}

func TestTimeSince(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("since", d1, d2))
	cd, ok := native.AsCalendarDuration(result[0])
	if !ok || cd.Years != 0 || cd.Months != 2 || cd.Days != 14 {
		t.Errorf("since = %+v, want {0 2 14}", cd)
	}
}

func TestTimeDiffInstants(t *testing.T) {
	r := timeRegistry(t)
	i1 := native.NewInstant(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	i2 := native.NewInstant(time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("diff", i1, i2))
	d, ok := native.AsClockDuration(result[0])
	if !ok || d != 2*time.Hour+30*time.Minute {
		t.Errorf("diff = %v, want 2h30m", d)
	}
}

// --- Comparison extended ---

func TestTimeCompare(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("compare", d1, d2))
	v, _ := native.AsInteger(result[0])
	if v != -1 {
		t.Errorf("compare(d1, d2) = %d, want -1", v)
	}

	result = runBoru(t, r, callTimeDot("compare", d1, d1))
	v, _ = native.AsInteger(result[0])
	if v != 0 {
		t.Errorf("compare(d1, d1) = %d, want 0", v)
	}
}

func TestTimeBetween(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))
	start := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	end := native.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("is-between", d, start, end))
	b, _ := native.AsBoolean(result[0])
	if !b {
		t.Error("expected is-between = true")
	}
}

func TestTimeEarliest(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("earliest", d1, d2))
	got := native.AsDate(result[0])
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 1 {
		t.Errorf("earliest = %v, want 2024-01-01", got)
	}
}

func TestTimeLatest(t *testing.T) {
	r := timeRegistry(t)
	d1 := native.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := native.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("latest", d1, d2))
	got := native.AsDate(result[0])
	if got.Year() != 2024 || got.Month() != 12 || got.Day() != 31 {
		t.Errorf("latest = %v, want 2024-12-31", got)
	}
}

// --- Conversion ---

func TestTimeToDate(t *testing.T) {
	r := timeRegistry(t)
	dt := native.NewDateTime(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-date", dt))
	got := native.AsDate(result[0])
	if got.Hour() != 0 || got.Day() != 15 {
		t.Errorf("to-date = %v, want midnight 2024-03-15", got)
	}
}

func TestTimeToTimeOfDay(t *testing.T) {
	r := timeRegistry(t)
	dt := native.NewDateTime(time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-time-of-day", dt))
	tod := native.AsTimeOfDay(result[0])
	if tod != 14*time.Hour+30*time.Minute {
		t.Errorf("to-time-of-day = %v, want 14h30m", tod)
	}
}

func TestTimeToDatetime(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-datetime", d))
	if !native.ParentOf(&result[0]).ConformsTo(native.TDateTime) {
		t.Errorf("to-datetime type = %s, want DateTime", native.ParentOf(&result[0]))
	}
}

func TestTimeToInstant(t *testing.T) {
	r := timeRegistry(t)
	dt := native.NewDateTime(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	tz := timeTT(t, r).NewTimezone(time.UTC)
	result := runBoru(t, r, callTimeDot("to-instant", dt, tz))
	if !native.ParentOf(&result[0]).ConformsTo(native.TInstant) {
		t.Errorf("to-instant type = %s, want Instant", native.ParentOf(&result[0]))
	}
}

func TestTimeToLocal(t *testing.T) {
	r := timeRegistry(t)
	ins := native.NewInstant(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	tz := timeTT(t, r).NewTimezone(time.UTC)
	result := runBoru(t, r, callTimeDot("to-local", ins, tz))
	if !native.ParentOf(&result[0]).ConformsTo(native.TDateTime) {
		t.Errorf("to-local type = %s, want DateTime", native.ParentOf(&result[0]))
	}
}

func TestTimeToUtc(t *testing.T) {
	r := timeRegistry(t)
	ins := native.NewInstant(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	result := runBoru(t, r, callTimeDot("to-utc", ins))
	if !native.ParentOf(&result[0]).ConformsTo(native.TDateTime) {
		t.Errorf("to-utc type = %s, want DateTime", native.ParentOf(&result[0]))
	}
}

// --- Rounding ---

func TestTimeStartOf(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("start-of", d, native.NewString("month")))
	got := native.AsDate(result[0])
	if got.Day() != 1 || got.Month() != 3 {
		t.Errorf("start-of month = %v, want 2024-03-01", got)
	}

	result = runBoru(t, r, callTimeDot("start-of", d, native.NewString("year")))
	got = native.AsDate(result[0])
	if got.Month() != 1 || got.Day() != 1 {
		t.Errorf("start-of year = %v, want 2024-01-01", got)
	}

	result = runBoru(t, r, callTimeDot("start-of", d, native.NewString("quarter")))
	got = native.AsDate(result[0])
	if got.Month() != 1 || got.Day() != 1 {
		t.Errorf("start-of quarter = %v, want 2024-01-01", got)
	}
}

func TestTimeEndOf(t *testing.T) {
	r := timeRegistry(t)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	result := runBoru(t, r, callTimeDot("end-of", d, native.NewString("month")))
	got := native.AsDate(result[0])
	if got.Day() != 31 || got.Month() != 3 {
		t.Errorf("end-of month = %v, want 2024-03-31", got)
	}

	result = runBoru(t, r, callTimeDot("end-of", d, native.NewString("year")))
	got = native.AsDate(result[0])
	if got.Month() != 12 || got.Day() != 31 {
		t.Errorf("end-of year = %v, want 2024-12-31", got)
	}
}

// --- Timezone ---

func TestTimeTzUtc(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("tz-utc"))
	loc := native.AsTimezone(result[0])
	if loc == nil || loc.String() != "UTC" {
		t.Errorf("tz-utc = %v, want UTC", loc)
	}
}

func TestTimeTzName(t *testing.T) {
	r := timeRegistry(t)
	result := runBoru(t, r, callTimeDot("tz-name", timeTT(t, r).NewTimezone(time.UTC)))
	s, _ := native.AsString(result[0])
	if s != "UTC" {
		t.Errorf("tz-name = %q, want UTC", s)
	}
}

func TestTimeTzOffset(t *testing.T) {
	r := timeRegistry(t)
	ins := native.NewInstant(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tz := timeTT(t, r).NewTimezone(time.UTC)
	result := runBoru(t, r, callTimeDot("tz-offset", ins, tz))
	s, _ := native.AsString(result[0])
	if s != "+00:00" {
		t.Errorf("tz-offset UTC = %q, want +00:00", s)
	}
}

// --- Parsing ---
// parse-date, parse-datetime, auto-date tests removed alongside
// the corresponding word handlers.

// --- add/sub with temporal types (module word extensions) ---
//
// The temporal overloads live on boru:time-util as word extensions of
// core add/sub; installing the module's exports transplants them, so
// the bare words dispatch them exactly as they did when they sat on
// the core signatures.

func TestAddDateCalendarDuration(t *testing.T) {
	r := timeRegistry(t)
	e := native.New(r)
	// "2024-01-31" date add 1 months → need date + CalendarDuration on stack
	d := native.NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	cd := timeTT(t, r).NewCalendarDuration(0, 1, 0)
	result, err := e.Run([]native.Value{cd, d, native.NewWord("add")})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	got := native.AsDate(result[0])
	// Go normalizes: Jan 31 + 1 month = March 2
	want := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("date add CalDur = %v, want %v", got, want)
	}
}

func TestSubDateCalendarDuration(t *testing.T) {
	r := timeRegistry(t)
	e := native.New(r)
	d := native.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	cd := timeTT(t, r).NewCalendarDuration(0, 1, 0)
	result, err := e.Run([]native.Value{cd, d, native.NewWord("sub")})
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	got := native.AsDate(result[0])
	want := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("date sub CalDur = %v, want %v", got, want)
	}
}

func TestAddInstantClockDuration(t *testing.T) {
	r := timeRegistry(t)
	e := native.New(r)
	ins := native.NewInstant(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	dur := timeTT(t, r).NewClockDuration(30 * time.Minute)
	result, err := e.Run([]native.Value{dur, ins, native.NewWord("add")})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	got := native.AsInstant(result[0])
	if got.Minute() != 30 {
		t.Errorf("instant add 30min = %v, want 10:30", got)
	}
}

// helper
func parseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parseDate(%q): %v", s, err)
	}
	return d
}
