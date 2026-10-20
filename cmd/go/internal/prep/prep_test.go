package prep

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runPrep invokes the exported Run entry point and captures streams.
func runPrep(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// writeManifest drops a boru.jsonic into a fresh temp dir.
func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boru.jsonic"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNameAndSynopsis(t *testing.T) {
	c := New()
	if c.Name() != "prep" {
		t.Errorf("Name = %q, want prep", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis should not be empty")
	}
}

func TestDoWritesJSON(t *testing.T) {
	dir := writeManifest(t, "name: my-mod\nversion: \"1.2.3\"\nfiles: [a.boru, b.boru]")
	m, err := Do(dir)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if m["name"] != "my-mod" {
		t.Errorf("name = %v, want my-mod", m["name"])
	}

	data, err := os.ReadFile(filepath.Join(dir, ".boru", "boru.json"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Error("output should end with a newline")
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["version"] != "1.2.3" {
		t.Errorf("version = %v, want 1.2.3", got["version"])
	}
	files, ok := got["files"].([]any)
	if !ok || len(files) != 2 {
		t.Errorf("files = %v, want two entries", got["files"])
	}
}

func TestDoMissingManifest(t *testing.T) {
	if _, err := Do(t.TempDir()); err == nil {
		t.Error("expected an error when boru.jsonic is absent")
	}
}

func TestDoInvalidJsonic(t *testing.T) {
	// An unterminated string is a hard jsonic syntax error.
	dir := writeManifest(t, `name: "unterminated`)
	_, err := Do(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid jsonic") {
		t.Errorf("err = %v, want invalid-jsonic error", err)
	}
}

func TestDoTopLevelMustBeMap(t *testing.T) {
	dir := writeManifest(t, "[1, 2, 3]")
	_, err := Do(dir)
	if err == nil || !strings.Contains(err.Error(), "must be a map") {
		t.Errorf("err = %v, want must-be-a-map error", err)
	}
}

func TestDoMkdirFailure(t *testing.T) {
	dir := writeManifest(t, "name: x")
	// Occupy the .boru slot with a regular file so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, ".boru"), []byte("in the way"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Do(dir); err == nil {
		t.Error("expected an error when .boru exists as a file")
	}
}

func TestRunWithDirArg(t *testing.T) {
	dir := writeManifest(t, "name: x")
	code, out, errOut := runPrep(t, dir)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	want := filepath.Join(dir, ".boru", "boru.json")
	if strings.TrimSpace(out) != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRunDefaultDir(t *testing.T) {
	dir := writeManifest(t, "name: x")
	t.Chdir(dir)
	code, out, errOut := runPrep(t)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if strings.TrimSpace(out) != filepath.Join(".", ".boru", "boru.json") {
		t.Errorf("stdout = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".boru", "boru.json")); err != nil {
		t.Errorf("output file not written: %v", err)
	}
}

func TestRunExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	proj := filepath.Join(home, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "boru.jsonic"), []byte("name: x"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runPrep(t, "~/proj")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(proj, ".boru", "boru.json")); err != nil {
		t.Errorf("output not written under expanded home: %v", err)
	}
}

func TestRunErrorExit(t *testing.T) {
	code, _, errOut := runPrep(t, t.TempDir())
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "error:") {
		t.Errorf("stderr = %q, want an error line", errOut)
	}
}

func TestCommandRunDelegates(t *testing.T) {
	dir := writeManifest(t, "name: x")
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "boru.json") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
