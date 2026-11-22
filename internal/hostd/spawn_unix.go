//go:build unix

package hostd

import "syscall"

// detached runs the process in a session of its own, so it keeps running once the terminal that started it closes.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
