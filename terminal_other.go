//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly && !solaris && !windows

package trifle

import "io"

// getTerminalWidth returns 0 on non-Unix platforms
func getTerminalWidth(w io.Writer) int {
	// Terminal width detection is not implemented for this platform
	return 0
}
