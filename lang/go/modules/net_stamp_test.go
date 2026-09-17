package modules

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// Runtime stamping of CUSTOM boru codec fns at resolveCodec (Phase 1 of
// design/legacy/RUNTIME-STAMPING.0.ignore): an armed registry compiles the map's
// decode/encode bodies to detached units so per-request invokeFn dispatch
// runs on the VM; an unarmed registry (the -no-compile contract) and the
// Go-backed built-in codecs are untouched. Positive + negative per
// lang/go/CLAUDE.md.

// customCodecSteps defines a newline-framed boru codec equivalent in shape to
// mini-redis's (convert / index / slice body — the compilable subset).
var customCodecSteps = []string{
	`import "boru:string-util"`,
	`def my-decode (fn [[buf:Any] [Any] [
		def txt (convert String buf)
		def idx (StringUtil.indexof "\n" txt)
		if (idx gte 0) [
			{msg: {line: (convert String (slice 0 idx buf))} rest: (slice (add 1 idx) (size buf) buf)}
		] [ {need: 1} ]
	]])`,
	`def my-encode (fn [[reply:Any] [Any] [ convert Bytes (join "" [reply.line "\n"]) ]])`,
	`def cdc {decode: my-decode/v encode: my-encode/v}`,
}

// stampNetReg builds the net-test registry, optionally armed for runtime
// stamping, and runs the given steps on it.
func stampNetReg(t *testing.T, armed bool, steps []string) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	if armed {
		reg.EnableRuntimeStamping()
	}
	engine := native.NewTop(reg)
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			t.Fatalf("parse %q: %v", step, pErr)
		}
		if _, err := engine.Run(vals); err != nil {
			t.Fatalf("run %q: %v", step, err)
		}
	}
	return reg
}

func codecRef(t *testing.T, v native.Value) *compiler.CompiledFnRef {
	t.Helper()
	fd, ok := v.Data.(core.FnDefInfo)
	if !ok {
		t.Fatalf("codec slot is not a fn value: %v", v)
	}
	for i := range fd.Signatures {
		if ref := compiler.CompiledRef(&fd.Signatures[i]); ref != nil {
			return ref
		}
	}
	return nil
}

// Armed: resolveCodec stamps both custom boru fns (finalized Prog present) and
// the symmetric client-side defaults alias the SAME stamped clones (one
// compile per direction, not two). Unarmed: nothing is stamped. Built-in
// Go codecs are never touched either way.
func TestResolveCodecStampsCustomBoruFns(t *testing.T) {
	armed := stampNetReg(t, true, customCodecSteps)
	cdcVal, ok := armed.Defs.Top("cdc")
	if !ok {
		t.Fatal("cdc not bound")
	}
	cf, err := resolveCodec(armed, cdcVal, "listen")
	if err != nil {
		t.Fatalf("resolveCodec: %v", err)
	}
	decRef, encRef := codecRef(t, cf.decode), codecRef(t, cf.encode)
	if decRef == nil || decRef.Prog == nil {
		t.Fatalf("armed decode must carry a finalized detached unit")
	}
	if encRef == nil || encRef.Prog == nil {
		t.Fatalf("armed encode must carry a finalized detached unit")
	}
	if got := codecRef(t, cf.decodeResp); got != decRef {
		t.Fatalf("decode-resp default must alias the stamped decode clone")
	}
	if got := codecRef(t, cf.encodeReq); got != encRef {
		t.Fatalf("encode-req default must alias the stamped encode clone")
	}

	unarmed := stampNetReg(t, false, customCodecSteps)
	cdcVal2, _ := unarmed.Defs.Top("cdc")
	cf2, err := resolveCodec(unarmed, cdcVal2, "listen")
	if err != nil {
		t.Fatalf("resolveCodec (unarmed): %v", err)
	}
	if codecRef(t, cf2.decode) != nil || codecRef(t, cf2.encode) != nil {
		t.Fatalf("unarmed registry must not stamp codec fns (-no-compile contract)")
	}

	// An ASYMMETRIC codec (explicit encode-req / decode-resp keys) stamps its
	// client-side fns independently of the server pair.
	asym := stampNetReg(t, true, append(customCodecSteps,
		`def cdc4 {decode: my-decode/v encode: my-encode/v encode-req: my-encode/v decode-resp: my-decode/v}`))
	cdc4Val, _ := asym.Defs.Top("cdc4")
	cf4, err := resolveCodec(asym, cdc4Val, "connect")
	if err != nil {
		t.Fatalf("resolveCodec (asymmetric): %v", err)
	}
	if r := codecRef(t, cf4.encodeReq); r == nil || r.Prog == nil {
		t.Fatalf("explicit encode-req must be stamped")
	}
	if r := codecRef(t, cf4.decodeResp); r == nil || r.Prog == nil {
		t.Fatalf("explicit decode-resp must be stamped")
	}

	// A built-in Go codec resolves through the same path with zero stamping —
	// its fast dispatch needs no unit.
	linesSteps := []string{`import "boru:net"`, `def cdc Net.lines`}
	armedLines := stampNetReg(t, true, linesSteps)
	linesVal, _ := armedLines.Defs.Top("cdc")
	cfL, err := resolveCodec(armedLines, linesVal, "listen")
	if err != nil {
		t.Fatalf("resolveCodec (Net.lines): %v", err)
	}
	if codecRef(t, cfL.decode) != nil || codecRef(t, cfL.encode) != nil {
		t.Fatalf("Go-backed built-in codec must not be stamped")
	}
}

