package native

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// Special path constants for stdio streams.
const (
	pathStdin  = "<stdin>"
	pathStdout = "<stdout>"
	pathStderr = "<stderr>"
)

// fileOpError codes a FileOps failure, preferring the code the failure
// ALREADY identifies itself by.
//
// A policy compile failure is not a read failure: the remedy is to change the
// policy, not the path, and the code is the only part of an Error a
// `case` arm can dispatch on. policy.Denied has carried that code
// (`permission_denied`, `capability_not_installed`, …) all along, and
// `lang/go/policy/error.go` says an engine adapter copies it onto the
// produced BoruError — this was the fileops half of that adapter, written
// first because REFERENCE.md's codes table is mostly about fileops. The
// general form now lives in policy_error.go and covers network, process,
// vault and terminal too; this function is the one site that wants a
// different DETAIL, so it keeps its own shape and shares the classifier.
//
// def is the word's own code (`read_error` / `write_error`), used when the
// failure is an ordinary I/O error with nothing more specific to say.
//
// The uninstalled-capability arm stays local rather than moving to the
// shared classifier because it is a different error TYPE: fileops signals
// an uninstalled capability with `notInstalledError`, not with a
// `*policy.Denied`, since the FileOps wrapper is swapped out wholesale
// rather than consulted and declined.
//
// The two answers are kept apart on purpose: a rule said no (widen the
// rule) versus the capability isn't there at all (install it).
func fileOpError(r *Registry, def, word, detail string, err error, pos SrcPos) error {
	var missing *notInstalledError
	if errors.As(err, &missing) {
		return r.BoruErrorAt("capability_not_installed", detail, word, pos)
	}
	// The shared adapter (policy_error.go), so fileops and every other gated
	// capability answer a compile failure with the same code. This site passes its OWN
	// detail: a file op has already built a "read: …" / "write: …" message
	// naming the path, which is more use to the reader than the blame trail.
	//
	// A policy compile failure keeps the adapter's UNPOSITIONED error: the adapter is
	// shared by every gated capability, and threading a position through it is
	// the rest of the positionless-error work (design note: the boru:io pilot
	// covers this word's OWN failures, not the policy layer's). The two arms
	// either side of it do carry the call's position.
	if coded, ok := policyRefusalCoded(r, word, detail, err); ok {
		return coded
	}
	return r.BoruErrorAt(def, detail, word, pos)
}

// DefaultExtensions returns the built-in file-extension→format-name map
// (lowercase keys, no leading dot). The mapping is many-to-one — several
// extensions may share one format (cfg/conf/ini→ini, yml/yaml→yaml) — and
// host-overridable via SetHostExtensions / RegisterFormat. An extension
// absent from the map decodes as plain text.
func DefaultExtensions() map[string]string {
	return map[string]string{
		"csv":      "csv",
		"tsv":      "tsv",
		"json":     "json",
		"jsonic":   "jsonic",
		"txt":      "text",
		"ini":      "ini",
		"cfg":      "ini",
		"conf":     "ini",
		"cnf":      "ini",
		"yml":      "yaml",
		"yaml":     "yaml",
		"toml":     "toml",
		"xml":      "xml",
		"json5":    "json5",
		"jsonc":    "jsonc",
		"zon":      "zon",
		"md":       "markdown",
		"markdown": "markdown",
		"rss":      "feed",
		"atom":     "feed",
	}
}

// formatFromExt returns the format name for path's extension, consulting
// the host extension registry (HostExtensions). Returns "" when the
// extension is unmapped or no registry is installed; the caller falls back
// to text.
func formatFromExt(r *Registry, path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext == "" {
		return ""
	}
	if m := HostExtensions(r); m != nil {
		if f, ok := m[ext]; ok {
			return f
		}
	}
	return ""
}

// normalizeLineEndings replaces all \r\n and \r with \n.
func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// denormalizeLineEndings converts \n to the specified ending.
func denormalizeLineEndings(s string, nl string) string {
	switch nl {
	case "crlf":
		return strings.ReplaceAll(s, "\n", "\r\n")
	default:
		return s
	}
}

// applyNL applies line ending normalization based on the nl option.
func applyNL(content string, nl string) string {
	switch nl {
	case "lf":
		return normalizeLineEndings(content)
	case "crlf":
		return denormalizeLineEndings(normalizeLineEndings(content), "crlf")
	case "raw":
		return content
	default:
		return normalizeLineEndings(content)
	}
}

