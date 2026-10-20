package vault

// audit_seam7_test.go — seam-7 coverage for audit.go: appendAudit's
// mkdir/open failure arms, defaultActor's "local" fallback (via the
// c7userCurrent seam), and runAudit / ReadAudit error and filter arms.

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"runtime"
	"strings"
	"testing"
)

// c7writeAudit writes raw JSONL bytes to the audit log at home, creating
// the vault folder first.
func c7writeAudit(t *testing.T, home, content string) {
	t.Helper()
	if err := os.MkdirAll(vaultFolder(home), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auditPath(home), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestC7AppendAuditMkdirFails(t *testing.T) {
	testHome(t) // neutralize EnvFolder/EnvSuffix
	home := t.TempDir()
	// Make .boru a regular file so MkdirAll(vaultFolder) fails with ENOTDIR.
	if err := os.WriteFile(vaultFolder(home), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	err := appendAudit(home, AuditEvent{Action: "test"})
	if err == nil {
		t.Fatal("expected MkdirAll failure, got nil")
	}
}

func TestC7AppendAuditOpenFails(t *testing.T) {
	testHome(t)
	home := t.TempDir()
	if err := os.MkdirAll(vaultFolder(home), 0700); err != nil {
		t.Fatal(err)
	}
	// Make the audit path a directory so OpenFile fails with EISDIR.
	if err := os.MkdirAll(auditPath(home), 0700); err != nil {
		t.Fatal(err)
	}
	err := appendAudit(home, AuditEvent{Action: "test"})
	if err == nil {
		t.Fatal("expected OpenFile failure, got nil")
	}
}

func TestC7DefaultActorFallback(t *testing.T) {
	old := c7userCurrent
	t.Cleanup(func() { c7userCurrent = old })

	c7userCurrent = func() (*user.User, error) { return nil, errors.New("no user") }
	if got := defaultActor(); got != "local" {
		t.Errorf("defaultActor with failing user.Current = %q, want local", got)
	}
	c7userCurrent = func() (*user.User, error) { return &user.User{Username: ""}, nil }
	if got := defaultActor(); got != "local" {
		t.Errorf("defaultActor with empty username = %q, want local", got)
	}
	c7userCurrent = func() (*user.User, error) { return &user.User{Username: "alice"}, nil }
	if got := defaultActor(); got != "user:alice" {
		t.Errorf("defaultActor with username = %q, want user:alice", got)
	}
}

func TestC7RunAuditFlagParseError(t *testing.T) {
	home := testHome(t)
	var out, errb bytes.Buffer
	if code := runAudit([]string{"-nope-not-a-flag"}, home, &out, &errb); code != 1 {
		t.Fatalf("bad flag: code=%d", code)
	}
}

func TestC7RunAuditNamespaceFilterError(t *testing.T) {
	home := testHome(t)
	var out, errb bytes.Buffer
	if code := runAudit([]string{"--namespace=!bad"}, home, &out, &errb); code != 1 {
		t.Fatalf("bad namespace: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "invalid namespace") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7RunAuditOpenNonNotExistError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a file used as a directory as path-not-found (IsNotExist), so it cannot provoke this error")
	}
	testHome(t)
	home := t.TempDir()
	// .boru is a file, so auditPath = <file>/vault.audit.jsonl → ENOTDIR (not IsNotExist).
	if err := os.WriteFile(vaultFolder(home), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := runAudit(nil, home, &out, &errb); code != 1 {
		t.Fatalf("expected open error, code=%d out=%q", code, out.String())
	}
	if !strings.Contains(errb.String(), "error:") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestC7RunAuditDecodeMalformedLine(t *testing.T) {
	home := testHome(t)
	// A bare number decodes past itself but mismatches the struct, exercising
	// the "skipping malformed audit line" arm without stalling the decoder.
	c7writeAudit(t, home, "123\n{\"ts\":\"t\",\"action\":\"a\",\"actor\":\"me\"}\n")
	var out, errb bytes.Buffer
	if code := runAudit(nil, home, &out, &errb); code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "skipping malformed audit line") {
		t.Errorf("expected malformed warning, stderr=%q", errb.String())
	}
	if !strings.Contains(out.String(), "action=a") {
		t.Errorf("valid line not printed: %q", out.String())
	}
}

func TestC7RunAuditFilters(t *testing.T) {
	home := testHome(t)
	c7writeAudit(t, home,
		"{\"ts\":\"t1\",\"action\":\"login\",\"actor\":\"root\"}\n"+
			"{\"ts\":\"t2\",\"action\":\"proxy\",\"actor\":\"agent\",\"alias\":\"proj:k\"}\n")

	// namespace filter: the no-alias event is skipped (ev.Alias == "").
	var out, errb bytes.Buffer
	if code := runAudit([]string{"--namespace=proj"}, home, &out, &errb); code != 0 {
		t.Fatalf("ns filter code=%d err=%q", code, errb.String())
	}
	if strings.Contains(out.String(), "action=login") {
		t.Errorf("no-alias event should be filtered out: %q", out.String())
	}
	if !strings.Contains(out.String(), "proj:k") {
		t.Errorf("proj alias event missing: %q", out.String())
	}

	// actor filter mismatch skips the non-matching event.
	out.Reset()
	errb.Reset()
	if code := runAudit([]string{"--actor=root"}, home, &out, &errb); code != 0 {
		t.Fatalf("actor filter code=%d", code)
	}
	if strings.Contains(out.String(), "actor=agent") {
		t.Errorf("agent event should be filtered by --actor=root: %q", out.String())
	}

	// A filter that matches nothing prints "(no matching events)".
	out.Reset()
	errb.Reset()
	if code := runAudit([]string{"--action=does-not-exist"}, home, &out, &errb); code != 0 {
		t.Fatalf("no-match code=%d", code)
	}
	if !strings.Contains(out.String(), "no matching events") {
		t.Errorf("expected no-matching message: %q", out.String())
	}
}

func TestC7ReadAuditArms(t *testing.T) {
	testHome(t)

	// Absent log → (nil, nil).
	empty := t.TempDir()
	if evs, err := ReadAudit(empty); err != nil || evs != nil {
		t.Fatalf("ReadAudit absent = %v, %v", evs, err)
	}

	// Non-IsNotExist open error: .boru is a file → ENOTDIR. Windows reports
	// that as path-not-found (IsNotExist), so the arm is Unix-only.
	if runtime.GOOS == "windows" {
		return
	}
	bad := t.TempDir()
	if err := os.WriteFile(vaultFolder(bad), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAudit(bad); err == nil {
		t.Fatal("expected ReadAudit open error")
	}

	// Malformed line is skipped; valid line is returned.
	home := t.TempDir()
	c7writeAudit(t, home, "not-json\n{\"action\":\"ok\"}\n")
	evs, err := ReadAudit(home)
	if err != nil {
		t.Fatalf("ReadAudit err=%v", err)
	}
	if len(evs) != 1 || evs[0].Action != "ok" {
		t.Fatalf("ReadAudit skipped-malformed = %+v", evs)
	}
}
