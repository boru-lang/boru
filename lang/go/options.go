package lang

import (
	"fmt"

	parser "github.com/boru-lang/boru/parser/go"
)

// ParseOptions parses a `--options` style jsonic string into a nested
// map. Syntax (plain jsonic — colon chains nest):
//
//	tape:initial:65536              => {tape:{initial:65536}}
//	tape:{initial:65536,grows:9}    => {tape:{initial:65536, grows:9}}
//	a:1,b:c:2                       => {a:1, b:{c:2}}
//
// It is the front half of the CLI's --options handling; ApplyOptions is
// the back half that maps the result onto an Options value.
func ParseOptions(s string) (map[string]any, error) {
	return parser.ParseConfig(s)
}

// ApplyOptions maps a parsed option map (from ParseOptions) onto o. It is
// strict: an unrecognised key — at the top level or within a known
// section — is an error rather than silently ignored, so a misspelled
// key fails loudly instead of being dropped. Known schema:
//
//	tape  : { initial : <int>, grows : <int>, factor : <number> }
//	steps : <int>   evaluation step budget (>= 1); unset = engine default
//	inplace : on | verify | off   the in-place compilation prototype
//	          (core/go/inplace.go); unset = the BORU_INPLACE default
//
// New option sections are added here as they are introduced.
func ApplyOptions(o *Options, m map[string]any) error {
	for key, val := range m {
		switch key {
		case "tape":
			sub, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("option %q must be a map (e.g. tape:initial:65536), got %T", key, val)
			}
			if err := applyTapeOptions(o, sub); err != nil {
				return err
			}
		case "steps":
			n, err := optInt(key, val)
			if err != nil {
				return err
			}
			// A budget of 0 would abort before the first step, so it is
			// rejected rather than silently meaning "unset" — that is what
			// keeps Registry.StepLimit's zero unambiguous.
			if n < 1 {
				return fmt.Errorf("option %q must be at least 1, got %d", key, n)
			}
			o.Steps = n
		case "inplace":
			mode, ok := map[string]InPlaceOption{"on": InPlaceOn, "verify": InPlaceVerify, "off": InPlaceForceOff}[fmt.Sprint(val)]
			if !ok {
				return fmt.Errorf("option %q must be on, verify or off, got %v", key, val)
			}
			o.InPlace = mode
		default:
			return fmt.Errorf("unknown option %q (known: tape, steps, inplace)", key)
		}
	}
	return nil
}

func applyTapeOptions(o *Options, m map[string]any) error {
	for key, val := range m {
		switch key {
		case "initial":
			n, err := optInt(key, val)
			if err != nil {
				return err
			}
			o.Tape.InitialSize = n
		case "grows":
			n, err := optInt(key, val)
			if err != nil {
				return err
			}
			o.Tape.MaxGrows = n
		case "factor":
			f, err := optFloat(key, val)
			if err != nil {
				return err
			}
			o.Tape.GrowthFactor = f
		default:
			return fmt.Errorf("unknown tape option %q (known: initial, grows, factor)", key)
		}
	}
	return nil
}

// optInt coerces a jsonic number (int or integral float64) to int.
func optInt(key string, val any) (int, error) {
	switch n := val.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n == float64(int(n)) {
			return int(n), nil
		}
		return 0, fmt.Errorf("option %q must be an integer, got %v", key, n)
	default:
		return 0, fmt.Errorf("option %q must be an integer, got %T", key, val)
	}
}

// optFloat coerces a jsonic number (int or float64) to float64.
func optFloat(key string, val any) (float64, error) {
	switch n := val.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("option %q must be a number, got %T", key, val)
	}
}