// parseFileOpts extracts options from a boru map value. fmtExplicit is
// true if the user explicitly set the fmt option. parserOpts holds every
// remaining (non-reserved) key, forwarded to an opts-aware decoder — e.g.
// `read foo.csv {fmt:'csv' field:{separation:';'}}` yields
// parserOpts={field:{separation:';'}}. It is nil when no extra keys exist.
func parseFileOpts(opts Value) (enc, format, mode, nl string, fmtExplicit bool, parserOpts map[string]any) {
	enc = "utf8"
	format = "text"
	mode = "write"
	nl = "lf"

	if !opts.Parent.Equal(TMap) || !IsConcrete(opts) {
		return
	}
	m, _ := AsMap(opts)

	if s, ok := MapFieldString(m, "enc"); ok {
		enc = s
	}
	if s, ok := MapFieldString(m, "fmt"); ok {
		format = s
		fmtExplicit = true
	}
	if s, ok := MapFieldString(m, "mode"); ok {
		mode = s
	}
	if s, ok := MapFieldString(m, "nl"); ok {
		nl = s
	}

	// Surface any non-reserved keys as parser options for the decoder.
	if raw, ok := ValueToAny(opts).(map[string]any); ok {
		delete(raw, "enc")
		delete(raw, "fmt")
		delete(raw, "mode")
		delete(raw, "nl")
		delete(raw, "atomic")
		if len(raw) > 0 {
			parserOpts = raw
		}
	}

	return
}

// jsonicToValue converts a jsonic parse result to a boru Value.
// This uses data context: all text becomes strings, not words.
func jsonicToValue(v any) (Value, error) {
	// A number-Sub-wrapped token (from parser.SafeParseData) carries its
	// exact source text, so "42.0" stays a Float and "42" an Integer (and
	// large integers keep full precision / raise integer_overflow). The
	// bare float64 case below is the fallback for parsers built without
	// the Sub (SafeParse / direct float64 callers), where the source text
	// is already gone and a whole-valued float collapses to Integer.
	if ev, ok, err := parser.ConvertParsedNumber(v); ok {
		return ev, err
	}
	switch val := v.(type) {
	case nil:
		return NewTypeLiteral(TNone), nil
	case bool:
		return NewBoolean(val), nil
	case float64:
		if val == float64(int64(val)) && !math.IsInf(val, 0) && !math.IsNaN(val) {
			return NewInteger(int64(val)), nil
		}
		return NewFloat(val), nil
	case string:
		return NewString(val), nil
	case []any:
		elems := make([]Value, len(val))
		for i, item := range val {
			e, err := jsonicToValue(item)
			if err != nil {
				return Value{}, err
			}
			elems[i] = e
		}
		return NewList(elems), nil
	case *jsonic.OrderedMap:
		// The parser's insertion-ordered node: preserve source key order
		// (the whole point) instead of sorting like the plain-map fallback.
		om := NewOrderedMap()
		for _, key := range val.Keys {
			child, err := jsonicToValue(val.Vals[key])
			if err != nil {
				return Value{}, err
			}
			om.Set(key, child)
		}
		return NewMap(om), nil
	case map[string]any:
		om := NewOrderedMap()
		for _, key := range sortedMapKeys(val) {
			child, err := jsonicToValue(val[key])
			if err != nil {
				return Value{}, err
			}
			om.Set(key, child)
		}
		return NewMap(om), nil
	case jsonic.Text:
		return NewString(val.Str), nil
	case jsonic.ListRef:
		return jsonicToValue(val.Val)
	case jsonic.MapRef:
		return jsonicToValue(val.Val)
	default:
		return Value{}, fmt.Errorf("unsupported jsonic type: %T", v)
	}
}

// sortedMapKeys returns map keys in sorted order for deterministic output.
// Thin alias for the shared sortedAnyMapKeys (transform.go), kept for the
// file-local callers.
func sortedMapKeys(m map[string]any) []string {
	return sortedAnyMapKeys(m)
}

// valueToJsonic converts a boru Value to a compact json/jsonic-compatible
// string. It is a thin wrapper over the canonical walk-based json encoder
// (emit.go) — the single emit code path — keeping its name and signature for
// the existing callers/tests. Compact, double-quoted keys (the json profile),
// byte-for-byte identical to the previous hand-rolled implementation.
func valueToJsonic(v Value) string {
	s, _ := encodeJSON(v, nil)
	return s
}

