# Time and Date Operations Word Design for boru

## Context

boru currently has no date/time types or words. The type system (`types.go`) is extensible, and `time` is already imported in `value.go`. This design defines time/date types and words for boru, drawing from the JavaScript Temporal API as the conceptual model and using Go's `time` package as the implementation backend. The design complements the existing dataframe words (`doc/DATAFRAME-WORDS.md`) — all scalar date words compose with `apply`, `sift`, `group`, etc. for column-level operations.

**Deliverable:** New design document at `lang/doc/design/TEMPORAL-WORDS.9.md`.

> **Status update (post Step 11 of legacy/TYPE-DECOUPLING.10.ignore):** the
> free-form text parsing surface originally proposed in this design
> has been **removed as a feature**. The temporal types remain (Date,
> DateTime, Instant, TimeOfDay, CalDuration, ClkDuration, Timezone,
> Timeout, Interval), as does construction from numeric Unix epochs
> (`unix` / `unix-ms` / `unix-ns`) and wall-clock (`now-local` /
> `today` / `today-utc`), and output-only formatting (`to-string`
> / `to-iso` / `format`). There is no symmetric text-input path —
> boru programs that need to read date strings should pre-parse them
> in their source system or use Unix epochs. Affected sections below
> are marked with **REMOVED**.

---

## Analysis: Is the JS Temporal API Sufficient?

### Where Temporal Is Sufficient

Temporal covers the vast majority of real-world date/time operations. Its type separation (PlainDate vs PlainTime vs Instant vs ZonedDateTime) is a genuine design improvement over prior datetime APIs. The explicit Duration type with calendar-aware components (years, months) solves a class of bugs that plague Go's `time.Duration`. For boru, Temporal provides a sound conceptual model for:

- Unambiguous date construction and parsing
- Component extraction (year, month, day, dayOfWeek, weekOfYear, etc.)
- Date arithmetic with calendar-aware durations
- Comparison and ordering
- Timezone conversions
- Formatting and display

### Where Temporal Falls Short (and Go Enhances)

| Gap | Temporal Limitation | Go Enhancement |
|-----|-------------------|----------------|
| **Monotonic clock** | No concept | `time.Time` carries monotonic reading for reliable elapsed measurement |
| **Flexible parsing** | ISO 8601 only | `time.Parse` with reference-time layouts handles arbitrary formats |
| **Batch performance** | JS heap allocation per object | Go `time.Time` is a struct (no GC pressure for million-row columns) |
| **Duration ambiguity** | 10-component Duration conflates calendar and clock spans | boru splits into CalDuration (years/months/days) and ClkDuration (hours down to ns) |
| **Business periods** | No quarter, fiscal year, start-of-period | boru adds `quarter`, `start-of`, `end-of` as first-class words |
| **Multiple calendars** | Supports Hebrew, Islamic, etc. | Go is Gregorian-only; boru defers non-Gregorian to future work |
| **Unix timestamps** | Requires going through Instant.epochNanoseconds | Go has direct `Unix()`, `UnixMilli()`, `UnixMicro()`, `UnixNano()` |

### Verdict

Temporal's conceptual model is sufficient as the design foundation. Go fills three practical gaps: monotonic elapsed time, batch/column performance, and flexible format parsing. boru uses Temporal's type distinctions as its mental model but implements against Go's `time.Time` internally.

---

## Recommended boru Type Hierarchy

```
Scalar/
  Time/
    Instant       -- UTC nanosecond timestamp (Go: time.Time in UTC)
    DateTime      -- date + time, no timezone (Go: time.Time with sentinel Location)
    Date          -- date only: year, month, day (Go: time.Time at midnight)
    TimeOfDay     -- time only: hour, min, sec, ns (Go: time.Duration from midnight)
    Duration/
      CalDuration -- calendar duration: years, months, days (custom struct; no Go equivalent)
      ClkDuration -- clock duration: hours, min, sec, ns (Go: time.Duration)
    Timezone      -- named timezone (Go: *time.Location)
```

### Why Not All 9 Temporal Types?

PlainYearMonth and PlainMonthDay are rare in data work. They can be represented as Date with conventions. Keeping the type count at 6 core types + 2 duration subtypes reduces cognitive load in a concatenative language where types are implicit on the stack.

### Why Two Duration Subtypes?

This is the key insight from comparing Temporal and Go. Calendar durations (1 month, 2 years) are fundamentally different from clock durations (3 hours, 500ms). A month is not a fixed number of nanoseconds. Go punts on this entirely (no month/year in Duration). Temporal conflates them into one type with 10 fields. boru makes the distinction explicit: adding a CalDuration to a Date yields a Date; adding a ClkDuration to a DateTime yields a DateTime.

