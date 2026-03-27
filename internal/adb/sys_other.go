//go:build !windows
// +build !windows

package adb

import "os/exec"

// setHideWindow is a no-op on non-Windows platforms.
func setHideWindow(cmd *exec.Cmd) {
    // intentionally empty
}
