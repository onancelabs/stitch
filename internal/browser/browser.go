// Package browser opens URLs in the user's default browser.
package browser

import (
	"os/exec"
	"runtime"
)

// Open opens url in the user's default browser. It is best-effort and
// non-blocking; callers should treat any returned error as non-fatal.
func Open(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default: // linux, *bsd, ...
		name, args = "xdg-open", []string{url}
	}
	return exec.Command(name, args...).Start()
}
