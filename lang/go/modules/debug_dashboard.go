package modules

import (
	"fmt"
	"strings"

	"github.com/boru-lang/boru/lang/go/native"
)

// This file implements the in-process, headless core of the live-dashboard
// surface (design/DEBUG-MODULE.0.md §7.1, Phase 5): the extensible widget
// contract and a one-shot snapshot render. A widget is a plain Map
//
//	{title: String  sample: String}
//
// where `sample` is boru source that produces the value to display. Using a
// String source (rather than a quoted code body) sidesteps map-literal
// auto-evaluation, so widgets compose cleanly as ordinary data — and any
// module can contribute widgets simply by exporting a function that returns
// a list of these maps (`Debug.dashboard SomeModule.widgets`).
//
// `Debug.dashboard [widgets]` renders a SNAPSHOT of every widget once, to
// the host output, and returns the rendered String. The live-refreshing TUI
// host (bubbletea, in cmd/go) that polls these widgets on a tick is the
// follow-on; the data contract here is what it renders.

// dashboardNatives returns the dashboard primitives, appended into
// debugNatives()'s slice (see debug.go).
func dashboardNatives() []native.NativeFunc {
	return []native.NativeFunc{
		{
			// Render a one-shot snapshot of a list of widget maps.
			Name: "debug-dashboard",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TString},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					widgets, err := native.RequireConcreteList(args[0], "Debug.dashboard")
					if err != nil {
						return nil, err
					}
					rendered := renderDashboard(r, widgets.Slice())
					fmt.Fprint(r.Output, rendered)
					return []native.Value{native.NewString(strings.TrimRight(rendered, "\n"))}, nil
				}),
			}},
		},
		{
			// Construct a widget map from a title and a sample source string.
			Name: "debug-widget",
			Signatures: []native.Signature{{
				// Canonical forward form: `Debug.widget TITLE SAMPLE-SOURCE`.
				Args:       []*native.Type{native.TString, native.TString},
				Returns:    []*native.Type{native.TMap},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					title, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					sample, err := args[1].AsConcreteString()
					if err != nil {
						return nil, err
					}
					om := native.NewOrderedMap()
					om.Set("title", native.NewString(title))
					om.Set("sample", native.NewString(sample))
					return []native.Value{native.NewMap(om)}, nil
				}),
			}},
		},
	}
}

// renderDashboard renders every widget once into a single string. Each
// widget's `sample` source is parsed and run in a fresh sub-engine; its
// top-of-stack result is shown under the widget's title. A widget that
// errors or is malformed renders an inline error line rather than aborting
// the whole snapshot — a dashboard must survive one bad panel.
func renderDashboard(parent *native.Registry, widgets []native.Value) string {
	var b strings.Builder
	for _, w := range widgets {
		title, sample := widgetFields(w)
		fmt.Fprintf(&b, "── %s ──\n", title)
		b.WriteString(sampleResult(parent, sample))
		b.WriteString("\n")
	}
	return b.String()
}

// widgetFields extracts (title, sample) from a widget map, with safe
// fallbacks for a missing/odd shape.
func widgetFields(w native.Value) (title, sample string) {
	title, sample = "(untitled)", ""
	m, err := native.AsMap(w)
	if err != nil || m == nil {
		return title, sample
	}
	if tv, ok := m.Get("title"); ok {
		if s, serr := tv.AsConcreteString(); serr == nil {
			title = s
		}
	}
	if sv, ok := m.Get("sample"); ok {
		if s, serr := sv.AsConcreteString(); serr == nil {
			sample = s
		}
	}
	return title, sample
}

// sampleResult parses and runs a widget's sample source in a sub-engine and
// renders its top-of-stack result. Failures render as a one-line message so
// one broken widget never takes down the snapshot.
func sampleResult(parent *native.Registry, sample string) string {
	if strings.TrimSpace(sample) == "" {
		return "(empty)"
	}
	if parent.ParseFunc == nil {
		return "(no parser)"
	}
	tokens, perr := parent.ParseFunc(sample)
	if perr != nil {
		return fmt.Sprintf("(parse error: %v)", perr)
	}
	res, rerr := native.New(parent).Run(tokens)
	if rerr != nil {
		return fmt.Sprintf("(error: %v)", rerr)
	}
	if len(res) == 0 {
		return native.FormatForPrint(native.NewNone())
	}
	return native.FormatForPrint(res[len(res)-1])
}
