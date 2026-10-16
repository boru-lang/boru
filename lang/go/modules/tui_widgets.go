package modules

import (
	"unicode"

	"github.com/boru-lang/boru/lang/go/native"
)

// The Tier-2 pure words of design/TUI.0.md §3: nine widget constructors
// (data in, data out — every one TSV-testable by equality), the style
// merge, the input edit fold, the focusable walk, and the quit marker.
// Nothing here touches a terminal; the trees they build are plain
// sendable data the P2 renderer consumes.

// buildWidget assembles a widget map: the canonical fields in canonical
// order, then any options overlaid (known keys replace in place, extra
// keys — size/style/id/… — append).
func buildWidget(kind string, fields []string, values []native.Value, opts native.Value, word string, r *native.Registry) ([]native.Value, error) {
	m := native.NewOrderedMap()
	m.Set("w", native.NewString(kind))
	for i, f := range fields {
		m.Set(f, values[i])
	}
	if native.DataOf(&opts) != nil {
		om, err := native.RequireConcreteMap(opts, word)
		if err != nil {
			return nil, r.BoruError("tui_error", word+": options must be a Map", word)
		}
		for _, k := range om.Keys() {
			if k == "w" {
				return nil, r.BoruError("tui_error",
					word+": w is reserved (the constructor sets the widget kind)", word)
			}
			v, _ := om.Get(k)
			m.Set(k, v)
		}
	}
	return []native.Value{native.NewMap(m)}, nil
}

// optsArg returns the optional trailing options map (a zero Value when
// the short arity was used).
func optsArg(args []native.Value, at int) native.Value {
	if len(args) > at {
		return args[at]
	}
	return native.Value{}
}

func tuiTextHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("tui_error", "text: expected a String", "text")
	}
	return buildWidget("text", []string{"text", "wrap"},
		[]native.Value{native.NewString(s), native.NewString("truncate")},
		optsArg(args, 1), "text", r)
}

func tuiRowsHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteList(args[0], "rows"); err != nil {
		return nil, r.BoruError("tui_error", "rows: children must be a List", "rows")
	}
	return buildWidget("rows", []string{"children", "gap"},
		[]native.Value{args[0], native.NewInteger(0)},
		optsArg(args, 1), "rows", r)
}

func tuiColsHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteList(args[0], "cols"); err != nil {
		return nil, r.BoruError("tui_error", "cols: children must be a List", "cols")
	}
	return buildWidget("cols", []string{"children", "gap"},
		[]native.Value{args[0], native.NewInteger(0)},
		optsArg(args, 1), "cols", r)
}

func tuiBoxHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteMap(args[0], "box"); err != nil {
		return nil, r.BoruError("tui_error", "box: child must be a widget Map", "box")
	}
	return buildWidget("box", []string{"child", "title", "border"},
		[]native.Value{args[0], native.NewString(""), native.NewString("line")},
		optsArg(args, 1), "box", r)
}

func tuiListViewHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteList(args[0], "list-view"); err != nil {
		return nil, r.BoruError("tui_error", "list-view: items must be a List", "list-view")
	}
	return buildWidget("list-view", []string{"items", "cursor"},
		[]native.Value{args[0], native.NewInteger(0)},
		optsArg(args, 1), "list-view", r)
}

func tuiTableHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteList(args[0], "table"); err != nil {
		return nil, r.BoruError("tui_error", "table: columns must be a List", "table")
	}
	if _, err := native.RequireConcreteList(args[1], "table"); err != nil {
		return nil, r.BoruError("tui_error", "table: rows must be a List", "table")
	}
	return buildWidget("table", []string{"columns", "rows", "cursor"},
		[]native.Value{args[0], args[1], native.NewInteger(0)},
		optsArg(args, 2), "table", r)
}

func tuiInputHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("tui_error", "input: expected a String value", "input")
	}
	return buildWidget("input", []string{"value", "cursor", "placeholder", "focus"},
		[]native.Value{native.NewString(s), native.NewInteger(int64(len([]rune(s)))),
			native.NewString(""), native.NewBoolean(false)},
		optsArg(args, 1), "input", r)
}

func tuiViewportHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteMap(args[0], "viewport"); err != nil {
		return nil, r.BoruError("tui_error", "viewport: child must be a widget Map", "viewport")
	}
	offset := native.NewList([]native.Value{native.NewInteger(0), native.NewInteger(0)})
	return buildWidget("viewport", []string{"child", "offset"},
		[]native.Value{args[0], offset},
		optsArg(args, 1), "viewport", r)
}

func tuiSpacerHandler(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	m := native.NewOrderedMap()
	m.Set("w", native.NewString("spacer"))
	return []native.Value{native.NewMap(m)}, nil
}

// tuiStyleHandler merges overrides over a base style map (§3.3 sugar).
func tuiStyleHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	base, err := native.RequireConcreteMap(args[0], "style")
	if err != nil {
		return nil, r.BoruError("tui_error", "style: base must be a Map", "style")
	}
	over, err := native.RequireConcreteMap(args[1], "style")
	if err != nil {
		return nil, r.BoruError("tui_error", "style: overrides must be a Map", "style")
	}
	m := native.NewOrderedMap()
	for _, k := range base.Keys() {
		v, _ := base.Get(k)
		m.Set(k, v)
	}
	for _, k := range over.Keys() {
		v, _ := over.Get(k)
		m.Set(k, v)
	}
	return []native.Value{native.NewMap(m)}, nil
}

// tuiQuitHandler wraps the final state in the quit marker (§2.5): a
// plain map keyed with the @-metadata convention.
func tuiQuitHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	m := native.NewOrderedMap()
	m.Set("@quit", args[0])
	return []native.Value{native.NewMap(m)}, nil
}

// isQuitMarker unwraps a quit marker's payload.
func isQuitMarker(v native.Value) (native.Value, bool) {
	mp, _ := native.AsMap(v)
	if mp == nil {
		return native.Value{}, false
	}
	payload, ok := mp.Get("@quit")
	return payload, ok
}

// ---- the edit fold (§3.4) ----

// tuiEditHandler folds one key event into an input widget map: the
// standard line-editing moves, value and cursor both rune-indexed.
func tuiEditHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	in, err := native.RequireConcreteMap(args[0], "edit")
	if err != nil {
		return nil, r.BoruError("tui_error", "edit: expected an input widget Map", "edit")
	}
	ev, err := native.RequireConcreteMap(args[1], "edit")
	if err != nil {
		return nil, r.BoruError("tui_error", "edit: expected a key event Map", "edit")
	}
	value := ""
	if v, ok := in.Get("value"); ok {
		if s, sErr := v.AsConcreteString(); sErr == nil {
			value = s
		}
	}
	runes := []rune(value)
	cursor := len(runes)
	if v, ok := in.Get("cursor"); ok {
		if n, nErr := v.AsConcreteInteger(); nErr == nil {
			cursor = int(n)
		}
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	key := ""
	if v, ok := ev.Get("key"); ok {
		key, _ = v.AsConcreteString()
	}
	char := ""
	if v, ok := ev.Get("char"); ok {
		char, _ = v.AsConcreteString()
	}
	ctrl := false
	if v, ok := ev.Get("mods"); ok {
		if l, lErr := native.RequireConcreteList(v, "edit"); lErr == nil {
			for i := 0; i < l.Len(); i++ {
				el := l.Get(i)
				if s, sErr := el.AsConcreteString(); sErr == nil && (s == "ctrl" || s == "alt") {
					ctrl = true
				}
			}
		}
	}

	switch {
	case key == "left":
		if cursor > 0 {
			cursor--
		}
	case key == "right":
		if cursor < len(runes) {
			cursor++
		}
	case key == "home":
		cursor = 0
	case key == "end":
		cursor = len(runes)
	case key == "backspace":
		if cursor > 0 {
			runes = append(runes[:cursor-1], runes[cursor:]...)
			cursor--
		}
	case key == "delete":
		if cursor < len(runes) {
			runes = append(runes[:cursor], runes[cursor+1:]...)
		}
	case char != "" && !ctrl && printableChar(char):
		ins := []rune(char)
		tail := append([]rune{}, runes[cursor:]...)
		runes = append(append(runes[:cursor], ins...), tail...)
		cursor += len(ins)
	default:
		// navigation/function keys and chords leave the input unchanged
	}

	out := native.NewOrderedMap()
	for _, k := range in.Keys() {
		v, _ := in.Get(k)
		out.Set(k, v)
	}
	out.Set("value", native.NewString(string(runes)))
	out.Set("cursor", native.NewInteger(int64(cursor)))
	return []native.Value{native.NewMap(out)}, nil
}

