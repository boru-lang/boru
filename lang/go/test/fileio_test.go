package test

import (
	"bytes"
	"github.com/boru-lang/boru/lang/go/native"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	parser "github.com/boru-lang/boru/parser/go"
)

// runWithFiles creates a registry with in-memory files and runs Boru.
func runWithFiles(t *testing.T, files map[string]string, expr string) (string, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[filepath.Clean(filepath.FromSlash(path))] = []byte(content) // the model keys a file by its resolved, OS-native path
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)
	registerIOWords(reg)

	values, err := parser.Parse(expr)
	if err != nil {
		return "", err
	}

	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		return "", err
	}

	return formatStack(result), nil
}

// runWithMem creates a registry with an in-memory FS, runs boru, and returns
// the MemFileOps so tests can inspect written files.
func runWithMem(t *testing.T, files map[string]string, expr string) (*capabilities.MemFileOps, string, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for path, content := range files {
		mem.Files[filepath.Clean(filepath.FromSlash(path))] = []byte(content) // the model keys a file by its resolved, OS-native path
	}

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)
	registerIOWords(reg)

	values, err := parser.Parse(expr)
	if err != nil {
		return mem, "", err
	}

	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		return mem, "", err
	}

	return mem, formatStack(result), nil
}

// --- read tests ---

func TestReadBasic(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"hello.txt": "hello world",
	}, `read (make Pathon "hello.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'hello world'" {
		t.Errorf("got %s, want 'hello world'", got)
	}
}

func TestReadPrefix(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"data.txt": "abc",
	}, `(make Pathon "data.txt") read`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'abc'" {
		t.Errorf("got %s, want 'abc'", got)
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := runWithFiles(t, nil, `read (make Pathon "nope.txt")`)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not exist") {
		t.Errorf("expected 'not exist' in error, got: %v", err)
	}
}

// --- line ending normalization ---

func TestReadNormalizesLineEndings(t *testing.T) {
	// Default nl:"lf" should convert \r\n to \n.
	got, err := runWithFiles(t, map[string]string{
		"crlf.txt": "line1\r\nline2\r\n",
	}, `read (make Pathon "crlf.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'line1\nline2\n'" {
		t.Errorf("got %q, want 'line1\\nline2\\n'", got)
	}
}

func TestReadNormalizesCR(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"cr.txt": "a\rb\r",
	}, `read (make Pathon "cr.txt")`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'a\nb\n'" {
		t.Errorf("got %q, want 'a\\nb\\n'", got)
	}
}

func TestReadRawLineEndings(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"raw.txt": "a\r\nb\r",
	}, `read (make Pathon "raw.txt") {nl:"raw"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "'a\r\nb\r'" {
		t.Errorf("got %q, want 'a\\r\\nb\\r'", got)
	}
}

// --- fmt:"lines" ---

func TestReadLines(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"lines.txt": "aaa\nbbb\nccc",
	}, `read (make Pathon "lines.txt") {fmt:"lines"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "['aaa' 'bbb' 'ccc']" {
		t.Errorf("got %s, want ['aaa' 'bbb' 'ccc']", got)
	}
}

// --- fmt:"json" ---

func TestReadJSON(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"data.json": `{"x":1,"y":"hello"}`,
	}, `read (make Pathon "data.json") {fmt:"json"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{x:1 y:'hello'}" {
		t.Errorf("got %s, want {x:1 y:'hello'}", got)
	}
}

func TestReadJSONArray(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"arr.json": `[1,2,3]`,
	}, `read (make Pathon "arr.json") {fmt:"json"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[1 2 3]" {
		t.Errorf("got %s, want [1 2 3]", got)
	}
}

// --- fmt:"jsonic" ---

