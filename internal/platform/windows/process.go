//go:build windows

package windows

import (
	"context"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// Command is exec.CommandContext for a program that must not open a console window, as a
// Script Command run by a windowed app would. Canceling ctx ends the program and every
// process it started, which a script's shell leaves running otherwise.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // runs what the caller asks, as exec does
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Cancel = func() error {
		dir, err := windows.GetSystemDirectory()
		if err == nil {
			kill := exec.CommandContext(context.Background(), dir+`\taskkill.exe`, "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)) //nolint:gosec // our own child
			kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
			if kill.Run() == nil {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
	return cmd
}

// EditFile opens a file in the editor Windows has for it (the "edit" verb), or Notepad when it
// has none. Never the "open" verb: that runs scripts rather than showing them.
func EditFile(path string) error {
	verb, _ := windows.UTF16PtrFromString("edit")
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err == nil {
		return nil
	}
	dir, err := windows.GetWindowsDirectory()
	if err != nil {
		return err
	}
	return exec.CommandContext(context.Background(), dir+`\notepad.exe`, path).Start() //nolint:gosec // the user's own file, in Notepad
}
