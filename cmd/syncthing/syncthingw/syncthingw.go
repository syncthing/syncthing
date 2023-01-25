//go:build windows
// +build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func main() {
	binary, err := getBinary("syncthing.exe")
	if err != nil {
		MessageBox(0, "Could not find syncthing.exe", err.Error(), 0)
		return
	}

	cmd := exec.Command(binary, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = cmd.Run()
	if err != nil {
		MessageBox(0, "Could not run syncthing.exe", err.Error(), 0)
		return
	}
}

func getBinary(exe string) (string, error) {
	// Check if exe exists in the same directory we are in.
	if this, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(this), exe)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Check if exe is in the path
	path, err := exec.LookPath(exe)
	if err == nil {
		return path, nil
	}

	return "", err
}

func MessageBox(hwnd uintptr, title, message string, flags uint) int {
	ret, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(message))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))),
		uintptr(flags))

	return int(ret)
}
