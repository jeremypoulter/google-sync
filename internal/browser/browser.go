// Package browser opens a URL in the user's default browser.
package browser

import (
	"os/exec"
	"runtime"
)

// Open starts the default browser on url. It returns once the launcher has
// started, not when the page has loaded.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
