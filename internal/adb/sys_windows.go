//go:build windows
// +build windows

package adb

import (
    "os/exec"
    "syscall"
)

// setHideWindow sets the SysProcAttr to hide the console window on Windows.
func setHideWindow(cmd *exec.Cmd) {
    if cmd == nil {
        return
    }
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
