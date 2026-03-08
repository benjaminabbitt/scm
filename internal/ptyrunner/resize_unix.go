//go:build !windows

package ptyrunner

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/aymanbagabas/go-pty"
	"golang.org/x/term"
)

// startResizeHandler starts a goroutine that handles terminal resize signals (SIGWINCH)
// and returns a function to stop the handler.
func startResizeHandler(ptty pty.Pty) func() {
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)

	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-done:
				return
			case <-resizeCh:
				resizePty(ptty)
			}
		}
	}()

	// Trigger initial resize
	resizePty(ptty)

	return func() {
		signal.Stop(resizeCh)
		close(resizeCh)
		close(done)
	}
}

// resizePty resizes the PTY to match the current terminal size.
func resizePty(ptty pty.Pty) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}

	_ = ptty.Resize(width, height)
}
