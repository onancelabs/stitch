package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeGitHub stands in for github.com's device-flow endpoints. pollResponses
// are returned in order from the access_token endpoint (the last one repeats).
func fakeGitHub(t *testing.T, pollResponses []map[string]any) (codeURL, tokenURL string) {
	t.Helper()
	n := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Error("device code request must ask for JSON")
		}
		_ = r.ParseForm()
		if r.Form.Get("client_id") == "" || r.Form.Get("scope") != "repo" {
			t.Errorf("bad device code request: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-123", "user_code": "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900, "interval": 1,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("device_code") != "dev-123" ||
			r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("bad poll request: %v", r.Form)
		}
		resp := pollResponses[min(n, len(pollResponses)-1)]
		n++
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/login/device/code", srv.URL + "/login/oauth/access_token"
}

// runFlow executes DeviceFlow against the fake with instant, recorded sleeps.
func runFlow(t *testing.T, pollResponses []map[string]any) (string, []time.Duration, error) {
	t.Helper()
	codeURL, tokenURL := fakeGitHub(t, pollResponses)
	oldCode, oldToken, oldSleep := deviceCodeURL, accessTokenURL, sleep
	var slept []time.Duration
	deviceCodeURL, accessTokenURL = codeURL, tokenURL
	sleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { deviceCodeURL, accessTokenURL, sleep = oldCode, oldToken, oldSleep })

	opened := ""
	tok, err := DeviceFlow("client-x", func(u string) error { opened = u; return nil },
		strings.NewReader("\n"), io.Discard)
	if err == nil && opened == "" {
		t.Error("browser should have been opened on success")
	}
	return tok, slept, err
}

func TestDeviceFlowHappyPath(t *testing.T) {
	tok, _, err := runFlow(t, []map[string]any{
		{"error": "authorization_pending"},
		{"access_token": "gho_abc", "token_type": "bearer"},
	})
	if err != nil || tok != "gho_abc" {
		t.Fatalf("want token gho_abc, got %q err=%v", tok, err)
	}
}

func TestDeviceFlowSlowDown(t *testing.T) {
	tok, slept, err := runFlow(t, []map[string]any{
		{"error": "slow_down"},
		{"access_token": "gho_abc"},
	})
	if err != nil || tok != "gho_abc" {
		t.Fatalf("want token, got %q err=%v", tok, err)
	}
	if len(slept) < 2 || slept[len(slept)-1] <= slept[0] {
		t.Errorf("slow_down must grow the poll interval: %v", slept)
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	_, _, err := runFlow(t, []map[string]any{{"error": "access_denied"}})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("want denial error, got %v", err)
	}
}

func TestDeviceFlowExpired(t *testing.T) {
	_, _, err := runFlow(t, []map[string]any{{"error": "expired_token"}})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("want expiry error, got %v", err)
	}
}
