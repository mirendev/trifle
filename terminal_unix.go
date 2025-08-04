//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly || solaris

package trifle

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// getTerminalWidth returns the width of the terminal, or 0 if it cannot be determined
func getTerminalWidth(w io.Writer) int {
	// Check if writer is a file
	if f, ok := w.(*os.File); ok {
		// Get terminal size using ioctl
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err == nil && ws.Col > 0 {
			return int(ws.Col)
		}
	}
	return 0 // Return 0 if not a terminal or can't get size
}
