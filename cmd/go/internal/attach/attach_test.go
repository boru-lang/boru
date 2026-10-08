// Session tests for the attach viewer: the dial and backend seams are
// swapped for a net.Pipe wire plus a VirtualBackend, so every arm of
// the handshake, the frame/title/bell/quit loop, and the local-event
// upstream runs against a scripted server — negatives paired
// throughout.
package attach

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/tuikit"
)

// wireEnd is the scripted server's half of the pipe: a background
// scanner feeds decoded client lines so pipe writes never deadlock.
type wireEnd struct {
	conn  net.Conn
	lines chan map[string]any
}

func newWireEnd(conn net.Conn) *wireEnd {
	w := &wireEnd{conn: conn, lines: make(chan map[string]any, 32)}
	go func() {
		defer close(w.lines)
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 0, 4096), wireLimit)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				w.lines <- m
			}
		}
	}()
	return w
}

func (w *wireEnd) recv(t *testing.T) map[string]any {
	t.Helper()
	select {
	case m, ok := <-w.lines:
		if !ok {
			t.Fatal("client closed the wire")
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no client line")
		return nil
	}
}

func (w *wireEnd) send(t *testing.T, m map[string]any) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.conn.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

// failWriteConn declines writes once tripped, forcing the upstream
// event-write error arm without racing the downstream reader.
type failWriteConn struct {
	net.Conn
	fail atomic.Bool
}

func (c *failWriteConn) Write(b []byte) (int, error) {
	if c.fail.Load() {
		return 0, errors.New("wire write declined")
	}
	return c.Conn.Write(b)
}

func swapSeams(t *testing.T, vb *tuikit.VirtualBackend, conn net.Conn, dialErr error) {
	t.Helper()
	oldDial, oldBackend := dialFn, newBackend
	t.Cleanup(func() { dialFn, newBackend = oldDial, oldBackend })
	newBackend = func() tuikit.Backend { return vb }
	dialFn = func(string) (net.Conn, error) {
		if dialErr != nil {
			return nil, dialErr
		}
		return conn, nil
	}
}

// runSession swaps the seams onto a fresh pipe, runs script(server) in
// the background, and returns attach's error. The client end may be
// wrapped before dialing via wrap.
func runSession(t *testing.T, vb *tuikit.VirtualBackend, wrap func(net.Conn) net.Conn, script func(*wireEnd)) error {
	t.Helper()
	clientEnd, serverEnd := net.Pipe()
	if wrap != nil {
		clientEnd = wrap(clientEnd)
	}
	swapSeams(t, vb, clientEnd, nil)
	s := newWireEnd(serverEnd)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverEnd.Close()
		script(s)
	}()
	err := attach("pipe", "tok")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server script did not finish")
	}
	return err
}

func acceptHello(t *testing.T, s *wireEnd) {
	t.Helper()
	hello := s.recv(t)
	if hello["tag"] != "attach" || hello["token"] != "tok" || hello["proto"] != float64(1) {
		t.Errorf("hello = %v", hello)
	}
	s.send(t, map[string]any{"tag": "accept", "proto": 1})
}

