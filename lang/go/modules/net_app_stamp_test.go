package modules

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// The LOAD-BEARING app regression (design/legacy/RUNTIME-STAMPING.0.ignore): the REAL
// design/examples/apps/mini-redis.boru, imported under an armed registry,
// must (a) stamp its module callbacks at load, and (b) answer a full driven
// command battery over real sockets byte-identically to the unarmed
// (pure-interpreter) build. This is the assertion that would have caught
// the earlier "mini-redis compiles" claim — probe success and functional
// tests are both blind to runtime stamping.

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// lang/go/modules/<this file> → repo root is three levels up.
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// miniRedisSession imports the real app, serves on an ephemeral port,
// drives cmds through MiniRedis.cmd, and returns the reply strings plus
// the app's module registry (for stamp attribution).
func miniRedisSession(t *testing.T, armed bool, cmds []string) ([]string, *native.Registry) {
	t.Helper()
	reg := stampNetReg(t, armed, nil)
	reg.BaseDir = repoRoot(t)
	engine := native.NewTop(reg)
	run := func(src string) []native.Value {
		t.Helper()
		vals, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		out, err := engine.Run(vals)
		if err != nil {
			t.Fatalf("run %q: %v", src, err)
		}
		return out
	}
	run(`import "boru:net"`)
	run(`import "./design/examples/apps/mini-redis.boru"`)
	run(`def ln (MiniRedis.serve {port: 0})`)
	run(`def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String (Net.addr ln).port)]))`)

	replies := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out := run(`MiniRedis.cmd ep ` + strconv.Quote(c))
		s, err := core.AsString(out[len(out)-1])
		if err != nil {
			t.Fatalf("cmd %q reply not a string: %v", c, out)
		}
		replies = append(replies, s)
	}

	// The module registry, reached through an export wrapper.
	run(`def w9 MiniRedis.cmd/v`)
	w, ok := reg.Defs.Top("w9")
	if !ok {
		t.Fatal("wrapper not bound")
	}
	fd, isFn := w.Data.(core.FnDefInfo)
	if !isFn || fd.Registry == nil {
		t.Fatal("MiniRedis.cmd is not a module fn wrapper")
	}
	return replies, fd.Registry
}

func moduleFnRef(t *testing.T, modReg *native.Registry, name string) *compiler.CompiledFnRef {
	t.Helper()
	v, ok := modReg.Defs.Top(name)
	if !ok {
		t.Fatalf("module fn %q not bound", name)
	}
	fd, isFn := v.Data.(core.FnDefInfo)
	if !isFn {
		t.Fatalf("module binding %q is not a fn", name)
	}
	for i := range fd.Signatures {
		if r := compiler.CompiledRef(&fd.Signatures[i]); r != nil {
			return r
		}
	}
	return nil
}

func TestMiniRedisAppStampedAndDifferential(t *testing.T) {
	cmds := []string{
		"PING",
		"SET greeting hello world",
		"GET greeting",
		"EXISTS greeting",
		"DEL greeting",
		"EXISTS greeting",
		"INCR counter",
		"INCR counter",
		"SET k1 a", "SET k2 b", "KEYS",
		"LPUSH mylist x", "RPUSH mylist y", "LLEN mylist", "LRANGE mylist 0 -1",
		"HSET h f v", "HGET h f", "HDEL h f", "HGET h f",
		"BOGUSCMD arg",
	}
	// Clock-dependent rows run separately with tolerance, not byte-compared.
	armedReplies, modReg := miniRedisSession(t, true, cmds)
	plainReplies, plainMod := miniRedisSession(t, false, cmds)

	// (a) Attribution: the app's module callbacks are stamped at load when
	// armed — the codec pair, the helpers, and the client fns. redis-serve
	// itself is not asserted (its body stores the handlers; whether it
	// stamps is not load-bearing).
	for _, name := range []string{
		"redis-decode", "redis-encode", // the custom codec (per-request, both directions)
		"kv-read", "arg-at", "now-ms", // handler helpers
		"redis-cmd", // the per-iteration client fn
	} {
		if ref := moduleFnRef(t, modReg, name); ref == nil || ref.Prog == nil {
			t.Errorf("armed load must stamp %s", name)
		}
		if moduleFnRef(t, plainMod, name) != nil {
			t.Errorf("unarmed load must NOT stamp %s", name)
		}
	}
	// Context note, deliberately NOT pinned either way: redis-connect
	// stamps under a CHECK-MODE import (the CLI's RunCompiled path) but not
	// under this harness's RUNTIME import — the module body's fn values are
	// constructed under different modes and its `Net.connect {…}` body sits
	// right at the provenance frontier. Both outcomes are sound (a decline
	// interprets, byte-identically); the differential below is the real
	// contract.

	// (b) Differential: byte-identical replies across the whole battery.
	for i, c := range cmds {
		if armedReplies[i] != plainReplies[i] {
			t.Errorf("cmd %q: armed=%q plain=%q", c, armedReplies[i], plainReplies[i])
		}
	}
	// Spot-pin a few absolute replies so both engines being wrong together
	// cannot pass.
	pins := map[string]string{
		"PING":                     "+PONG",
		"SET greeting hello world": "+OK",
		"GET greeting":             "hello world",
		"BOGUSCMD arg":             "-ERR unknown command 'BOGUSCMD'",
	}
	for i, c := range cmds {
		if want, pinned := pins[c]; pinned && armedReplies[i] != want {
			t.Errorf("cmd %q: reply %q, want %q", c, armedReplies[i], want)
		}
	}

	// (c) EXPIRE/TTL: clock-tolerant — both modes must land in [95, 100].
	for _, armed := range []bool{true, false} {
		replies, _ := miniRedisSession(t, armed, []string{
			"SET tkey v", "EXPIRE tkey 100", "TTL tkey",
		})
		ttl, err := strconv.Atoi(strings.TrimPrefix(replies[2], ":"))
		if err != nil || ttl < 95 || ttl > 100 {
			t.Errorf("armed=%v: TTL reply %q outside [95,100]", armed, replies[2])
		}
	}
}
