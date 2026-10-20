package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// End-to-end tests for the networking verification apps written in boru
// (design/examples/apps/): the todo REST API, the mini-redis server, and
// the streaming/resumable mini-S3. Each test loads the example file as a
// FILE MODULE (exercising the file-module loader too) and drives it over
// a real loopback socket on an ephemeral port.

// runAppSteps loads the named example app into an in-memory FS at
// /apps/<name> and runs the steps on a fully-wired registry (module
// resolver + parser + fileops).
func runAppSteps(t *testing.T, appFiles []string, steps []string) ([]native.Value, error) {
	t.Helper()
	mem := capabilities.NewMem()
	for _, name := range appFiles {
		src, err := os.ReadFile("../../../design/examples/apps/" + name)
		if err != nil {
			t.Fatalf("read app %s: %v", name, err)
		}
		mem.Files[filepath.FromSlash("/apps/"+name)] = src // the model's OS-native key
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	native.SetHostFileOps(reg, mem)
	modules.InstallResolver(reg)
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func appLastString(t *testing.T, vals []native.Value) string {
	t.Helper()
	if len(vals) == 0 {
		t.Fatal("no result")
	}
	s, err := core.AsString(vals[len(vals)-1])
	if err != nil {
		t.Fatalf("expected String result, got %v", vals)
	}
	return s
}

// The todo REST API (todo-api.boru): CRUD over the http codec with :id
// route params, JSON bodies, tombstone deletes, and the request-count
// wrap middleware — the NETWORK-SERVERS.0.md §6.4 shape end to end.
func TestAppTodoAPI(t *testing.T) {
	out, err := runAppSteps(t, []string{"todo-api.boru"}, []string{
		`import "/apps/todo-api.boru"`,
		`import "boru:net"`,
		`def ln (TodoApi.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.http})`,
		// create
		`def c1 (call {method:"POST" path:"/todos" headers:{content-type:"application/json"} body:"{\"text\": \"buy milk\"}"} ep {timeout: 5000})`,
		// create without a body → 400
		`def c2 (call {method:"POST" path:"/todos"} ep {timeout: 5000})`,
		// read one (route param), update, list, delete, re-read (404), stats
		`def g1 (call {method:"GET" path:"/todos/1"} ep {timeout: 5000})`,
		`def m1 (call {method:"PUT" path:"/todos/1" headers:{content-type:"application/json"} body:"{\"done\": true}"} ep {timeout: 5000})`,
		`def l1 (call {method:"GET" path:"/todos"} ep {timeout: 5000})`,
		`def d1 (call {method:"DELETE" path:"/todos/1"} ep {timeout: 5000})`,
		`def g2 (call {method:"GET" path:"/todos/1"} ep {timeout: 5000})`,
		`def s1 (call {method:"GET" path:"/stats"} ep {timeout: 5000})`,
		`Net.close ep;`,
		`Net.close ln;`,
		`join "|" [(convert String c1.status) (convert String c2.status) (convert String g1.body)
		           (convert String m1.body) (convert String l1.body) (convert String d1.status)
		           (convert String g2.status) (convert String s1.body)]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	if len(parts) != 8 {
		t.Fatalf("expected 8 fields, got %q", got)
	}
	if parts[0] != "201" {
		t.Errorf("POST status = %s, want 201", parts[0])
	}
	if parts[1] != "400" {
		t.Errorf("bodyless POST status = %s, want 400", parts[1])
	}
	if !strings.Contains(parts[2], `"buy milk"`) || !strings.Contains(parts[2], `"done":false`) {
		t.Errorf("GET one = %s", parts[2])
	}
	if !strings.Contains(parts[3], `"done":true`) {
		t.Errorf("PUT reply = %s, want done:true", parts[3])
	}
	if !strings.Contains(parts[4], `"buy milk"`) {
		t.Errorf("list = %s", parts[4])
	}
	if parts[5] != "204" {
		t.Errorf("DELETE status = %s, want 204", parts[5])
	}
	if parts[6] != "404" {
		t.Errorf("GET after delete = %s, want 404", parts[6])
	}
	if !strings.Contains(parts[7], "hits") {
		t.Errorf("stats = %s, want a hits count from the wrap middleware", parts[7])
	}
}

// The mini-redis server (mini-redis.boru): the common Redis commands
// over a custom boru codec (inline commands in, RESP-flavoured lines
// out) — the NETWORK-SERVERS.0.md §6.6 custom-protocol story as a
// real app. The client side is plain Net.lines.
func TestAppMiniRedis(t *testing.T) {
	out, err := runAppSteps(t, []string{"mini-redis.boru"}, []string{
		`import "/apps/mini-redis.boru"`,
		`import "boru:net"`,
		`def ln (MiniRedis.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (MiniRedis.connect (join "" ["127.0.0.1:" (convert String port)]))`,
		`def r-ping (MiniRedis.cmd ep "PING")`,
		`def r-set (MiniRedis.cmd ep "SET greeting hello world")`,
		`def r-get (MiniRedis.cmd ep "GET greeting")`,
		`def r-exists (MiniRedis.cmd ep "EXISTS greeting")`,
		`def r-incr1 (MiniRedis.cmd ep "INCR counter")`,
		`def r-incr2 (MiniRedis.cmd ep "INCR counter")`,
		`def r-keys (MiniRedis.cmd ep "KEYS *")`,
		`def r-del (MiniRedis.cmd ep "DEL greeting")`,
		`def r-get2 (MiniRedis.cmd ep "GET greeting")`,
		`def r-keys2 (MiniRedis.cmd ep "KEYS *")`,
		`MiniRedis.cmd ep "RPUSH mylist a" drop`,
		`MiniRedis.cmd ep "RPUSH mylist b" drop`,
		`def r-lpush (MiniRedis.cmd ep "LPUSH mylist z")`,
		`def r-lrange (MiniRedis.cmd ep "LRANGE mylist 0 -1")`,
		`def r-llen (MiniRedis.cmd ep "LLEN mylist")`,
		`MiniRedis.cmd ep "HSET h f1 v1" drop`,
		`def r-hget (MiniRedis.cmd ep "HGET h f1")`,
		`def r-hmiss (MiniRedis.cmd ep "HGET h nope")`,
		`def r-expire (MiniRedis.cmd ep "EXPIRE counter 100")`,
		`def r-ttl (MiniRedis.cmd ep "TTL counter")`,
		`def r-ttl-none (MiniRedis.cmd ep "TTL mylist")`,
		// lazy expiry: EXPIRE 0 lapses immediately
		`MiniRedis.cmd ep "EXPIRE counter 0" drop`,
		`def r-lapsed (MiniRedis.cmd ep "GET counter")`,
		`def r-bogus (MiniRedis.cmd ep "BOGUS x")`,
		`Net.close ep;`,
		`Net.close ln;`,
		`join "|" [r-ping r-set r-get r-exists r-incr1 r-incr2 r-keys r-del r-get2 r-keys2
		           r-lpush r-lrange r-llen r-hget r-hmiss r-expire r-ttl r-ttl-none r-bogus r-lapsed]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	want := []struct {
		i    int
		name string
		val  string
	}{
		{0, "PING", "+PONG"},
		{1, "SET", "+OK"},
		{2, "GET", "hello world"},
		{3, "EXISTS", ":1"},
		{4, "INCR", ":1"},
		{5, "INCR again", ":2"},
		{7, "DEL", ":1"},
		{8, "GET after DEL", "$-1"},
		{9, "KEYS after DEL", "counter"},
		{10, "LPUSH", ":3"},
		{11, "LRANGE 0 -1", "z a b"},
		{12, "LLEN", ":3"},
		{13, "HGET", "v1"},
		{14, "HGET miss", "$-1"},
		{15, "EXPIRE", ":1"},
	}
	if len(parts) != 20 {
		t.Fatalf("expected 20 replies, got %d: %q", len(parts), got)
	}
	for _, w := range want {
		if parts[w.i] != w.val {
			t.Errorf("%s = %q, want %q", w.name, parts[w.i], w.val)
		}
	}
	if !strings.Contains(parts[6], "greeting") || !strings.Contains(parts[6], "counter") {
		t.Errorf("KEYS = %q, want both keys", parts[6])
	}
	if parts[16] != ":99" && parts[16] != ":100" {
		t.Errorf("TTL = %q, want :99 or :100 (seconds remaining)", parts[16])
	}
	if parts[17] != ":-2" {
		t.Errorf("TTL of a non-string key = %q, want :-2", parts[17])
	}
	if !strings.Contains(parts[18], "-ERR") || !strings.Contains(parts[18], "BOGUS") {
		t.Errorf("unknown command = %q, want -ERR naming it", parts[18])
	}
	if parts[19] != "$-1" {
		t.Errorf("GET after EXPIRE 0 = %q, want $-1 (lazy expiry)", parts[19])
	}
}

// The mini-S3 object store (mini-s3.boru) and its raw-socket client
// (mini-s3-client.boru) — both on the LOW-LEVEL tier: hand-framed HTTP
// over recv-until/recv-bytes, bodies streamed in bounded 64 KiB chunks
// both ways, and RESUMABLE uploads/downloads (HEAD → x-size resume
// point, PUT x-offset with 409 on a wrong offset, GET with Range → 206).
func TestAppMiniS3StreamingAndResumption(t *testing.T) {
	out, err := runAppSteps(t, []string{"mini-s3.boru", "mini-s3-client.boru"}, []string{
		`import "/apps/mini-s3.boru"`,
		`import "/apps/mini-s3-client.boru"`,
		`import "boru:net"`,
		`import "boru:string-util"`,
		`def ln (MiniS3.serve {port: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (MiniS3Client.dial (join "" ["127.0.0.1:" (convert String port)]))`,
		// a 131072-byte object, uploaded as an interrupted 100000-byte
		// part followed by a resumed remainder
		`def big (convert Bytes (StringUtil.repeat 8192 "0123456789abcdef"))`,
		`def part1 (slice 0 100000 big)`,
		`def part2 (slice 100000 131072 big)`,
		`def r1 (MiniS3Client.req sock "PUT" "/b/data.bin" [] part1)`,
		`def h1 (MiniS3Client.req sock "HEAD" "/b/data.bin" [] (convert Bytes ""))`,
		`def h1s (h1.headers get "x-size")`,
		`def r2 (MiniS3Client.req sock "PUT" "/b/data.bin" ["x-offset: 0"] part2)`,
		`def r2s (r2.headers get "x-size")`,
		`def r3 (MiniS3Client.req sock "PUT" "/b/data.bin" ["x-offset: 100000"] part2)`,
		`def r3s (r3.headers get "x-size")`,
		`def g1 (MiniS3Client.req sock "GET" "/b/data.bin" [] (convert Bytes ""))`,
		`def g2 (MiniS3Client.req sock "GET" "/b/data.bin" ["range: bytes=5-14"] (convert Bytes ""))`,
		`def g2r (g2.headers get "content-range")`,
		`def g3 (MiniS3Client.req sock "GET" "/b/data.bin" ["range: bytes=131062-"] (convert Bytes ""))`,
		`def g4 (MiniS3Client.req sock "GET" "/b/data.bin" ["range: bytes=999999-"] (convert Bytes ""))`,
		`MiniS3Client.req sock "PUT" "/b/other.txt" [] (convert Bytes "hi") drop`,
		`def l1 (MiniS3Client.req sock "GET" "/b" [] (convert Bytes ""))`,
		`def d1 (MiniS3Client.req sock "DELETE" "/b/other.txt" [] (convert Bytes ""))`,
		`def g5 (MiniS3Client.req sock "GET" "/b/other.txt" [] (convert Bytes ""))`,
		`Net.close sock;`,
		`Net.close ln;`,
		`join "|" [(convert String r1.status) h1s (convert String r2.status) r2s
		           (convert String r3.status) r3s
		           (convert String g1.status) (convert String (size g1.body)) (convert String (g1.body eq big))
		           (convert String g2.status) (convert String g2.body) g2r
		           (convert String g3.status) (convert String g3.body)
		           (convert String g4.status)
		           (convert String l1.body)
		           (convert String d1.status) (convert String g5.status)]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := appLastString(t, out)
	parts := strings.Split(got, "|")
	if len(parts) != 18 {
		t.Fatalf("expected 18 fields, got %d: %q", len(parts), got)
	}
	want := []struct {
		i    int
		name string
		val  string
	}{
		{0, "initial PUT status", "200"},
		{1, "HEAD x-size (resume point)", "100000"},
		{2, "wrong-offset PUT status", "409"},
		{3, "409 x-size tells the client where to resume", "100000"},
		{4, "resumed PUT status", "200"},
		{5, "final size after resume", "131072"},
		{6, "GET status", "200"},
		{7, "GET size", "131072"},
		{8, "streamed round-trip equality", "true"},
		{9, "range GET status", "206"},
		{10, "range body", "56789abcde"},
		{11, "content-range", "bytes 5-14/131072"},
		{12, "open-ended range status", "206"},
		{13, "open-ended range body (last 10 bytes)", "6789abcdef"},
		{14, "unsatisfiable range status", "416"},
		{16, "DELETE status", "204"},
		{17, "GET after delete", "404"},
	}
	for _, w := range want {
		if parts[w.i] != w.val {
			t.Errorf("%s = %q, want %q", w.name, parts[w.i], w.val)
		}
	}
	if !strings.Contains(parts[15], "b/data.bin") || !strings.Contains(parts[15], "b/other.txt") {
		t.Errorf("bucket listing = %q, want both keys", parts[15])
	}
}