### Why Instant Separate from DateTime?

This is Temporal's best idea. An Instant is an absolute point in time (UTC). A DateTime is a wall-clock reading without timezone context. Confusing these is the source of countless bugs. In Go, both are `time.Time`, distinguished only by Location. boru makes the distinction at the type level.

---

## Complete Word List

### Notation

- `D` = Date, `DT` = DateTime, `Ins` = Instant, `ToD` = TimeOfDay
- `CD` = CalDuration, `CkD` = ClkDuration, `TZ` = Timezone
- `S` = String, `I` = Integer, `N` = Number, `B` = Boolean

---

### 1. Construction (12 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `date` | `S -> D` | `"2024-03-15" date` | Parse ISO 8601 date |
| `date` | `I I I -> D` | `date 2024 3 15` | Year, month, day via suffix |
| `datetime` | `S -> DT` | `"2024-03-15T10:30:00" datetime` | Parse ISO 8601 datetime |
| `datetime` | `D ToD -> DT` | `mydate mytime datetime` | Combine date + time |
| `instant` | `S -> Ins` | `"2024-03-15T10:30:00Z" instant` | Parse ISO 8601 with Z/offset |
| `instant` | `DT TZ -> Ins` | `my-dt "UTC" tz instant` | Anchor datetime in timezone |
| `time-of-day` | `S -> ToD` | `"14:30:00" time-of-day` | Parse time string |
| `time-of-day` | `I I -> ToD` | `time-of-day 14 30` | Hours, minutes via suffix |
| `tz` | `S -> TZ` | `"America/New_York" tz` | Load IANA timezone |
| `unix` | `I -> Ins` | `1710500000 unix` | Unix seconds → Instant |
| `unix-ms` | `I -> Ins` | `1710500000000 unix-ms` | Unix milliseconds → Instant |
| `unix-ns` | `I -> Ins` | `1710500000000000000 unix-ns` | Unix nanoseconds → Instant |

---

### 2. Current Time (5 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `now` | `-> Ins` | `now` | Current instant (UTC) |
| `now-local` | `-> DT` | `now-local` | Current local datetime |
| `today` | `-> D` | `today` | Current local date |
| `today-utc` | `-> D` | `today-utc` | Current UTC date |
| `elapsed` | `Ins -> CkD` | `start elapsed` | Monotonic elapsed since instant (Go-only feature) |

---

### 3. Component Extraction (18 words)

All work on Date, DateTime, and Instant (Instant treated as UTC).

| Word | Signature | Example | Output |
|------|-----------|---------|--------|
| `year` | `D\|DT\|Ins -> I` | `"2024-03-15" date year` | `2024` |
| `month` | `D\|DT\|Ins -> I` | `"2024-03-15" date month` | `3` (1-12) |
| `day` | `D\|DT\|Ins -> I` | `"2024-03-15" date day` | `15` |
| `hour` | `DT\|Ins\|ToD -> I` | `"14:30" time-of-day hour` | `14` |
| `minute` | `DT\|Ins\|ToD -> I` | same pattern | `30` |
| `second` | `DT\|Ins\|ToD -> I` | same pattern | |
| `nanosecond` | `DT\|Ins\|ToD -> I` | same pattern | |
| `weekday` | `D\|DT\|Ins -> I` | `"2024-03-15" date weekday` | `5` (1=Mon, 7=Sun, ISO) |
| `weekday-name` | `D\|DT\|Ins -> S` | `"2024-03-15" date weekday-name` | `"Friday"` |
| `month-name` | `D\|DT\|Ins -> S` | `"2024-03-15" date month-name` | `"March"` |
| `year-day` | `D\|DT\|Ins -> I` | `"2024-03-15" date year-day` | `75` (1-366) |
| `iso-week` | `D\|DT\|Ins -> I` | `"2024-03-15" date iso-week` | `11` |
| `quarter` | `D\|DT\|Ins -> I` | `"2024-03-15" date quarter` | `1` (1-4; neither Temporal nor Go provides this) |
| `days-in-month` | `D\|DT\|Ins -> I` | `"2024-02-01" date days-in-month` | `29` |
| `days-in-year` | `D\|DT\|Ins -> I` | `"2024-01-01" date days-in-year` | `366` |
| `is-leap-year` | `D\|DT\|Ins -> B` | `"2024-01-01" date is-leap-year` | `true` |
| `to-unix` | `Ins -> I` | `now to-unix` | Unix seconds |
| `to-unix-ms` | `Ins -> I` | `now to-unix-ms` | Unix milliseconds |

