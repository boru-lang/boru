package eng

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoRawDataNilProbes is the regression gate for the `Data == nil`
// migration (see eng/go/CLAUDE.md "Payload-presence vs value-mode").
//
// `Value.Data == nil` is an overloaded probe: depending on the call
// site it has meant "this is a bare type literal", "this is not a
// concrete value", or "there is no payload to read" — three intents
// that genuinely diverge on carriers (a list/map carrier has Data !=
// nil but is not concrete) and on None. Consumer code must therefore
// state its intent through a named predicate instead of the raw probe:
//
//	IsBareTypeNode(v)  — v is its own lattice node (Data == nil && !Carrier)
//	IsConcrete(v)      — v carries a real, readable payload
//	IsTypeLiteral(v)   — IsBareTypeNode minus the None literal
//	RequireConcreteList/Map(v, op) — accessor guards
//
// Since no-copy phase 4 the payload is read through core.DataOf (the
// field is the content's, unexported), so the probe is spelled
// `DataOf(&v) == nil` as well as the historical `v.Data == nil`; the gate
// matches both.
//
// A small allowlist of files legitimately tests payload presence at the
// lowest level (the value/predicate/classifier home, the rendering and
// comparison primitives, and carrier construction). Everywhere else the
// raw probe is forbidden; add the intent-named predicate instead of
// extending the allowlist.
func TestNoRawDataNilProbes(t *testing.T) {
	allow := map[string]bool{
		"value.go":                    true, // Value struct + As* payload accessors + String
		"util.go":                     true, // defines IsConcrete / IsBareTypeNode / IsTypeLiteral
		"shape.go":                    true, // the ValueShape classifier
		"payload.go":                  true, // Payload variant definitions
		"equal.go":                    true, // value-equality primitive (carrier-inclusive)
		"canon.go":                    true, // canonical rendering
		"print.go":                    true, // String / JSON rendering
		"trace.go":                    true, // debug rendering
		"compare_scalar_behaviors.go": true, // comparison primitive (carrier-inclusive)
		"carrier.go":                  true, // carrier construction & inspection
		"value_mode_test.go":          true, // documents the divergence on purpose
		"data_nil_gate_test.go":       true, // this gate (mentions the pattern in prose)
	}

	probe := regexp.MustCompile(`(?:\.Data|DataOf\((?:[^()]|\([^()]*\))*\))\s*==\s*nil`)
	var offenders []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if allow[filepath.Base(path)] {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(src), "\n") {
			// Ignore matches inside line comments.
			if idx := strings.Index(line, "//"); idx >= 0 {
				line = line[:idx]
			}
			if probe.MatchString(line) {
				offenders = append(offenders, path+":"+itoa(i+1)+":"+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("raw `DataOf(&v) == nil` / `.Data == nil` probe(s) found outside the allowlist — "+
			"use IsBareTypeNode / IsConcrete / IsTypeLiteral instead:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
