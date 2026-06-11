// Package auth stores and resolves forge tokens (OS keychain + env fallback).
package auth

import (
	"fmt"
	"os"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "stitch"

// DefaultHost is the only forge host stitch talks to today.
const DefaultHost = "github.com"

// ResolveToken returns a token for host from the OS keychain, falling back to
// the GITHUB_TOKEN / GH_TOKEN environment variables.
func ResolveToken(host string) (string, error) {
	if t, err := keyring.Get(keyringService, host); err == nil {
		if t = strings.TrimSpace(t); t != "" {
			return t, nil
		}
	}
	for _, e := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(e)); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no GitHub token found; run 'st auth' or set GITHUB_TOKEN")
}

// Store saves a token for host in the OS keychain.
func Store(host, token string) error {
	return keyring.Set(keyringService, host, token)
}

// Delete removes the stored token for host.
func Delete(host string) error {
	return keyring.Delete(keyringService, host)
}
