// +build linux darwin freebsd netbsd openbsd solaris dragonfly
// +build !appengine

package pb

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrPoolWasStarted = errors.New("Bar pool was started")

var (
	echoLockMutex    sync.Mutex
	origTermStatePtr *unix.Termios
	tty              *os.File
)

func init() {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()

	var err error
	tty, err = os.Open("/dev/tty")
	if err != nil {
		tty = os.Stdin
	}
}

// terminalWidth returns width of the terminal.
func terminalWidth() (int, error) {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()

	fd := int(tty.Fd())

	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, err
	}

	return int(ws.Col), nil
}

func lockEcho() (quit chan int, err error) {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()
	if origTermStatePtr != nil {
		return quit, ErrPoolWasStarted
	}

	fd := int(tty.Fd())

	oldTermStatePtr, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return nil, fmt.Errorf("Can't get terminal settings: %v", err)
	}

	oldTermios := *oldTermStatePtr
	newTermios := oldTermios
	newTermios.Lflag &^= syscall.ECHO
	newTermios.Lflag |= syscall.ICANON | syscall.ISIG
	newTermios.Iflag |= syscall.ICRNL
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &newTermios); err != nil {
		return nil, fmt.Errorf("Can't set terminal settings: %v", err)
	}

	quit = make(chan int, 1)
	go catchTerminate(quit)
	return
}

func unlockEcho() error {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()
	if origTermStatePtr == nil {
		return nil
	}

	fd := int(tty.Fd())

	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, origTermStatePtr); err != nil {
		return fmt.Errorf("Can't set terminal settings: %v", err)
	}

	origTermStatePtr = nil

	return nil
}

// listen exit signals and restore terminal state
func catchTerminate(quit chan int) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGKILL)
	defer signal.Stop(sig)
	select {
	case <-quit:
		unlockEcho()
	case <-sig:
		unlockEcho()
	}
}