---

### 4. Duration Construction (12 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `years` | `I -> CD` | `2 years` | CalDuration |
| `months` | `I -> CD` | `6 months` | CalDuration |
| `weeks` | `I -> CD` | `1 weeks` | CalDuration (= 7 days) |
| `days` | `I -> CD` | `30 days` | CalDuration (calendar days, not fixed seconds) |
| `hours` | `N -> CkD` | `3 hours` | ClkDuration |
| `minutes` | `N -> CkD` | `90 minutes` | ClkDuration |
| `seconds` | `N -> CkD` | `30 seconds` | ClkDuration |
| `ms` | `N -> CkD` | `500 ms` | Milliseconds |
| `us` | `N -> CkD` | `100 us` | Microseconds |
| `ns` | `I -> CkD` | `1000 ns` | Nanoseconds |
| `cal-dur` | `I I I -> CD` | `cal-dur 1 6 15` | years, months, days |
| `duration` | `S -> CD\|CkD` | `"P1Y6M" duration` | Parse ISO 8601 duration |

---

### 5. Duration Extraction (8 words)

| Word | Signature | Example | Output |
|------|-----------|---------|--------|
| `total-hours` | `CkD -> N` | `90 minutes total-hours` | `1.5` |
| `total-minutes` | `CkD -> N` | `2 hours total-minutes` | `120.0` |
| `total-seconds` | `CkD -> N` | same pattern | |
| `total-ms` | `CkD -> N` | same pattern | |
| `dur-years` | `CD -> I` | `"P1Y6M" duration dur-years` | `1` (component, not total) |
| `dur-months` | `CD -> I` | same pattern | `6` |
| `dur-days` | `CD -> I` | same pattern | |
| `dur-sign` | `CD\|CkD -> I` | negative dur → `-1` | -1, 0, or 1 |

---

### 6. Arithmetic (7 words)

Uses existing `add` and `sub` words with NEW signatures.

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `add` | `D CD -> D` | `"2024-01-31" date add 1 months` → `2024-02-29` | Calendar-aware; Go: `AddDate` |
| `add` | `DT CkD -> DT` | `my-dt add 3 hours` | Clock arithmetic |
| `add` | `Ins CkD -> Ins` | `now add 30 minutes` | Absolute time arithmetic |
| `sub` | same patterns | `"2024-03-15" date sub 1 months` → `2024-02-15` | |
| `until` | `D D -> CD` | `"2024-01-01" date until "2024-03-15" date` | Duration between dates |
| `since` | `D D -> CD` | `"2024-03-15" date since "2024-01-01" date` | Reverse of until |
| `diff` | `Ins Ins -> CkD` | `start diff end` | Precise time difference; Go: `Sub` |

Type safety: adding CalDuration to Instant is a type error (months have no fixed ns equivalent). Must convert to DateTime + timezone first.

---

### 7. Comparison (7 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `is-before` | `D D -> B` | `d1 is-before d2` | Works for D, DT, Ins (same type) |
| `is-after` | `D D -> B` | `d1 is-after d2` | |
| `eq` | existing | already works | Extend for date types |
| `compare` | `D D -> I` | `d1 compare d2` | -1, 0, 1 for sorting |
| `is-between` | `D D D -> B` | `d is-between start end` | Is d in [start, end]? |
| `earliest` | `D D -> D` | `d1 earliest d2` | Min of two dates |
| `latest` | `D D -> D` | `d1 latest d2` | Max of two dates |

---

### 8. Type Conversion (9 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `to-date` | `DT\|Ins -> D` | `my-dt to-date` | Strip time components |
| `to-time-of-day` | `DT\|Ins -> ToD` | `my-dt to-time-of-day` | Strip date components |
| `to-datetime` | `D -> DT` | `"2024-03-15" date to-datetime` | Add midnight |
| `to-instant` | `DT TZ -> Ins` | `my-dt "UTC" tz to-instant` | Anchor in timezone |
| `to-local` | `Ins TZ -> DT` | `now "Europe/Dublin" tz to-local` | View in timezone |
| `to-utc` | `Ins -> DT` | `now to-utc` | View as UTC datetime |
| `to-string` | `D\|DT\|Ins -> S` | `today to-string` | ISO 8601 output |
| `format` | `D\|DT\|Ins S -> S` | `today format "02 Jan 2006"` | Go reference-time layout |
| `to-iso` | `D\|DT\|Ins -> S` | `now to-iso` | Always full ISO 8601 |

---

