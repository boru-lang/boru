package test

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// TestBytes runs every line of bytes.tsv as a parse+run+compare test.
// Format mirrors binary.tsv: <expr>\t<expected>[\t<error-code>].
func TestBytes(t *testing.T) {
	f, err := os.Open("bytes.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	ran := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "\t", 3)
		expr := parts[0]
		expected := ""
		if len(parts) > 1 {
			expected = parts[1]
		}
		errorCode := ""
		if len(parts) > 2 {
			errorCode = parts[2]
		}

		ran++
		t.Run(fmt.Sprintf("L%d_%s", lineNum, expr), func(t *testing.T) {
			values, perr := parser.Parse(expr)
			if perr != nil {
				if errorCode != "" {
					checkErrorCode(t, expr, perr, errorCode)
					return
				}
				t.Fatalf("parse error: %v", perr)
			}

			reg, rerr := native.DefaultRegistry()
			if rerr != nil {
				t.Fatal(rerr)
			}
			e := native.NewTop(reg)
			result, rerr := e.Run(values)

			if errorCode != "" {
				if rerr == nil {
					t.Errorf("\n  expr: %s\n  expected error %q but got result: %s",
						expr, errorCode, formatStack(result))
					return
				}
				checkErrorCode(t, expr, rerr, errorCode)
				return
			}

			if rerr != nil {
				t.Fatalf("engine error: %v", rerr)
			}

			got := formatStack(result)
			if got != expected {
				t.Errorf("\n  expr: %s\n  got:  %q\n  want: %q", expr, got, expected)
			}
		})
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("no test cases found in bytes.tsv")
	}
}

// TestBytesGoBridge covers the eng []byte <-> Bytes bridge. Importing the
// native package installs the bridge (registerBytesType runs at package
// init), so core.FromNative / core.ToNative round-trip through the Bytes type.
func TestBytesGoBridge(t *testing.T) {
	if _, err := native.DefaultRegistry(); err != nil {
		t.Fatal(err)
	}

	raw := []byte{0x68, 0x69} // "hi"
	v := core.FromNative(raw)
	if core.ParentOf(&v) == nil || !core.ParentOf(&v).ConformsTo(native.TBytes) {
		t.Fatalf("FromNative([]byte) returned %v, want a Bytes value", core.ParentOf(&v))
	}

	// copy-on-ingest: mutating the source slice must not affect the value.
	raw[0] = 0x00
	back, ok := core.ToNative(v).([]byte)
	if !ok {
		t.Fatalf("ToNative(Bytes) returned %T, want []byte", core.ToNative(v))
	}
	if string(back) != "hi" {
		t.Errorf("round-trip got %q, want %q (copy-on-ingest failed?)", string(back), "hi")
	}

	// negative: a non-Bytes value must not come back as []byte.
	if _, ok := core.ToNative(native.NewString("x")).([]byte); ok {
		t.Error("ToNative(String) unexpectedly returned []byte")
	}

	// copy-on-export: mutating the slice ToNative hands back must not corrupt
	// the immutable Bytes value (the export twin of copy-on-ingest).
	v2 := core.FromNative([]byte{0x61, 0x62}) // "ab"
	exported, ok := core.ToNative(v2).([]byte)
	if !ok {
		t.Fatalf("ToNative(Bytes) returned %T, want []byte", core.ToNative(v2))
	}
	exported[0] = 0x00
	again, _ := core.ToNative(v2).([]byte)
	if string(again) != "ab" {
		t.Errorf("mutating an exported slice corrupted the Bytes value: got %q, want %q", string(again), "ab")
	}
}
