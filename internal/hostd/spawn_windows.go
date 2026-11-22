//go:build windows

package hostd

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// detached runs the process apart from the console that started it, in a group of its own, with its window hidden.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}
