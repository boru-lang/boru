// Wave-4 coverage for the buildrt tails: the CompileTry arm of Eval,
// Main's OptionsBlob parse-error arm, and DecodePayload's corrupt-
// JSON-with-valid-trailer arm.
package buildrt

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

func TestW4EvalCompileTry(t *testing.T) {
	var out bytes.Buffer
	if err := Eval(&out, "2 mul 21", lang.Options{}, CompileTry); err != nil {
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
	copy(footer[:len(magic)], magic)
	binary.BigEndian.PutUint64(footer[len(magic):], uint64(len(body)))
	image = append(image, footer...)

	if _, ok, err := DecodePayload(image); err == nil || ok {
		t.Errorf("DecodePayload(corrupt body) = (ok=%v, err=%v), want an error", ok, err)
	}
}

// legacyFooter builds a trailer using an explicit magic, standing in for
// an executable produced by an earlier, differently-named release.
func legacyFooter(magicStr string, bodyLen int) []byte {
	footer := make([]byte, len(magicStr)+lenSize)
	copy(footer, magicStr)
	binary.BigEndian.PutUint64(footer[len(magicStr):], uint64(bodyLen))
	return footer
}

// TestDecodesEveryHistoricalExecMagic pins the compatibility contract for
// self-embedding executables. The magics differ in LENGTH across renames
// ("AQLEXEC\x01" is 8 bytes, "BORUEXEC\x01" is 9), so a reader that took
// the length field's offset from len(magic) would read the payload size out
// of the wrong bytes and silently fail to recognise the executable — which
// is exactly what the AQL -> BORU rename did.
func TestDecodesEveryHistoricalExecMagic(t *testing.T) {
	for _, m := range []string{"VLTEXEC\x01", "BORUEXEC\x01", "AQLEXEC\x01"} {
		body, err := json.Marshal(Config{Source: "1 print"})
		if err != nil {
			t.Fatal(err)
		}
		image := append([]byte("host-binary-bytes"), body...)
		image = append(image, legacyFooter(m, len(body))...)

		cfg, ok, err := DecodePayload(image)
		if err != nil || !ok {
			t.Errorf("DecodePayload with magic %q: ok=%v err=%v", m, ok, err)
			continue
		}
		if cfg.Source != "1 print" {
			t.Errorf("magic %q: source = %q, want \"1 print\"", m, cfg.Source)
		}

		// And through the file path, which reads only the tail.
		p := writeImage(t, image)
		cfg, ok, err = ReadEmbeddedPayload(p)
		if err != nil || !ok {
			t.Errorf("ReadEmbeddedPayload with magic %q: ok=%v err=%v", m, ok, err)
			continue
		}
		if cfg.Source != "1 print" {
			t.Errorf("magic %q via file: source = %q", m, cfg.Source)
		}
	}
}

// A plain host binary must still be reported as "not a built executable".
func TestPlainBinaryIsNotDetected(t *testing.T) {
	p := writeImage(t, []byte("just a normal binary with no trailer at all"))
	if _, ok, err := ReadEmbeddedPayload(p); ok || err != nil {
		t.Errorf("plain binary: ok=%v err=%v, want false/nil", ok, err)
	}
}
