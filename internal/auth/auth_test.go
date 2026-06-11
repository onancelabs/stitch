package auth

import (
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func TestResolveToken(t *testing.T) {
	// Keyring value takes precedence.
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if err := Store(DefaultHost, "kr-token"); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveToken(DefaultHost); err != nil || got != "kr-token" {
		t.Fatalf("keyring token: got %q err %v", got, err)
	}

	// Falls back to the environment when the keyring is empty.
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "env-token")
	if got, err := ResolveToken(DefaultHost); err != nil || got != "env-token" {
		t.Fatalf("env token: got %q err %v", got, err)
	}

	// Errors when nothing is configured.
	keyring.MockInit()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if _, err := ResolveToken(DefaultHost); err == nil {
		t.Fatal("expected an error when no token is configured")
	}
}