func doRead(r *Registry, path, enc, format, nl string, opts map[string]any, pos SrcPos) ([]Value, error) {
	var data []byte
	var err error

	if path == pathStdout || path == pathStderr {
		return nil, r.BoruErrorAt("read_error", "read: cannot read from an output stream", "read", pos)
	}

	// read_error, not a bare fmt.Errorf. These were the only UNCODED
	// failures left in the word, and they are the two most ordinary ones —
	// a missing path and an unreadable stream — so `do [IO.read p] error
	// [dot code]` answered `None` for exactly the cases a handler wants to
	// branch on, while the word's own decode/format failures below were
	// already `read_error`. It also stops a compiled-mode read failure
	// re-running the whole program on the interpreter: runtimeShouldFallback
	// treats a FOREIGN error as "retry", a BoruError as "surface" — the
	// same reasoning the {exclusive} write path documents below.
	if path == pathStdin {
		// Through the SHARED buffered reader, not r.Input directly. IO.read-line
		// reads ahead by necessity, so a whole-stream read that went straight to
		// r.Input would skip whatever read-line had already buffered — silently.
		// One reader means `IO.read-line` then `IO.read` yields the rest of the
		// stream (io_lines.go). With no line read first the buffer is empty and
		// this is byte-for-byte the old io.ReadAll(r.Input).
		// Through the holder's LOCKED drain, not io.ReadAll over the reader
		// it hands back: the buffer is shared with IO.read-line and with
		// every concurrency fork, so a read outside the lock races the same
		// way the line read did.
		data, err = stdinReadAll(r)
		if err != nil {
			return nil, r.BoruErrorAt("read_error", fmt.Sprintf("read: stdin: %v", err), "read", pos)
		}
	} else {
		data, err = EffectiveFileOps(r).ReadFile(path)
		if err != nil {
			return nil, fileOpError(r, "read_error", "read", fmt.Sprintf("read: %v", err), err, pos)
		}
	}

	// Decode BEFORE newline normalization: a utf16 CRLF is `\r\x00\n\x00`
	// on the wire, so applyNL over the raw bytes would corrupt the stream.
	text, err := decodeEnc(data, enc)
	if err != nil {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", err), "read")
	}
	content := applyNL(text, nl)

	f, ok := HostFormats(r)[format]
	if !ok {
		return nil, r.BoruError("read_error", fmt.Sprintf("read: unknown format: %s", format), "read")
	}

	var result []Value
	if d, ok := f.(DecodeOpter); ok && len(opts) > 0 {
		result, err = d.DecodeOpts(content, opts)
	} else {
		result, err = f.Decode(content)
	}
	if err != nil {
		return nil, r.BoruErrorAt("read_error", fmt.Sprintf("read: %v", err), "read", pos)
	}

	// Store table data in SQLite for formats that produce tables.
	if HostSQLite(r) != nil && len(result) == 1 {
		if td, ok := result[0].Data.(TableData); ok {
			// Derive table name from file path (basename without extension).
			baseName := path
			if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
				baseName = baseName[idx+1:]
			}
			if idx := strings.LastIndex(baseName, "\\"); idx >= 0 {
				baseName = baseName[idx+1:]
			}
			if idx := strings.LastIndex(baseName, "."); idx >= 0 {
				baseName = baseName[:idx]
			}

			if err := HostSQLite(r).StoreTable(baseName, td); err != nil {
				return nil, r.BoruErrorAt("read_error",
					fmt.Sprintf("read: sqlite store: %v", err), "read", pos)
			}
			td.SQLite = true
			td.TableName = baseName
			result[0] = NewValueRaw(TList, td)
		}
	}

	return result, nil
}

// validateWriteOptionCombo is doWrite's FIRST fallible step, and a pure
// function of the option flags alone: {exclusive} means "create, failing
// if the path exists", which cannot be reconciled with an append or with
// the write-temp-then-rename atomic path.
//
// Split out so the check-mode mirror can run it in the handler's ORDER.
// encodeEnc comes later, so a mirror that validated only the encoding
// reported the wrong error whenever both were wrong — and reported
// nothing at all when only this was.
func validateWriteOptionCombo(r *Registry, atomic, exclusive bool, mode string, pos SrcPos) error {
	if exclusive && (atomic || mode == "append") {
		return r.BoruErrorAt("write_error", "write: {exclusive} cannot combine with {atomic} or append mode", "write", pos)
	}
	return nil
}

