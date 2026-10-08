package tuikit

import (
	"strings"
	"testing"
)

// helpers ---------------------------------------------------------------

func w(kind string, kv ...any) map[string]any {
	m := map[string]any{"w": kind}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func mustRender(t *testing.T, tree any, cols, rows int) *RenderResult {
	t.Helper()
	res, err := Render(tree, cols, rows)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return res
}

func screenOf(res *RenderResult) []string { return renderRows(res.Frame) }

func renderErr(t *testing.T, tree any) string {
	t.Helper()
	_, err := Render(tree, 10, 4)
	if err == nil {
		t.Fatal("render succeeded, want error")
	}
	return err.Error()
}

// goldens ---------------------------------------------------------------

// The composite golden: the §5.2 app shape — a boxed focused input, a
// list with a cursor, a status line — pinned cell for cell.
func TestRenderCompositeGolden(t *testing.T) {
	tree := w("rows", "children", []any{
		w("box", "title", "new todo", "border", "round", "size", 3,
			"child", w("input", "value", "buy", "cursor", 3, "focus", true)),
		w("list-view", "items", []any{"[ ] buy milk", w("label", "w"), "x"}, "cursor", 1,
			"size", map[string]any{"flex": 1}),
		w("text", "text", "q quit", "size", 1, "style", map[string]any{"fg": "grey"}),
	})
	// the {label:…} item form needs a real label map, not a widget map
	tree["children"].([]any)[1].(map[string]any)["items"] = []any{
		"[ ] buy milk",
		map[string]any{"label": "[x] walk dog"},
	}
	res := mustRender(t, tree, 16, 6)
	rows := screenOf(res)
	want := []string{
		"╭ new todo ────╮",
		"│buy           │",
		"╰──────────────╯",
		"[ ] buy milk    ",
		"[x] walk dog    ",
		"q quit          ",
	}
	for i, line := range want {
		if rows[i] != line {
			t.Errorf("row %d = %q, want %q", i, rows[i], line)
		}
	}
	if res.Cursor == nil || res.Cursor.X != 4 || res.Cursor.Y != 1 {
		t.Errorf("cursor = %+v, want (4,1)", res.Cursor)
	}
	// cursor row reversed, status row grey, header bold-free
	if c, _ := res.Frame.At(0, 4); !c.Style.Reverse {
		t.Errorf("list cursor row not reversed: %+v", c.Style)
	}
	if c, _ := res.Frame.At(0, 5); c.Style.FG != "grey" {
		t.Errorf("status style = %+v", c.Style)
	}
}

// CJK + emoji widths: wide runes occupy two columns, stay contiguous in
// the projection, and clip cleanly at the edge.
func TestRenderWideRunesGolden(t *testing.T) {
	res := mustRender(t, w("text", "text", "漢字🙂ok"), 9, 1)
	if got := screenOf(res)[0]; got != "漢字🙂ok " {
		t.Errorf("wide row = %q", got)
	}
	// the continuation cell is marked, the glyph cell carries width 2
	if c, _ := res.Frame.At(0, 0); c.Content != "漢" || c.Width != 2 {
		t.Errorf("glyph cell = %+v", c)
	}
	if c, _ := res.Frame.At(1, 0); c.Width != -1 {
		t.Errorf("continuation cell = %+v", c)
	}
	// a wide rune that would split at the clip edge is dropped
	res = mustRender(t, w("text", "text", "ab漢"), 3, 1)
	if got := screenOf(res)[0]; got != "ab " {
		t.Errorf("clipped wide row = %q", got)
	}
	// combining mark joins its base cell; a leading combining mark is dropped
	res = mustRender(t, w("text", "text", "éx"), 4, 1)
	if c, _ := res.Frame.At(0, 0); c.Content != "é" {
		t.Errorf("combining join = %+v", c)
	}
	res = mustRender(t, w("text", "text", "́a"), 4, 1)
	if got := screenOf(res)[0]; got != "a   " {
		t.Errorf("leading combining = %q", got)
	}
}

func TestRenderTextWrap(t *testing.T) {
	res := mustRender(t, w("text", "text", "abcdef", "wrap", "wrap"), 3, 3)
	rows := screenOf(res)
	if rows[0] != "abc" || rows[1] != "def" || rows[2] != "   " {
		t.Errorf("wrap = %q", rows)
	}
	// a wide rune that can never fit breaks the wrap loop
	res = mustRender(t, w("text", "text", "漢", "wrap", "wrap"), 1, 2)
	if got := screenOf(res)[0]; got != " " {
		t.Errorf("unfittable wide = %q", got)
	}
}

func TestRenderStacksGapsAndSpecs(t *testing.T) {
	// cols: fixed 2 | pct 50 (of 10) | fit | flex remainder, gap 1
	tree := w("cols", "gap", 1, "children", []any{
		w("text", "text", "aa", "size", 2),
		w("text", "text", "bb", "size", map[string]any{"pct": 50}),
		w("text", "text", "cc", "size", "fit"),
		w("text", "text", "dd"),
	})
	res := mustRender(t, tree, 13, 1)
	// avail = 13 − 3 gaps = 10: fixed 2 + pct 5 + fit 2 + flex 1 ("dd"→"d")
	if got := screenOf(res)[0]; got != "aa bb    cc d" {
		t.Errorf("cols = %q", got)
	}
	// rows with a spacer pushing the last line down
	tree = w("rows", "children", []any{
		w("text", "text", "top", "size", 1),
		w("spacer"),
		w("text", "text", "bot", "size", 1),
	})
	res = mustRender(t, tree, 3, 4)
	rows := screenOf(res)
	if rows[0] != "top" || rows[3] != "bot" || rows[1] != "   " {
		t.Errorf("spacer rows = %q", rows)
	}
	// a zero-sized child paints nothing (the clipped-away arm)
	tree = w("rows", "children", []any{
		w("text", "text", "x", "size", 0),
		w("text", "text", "y"),
	})
	res = mustRender(t, tree, 1, 1)
	if got := screenOf(res)[0]; got != "y" {
		t.Errorf("zero-size child = %q", got)
	}
}

func TestRenderBoxVariants(t *testing.T) {
	res := mustRender(t, w("box", "border", "double",
		"child", w("text", "text", "x")), 4, 3)
	rows := screenOf(res)
	if rows[0] != "╔══╗" || rows[1] != "║x ║" || rows[2] != "╚══╝" {
		t.Errorf("double box = %q", rows)
	}
	// border none + pad
	res = mustRender(t, w("box", "border", "none", "pad", []any{1, 1},
		"child", w("text", "text", "x")), 3, 3)
	if got := screenOf(res)[1]; got != " x " {
		t.Errorf("padded borderless = %q", got)
	}
	// long titles truncate inside the top border
	res = mustRender(t, w("box", "title", "abcdefgh", "border", "line"), 7, 2)
	if got := screenOf(res)[0]; got != "┌ abc─┐" && !strings.HasPrefix(got, "┌ abc") {
		t.Errorf("truncated title = %q", got)
	}
	// too small for a border: nothing painted, no panic
	res = mustRender(t, w("box", "child", w("text", "text", "x")), 1, 1)
	if got := screenOf(res)[0]; got != " " {
		t.Errorf("tiny box = %q", got)
	}
	// childless box with border still draws the frame
	res = mustRender(t, w("box"), 3, 2)
	if got := screenOf(res)[0]; got != "┌─┐" {
		t.Errorf("childless box = %q", got)
	}
}

func TestRenderListScrollAndTable(t *testing.T) {
	items := []any{"a", "b", "c", "d", "e"}
	res := mustRender(t, w("list-view", "items", items, "cursor", 4), 3, 2)
	rows := screenOf(res)
	if rows[0] != "d  " || rows[1] != "e  " {
		t.Errorf("scrolled list = %q", rows)
	}
	if c, _ := res.Frame.At(0, 1); !c.Style.Reverse {
		t.Error("scrolled cursor row not reversed")
	}

	table := w("table",
		"columns", []any{
			map[string]any{"title": "id", "width": 3},
			map[string]any{"title": "done"},
		},
		"rows", []any{
			[]any{1, true},
			[]any{2, false, "extra-ignored"},
			[]any{"3"}, // shorter than the column set
		},
		"cursor", 1)
	res = mustRender(t, table, 12, 3)
	rows = screenOf(res)
	if rows[0] != "id  done    " {
		t.Errorf("header = %q", rows[0])
	}
	if rows[1] != "1   true    " {
		t.Errorf("row0 = %q", rows[1])
	}
	if rows[2] != "2   false   " {
		t.Errorf("row1 = %q", rows[2])
	}
	if c, _ := res.Frame.At(0, 0); !c.Style.Bold {
		t.Error("header not bold")
	}
	if c, _ := res.Frame.At(0, 2); !c.Style.Reverse {
		t.Error("table cursor row not reversed")
	}
	// cursor beyond the body window scrolls the data rows
	res = mustRender(t, table, 12, 2)
	if got := screenOf(res)[1]; !strings.HasPrefix(got, "2") {
		t.Errorf("scrolled table row = %q", got)
	}
	// non-integral float cells fall to the %v projection
	weird := w("table",
		"columns", []any{map[string]any{"title": "v", "width": 4}},
		"rows", []any{[]any{1.5}})
	res = mustRender(t, weird, 5, 2)
	if got := screenOf(res)[1]; !strings.HasPrefix(got, "1.5") {
		t.Errorf("float cell = %q", got)
	}
}

func TestRenderInputForms(t *testing.T) {
	// placeholder shows grey when the value is empty
	res := mustRender(t, w("input", "placeholder", "type here"), 10, 1)
	if got := screenOf(res)[0]; got != "type here " {
		t.Errorf("placeholder = %q", got)
	}
	if c, _ := res.Frame.At(0, 0); c.Style.FG != "grey" {
		t.Errorf("placeholder style = %+v", c.Style)
	}
	if res.Cursor != nil {
		t.Error("unfocused input placed a cursor")
	}
	// wide-rune value: the cursor index is a rune index, X advances by width
	res = mustRender(t, w("input", "value", "漢字ab", "cursor", 2, "focus", true), 10, 1)
	if res.Cursor == nil || res.Cursor.X != 4 {
		t.Errorf("wide cursor = %+v", res.Cursor)
	}
	// cursor past the rect clamps to the last column
	res = mustRender(t, w("input", "value", "abcdef", "cursor", 99, "focus", true), 4, 1)
	if res.Cursor == nil || res.Cursor.X != 3 {
		t.Errorf("clamped cursor = %+v", res.Cursor)
	}
	// two focused inputs: ambiguous, no cursor (§3.4)
	tree := w("rows", "children", []any{
		w("input", "value", "a", "focus", true, "size", 1),
		w("input", "value", "b", "focus", true, "size", 1),
	})
	res = mustRender(t, tree, 4, 2)
	if res.Cursor != nil {
		t.Error("two focused inputs still placed a cursor")
	}
}

// A masked input paints one narrow bullet per rune — the value itself
// never reaches the frame — and the cursor advances one cell per rune
// even for wide-rune values. The placeholder is a hint, not a secret,
// so it stays unmasked; a wrong-typed mask is declined in paint and in
// measure.
func TestRenderInputMask(t *testing.T) {
	res := mustRender(t, w("input", "value", "hunter2", "mask", true), 10, 1)
	if got := screenOf(res)[0]; got != "•••••••   " {
		t.Errorf("masked value = %q", got)
	}
	if got := screenOf(res)[0]; strings.Contains(got, "hunter") {
		t.Errorf("secret leaked into the frame: %q", got)
	}
	// wide-rune value: bullets are narrow, the cursor sits at the rune
	// index (unmasked, the same cursor lands at X=4 — see
	// TestRenderInputForms)
	res = mustRender(t, w("input", "value", "漢字ab", "cursor", 2, "focus", true, "mask", true), 10, 1)
	if got := screenOf(res)[0]; got != "••••      " {
		t.Errorf("masked wide value = %q", got)
	}
	if res.Cursor == nil || res.Cursor.X != 2 {
		t.Errorf("masked cursor = %+v", res.Cursor)
	}
	// an empty masked input still shows its (unmasked) placeholder
	res = mustRender(t, w("input", "value", "", "mask", true, "placeholder", "passphrase"), 12, 1)
	if got := screenOf(res)[0]; got != "passphrase  " {
		t.Errorf("masked placeholder = %q", got)
	}
	// wrong-typed mask: declined in paint and in the measure path
	if got := renderErr(t, w("input", "mask", 5)); !strings.Contains(got, "mask: must be a Boolean") {
		t.Errorf("paint mask error = %q", got)
	}
	if _, _, err := measureFit(w("input", "value", "x", "mask", 5), 80); err == nil ||
		!strings.Contains(err.Error(), "mask: must be a Boolean") {
		t.Errorf("measure mask error = %v", err)
	}
	// masked measure counts runes, not display width
	mw, mh, err := measureFit(w("input", "value", "漢字", "mask", true), 80)
	if err != nil || mw != 3 || mh != 1 {
		t.Errorf("masked measure = %d,%d,%v", mw, mh, err)
	}
	uw, _, err := measureFit(w("input", "value", "漢字"), 80)
	if err != nil || uw != 5 {
		t.Errorf("unmasked measure = %d,%v", uw, err)
	}
}

func TestRenderViewport(t *testing.T) {
	child := w("rows", "children", []any{
		w("text", "text", "row0", "size", 1),
		w("text", "text", "row1", "size", 1),
		w("text", "text", "row2", "size", 1),
	})
	res := mustRender(t, w("viewport", "child", child, "offset", []any{1, 1}), 3, 2)
	rows := screenOf(res)
	if rows[0] != "ow1" || rows[1] != "ow2" {
		t.Errorf("viewport window = %q", rows)
	}
	// no child: nothing to do
	res = mustRender(t, w("viewport"), 3, 2)
	if got := screenOf(res)[0]; got != "   " {
		t.Errorf("childless viewport = %q", got)
	}
	// an enormous fit clamps at the canvas caps instead of allocating away
	res = mustRender(t, w("viewport",
		"child", w("text", "text", strings.Repeat("x", 3000))), 4, 1)
	if got := screenOf(res)[0]; got != "xxxx" {
		t.Errorf("capped viewport = %q", got)
	}
	// a focused input inside a viewport must not leak a virtual cursor
	res = mustRender(t, w("viewport",
		"child", w("input", "value", "a", "focus", true)), 4, 1)
	if res.Cursor != nil {
		t.Error("viewport leaked a virtual cursor position")
	}
}

func TestRenderStyleInheritance(t *testing.T) {
	tree := w("rows", "style", map[string]any{"fg": "red", "bold": true},
		"children", []any{
			w("text", "text", "a", "size", 1),
			w("text", "text", "b", "size", 1, "style", map[string]any{"fg": "blue", "underline": true, "italic": true, "reverse": false}),
		})
	res := mustRender(t, tree, 1, 2)
	top, _ := res.Frame.At(0, 0)
	if top.Style.FG != "red" || !top.Style.Bold {
		t.Errorf("inherited = %+v", top.Style)
	}
	bot, _ := res.Frame.At(0, 1)
	if bot.Style.FG != "blue" || !bot.Style.Bold || !bot.Style.Underline || !bot.Style.Italic {
		t.Errorf("overridden = %+v", bot.Style)
	}
}

// measurement ------------------------------------------------------------

func TestMeasureFitShapes(t *testing.T) {
	cases := []struct {
		tree   any
		availW int
		wantW  int
		wantH  int
	}{
		{w("text", "text", "漢ab"), 80, 4, 1},
		{w("text", "text", "abcdef", "wrap", "wrap"), 3, 3, 2},
		{w("rows", "gap", 1, "children", []any{
			w("text", "text", "abc"), w("text", "text", "z"),
		}), 80, 3, 3},
		{w("cols", "children", []any{
			w("text", "text", "ab"), w("text", "text", "cd"),
		}), 80, 4, 1},
		{w("box", "child", w("text", "text", "xy"), "pad", []any{1, 0}), 80, 6, 3},
		{w("box", "border", "none"), 80, 0, 0},
		{w("list-view", "items", []any{"a", "long"}), 80, 4, 2},
		{w("table", "columns", []any{
			map[string]any{"title": "aa"}, map[string]any{"title": "b", "width": 2},
		}, "rows", []any{[]any{1, 2}}), 80, 7, 2},
		{w("input", "value", "ab"), 80, 3, 1},
		{w("viewport", "child", w("text", "text", "abc")), 80, 3, 1},
		{w("viewport"), 80, 0, 0},
		{w("spacer"), 80, 0, 0},
	}
	for i, c := range cases {
		gotW, gotH, err := measureFit(c.tree, c.availW)
		if err != nil || gotW != c.wantW || gotH != c.wantH {
			t.Errorf("case %d: fit = %d,%d,%v want %d,%d", i, gotW, gotH, err, c.wantW, c.wantH)
		}
	}
	// fit sizing drives measurement through the stack path
	tree := w("rows", "children", []any{
		w("box", "size", "fit", "child", w("text", "text", "hi")),
		w("text", "text", "rest"),
	})
	res := mustRender(t, tree, 6, 4)
	if got := screenOf(res)[3]; got != "rest  " {
		t.Errorf("fit stack = %q", screenOf(res))
	}
}

// error matrix ------------------------------------------------------------

func TestRenderErrorMatrix(t *testing.T) {
	cases := []struct {
		name string
		tree any
		want string
	}{
		{"non-map root", 5, "must be a Map"},
		{"missing kind", map[string]any{}, "needs a w: kind"},
		{"wrong-typed kind", map[string]any{"w": 5}, "needs a w: kind"},
		{"unknown kind", w("blink"), "unknown kind"},
		{"style non-map", w("text", "text", "x", "style", 5), "style: must be a Map"},
		{"style fg", w("text", "text", "x", "style", map[string]any{"fg": 5}), "fg: must be a String"},
		{"style bold", w("text", "text", "x", "style", map[string]any{"bold": "y"}), "must be a Boolean"},
		{"text non-string", w("text", "text", 5), "text: must be a String"},
		{"wrap non-string", w("text", "text", "x", "wrap", 5), "wrap: must be a String"},
		{"children non-list", w("rows", "children", 5), "children: must be a List"},
		{"gap non-int", w("rows", "gap", "x", "children", []any{}), "gap: must be an Integer"},
		{"child non-map", w("rows", "children", []any{5}), "must be a Map"},
		{"child size bad", w("rows", "children", []any{w("text", "text", "x", "size", true)}), "size: must be"},
		{"child paint error", w("rows", "children", []any{w("nope")}), "unknown kind"},
		{"fit measure error", w("rows", "children", []any{
			map[string]any{"w": "text", "text": 5, "size": "fit"},
		}), "text: must be a String"},
		{"cols fit measure error", w("cols", "children", []any{
			map[string]any{"w": "text", "text": 5, "size": "fit"},
		}), "text: must be a String"},
		{"box border type", w("box", "border", 5), "border: must be a String"},
		{"box border unknown", w("box", "border", "dotted"), "unknown kind"},
		{"box pad type", w("box", "pad", 5), "pad: must be a List"},
		{"box pad arity", w("box", "pad", []any{1}), "pad: must be [x y]"},
		{"box pad negative", w("box", "pad", []any{-1, 0}), "non-negative"},
		{"box title type", w("box", "title", 5), "title: must be a String"},
		{"box child error", w("box", "child", w("nope")), "unknown kind"},
		{"list items type", w("list-view", "items", 5), "items: must be a List"},
		{"list item shape", w("list-view", "items", []any{5}), "items must be Strings"},
		{"list item map no label", w("list-view", "items", []any{map[string]any{"x": 1}}), "items must be Strings"},
		{"list cursor type", w("list-view", "items", []any{}, "cursor", "x"), "cursor: must be an Integer"},
		{"table columns type", w("table", "columns", 5), "columns: must be a List"},
		{"table column shape", w("table", "columns", []any{5}), "columns must be"},
		{"table column title", w("table", "columns", []any{map[string]any{"title": 5}}), "title: must be a String"},
		{"table column width", w("table", "columns", []any{map[string]any{"width": "x"}}), "width: must be an Integer"},
		{"table rows type", w("table", "columns", []any{}, "rows", 5), "rows: must be a List"},
		{"table row shape", w("table", "columns", []any{}, "rows", []any{5}), "rows must be Lists"},
		{"table cursor type", w("table", "columns", []any{}, "rows", []any{}, "cursor", "x"), "cursor: must be an Integer"},
		{"input value type", w("input", "value", 5), "value: must be a String"},
		{"input placeholder type", w("input", "placeholder", 5), "placeholder: must be a String"},
		{"input cursor type", w("input", "cursor", "x"), "cursor: must be an Integer"},
		{"input focus type", w("input", "focus", "x"), "focus: must be a Boolean"},
		{"viewport offset type", w("viewport", "child", w("spacer"), "offset", 5), "offset: must be a List"},
		{"viewport offset arity", w("viewport", "child", w("spacer"), "offset", []any{1}), "offset: must be [x y]"},
		{"viewport offset negative", w("viewport", "child", w("spacer"), "offset", []any{0, -1}), "non-negative"},
		{"viewport child measure", w("viewport", "child", map[string]any{"w": "text", "text": 5}), "text: must be a String"},
		{"viewport child paint", w("viewport", "child", w("rows", "children", []any{5})), "must be a Map"},
	}
	for _, c := range cases {
		if got := renderErr(t, c.tree); !strings.Contains(got, c.want) {
			t.Errorf("%s: error = %q, want substring %q", c.name, got, c.want)
		}
	}
}

// itemLabel / scrollOffset / cellText direct arms -------------------------

func TestRenderSmallHelpers(t *testing.T) {
	if got, err := itemLabel("s"); got != "s" || err != nil {
		t.Error("string item")
	}
	if got, err := itemLabel(map[string]any{"label": "l"}); got != "l" || err != nil {
		t.Error("label item")
	}
	if off := scrollOffset(0, 5, 0); off != 0 {
		t.Errorf("h=0 offset = %d", off)
	}
	if off := scrollOffset(9, 10, 3); off != 7 {
		t.Errorf("deep offset = %d", off)
	}
	if off := scrollOffset(9, 8, 3); off != 5 {
		t.Errorf("clamped offset = %d", off)
	}
	if got := cellText(nil); got != "<nil>" {
		t.Errorf("nil cell = %q", got)
	}
}

// An out-of-range cursor over a short list must clamp, not panic
// (review finding: cursor 10 over one item in a 10-row window drove
// the scroll offset negative and indexed below zero).
func TestScrollCursorBeyondShortList(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("render panicked: %v", rec)
		}
	}()
	for _, tree := range []map[string]any{
		{"w": "list-view", "items": []any{"only"}, "cursor": 10},
		{"w": "table", "columns": []any{map[string]any{"title": "c"}},
			"rows": []any{[]any{"only"}}, "cursor": 10},
	} {
		res, err := Render(tree, 20, 10)
		if err != nil {
			t.Fatalf("%v render = %v", tree["w"], err)
		}
		joined := ""
		for y := 0; y < res.Frame.Rows; y++ {
			for x := 0; x < res.Frame.Cols; x++ {
				if c, ok := res.Frame.At(x, y); ok {
					joined += c.Content
				}
			}
		}
		if !strings.Contains(joined, "onl") { // table columns may truncate
			t.Fatalf("%v clamped frame lost its row: %q", tree["w"], joined)
		}
	}
	// the pure helper's clamp arms
	if got := scrollOffset(10, 1, 10); got != 0 {
		t.Fatalf("scrollOffset(10,1,10) = %d, want 0", got)
	}
	if got := scrollOffset(5, 100, 3); got != 3 {
		t.Fatalf("scrollOffset(5,100,3) = %d, want 3", got)
	}
}