func TestAttachCommandSurface(t *testing.T) {
	c := New()
	if c.Name() != "attach" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Synopsis() == "" {
		t.Error("Synopsis() is empty")
	}
	var stdout, stderr bytes.Buffer
	if code := c.Run([]string{"--bogus"}, nil, &stdout, &stderr); code != 1 {
		t.Errorf("bad flag Run = %d, want 1", code)
	}
	stderr.Reset()
	if code := c.Run(nil, nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("no-arg Run = %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := c.Run([]string{"a", "b"}, nil, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("two-arg Run = %d, stderr %q", code, stderr.String())
	}
}

// The full choreography through Run: handshake, five upstream event
// shapes, title, a cursor frame, a plain frame, bell, quit.
func TestAttachRunSession(t *testing.T) {
	vb := tuikit.NewVirtualBackend(20, 4)
	for _, ev := range []tuikit.Event{
		{Tag: "key", Key: "a", Char: "a", Mods: []string{"ctrl"}},
		{Tag: "resize", Cols: 30, Rows: 5},
		{Tag: "mouse", Kind: "press", X: 3, Y: 1},
		{Tag: "paste", Text: "pasted"},
		{Tag: "focus", Gained: true},
	} {
		vb.Inject(ev)
	}
	clientEnd, serverEnd := net.Pipe()
	swapSeams(t, vb, clientEnd, nil)
	s := newWireEnd(serverEnd)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverEnd.Close()
		acceptHello(t, s)
		key := s.recv(t)
		mods, _ := key["mods"].([]any)
		if key["tag"] != "key" || key["key"] != "a" || len(mods) != 1 || mods[0] != "ctrl" {
			t.Errorf("key up = %v", key)
		}
		if rs := s.recv(t); rs["tag"] != "resize" || rs["cols"] != float64(30) || rs["rows"] != float64(5) {
			t.Errorf("resize up = %v", rs)
		}
		if ms := s.recv(t); ms["tag"] != "mouse" || ms["kind"] != "press" || ms["x"] != float64(3) || ms["y"] != float64(1) {
			t.Errorf("mouse up = %v", ms)
		}
		if ps := s.recv(t); ps["tag"] != "paste" || ps["text"] != "pasted" {
			t.Errorf("paste up = %v", ps)
		}
		if fs := s.recv(t); fs["tag"] != "focus" || fs["gained"] != true {
			t.Errorf("focus up = %v", fs)
		}
		s.send(t, map[string]any{"tag": "title", "text": "remote"})
		s.send(t, map[string]any{"tag": "frame", "tree": map[string]any{
			"w": "input", "value": "ab", "cursor": 2, "focus": true}})
		s.send(t, map[string]any{"tag": "frame", "tree": map[string]any{
			"w": "text", "text": "hi"}})
		s.send(t, map[string]any{"tag": "bell"})
		s.send(t, map[string]any{"tag": "quit"})
	}()
	var stdout, stderr bytes.Buffer
	code := New().Run([]string{"--token", "tok", "pipe"}, nil, &stdout, &stderr)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server script did not finish")
	}
	if code != 0 || !strings.Contains(stdout.String(), "session ended") {
		t.Fatalf("Run = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if vb.Title() != "remote" {
		t.Errorf("title = %q", vb.Title())
	}
	if vb.Presents() != 2 || vb.Bells() != 1 {
		t.Errorf("presents = %d bells = %d", vb.Presents(), vb.Bells())
	}
	if first := strings.Join(vb.ScreenAt(0), "\n"); !strings.Contains(first, "ab") {
		t.Errorf("first frame = %q", first)
	}
	if last := strings.Join(vb.Screen(), "\n"); !strings.Contains(last, "hi") {
		t.Errorf("last frame = %q", last)
	}
	if _, _, visible := vb.Cursor(); visible {
		t.Error("cursor still visible after the plain frame")
	}
}

func TestAttachOpenAndDialFailures(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	vb.OpenErr = errors.New("no local terminal")
	swapSeams(t, vb, nil, nil)
	var stdout, stderr bytes.Buffer
	if code := New().Run([]string{"x"}, nil, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "attach: no local terminal") {
		t.Fatalf("open failure Run = %d, stderr %q", code, stderr.String())
	}

	swapSeams(t, tuikit.NewVirtualBackend(4, 2), nil, errors.New("no route"))
	if err := attach("x", ""); err == nil || !strings.Contains(err.Error(), "no route") {
		t.Fatalf("dial failure = %v", err)
	}
}

func TestAttachHandshakeArms(t *testing.T) {
	// the server hangs up before the hello line lands
	clientEnd, serverEnd := net.Pipe()
	_ = serverEnd.Close()
	swapSeams(t, tuikit.NewVirtualBackend(4, 2), clientEnd, nil)
	if err := attach("pipe", ""); err == nil || !strings.Contains(err.Error(), "closed pipe") {
		t.Fatalf("dead pipe = %v", err)
	}

	// the server reads the hello then closes without replying
	err := runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		_ = s.recv(t)
	})
	if err == nil || !strings.Contains(err.Error(), "connection closed during handshake") {
		t.Fatalf("silent close = %v", err)
	}

	// an explicit denial
	err = runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		_ = s.recv(t)
		s.send(t, map[string]any{"tag": "deny", "why": "busy"})
	})
	if err == nil || !strings.Contains(err.Error(), "denied: busy") {
		t.Fatalf("deny = %v", err)
	}

	// a reply that is neither accept nor deny
	err = runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		_ = s.recv(t)
		s.send(t, map[string]any{"tag": "weird"})
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected handshake reply weird") {
		t.Fatalf("weird reply = %v", err)
	}
}

