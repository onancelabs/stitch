package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubClientID is the OAuth App client id embedded at release time. Until
// the app is registered it is empty and `st auth` explains the fallback. The
// CLI lets STITCH_GITHUB_CLIENT_ID override it.
var GitHubClientID = "Ov23liuJYyC4oTvRr5Uk"

// Endpoint and clock seams; tests override these.
var (
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
	sleep          = time.Sleep
)

type deviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResp struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// postForm POSTs form values asking for a JSON response and decodes it into v.
func postForm(endpoint string, form url.Values, v any) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("github returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// DeviceFlow runs GitHub's OAuth device authorization flow and returns an
// access token. openBrowser is best-effort; the verification URL and one-time
// code are always printed to out so the flow works over SSH.
func DeviceFlow(clientID string, openBrowser func(string) error, in io.Reader, out io.Writer) (string, error) {
	var dc deviceCode
	if err := postForm(deviceCodeURL, url.Values{"client_id": {clientID}, "scope": {"repo"}}, &dc); err != nil {
		return "", fmt.Errorf("requesting device code: %v", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return "", fmt.Errorf("github did not return a device code (is the client id valid?)")
	}

	fmt.Fprintf(out, "First, copy your one-time code: %s\n", dc.UserCode)
	fmt.Fprintf(out, "Press Enter to open %s in your browser...", dc.VerificationURI)
	_, _ = bufio.NewReader(in).ReadString('\n') // best effort; EOF is fine
	if err := openBrowser(dc.VerificationURI); err != nil {
		fmt.Fprintf(out, "\n(could not open a browser; visit %s manually)\n", dc.VerificationURI)
	}
	fmt.Fprint(out, "Waiting for authorization...")

	interval := time.Duration(max(dc.Interval, 1)) * time.Second
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		sleep(interval)
		var tr tokenResp
		if err := postForm(accessTokenURL, url.Values{
			"client_id":   {clientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &tr); err != nil {
			return "", fmt.Errorf("polling for token: %v", err)
		}
		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				fmt.Fprintln(out, " done.")
				return tr.AccessToken, nil
			}
		case "authorization_pending":
			// keep waiting
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return "", fmt.Errorf("the one-time code expired before authorization; run 'st auth' again")
		case "access_denied":
			return "", fmt.Errorf("authorization was denied")
		default:
			return "", fmt.Errorf("github: %s (%s)", tr.Error, tr.ErrorDesc)
		}
	}
	return "", fmt.Errorf("timed out waiting for authorization; run 'st auth' again")
}
