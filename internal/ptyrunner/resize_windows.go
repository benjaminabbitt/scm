//go:build windows

package ptyrunner

import (
	"github.com/aymanbagabas/go-pty"
)

// startResizeHandler returns a no-op stop function on Windows.
// Windows ConPTY handles terminal resize automatically.
func startResizeHandler(ptty pty.Pty) func() {
	return func() {}
}
