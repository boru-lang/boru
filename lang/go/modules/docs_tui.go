package modules

func init() {
	registerDocs("boru:tui", map[string]string{
		"open":           "Take the terminal over (raw mode + alt-screen) via the registered backend, returning a Terminal handle.",
		"close":          "Restore the terminal and release the handle; idempotent.",
		"dims":           "The terminal's current dimensions, as {cols rows}.",
		"read-event":     "The next decoded input event as a tagged map; with {within: ms}, None on deadline.",
		"deliver-events": "Deliver decoded input events to a process mailbox instead of pulling: the Tier-1 active mode (one delivery per terminal; read-event refuses while it owns the stream).",
		"print-at":       "Write styled text into the offscreen grid at cell (x, y), clipping at the edges.",
		"clear":          "Clear the offscreen grid.",
		"show":           "Present the offscreen grid to the terminal (the backend diffs).",
		"title":          "Set the terminal window title.",
		"bell":           "Ring the terminal bell.",
		"run":            "Run an app map {init update view}: events fold through update, the pure view renders, run blocks until quit and returns the final state.",
		"serve":          "Serve an app map to remote viewers over {tcp token viewers reattach}: trees down, events up (attach with `boru attach`); up to viewers concurrent viewers (default 1), reattach keeps the app alive across viewer loss; returns the final state.",
		"quit":           "Wrap the final state in the quit marker an update returns to end the app.",
		"text":           "Build a text widget: a styled leaf line (wrap: \"wrap\" for multi-line).",
		"rows":           "Build a vertical stack of child widgets.",
		"cols":           "Build a horizontal stack of child widgets.",
		"box":            "Build a bordered box around a child widget (title, border, pad).",
		"list-view":      "Build a scrolling list with a highlighted cursor row.",
		"table":          "Build a table from column specs and data rows, with a cursor row.",
		"input":          "Build a single-line input widget rendering state-owned text; {mask: true} paints one bullet per rune (passphrases).",
		"viewport":       "Build a scroll window over a child widget at an offset.",
		"spacer":         "Build a flexible filler widget.",
		"style":          "Merge a style-override map over a base style map.",
		"edit":           "Fold one key event into an input widget: the standard line-editing moves.",
		"focusable":      "The id-carrying widgets of a tree, in document order.",
		"colorize":       "Wrap text in a style map's ANSI sequence plus a reset; {profile: \"256\"|\"16\"|\"none\"} degrades like the renderer. Pure — no terminal needed.",
		"strip-ansi":     "Remove ANSI escape sequences from a string (round-trips colorize).",
		"text-width":     "The display width of text in terminal cells: escapes stripped, wide runes 2, combining marks 0.",
	})

	// Every widget word returns plain DATA — a map with a `w:` tag — which
	// is the thing to understand about this module and which a return type
	// of Map does not convey: a view is a value you build, nest and
	// compare, not a side effect. Results are from verified
	// lang/spec/module-tui.tsv rows.
	registerExamples("boru:tui", map[string][]string{
		"text": {
			`Tui.text "hi"                                    ;# {w:'text' text:'hi' wrap:'truncate'}`,
			`Tui.text "hi" {size: 1}                          ;# widgets are DATA, not draw calls`,
		},
		"rows":      {`Tui.rows [(Tui.text "a") (Tui.text "b")] {gap: 2}   ;# vertical stack`},
		"cols":      {`Tui.cols [(Tui.text "a") (Tui.text "b")]            ;# horizontal stack`},
		"box":       {`Tui.box (Tui.spacer) {title: "t"}                ;# {w:'box' … title:'t' border:'line'}`},
		"spacer":    {`Tui.spacer                                       ;# {w:'spacer'} — flexible empty space`},
		"list-view": {`Tui.list-view ["a" "b"] {cursor: 0}              ;# a selectable list`},
		"table":     {`Tui.table [{title: "c"}] [[1]]                   ;# columns, then rows`},
		"input":     {`Tui.input "ab"                                   ;# {w:'input' value:'ab' cursor:2 …}`},
		"viewport":  {`Tui.viewport (Tui.spacer)                        ;# a scrollable window over a child`},
		"style":     {`Tui.style {fg: "red"} {fg: "blue" bold: true}    ;# {fg:'blue' bold:true} — later wins`},
		"run": {
			`Tui.run {                                        ;# the model/update/view loop`,
			`  init:   {count: 0}`,
			`  update: ([m:Map e:Map] => [ … ])`,
			`  view:   ([m:Map] => [(Tui.text (convert String m.count))])`,
			`}`,
		},
		"serve":      {`Tui.serve app {port: 8080}                       ;# the same app over the wire`},
		"open":       {`def t (Tui.open)                                 ;# raw terminal control; Tui.close t when done`},
		"colorize":   {`Tui.colorize "red" "danger"                      ;# ANSI-wrapped text`},
		"strip-ansi": {`Tui.strip-ansi coloured                          ;# the plain text back`},
		"text-width": {`Tui.text-width "日本"                            ;# 4 — display columns, not bytes`},
	})
}