func doWrite(r *Registry, path, content, enc, format, mode, nl string, atomic, exclusive bool, pos SrcPos) ([]Value, error) {
	content = applyNL(content, nl)

	if err := validateWriteOptionCombo(r, atomic, exclusive, mode, pos); err != nil {
		return nil, err
	}

	// Handle stdout/stderr special paths.
	if path == pathStdout || path == pathStderr {
		var w io.Writer
		if path == pathStdout {
			w = r.Output
		} else {
			w = r.ErrOutput
		}
		if _, err := fmt.Fprint(w, content); err != nil {
			return nil, r.BoruErrorAt("write_error", fmt.Sprintf("write: %v", err), "write", pos)
		}
		return []Value{NewString(path)}, nil
	}

	// Append merges at the TEXT level — decode the existing bytes, then
	// re-encode the concatenation once. For utf8 the decode is a
	// pass-through so this is the historical byte-append; for utf16 a
	// byte-append would splice a second BOM into the middle of the file.
	if mode == "append" {
		if existing, rerr := EffectiveFileOps(r).ReadFile(path); rerr == nil {
			prev, derr := decodeEnc(existing, enc)
			if derr != nil {
				return nil, r.BoruErrorAt("write_error", fmt.Sprintf("write: append: %v", derr), "write", pos)
			}
			content = prev + content
		}
	}

	data, encErr := encodeEnc(content, enc)
	if encErr != nil {
		return nil, r.BoruErrorAt("write_error", fmt.Sprintf("write: %v", encErr), "write", pos)
	}

	// effect ledger (core effects.go): a filesystem write is an observable
	// effect the compiled-mode fallback cannot un-do, so it counts against
	// the silent re-run. Noted on the ATTEMPT: an OS WriteFile can create
	// or truncate the target before failing, so even the error path may
	// already have mutated the filesystem.
	// {exclusive} opens with O_EXCL BEFORE noting an effect: a compile failure
	// (the path already exists) mutates nothing, so it stays a clean
	// re-runnable write_error rather than tripping the compiled effect
	// fence. writeExclusive notes the effect only once the create lands.
	if exclusive {
		if err := writeExclusive(r, path, data); err != nil {
			// A BoruError (not a bare fmt.Errorf) so the compiled runtime
			// treats the compile failure as intentional and does not attempt a
			// fallback — which a prior statement's effect would block,
			// surfacing as a spurious internal_error (compiled_fullcorpus).
			return nil, fileOpError(r, "write_error", "write", fmt.Sprintf("write: %v", err), err, pos)
		}
		return []Value{NewString(path)}, nil
	}
	r.NoteEffect()
	// write_error for the same reason the {exclusive} branch above already
	// gives: a coded BoruError is a deliberate compile failure the compiled runtime
	// surfaces, where a foreign error triggers an interpreter re-run that
	// the effect just noted would block — reappearing as a spurious
	// internal_error. It also gives the failure a code to dispatch on.
	if atomic {
		if err := writeAtomic(r, path, data); err != nil {
			return nil, fileOpError(r, "write_error", "write", fmt.Sprintf("write: %v", err), err, pos)
		}
		return []Value{NewString(path)}, nil
	}
	if err := EffectiveFileOps(r).WriteFile(path, data, 0644); err != nil {
		return nil, fileOpError(r, "write_error", "write", fmt.Sprintf("write: %v", err), err, pos)
	}

	return []Value{NewString(path)}, nil
}

// writeExclusive is the {exclusive:true} write path: it creates path with
// O_EXCL — a true atomic create that fails if the path already exists,
// with no check-then-write TOCTOU — writes the data, and closes.
func writeExclusive(r *Registry, path string, data []byte) error {
	h, err := EffectiveFileOps(r).Open(path, capabilities.OpenOpts{Write: true, Create: true, Exclusive: true})
	if err != nil {
		return err // EEXIST (or a gate compile failure): nothing was created
	}
	r.NoteEffect() // the file now exists — an observable effect
	_, werr := h.Write(data)
	cerr := h.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// writeAtomic is the {atomic:true} write path: the bytes land in a temp
// file IN THE TARGET'S DIRECTORY (rename atomicity holds only within one
// filesystem — the global tmp root may be a different mount), then a
// rename replaces the target in one step. A failure removes the temp
// (best-effort) rather than stranding it — in particular a backend
// without rename (a minimal mount) declines CLEANLY.
func writeAtomic(r *Registry, path string, data []byte) error {
	ops := EffectiveFileOps(r)
	dir := filepath.Dir(path)
	tmp, err := ops.TempFile(dir, ".boru-atomic-*")
	if err != nil {
		return fmt.Errorf("atomic: %w", err)
	}
	if err := ops.WriteFile(tmp, data, 0o644); err != nil {
		_ = ops.Remove(tmp, false)
		return fmt.Errorf("atomic: %w", err)
	}
	if err := ops.Rename(tmp, path); err != nil {
		_ = ops.Remove(tmp, false)
		return fmt.Errorf("atomic: %w", err)
	}
	return nil
}