### 9. Rounding / Period Boundaries (4 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `round` | `DT\|Ins S -> DT\|Ins` | `my-dt round "hour"` | Round to nearest unit |
| `truncate` | `DT\|Ins S -> DT\|Ins` | `my-dt truncate "day"` | Truncate to unit |
| `start-of` | `D\|DT S -> D\|DT` | `today start-of "month"` | First of month/quarter/year/week |
| `end-of` | `D\|DT S -> D\|DT` | `today end-of "quarter"` | Last of period |

Units: `"year"`, `"quarter"`, `"month"`, `"week"`, `"day"`, `"hour"`, `"minute"`, `"second"`.

---

### 10. Timezone (6 words)

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| `tz` | `S -> TZ` | `"America/New_York" tz` | Load timezone |
| `tz-utc` | `-> TZ` | `tz-utc` | UTC constant |
| `tz-local` | `-> TZ` | `tz-local` | System local |
| `tz-name` | `TZ -> S` | `"America/New_York" tz tz-name` | Name string |
| `tz-offset` | `Ins TZ -> S` | `now "America/New_York" tz tz-offset` | `"-04:00"` (DST-aware) |
| `is-dst` | `Ins TZ -> B` | `now "America/New_York" tz is-dst` | Is DST active? |

---

### 11. Parsing — **REMOVED**

> All four words below were removed as a feature in Step 11 of
> legacy/TYPE-DECOUPLING.10.ignore. The parser-coupling between the temporal types
> and free-form text was the last remaining reason for eng/ to know
> about temporal types; removing the parsing surface eliminates that
> coupling. The original proposal is retained below struck through
> for design-history reasons.

| Word | Signature | Example | Notes |
|------|-----------|---------|-------|
| ~~`date`~~ (ISO-string form) | ~~`S -> D`~~ | ~~`"2024-03-15" date`~~ | **REMOVED** |
| ~~`parse-date`~~ | ~~`S S -> D`~~ | ~~`"15/03/2024" parse-date "02/01/2006"`~~ | **REMOVED** |
| ~~`parse-datetime`~~ | ~~`S S -> DT`~~ | ~~`"Mar 15, 2024 2:30PM" parse-datetime "Jan 02, 2006 3:04PM"`~~ | **REMOVED** |
| ~~`auto-date`~~ | ~~`S -> D`~~ | ~~`"March 15, 2024" auto-date`~~ | **REMOVED** |

~~`auto-date` tries ISO 8601, RFC 2822, US format, European format, etc. in sequence. Neither Temporal nor Go provides this out of the box, but it's essential for messy real-world data.~~

---

## Word Summary

| Category | Count | Words |
|----------|-------|-------|
| Construction | 12 | `date`, `datetime`, `instant`, `time-of-day`, `tz`, `unix`, `unix-ms`, `unix-ns`, `cal-dur`, `duration` |
| Current Time | 5 | `now`, `now-local`, `today`, `today-utc`, `elapsed` |
| Extraction | 18 | `year`, `month`, `day`, `hour`, `minute`, `second`, `nanosecond`, `weekday`, `weekday-name`, `month-name`, `year-day`, `iso-week`, `quarter`, `days-in-month`, `days-in-year`, `is-leap-year`, `to-unix`, `to-unix-ms` |
| Duration Construction | 12 | `years`, `months`, `weeks`, `days`, `hours`, `minutes`, `seconds`, `ms`, `us`, `ns`, `cal-dur`, `duration` |
| Duration Extraction | 8 | `total-hours`, `total-minutes`, `total-seconds`, `total-ms`, `dur-years`, `dur-months`, `dur-days`, `dur-sign` |
| Arithmetic | 7 | `add` (extend), `sub` (extend), `until`, `since`, `diff` |
| Comparison | 7 | `is-before`, `is-after`, `eq` (extend), `compare`, `is-between`, `earliest`, `latest` |
| Conversion | 9 | `to-date`, `to-time-of-day`, `to-datetime`, `to-instant`, `to-local`, `to-utc`, `to-string`, `format`, `to-iso` |
| Rounding | 4 | `round` (extend), `truncate` (extend), `start-of`, `end-of` |
| Timezone | 6 | `tz`, `tz-utc`, `tz-local`, `tz-name`, `tz-offset`, `is-dst` |
| ~~Parsing~~ | ~~4~~ | ~~`date`, `parse-date`, `parse-datetime`, `auto-date`~~ — **REMOVED**, see Step 11 of legacy/TYPE-DECOUPLING.10.ignore |
| **Total** | **~92** | (some overlap: `date`, `duration` appear in multiple categories) |

