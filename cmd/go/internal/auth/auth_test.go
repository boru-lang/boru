package auth

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- Unit tests: password hashing ---

func TestHashPasswordAndCheck(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("hash is empty")
	}
	if err := CheckPassword(hash, "mypassword"); err != nil {
		t.Errorf("CheckPassword failed for correct password: %s", err)
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, err := HashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPassword(hash, "wrong"); err == nil {
		t.Error("CheckPassword should fail for wrong password")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Error("same password should produce different hashes (different salt)")
	}
}

// --- Unit tests: token generation ---

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64", len(token))
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("token is not valid hex: %s", err)
	}
}

func TestGenerateTokenUniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatal(err)
		}
		if tokens[tok] {
			t.Errorf("duplicate token generated: %s", tok)
		}
		tokens[tok] = true
	}
}

// --- Unit tests: UserStore ---

func TestUserStoreRegister(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	err := store.Register("alice@example.com", "alice", "password123")
	if err != nil {
		t.Fatal(err)
	}

	users, err := store.LoadUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := users["alice"]
	if u.Email != "alice@example.com" {
		t.Errorf("email = %q, want alice@example.com", u.Email)
	}
	if u.Username != "alice" {
		t.Errorf("username = %q, want alice", u.Username)
	}
	if u.PasswordHash == "" {
		t.Error("password hash is empty")
	}
	if u.CreatedAt == "" {
		t.Error("created_at is empty")
	}
}

func TestUserStoreRegisterDuplicateUsername(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("a@example.com", "alice", "pass1")
	err := store.Register("b@example.com", "alice", "pass2")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
	if !strings.Contains(err.Error(), "username already taken") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreRegisterDuplicateEmail(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("same@example.com", "alice", "pass1")
	err := store.Register("same@example.com", "bob", "pass2")
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	if !strings.Contains(err.Error(), "email already registered") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreLogin(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	token, user, err := store.Login("alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Error("token is empty")
	}
	if user.Username != "alice" {
		t.Errorf("username = %q, want alice", user.Username)
	}
}

func TestUserStoreLoginWrongPassword(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	_, _, err := store.Login("alice", "wrongpassword")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreLoginUnknownUser(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	_, _, err := store.Login("nobody", "password")
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestUserStoreValidateToken(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	store.Register("alice@example.com", "alice", "password123")
	token, _, _ := store.Login("alice", "password123")

	username, err := store.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if username != "alice" {
		t.Errorf("username = %q, want alice", username)
	}
}

func TestUserStoreValidateTokenInvalid(t *testing.T) {
	dir := t.TempDir()
	store := NewUserStore(dir)

	_, err := store.ValidateToken("nonexistenttoken")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

// --- Unit tests: client user storage ---

func TestClientUserRoundTrip(t *testing.T) {
	homeDir := t.TempDir()

	cu := &ClientUser{
		Username: "alice",
		Email:    "alice@example.com",
		Token:    "abc123def456",
		Registry: "http://localhost:8080",
	}

	if err := SaveClientUser(homeDir, cu); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadClientUser(homeDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Username != cu.Username {
		t.Errorf("username = %q, want %q", loaded.Username, cu.Username)
	}
	if loaded.Email != cu.Email {
		t.Errorf("email = %q, want %q", loaded.Email, cu.Email)
	}
	if loaded.Token != cu.Token {
		t.Errorf("token = %q, want %q", loaded.Token, cu.Token)
	}
	if loaded.Registry != cu.Registry {
		t.Errorf("registry = %q, want %q", loaded.Registry, cu.Registry)
	}
}

func TestClientUserFilePermissions(t *testing.T) {
	homeDir := t.TempDir()

	cu := &ClientUser{
		Username: "alice",
		Email:    "alice@example.com",
		Token:    "abc123",
		Registry: "http://localhost:8080",
	}
	SaveClientUser(homeDir, cu)

	info, err := os.Stat(filepath.Join(homeDir, ".boru", "user.jsonic"))
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if runtime.GOOS != "windows" && perm != 0600 { // Windows has no permission bits
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

// --- Unit tests: ReadPassword ---

func TestReadPasswordFromReader(t *testing.T) {
	input := strings.NewReader("secret\n")
	ir := NewInputReader(input)
	var out bytes.Buffer
	pw, err := ir.ReadPassword("Password: ", &out)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "secret" {
		t.Errorf("password = %q, want secret", pw)
	}
	if !strings.Contains(out.String(), "Password: ") {
		t.Errorf("expected prompt in output, got %q", out.String())
	}
}
