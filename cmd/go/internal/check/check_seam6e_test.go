package check

// check_seam6e_test.go — seam-arm tests (design/TEST-SEAMS.10.md) for the
// init-error arms of Emit/Run/Preflight and Run's --json marshal-error arm.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func swapLangNewFail(t *testing.T) {
	t.Helper()
	orig := langNew
	langNew = func(...lang.Options) (*lang.Boru, error) { return nil, errors.New("init boom") }
	t.Cleanup(func() { langNew = orig })
}

func TestRunInitError(t *testing.T) {
	swapLangNewFail(t)
	var stdout, stderr bytes.Buffer
	err := Run(&stdout, &stderr, "add 1 2", "", 0, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "init error") {
		t.Fatalf("err = %v, want init error", err)
	}
}

func TestPreflightInitError(t *testing.T) {
	swapLangNewFail(t)
	var stderr bytes.Buffer
	err := Preflight(&stderr, "add 1 2", "", 0, false)
	if err == nil || !strings.Contains(err.Error(), "init error") {
		t.Fatalf("err = %v, want init error", err)
	}
}

func TestRunJSONMarshalError(t *testing.T) {
	orig := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshalIndent = orig })
	var stdout, stderr bytes.Buffer
	err := Run(&stdout, &stderr, "add 1 2", "", 0, true, false, false)
	if err == nil || !strings.Contains(err.Error(), "json marshal") {
		t.Fatalf("err = %v, want json marshal error", err)
	}
}