func printableChar(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// ---- the focusable walk (§3.4) ----

func tuiFocusableHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if _, err := native.RequireConcreteMap(args[0], "focusable"); err != nil {
		return nil, r.BoruError("tui_error", "focusable: expected a widget tree Map", "focusable")
	}
	var ids []native.Value
	collectFocusable(args[0], &ids)
	return []native.Value{native.NewList(ids)}, nil
}

func collectFocusable(v native.Value, ids *[]native.Value) {
	mp, _ := native.AsMap(v)
	if mp == nil {
		return
	}
	if idv, ok := mp.Get("id"); ok {
		if s, err := idv.AsConcreteString(); err == nil {
			*ids = append(*ids, native.NewString(s))
		}
	}
	if child, ok := mp.Get("child"); ok {
		collectFocusable(child, ids)
	}
	if children, ok := mp.Get("children"); ok {
		if l, err := native.RequireConcreteList(children, "focusable"); err == nil {
			for i := 0; i < l.Len(); i++ {
				collectFocusable(l.Get(i), ids)
			}
		}
	}
}

// tuiWidgetNatives lists the Tier-2 pure words.
func tuiWidgetNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	two := func(name string, h native.Handler, first *native.Type) native.NativeFunc {
		return native.NativeFunc{Name: name, Signatures: []native.Signature{
			{Args: T(first, native.TMap), Impl: native.Go(h), Returns: T(native.TMap), BarrierPos: -1},
			{Args: T(first), Impl: native.Go(h), Returns: T(native.TMap), BarrierPos: -1},
		}}
	}
	return []native.NativeFunc{
		two("text", tuiTextHandler, native.TString),
		two("rows", tuiRowsHandler, native.TList),
		two("cols", tuiColsHandler, native.TList),
		two("box", tuiBoxHandler, native.TMap),
		two("list-view", tuiListViewHandler, native.TList),
		two("input", tuiInputHandler, native.TString),
		two("viewport", tuiViewportHandler, native.TMap),
		{Name: "table", Signatures: []native.Signature{
			{Args: T(native.TList, native.TList, native.TMap), Impl: native.Go(tuiTableHandler), Returns: T(native.TMap), BarrierPos: -1},
			{Args: T(native.TList, native.TList), Impl: native.Go(tuiTableHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
		{Name: "spacer", Signatures: []native.Signature{
			{Args: T(), Impl: native.Go(tuiSpacerHandler), Returns: T(native.TMap), BarrierPos: 0},
		}},
		{Name: "style", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TMap), Impl: native.Go(tuiStyleHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
		{Name: "edit", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TMap), Impl: native.Go(tuiEditHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
		{Name: "focusable", Signatures: []native.Signature{
			{Args: T(native.TMap), Impl: native.Go(tuiFocusableHandler), Returns: T(native.TList), BarrierPos: -1},
		}},
		{Name: "quit", Signatures: []native.Signature{
			{Args: T(native.TAny), Impl: native.Go(tuiQuitHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
	}
}
