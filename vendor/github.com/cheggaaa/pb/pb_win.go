// +build windows

package pb

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

var tty = os.Stdin

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	// GetConsoleScreenBufferInfo retrieves information about the
	// specified console screen buffer.
	// http://msdn.microsoft.com/en-us/library/windows/desktop/ms683171(v=vs.85).aspx
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")

	// GetConsoleMode retrieves the current input mode of a console's
	// input buffer or the current output mode of a console screen buffer.
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms683167(v=vs.85).aspx
	getConsoleMode = kernel32.NewProc("GetConsoleMode")

	// SetConsoleMode sets the input mode of a console's input buffer
	// or the output mode of a console screen buffer.
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms686033(v=vs.85).aspx
	setConsoleMode = kernel32.NewProc("SetConsoleMode")

	// SetConsoleCursorPosition sets the cursor position in the
	// specified console screen buffer.
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms686025(v=vs.85).aspx
	setConsoleCursorPosition = kernel32.NewProc("SetConsoleCursorPosition")
)

type (
	// Defines the coordinates of the upper left and lower right corners
	// of a rectangle.
	// See
	// http://msdn.microsoft.com/en-us/library/windows/desktop/ms686311(v=vs.85).aspx
	smallRect struct {
		Left, Top, Right, Bottom int16
	}

	// Defines the coordinates of a character cell in a console screen
	// buffer. The origin of the coordinate system (0,0) is at the top, left cell
	// of the buffer.
	// See
	// http://msdn.microsoft.com/en-us/library/windows/desktop/ms682119(v=vs.85).aspx
	coordinates struct {
		X, Y int16
	}

	word int16

	// Contains information about a console screen buffer.
	// http://msdn.microsoft.com/en-us/library/windows/desktop/ms682093(v=vs.85).aspx
	consoleScreenBufferInfo struct {
		dwSize              coordinates
		dwCursorPosition    coordinates
		wAttributes         word
		srWindow            smallRect
		dwMaximumWindowSize coordinates
	}
)

// terminalWidth returns width of the terminal.
func terminalWidth() (width int, err error) {
	var info consoleScreenBufferInfo
	_, _, e := syscall.Syscall(procGetConsoleScreenBufferInfo.Addr(), 2, uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&info)), 0)
	if e != 0 {
		return 0, error(e)
	}
	return int(info.dwSize.X) - 1, nil
}

func getCursorPos() (pos coordinates, err error) {
	var info consoleScreenBufferInfo
	_, _, e := syscall.Syscall(procGetConsoleScreenBufferInfo.Addr(), 2, uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&info)), 0)
	if e != 0 {
		return info.dwCursorPosition, error(e)
	}
	return info.dwCursorPosition, nil
}

func setCursorPos(pos coordinates) error {
	_, _, e := syscall.Syscall(setConsoleCursorPosition.Addr(), 2, uintptr(syscall.Stdout), uintptr(uint32(uint16(pos.Y))<<16|uint32(uint16(pos.X))), 0)
	if e != 0 {
		return error(e)
	}
	return nil
}

var ErrPoolWasStarted = errors.New("Bar pool was started")

var echoLocked bool
var echoLockMutex sync.Mutex

var oldState word

func lockEcho() (quit chan int, err error) {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()
	if echoLocked {
		err = ErrPoolWasStarted
		return
	}
	echoLocked = true

	if _, _, e := syscall.Syscall(getConsoleMode.Addr(), 2, uintptr(syscall.Stdout), uintptr(unsafe.Pointer(&oldState)), 0); e != 0 {
		err = fmt.Errorf("Can't get terminal settings: %v", e)
		return
	}

	newState := oldState
	const ENABLE_ECHO_INPUT = 0x0004
	const ENABLE_LINE_INPUT = 0x0002
	newState = newState & (^(ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT))
	if _, _, e := syscall.Syscall(setConsoleMode.Addr(), 2, uintptr(syscall.Stdout), uintptr(newState), 0); e != 0 {
		err = fmt.Errorf("Can't set terminal settings: %v", e)
		return
	}
	return
}

func unlockEcho() (err error) {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()
	if !echoLocked {
		return
	}
	echoLocked = false
	if _, _, e := syscall.Syscall(setConsoleMode.Addr(), 2, uintptr(syscall.Stdout), uintptr(oldState), 0); e != 0 {
		err = fmt.Errorf("Can't set terminal settings")
	}
	return
}
