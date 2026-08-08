package basic_test

// The Go half of the base-layer parity corpus (basic/spec/*.tsv).
//
// basic/ts/src/basicspec.test.ts is the other half. The two runners share
// NO code — they read the same files and each builds the registry and
// renders the residual independently, exactly as core/spec's and
// parser/spec's pairs do. That independence is the point: shared
// scaffolding can hide the same bug from both engines
// (design/CORE-GO-TS-DEFECTS.0.md, blind spot 9).
//
// This is a SPEC, not a differential: the `expected` column is the
// documented contract, so a row can legitimately fail on BOTH engines —
// which is exactly the class of defect an agreement-only corpus is
// structurally blind to.

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	basic "github.com/boru-lang/boru/basic/go"
	core "github.com/boru-lang/boru/core/go"
)

const basicSpecDir = "../spec"

type basicSpecRow struct {
	file     string
	line     int
	prog     string
	expected string
	note     string
}

func parseBasicSpec(t *testing.T, path string) []basicSpecRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var rows []basicSpecRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		// A line is a COMMENT only when it starts with '#' AND carries no
		// tab — '#' is boru's own comment marker, so a program may begin
		// with one.
		if strings.TrimSpace(line) == "" ||
			(strings.HasPrefix(line, "#") && !strings.Contains(line, "\t")) {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			t.Fatalf("%s:%d: want at least 2 tab-separated columns, got %d", path, n, len(parts))
		}
		row := basicSpecRow{file: filepath.Base(path), line: n, prog: parts[0], expected: parts[1]}
		if len(parts) > 2 {
			row.note = parts[2]
		}
		rows = append(rows, row)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

// basicSpecToken builds one program token: a decimal is an Integer, '…' is
// a String, anything else is a Word. Deliberately tiny — the corpus is
// about the WORDS, so the token notation stays out of the way.
func basicSpecToken(tok string) core.Value {
	if strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'") && len(tok) >= 2 {
		return core.NewString(tok[1 : len(tok)-1])
	}
	// A capitalised builtin name is that type's LITERAL, so a row can pass
	// a bare type where a word expects a concrete value — the refusal path
	// is otherwise unreachable from the corpus.
	switch tok {
	case "List":
		return core.NewTypeLiteral(core.TList)
	case "Integer":
		return core.NewTypeLiteral(core.TInteger)
	}
	if n, err := strconv.ParseInt(tok, 10, 64); err == nil {
		return core.NewInteger(n)
	}
	return core.NewWord(tok)
}

// basicSpecRegistry registers ONLY the stack vocabulary, so a row's result
// depends on nothing else in the layer.
func basicSpecRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	for _, nf := range basic.StackNatives {
		r.RegisterNativeFunc(nf)
	}
	r.RegisterNativeFunc(basic.ControlNatives[0]) // `do`
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	return r
}

// basicSpecProgram turns a row's token list into values, assembling
// bracketed runs into LIST values. The corpus notation has no parser
// behind it, so `do [ 1 2 ]` needs the brackets to become one list Value
// rather than three tokens — the same assembly the TS runner does
// independently.
func basicSpecProgram(t *testing.T, toks []string) []core.Value {
	t.Helper()
	var out []core.Value
	for i := 0; i < len(toks); i++ {
		if toks[i] != "[" {
			out = append(out, basicSpecToken(toks[i]))
			continue
		}
		depth := 1
		j := i + 1
		for ; j < len(toks) && depth > 0; j++ {
			if toks[j] == "[" {
				depth++
			}
			if toks[j] == "]" {
				depth--
			}
		}
		if depth != 0 {
			t.Fatalf("unbalanced [ in program %v", toks)
		}
		// Eval=true mirrors what the PARSER produces for a `[…]` literal;
		// a raw NewList differs in a way `do` notices on an empty body.
		out = append(out, core.NewEvalList(basicSpecProgram(t, toks[i+1:j-1])))
		i = j - 1
	}
	return out
}

func TestBasicSpec(t *testing.T) {
	entries, err := os.ReadDir(basicSpecDir)
	if err != nil {
		t.Fatalf("read %s: %v", basicSpecDir, err)
	}
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		path := filepath.Join(basicSpecDir, e.Name())
		rows := parseBasicSpec(t, path)
		t.Run(strings.TrimSuffix(e.Name(), ".tsv"), func(t *testing.T) {
			for _, row := range rows {
				total++
				prog := basicSpecProgram(t, strings.Fields(row.prog))
				got := ""
				out, rerr := core.NewTop(basicSpecRegistry(t)).Run(prog)
				if rerr != nil {
					var be *core.BoruError
					if errors.As(rerr, &be) {
						got = "ERROR:" + be.Code
					} else {
						got = "ERROR:non_boru:" + rerr.Error()
					}
				} else {
					parts := make([]string, 0, len(out))
					for _, v := range out {
						parts = append(parts, core.CanonValue(v))
					}
					got = strings.Join(parts, " ")
				}
				if got != row.expected {
					t.Errorf("%s:%d  %s\n  got  %q\n  want %q\n  (%s)",
						row.file, row.line, row.prog, got, row.expected, row.note)
				}
			}
		})
	}
	if total == 0 {
		t.Fatal("basic/spec produced no rows — the corpus is not being read")
	}
	t.Logf("basic/spec: %d rows", total)
}