func TestAttachLoopArms(t *testing.T) {
	textFrame := map[string]any{"tag": "frame", "tree": map[string]any{"w": "text", "text": "x"}}
	cursorFrame := map[string]any{"tag": "frame", "tree": map[string]any{
		"w": "input", "value": "a", "cursor": 1, "focus": true}}

	// a tree the renderer rejects
	err := runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, map[string]any{"tag": "frame", "tree": 5})
	})
	if err == nil || !strings.Contains(err.Error(), "bad frame") {
		t.Fatalf("bad frame = %v", err)
	}

	// present failure
	vb := tuikit.NewVirtualBackend(4, 2)
	vb.PresentErr = errors.New("present declined")
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, textFrame)
	})
	if err == nil || !strings.Contains(err.Error(), "present declined") {
		t.Fatalf("present failure = %v", err)
	}

	// cursor failure on the visible-cursor frame…
	vb = tuikit.NewVirtualBackend(4, 2)
	vb.CursorErr = errors.New("cursor declined")
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, cursorFrame)
	})
	if err == nil || !strings.Contains(err.Error(), "cursor declined") {
		t.Fatalf("visible-cursor failure = %v", err)
	}
	// …and on the hidden-cursor frame
	vb = tuikit.NewVirtualBackend(4, 2)
	vb.CursorErr = errors.New("cursor declined")
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, textFrame)
	})
	if err == nil || !strings.Contains(err.Error(), "cursor declined") {
		t.Fatalf("hidden-cursor failure = %v", err)
	}

	// title failure; a non-string title is skipped (session then quits)
	vb = tuikit.NewVirtualBackend(4, 2)
	vb.TitleErr = errors.New("title declined")
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, map[string]any{"tag": "title", "text": "x"})
	})
	if err == nil || !strings.Contains(err.Error(), "title declined") {
		t.Fatalf("title failure = %v", err)
	}
	err = runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, map[string]any{"tag": "title", "text": 5})
		s.send(t, map[string]any{"tag": "quit"})
	})
	if err != nil {
		t.Fatalf("non-string title = %v", err)
	}

	// bell failure
	vb = tuikit.NewVirtualBackend(4, 2)
	vb.BellErr = errors.New("bell declined")
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, map[string]any{"tag": "bell"})
	})
	if err == nil || !strings.Contains(err.Error(), "bell declined") {
		t.Fatalf("bell failure = %v", err)
	}

	// the server vanishing mid-session
	err = runSession(t, tuikit.NewVirtualBackend(4, 2), nil, func(s *wireEnd) {
		acceptHello(t, s)
	})
	if err == nil || !strings.Contains(err.Error(), "server closed the connection") {
		t.Fatalf("server close = %v", err)
	}

	// the local terminal vanishing ends the session cleanly
	vb = tuikit.NewVirtualBackend(4, 2)
	err = runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		_ = vb.Close()
		for range s.lines { // drain until the client hangs up
		}
	})
	if err != nil {
		t.Fatalf("local close = %v", err)
	}

	// an upstream event write failing after the handshake
	vb = tuikit.NewVirtualBackend(4, 2)
	var fc *failWriteConn
	err = runSession(t, vb, func(c net.Conn) net.Conn {
		fc = &failWriteConn{Conn: c}
		return fc
	}, func(s *wireEnd) {
		acceptHello(t, s)
		fc.fail.Store(true)
		vb.Inject(tuikit.Event{Tag: "key", Key: "x", Char: "x"})
		for range s.lines { // hold the conn open until the client fails
		}
	})
	if err == nil || !strings.Contains(err.Error(), "wire write declined") {
		t.Fatalf("event write failure = %v", err)
	}
}

// The production seam defaults: a real TCP dial (paired with a
// malformed-address failure) and a real termback construction.
func TestAttachSeamDefaults(t *testing.T) {
	if b := newBackend(); b == nil {
		t.Fatal("newBackend returned nil")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if conn, aErr := ln.Accept(); aErr == nil {
			_ = conn.Close()
		}
	}()
	conn, dErr := dialFn(ln.Addr().String())
	if dErr != nil {
		t.Fatalf("default dial: %v", dErr)
	}
	_ = conn.Close()
	if _, dErr := dialFn("definitely-not-an-address"); dErr == nil {
		t.Fatal("malformed-address dial succeeded")
	}
}

// A local resize re-geometries subsequent frame renders (review
// finding: the client used the Open-time dimensions forever, clipping
// resized sessions until reconnect).
func TestAttachResizeUpdatesRenderGeometry(t *testing.T) {
	vb := tuikit.NewVirtualBackend(10, 3)
	err := runSession(t, vb, nil, func(s *wireEnd) {
		acceptHello(t, s)
		s.send(t, map[string]any{"tag": "frame", "tree": map[string]any{"w": "text", "text": "a"}})
		// the viewer grows; the server hears about it…
		vb.Inject(tuikit.Event{Tag: "resize", Cols: 30, Rows: 6})
		if rs := s.recv(t); rs["tag"] != "resize" || rs["cols"] != float64(30) {
			t.Errorf("resize upstream = %v", rs)
		}
		// …and the NEXT frame lays out at the new size
		s.send(t, map[string]any{"tag": "frame", "tree": map[string]any{"w": "text", "text": "b"}})
		s.send(t, map[string]any{"tag": "quit"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if vb.Presents() != 2 {
		t.Fatalf("presents = %d", vb.Presents())
	}
	if last := vb.Screen(); len(last) != 6 || len([]rune(last[0])) != 30 {
		t.Fatalf("post-resize frame = %dx%d rows, want 30x6", len([]rune(last[0])), len(last))
	}
}