**Unique new words: ~70. Extended existing words: `add`, `sub`, `eq`, `compare`, `round`, `truncate` (6).**

---

## Composable Workflow Examples

### Age calculation from birthdays
```boru
people apply birthday [date]
       mutate {age:(birthday until today dur-years)}
       sortby age
```

### Monthly revenue aggregation
```boru
sales apply order_date [date]
      sift {order_date:(is-after "2023-01-01" date)}
      mutate {month_start:(order_date start-of "month")}
      groupby month_start {revenue:(sum amount)}
      sortby month_start
```

### Timezone conversion for global team
```boru
meetings apply utc_time [instant]
         mutate {
           ny_time:(utc_time "America/New_York" tz to-local format "Jan 02, 3:04 PM")
           tokyo_time:(utc_time "Asia/Tokyo" tz to-local format "Jan 02, 3:04 PM")
         }
```

### Invoice due dates
```boru
invoices apply invoice_date [date]
         mutate {due_date:(invoice_date add 30 days)}
         sift {due_date:(is-before today)}
         sortby due_date
```

### Pipeline timing (Go monotonic clock)
```boru
now
... expensive-operation ...
elapsed total-seconds
# precise elapsed time, immune to clock adjustment
```

### ~~Parsing messy date formats~~ — **REMOVED**
```boru
# REMOVED in Step 11 of legacy/TYPE-DECOUPLING.10.ignore; left for design history only.
raw-data apply date_str [auto-date]
         dropna date_str
         mutate {yr:(date_str year)}
         groupby yr {n:(count)}
```

---

## Implementation Priority

| Phase | Words | Coverage |
|-------|-------|----------|
| **Phase 1 (Core)** | `now-local`, `today`, `unix`, `unix-ms`, `year`, `month`, `day`, `weekday`, `add`, `sub`, `until`, `is-before`, `is-after`, `to-string`, `format` (~~`parse-date`, `date`, `datetime`, `instant`~~ — removed Step 11) | 80% of use cases |
| **Phase 2 (Duration)** | `years`, `months`, `weeks`, `days`, `hours`, `minutes`, `seconds`, `diff`, `elapsed`, duration extraction words | Duration arithmetic |
| **Phase 3 (Business)** | `quarter`, `start-of`, `end-of`, `iso-week`, ~~`auto-date`~~, `is-between`, `earliest`, `latest` | Business analytics |
| **Phase 4 (Timezone)** | `tz`, `to-local`, `to-instant`, `to-utc`, `tz-offset`, `is-dst` | Global data |

---

## Files to Create

Following the existing `native_*.go` pattern:

- `lang/go/internal/engine/native_temporal_construct.go` — date, datetime, instant, time-of-day, unix
- `lang/go/internal/engine/native_temporal_now.go` — now, now-local, today, today-utc, elapsed
- `lang/go/internal/engine/native_temporal_extract.go` — year, month, day, hour, minute, second, weekday, etc.
- `lang/go/internal/engine/native_temporal_duration.go` — years, months, days, hours, etc., duration extraction
- `lang/go/internal/engine/native_temporal_arithmetic.go` — add/sub signatures, until, since, diff
- `lang/go/internal/engine/native_temporal_compare.go` — is-before, is-after, is-between, earliest, latest
- `lang/go/internal/engine/native_temporal_convert.go` — to-date, to-datetime, to-instant, to-local, format
- `lang/go/internal/engine/native_temporal_round.go` — round, truncate, start-of, end-of
- `lang/go/internal/engine/native_temporal_tz.go` — tz, tz-utc, tz-local, tz-name, tz-offset, is-dst
- ~~`lang/go/internal/engine/native_temporal_parse.go` — parse-date, parse-datetime, auto-date~~ — **REMOVED** (Step 11)

## Files to Modify

- `lang/go/internal/engine/types.go` — add `Scalar/Time/*` type paths and IDs (lines 36-59, 112-150)
- `lang/go/internal/engine/value.go` — add `NewDate()`, `NewDateTime()`, `NewInstant()`, etc. constructors
- `lang/go/internal/engine/registry.go` — register all new words in `registerBuiltins()`
- `lang/go/internal/engine/native_math_add.go` — extend `add` with temporal signatures
- `lang/go/internal/engine/native_math_sub.go` — extend `sub` with temporal signatures

## Verification

- `lang/go/test/temporal_test.go` — test each word category
- Run: `cd boru && go test ./... -run TestTemporal -v`
- Test composition with dataframe words: `apply`, `sift`, `mutate`, `groupby`
