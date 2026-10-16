//go:build js && wasm

// Command wpg/wasm compiles the boru engine to WebAssembly.
// Build (from the wpg directory): GOOS=js GOARCH=wasm go build -o boru.wasm ./wasm
package main

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"syscall/js"

	"github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/formatter"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

func main() {
	instance, err := lang.New()
	if err != nil {
		js.Global().Get("console").Call("error", "boru init failed: "+err.Error())
		return
	}
	instance.SetFileOps(lang.NewMemFileOps())

	// boru:tui renders into the page (tuiweb.go): the playground IS a
	// terminal-shaped surface, so full-screen apps run in the browser.
	if err := modules.RegisterHostTui(instance.NativeRegistry(), modules.TuiSpec{
		Name: "web",
		Open: func() (tuikit.Backend, error) { return newWebBackend(), nil },
	}); err != nil {
		js.Global().Get("console").Call("error", "boru tui backend: "+err.Error())
	}
	installTuiInput()

	var evalMu sync.Mutex
	var outBuf bytes.Buffer
	instance.SetOutput(&outBuf)

	runCode := func(code string) map[string]any {
		evalMu.Lock()
		defer evalMu.Unlock()
		outBuf.Reset()
		result, err := instance.Run(code)
		printed := outBuf.String()
		if err != nil {
			return map[string]any{"error": err.Error(), "output": printed}
		}
		parts := make([]string, len(result))
		for i, v := range result {
			parts[i] = fmt.Sprintf("%v", v)
		}
		return map[string]any{"result": strings.Join(parts, " "), "output": printed}
	}

	js.Global().Set("boruEval", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return map[string]any{"error": "missing code argument"}
		}
		return runCode(args[0].String())
	}))

	// boruEvalAsync(code, callback) runs on a goroutine so a BLOCKING
	// program — a Tui.run session — leaves the browser event loop free
	// to deliver key events; the callback receives the same result map.
	js.Global().Set("boruEvalAsync", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return map[string]any{"error": "missing code or callback argument"}
		}
		code := args[0].String()
		cb := args[1]
		go func() {
			cb.Invoke(js.ValueOf(runCode(code)))
		}()
		return nil
	}))

	js.Global().Set("boruFmt", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return ""
		}
		return formatter.Format(args[0].String())
	}))

	// Signal that the WASM module is ready.
	if cb := js.Global().Get("__boruReady"); !cb.IsUndefined() && !cb.IsNull() {
		cb.Invoke()
	}

	// Block forever so the Go runtime stays alive.
	select {}
}
