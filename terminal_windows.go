//go:build windows

package trifle

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

type (
	short int16
	coord struct {
		x short
		y short
	}
	smallRect struct {
		left   short
		top    short
		right  short
		bottom short
	}
	consoleScreenBufferInfo struct {
		size              coord
		cursorPosition    coord
		attributes        uint16
		window            smallRect
		maximumWindowSize coord
	}
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

// getTerminalWidth returns the width of the terminal on Windows, or 0 if it cannot be determined
func getTerminalWidth(w io.Writer) int {
	// Check if writer is a file
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}

	// Get the file descriptor
	fd := f.Fd()

	// Try to get console screen buffer info
	var info consoleScreenBufferInfo
	ret, _, _ := procGetConsoleScreenBufferInfo.Call(fd, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return 0
	}

	// Calculate width from the window coordinates
	width := int(info.window.right - info.window.left + 1)
	if width > 0 {
		return width
	}

	return 0
}
