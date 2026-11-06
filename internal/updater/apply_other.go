//go:build !windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// installDeb upgrades the Debian package in place. pkexec shows the desktop's
// password prompt; apt-get keeps installed dependencies and the user's data.
func installDeb(ctx context.Context, path string) error {
	// apt reads the file as an unprivileged sandbox user.
	if err := os.Chmod(path, 0o644); err != nil {
		return err
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return errors.New("pkexec is not available to ask for the administrator password; " +
			"download the .deb from the release page and install it with: sudo apt install ./<file>.deb")
	}
	out, err := exec.CommandContext(ctx, pkexec, "/usr/bin/apt-get", "install", "-y", "--no-install-recommends", path).
		CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return errors.New("update cancelled: administrator permission was not granted")
		}
		return fmt.Errorf("package install failed: %v\n%s", err, lastLines(string(out), 8))
	}
	return nil
}

func runInstaller(string, string) error {
	return errors.New("installer updates are only supported on Windows")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
