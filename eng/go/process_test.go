package eng

import (
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
)

// The process substrate (process.go): bounded mailboxes, overflow
// policies, the name registry, and shutdown wake-ups. The language-level
// words are covered in lang/go/test/process_service_test.go; these tests
// pin the kernel mailbox discipline directly, including the negative
// paths (overflow compile failure, shutdown errors) per the repo's
// positive/negative pairing rule.

func TestProcessMailboxFIFO(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 8, core.OverflowBlock)
	if err := rt.Insert(p); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 3; i++ {
		if err := p.Send(core.NewInteger(i)); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	for i := int64(1); i <= 3; i++ {
		msg, ok, err := p.PopFront(0, false)
		if err != nil || !ok {
			t.Fatalf("pop %d: ok=%v err=%v", i, ok, err)
		}
		if n, _ := core.AsInteger(msg); n != i {
			t.Errorf("pop %d: got %d (mailbox must be FIFO)", i, n)
		}
	}
}

func TestProcessMailboxOverflowFail(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 1, core.OverflowFail)
	_ = rt.Insert(p)
	if err := p.Send(core.NewInteger(1)); err != nil {
		t.Fatalf("first send must fit: %v", err)
	}
	if err := p.Send(core.NewInteger(2)); err != core.ErrMailboxOverload {
		t.Errorf("second send into a full 'fail' mailbox: got %v, want ErrMailboxOverload", err)
	}
	// After draining one, sends succeed again.
	if _, ok, _ := p.PopFront(0, false); !ok {
		t.Fatal("drain failed")
	}
	if err := p.Send(core.NewInteger(3)); err != nil {
		t.Errorf("send after drain: %v", err)
	}
}

func TestProcessMailboxOverflowDrop(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 1, core.OverflowDrop)
	_ = rt.Insert(p)
	_ = p.Send(core.NewInteger(1))
	if err := p.Send(core.NewInteger(2)); err != nil {
		t.Fatalf("drop overflow must not error: %v", err)
	}
	msg, ok, _ := p.PopFront(0, false)
	if !ok {
		t.Fatal("pop failed")
	}
	if n, _ := core.AsInteger(msg); n != 1 {
		t.Errorf("kept message = %d, want 1 (the NEW message is dropped)", n)
	}
	if p.MailboxDepth() != 0 {
		t.Errorf("depth = %d, want 0", p.MailboxDepth())
	}
}

func TestProcessMailboxOverflowBlockUnblocksOnDrain(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 1, core.OverflowBlock)
	_ = rt.Insert(p)
	_ = p.Send(core.NewInteger(1))
	done := make(chan error, 1)
	go func() { done <- p.Send(core.NewInteger(2)) }()
	select {
	case err := <-done:
		t.Fatalf("blocked send returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if _, ok, _ := p.PopFront(0, false); !ok {
		t.Fatal("drain failed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unblocked send errored: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked send never unblocked after drain")
	}
}

func TestProcessPopFrontDeadline(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 4, core.OverflowBlock)
	_ = rt.Insert(p)
	start := time.Now()
	_, ok, err := p.PopFront(60*time.Millisecond, true)
	if err != nil {
		t.Fatalf("deadline pop errored: %v", err)
	}
	if ok {
		t.Fatal("empty mailbox returned a message")
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("deadline fired after %v, want >= ~60ms", elapsed)
	}
}

func TestProcessSendToClosedIsDropped(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 4, core.OverflowBlock)
	_ = rt.Insert(p)
	p.Close()
	if err := p.Send(core.NewInteger(1)); err != nil {
		t.Errorf("send to closed process must be a silent drop, got %v", err)
	}
	if p.MailboxDepth() != 0 {
		t.Error("closed mailbox accepted a message")
	}
}

func TestProcessRuntimeShutdownWakesReceiver(t *testing.T) {
	rt := core.NewProcessRuntime()
	p := core.NewProcess(rt, 4, core.OverflowBlock)
	_ = rt.Insert(p)
	done := make(chan error, 1)
	go func() {
		_, _, err := p.PopFront(0, false)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	rt.Shutdown()
	select {
	case err := <-done:
		if err != core.ErrRuntimeDown {
			t.Errorf("blocked receive after shutdown: got %v, want ErrRuntimeDown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked receive never woke on shutdown")
	}
	// Inserting into a down runtime is declined.
	if err := rt.Insert(core.NewProcess(rt, 1, core.OverflowBlock)); err != core.ErrRuntimeDown {
		t.Errorf("insert after shutdown: got %v, want ErrRuntimeDown", err)
	}
}

func TestProcessNameRegistry(t *testing.T) {
	rt := core.NewProcessRuntime()
	a := core.NewProcess(rt, 4, core.OverflowBlock)
	b := core.NewProcess(rt, 4, core.OverflowBlock)
	_ = rt.Insert(a)
	_ = rt.Insert(b)

	if err := rt.RegisterName("worker", a); err != nil {
		t.Fatal(err)
	}
	if got, ok := rt.Whereis("worker"); !ok || got != a {
		t.Fatal("whereis after register")
	}
	// Re-register rebinds; the previous holder loses the name.
	if err := rt.RegisterName("worker", b); err != nil {
		t.Fatal(err)
	}
	if got, _ := rt.Whereis("worker"); got != b {
		t.Fatal("re-register did not rebind")
	}
	if a.Name() != "" {
		t.Error("previous holder kept the name")
	}
	// Removing a process drops its name.
	rt.Remove(b)
	if _, ok := rt.Whereis("worker"); ok {
		t.Error("name survived process removal")
	}
	// Unknown names are a miss, and unregistering them is a no-op.
	if _, ok := rt.Whereis("nope"); ok {
		t.Error("unknown name resolved")
	}
	rt.UnregisterName("nope")
}

func TestForkConcurrentSharesProcessRuntime(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if r.Procs == nil {
		t.Fatal("NewRegistry must create the process runtime")
	}
	fork := r.ForkConcurrent()
	if fork.Procs != r.Procs {
		t.Error("ForkConcurrent must share the parent's ProcessRuntime pointer")
	}
}
