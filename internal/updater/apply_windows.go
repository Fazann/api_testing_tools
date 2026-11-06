//go:build windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

func installDeb(context.Context, string) error {
	return errors.New(".deb updates are only supported on Linux")
}

// runInstaller starts the NSIS installer silently once this process has
// quit (the installer must overwrite our executable), then relaunches the
// app. `start` goes through ShellExecute, so the installer's UAC prompt is
// shown; the ping is a short delay that lets the app exit first.
func runInstaller(installer, exe string) error {
	line := fmt.Sprintf(`cmd.exe /d /c ping -n 3 127.0.0.1 >nul & start "" /wait "%s" /S & start "" "%s"`, installer, exe)
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       line,
		HideWindow:    true,
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start the installer: %w", err)
	}
	return cmd.Process.Release()
}