// The custom-codec echo service answers identically with the codec stamped
// (armed) and interpreted (unarmed) — the end-to-end differential over real
// loopback sockets, exercising the stamped decode/encode per request.
func TestCustomCodecStampedDifferential(t *testing.T) {
	echoSteps := append(append([]string{`import "boru:net"`}, customCodecSteps...),
		`def svc (service {})`,
		`add {} ([req:Map state:Any] => [ {line: req.line} ]) svc`,
		`def ln (Net.listen {tcp: 0 codec: cdc} svc)`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)]) codec: Net.lines})`,
	)
	probe := `(call {line: "ping pong"} ep {timeout: 5000}).line`

	run := func(armed bool) string {
		reg := stampNetReg(t, armed, echoSteps)
		engine := native.NewTop(reg)
		vals, err := parser.Parse(probe)
		if err != nil {
			t.Fatal(err)
		}
		out, err := engine.Run(vals)
		if err != nil {
			t.Fatalf("probe (armed=%v): %v", armed, err)
		}
		s, err := core.AsString(out[len(out)-1])
		if err != nil {
			t.Fatalf("probe result (armed=%v): %v", armed, out)
		}
		return s
	}

	armed, unarmed := run(true), run(false)
	if armed != unarmed || armed != "ping pong" {
		t.Fatalf("stamped codec diverged: armed=%q unarmed=%q want %q", armed, unarmed, "ping pong")
	}
}

// A frame split across two writes drives the stamped decode through its
// {need: 1} partial-read arm and then the completed parse — the same reply
// as the interpreted codec.
func TestCustomCodecStampedSplitWrite(t *testing.T) {
	serveSteps := append(append([]string{`import "boru:net"`, `import "boru:time-util"`}, customCodecSteps...),
		`def svc (service {})`,
		`add {} ([req:Map state:Any] => [ {line: req.line} ]) svc`,
		`def ln (Net.listen {tcp: 0 codec: cdc} svc)`,
	)
	probeSteps := []string{
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)])})`,
		`Net.send-bytes (convert Bytes "hel") sock;`,
		`TimeUtil.sleep 60;`,
		`Net.send-bytes (convert Bytes "lo\n") sock;`,
		`def reply (convert String (Net.recv-until sock (convert Bytes "\n") {within: 5000}))`,
		`Net.close sock;`,
		`reply`,
	}

	run := func(armed bool) string {
		reg := stampNetReg(t, armed, serveSteps)
		engine := native.NewTop(reg)
		var out []native.Value
		for _, step := range probeSteps {
			vals, err := parser.Parse(step)
			if err != nil {
				t.Fatal(err)
			}
			out, err = engine.Run(vals)
			if err != nil {
				t.Fatalf("split probe step %q (armed=%v): %v", step, armed, err)
			}
		}
		s, err := core.AsString(out[len(out)-1])
		if err != nil {
			t.Fatalf("split probe result (armed=%v): %v", armed, out)
		}
		return s
	}

	armed, unarmed := run(true), run(false)
	if armed != unarmed || !strings.Contains(armed, "hello") {
		t.Fatalf("split-write need path diverged: armed=%q unarmed=%q", armed, unarmed)
	}
}
