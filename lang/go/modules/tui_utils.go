package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// The general terminal utilities of design/legacy/TUI-UTILITIES.0.ignore: pure
// words on the module's no-backend tier, exposing the SAME color model,
// degradation tables, and width tables the framework renders with — to
// plain CLI scripts that never open the terminal. No policy scope, no
// goroutines, no state.

func tuiColorizeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	st, err := styleFromMap(args[0], "colorize", r)
	if err != nil {
		return nil, err
	}
	text, tErr := args[1].AsConcreteString()
	if tErr != nil {
		return nil, r.BoruError("tui_error", "colorize: expected the text as a String", "colorize")
	}
	profileName := ""
	if len(args) > 2 {
		opts, oErr := native.RequireConcreteMap(args[2], "colorize")
		if oErr != nil {
			return nil, r.BoruError("tui_error", "colorize: options must be a Map", "colorize")
		}
		if profileName, oErr = tuiOptString(opts, "profile"); oErr != nil {
			return nil, r.BoruError("tui_error", "colorize: "+oErr.Error(), "colorize")
		}
	}
	profile, ok := tuikit.ParseProfile(profileName)
	if !ok {
		return nil, r.BoruError("tui_error", `colorize: profile: must be "truecolor", "256", "16" or "none"`, "colorize")
	}
	seq := tuikit.SGR(st, profile)
	if seq == "" {
		// the attribute-less style is the identity: no stray escapes
		return []native.Value{native.NewString(text)}, nil
	}
	return []native.Value{native.NewString(seq + text + tuikit.SGRReset)}, nil
}

func tuiStripAnsiHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("tui_error", "strip-ansi: expected a String", "strip-ansi")
	}
	return []native.Value{native.NewString(tuikit.StripANSI(s))}, nil
}

func tuiTextWidthHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("tui_error", "text-width: expected a String", "text-width")
	}
	// DISPLAY width: escapes occupy no cells, so strip before counting
	return []native.Value{native.NewInteger(int64(tuikit.StringWidth(tuikit.StripANSI(s))))}, nil
}

// tuiUtilNatives lists the standalone terminal utilities.
func tuiUtilNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return []native.NativeFunc{
		{Name: "colorize", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TString, native.TMap), Impl: native.Go(tuiColorizeHandler),
				Returns: T(native.TString), BarrierPos: -1},
			{Args: T(native.TMap, native.TString), Impl: native.Go(tuiColorizeHandler),
				Returns: T(native.TString), BarrierPos: -1},
		}},
		{Name: "strip-ansi", Signatures: []native.Signature{
			{Args: T(native.TString), Impl: native.Go(tuiStripAnsiHandler),
				Returns: T(native.TString), BarrierPos: -1},
		}},
		{Name: "text-width", Signatures: []native.Signature{
			{Args: T(native.TString), Impl: native.Go(tuiTextWidthHandler),
				Returns: T(native.TInteger), BarrierPos: -1},
		}},
	}
}
