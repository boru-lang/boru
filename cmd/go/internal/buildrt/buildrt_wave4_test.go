// Wave-4 coverage for the buildrt tails: the CompileTry arm of Eval,
// Main's OptionsBlob parse-error arm, and DecodePayload's corrupt-
// JSON-with-valid-trailer arm.
package buildrt

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func TestW4EvalCompileTry(t *testing.T) {
	var out bytes.Buffer
	if err := Eval(&out, "2 mul 21", lang.Options{}); err != nil {
		t.Fatalf("Eval(CompileTry): %s", err)
	}
	if !strings.Contains(out.String(), "42") {
		t.Errorf("output = %q, want 42", out.String())
	}
}

func TestW4MainOptionsBlobParseError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main(Config{Source: "1", OptionsBlob: "{{"}, nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Main(bad OptionsBlob) = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr = %q, want a parse error", stderr.String())
	}
}

func TestW4DecodePayloadCorruptBody(t *testing.T) {
	// Valid trailer (magic + length) over a body that is not JSON.
	body := []byte("{definitely not json")
	image := make([]byte, 0, len(body)+footerSize)
	image = append(image, body...)
	footer := make([]byte, footerSize)
	copy(footer[:magicSize], magic)
	binary.BigEndian.PutUint64(footer[magicSize:], uint64(len(body)))
	image = append(image, footer...)

	if _, ok, err := DecodePayload(image); err == nil || ok {
		t.Errorf("DecodePayload(corrupt body) = (ok=%v, err=%v), want an error", ok, err)
	}
}