func TestReadJsonic(t *testing.T) {
	got, err := runWithFiles(t, map[string]string{
		"config.jsonic": `{x:1, y:hello}`,
	}, `read (make Pathon "config.jsonic") {fmt:"jsonic"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{x:1 y:'hello'}" {
		t.Errorf("got %s, want {x:1 y:'hello'}", got)
	}
}

// --- write tests ---

func TestWriteBasic(t *testing.T) {
	mem, got, err := runWithMem(t, nil, `write (make Pathon "out.txt") "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "out.txt" {
		t.Errorf("return: got %s, want out.txt (Pathon)", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "hello" {
		t.Errorf("file content: got %q, want %q", content, "hello")
	}
}

func TestWriteForward(t *testing.T) {
	// write path content — both forward
	mem, got, err := runWithMem(t, nil, `write (make Pathon "out.txt") "hello"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "out.txt" {
		t.Errorf("return: got %s, want out.txt (Pathon)", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "hello" {
		t.Errorf("file content: got %q, want %q", content, "hello")
	}
}

func TestWriteWithExprContent(t *testing.T) {
	// write path (expression) — content from paren expression
	mem, got, err := runWithMem(t, nil, `write (make Pathon "out.txt") (upper "hello")`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "out.txt" {
		t.Errorf("return: got %s, want out.txt (Pathon)", got)
	}
	content := string(mem.Files["out.txt"])
	if content != "HELLO" {
		t.Errorf("file content: got %q, want %q", content, "HELLO")
	}
}

func TestWriteAppend(t *testing.T) {
	mem, _, err := runWithMem(t, map[string]string{
		"log.txt": "line1\n",
	}, `write (make Pathon "log.txt") "line2\n" {mode:"append"}`)
	if err != nil {
		t.Fatal(err)
	}
	content := string(mem.Files["log.txt"])
	if content != "line1\nline2\n" {
		t.Errorf("file content: got %q, want %q", content, "line1\nline2\n")
	}
}

func TestWriteReturnsPath(t *testing.T) {
	_, got, err := runWithMem(t, nil, `write (make Pathon "result.txt") "data"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "result.txt" {
		t.Errorf("got %s, want result.txt (Pathon)", got)
	}
}

// --- read/write roundtrip ---

func TestReadWriteRoundtrip(t *testing.T) {
	mem := capabilities.NewMem()
	mem.Files["src.txt"] = []byte("the content")

	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, mem)
	registerIOWords(reg)

	// Write with all forward args to be explicit
	values, err := parser.Parse(`write (make Pathon "dst.txt") (read (make Pathon "src.txt"))`)
	if err != nil {
		t.Fatal(err)
	}

	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}

	got := formatStack(result)
	if got != "dst.txt" {
		t.Errorf("return: got %s, want dst.txt (Pathon)", got)
	}

	content := string(mem.Files["dst.txt"])
	if content != "the content" {
		t.Errorf("file content: got %q, want %q", content, "the content")
	}
}

// --- write with crlf ---

func TestWriteCRLF(t *testing.T) {
	mem, _, err := runWithMem(t, nil, `write (make Pathon "out.txt") "a\nb\n" {nl:"crlf"}`)
	if err != nil {
		t.Fatal(err)
	}
	content := string(mem.Files["out.txt"])
	if content != "a\r\nb\r\n" {
		t.Errorf("file content: got %q, want %q", content, "a\r\nb\r\n")
	}
}

// --- stdio word tests ---

// runWithStdio creates a registry with custom stdin/stdout/stderr and runs Boru.
func runWithStdio(t *testing.T, stdin string, expr string) (stdout, stderr, stack string, err error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, capabilities.NewMem())
	registerIOWords(reg)

	var outBuf, errBuf bytes.Buffer
	reg.Output = &outBuf
	reg.ErrOutput = &errBuf
	reg.Input = strings.NewReader(stdin)

	values, parseErr := parser.Parse(expr)
	if parseErr != nil {
		return "", "", "", parseErr
	}

	eng := native.NewTop(reg)
	result, runErr := eng.Run(values)
	if runErr != nil {
		return outBuf.String(), errBuf.String(), "", runErr
	}

	return outBuf.String(), errBuf.String(), formatStack(result), nil
}

func TestStdinWord(t *testing.T) {
	// stdin should push a special path string.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	native.SetHostFileOps(reg, capabilities.NewMem())
	registerIOWords(reg)
	var buf bytes.Buffer
	reg.Output = &buf

	values, err := parser.Parse(`stdin`)
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got != "stdin" {
		t.Errorf("stdin word: got %s, want stdin (the stdin Stream atom)", got)
	}
}

func TestStdoutWord(t *testing.T) {
	reg, err := native.DefaultRegistry()
	registerIOWords(reg)
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	values, err := parser.Parse(`stdout`)
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got != "stdout" {
		t.Errorf("stdout word: got %s, want stdout (the stdout Stream atom)", got)
	}
}

func TestStderrWord(t *testing.T) {
	reg, err := native.DefaultRegistry()
	registerIOWords(reg)
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	values, err := parser.Parse(`stderr`)
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)
	result, err := eng.Run(values)
	if err != nil {
		t.Fatal(err)
	}
	got := formatStack(result)
	if got != "stderr" {
		t.Errorf("stderr word: got %s, want stderr (the stderr Stream atom)", got)
	}
}

func TestReadStdin(t *testing.T) {
	_, _, stack, err := runWithStdio(t, "hello from stdin", `read (stdin)`)
	if err != nil {
		t.Fatal(err)
	}
	if stack != "'hello from stdin'" {
		t.Errorf("read stdin: got %s, want 'hello from stdin'", stack)
	}
}

func TestWriteStdout(t *testing.T) {
	stdout, _, stack, err := runWithStdio(t, "", `write (stdout) "hello out"`)
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "hello out" {
		t.Errorf("stdout content: got %q, want %q", stdout, "hello out")
	}
	if stack != "stdout" {
		t.Errorf("return value: got %s, want stdout (the stdout Stream atom)", stack)
	}
}

func TestWriteStderr(t *testing.T) {
	_, stderr, stack, err := runWithStdio(t, "", `write (stderr) "error msg"`)
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "error msg" {
		t.Errorf("stderr content: got %q, want %q", stderr, "error msg")
	}
	if stack != "stderr" {
		t.Errorf("return value: got %s, want stderr (the stderr Stream atom)", stack)
	}
}

func TestReadStdinWithFormat(t *testing.T) {
	_, _, stack, err := runWithStdio(t, `{"x":1}`, `read (stdin) {fmt:"json"}`)
	if err != nil {
		t.Fatal(err)
	}
	if stack != "{x:1}" {
		t.Errorf("read stdin json: got %s, want {x:1}", stack)
	}
}

func TestReadStdinLines(t *testing.T) {
	_, _, stack, err := runWithStdio(t, "aaa\nbbb\nccc", `read (stdin) {fmt:"lines"}`)
	if err != nil {
		t.Fatal(err)
	}
	if stack != "['aaa' 'bbb' 'ccc']" {
		t.Errorf("read stdin lines: got %s, want ['aaa' 'bbb' 'ccc']", stack)
	}
}
