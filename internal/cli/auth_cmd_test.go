package cli

import (
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// Without a client id configured, plain `st auth` must fail with actionable
// guidance instead of silently prompting for a paste like the old default.
func TestAuthWithoutClientIDExplains(t *testing.T) {
	keyring.MockInit()
	t.Setenv("STITCH_GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	err := execStitch("auth")
	if err == nil {
		t.Fatal("expected an error when no OAuth client id is configured")
	}
	for _, want := range []string{"STITCH_GITHUB_CLIENT_ID", "--token"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestAuthTokenFlagStillStores(t *testing.T) {
	keyring.MockInit()
	runSt(t, "auth", "--token", "tok-123")
	runSt(t, "auth", "--status") // prints "configured"; success is enough
	got, err := keyring.Get("stitch", "github.com")
	if err != nil || got != "tok-123" {
		t.Fatalf("token not stored host-keyed: %q err=%v", got, err)
	}
}
