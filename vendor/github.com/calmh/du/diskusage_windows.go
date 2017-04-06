package du

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Get returns the Usage of a given path, or an error if usage data is
// unavailable.
func Get(path string) (Usage, error) {
	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	var u Usage

	pathw, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return Usage{}, err
	}

	ret, _, err := c.Call(
		uintptr(unsafe.Pointer(pathw)),
		uintptr(unsafe.Pointer(&u.FreeBytes)),
		uintptr(unsafe.Pointer(&u.TotalBytes)),
		uintptr(unsafe.Pointer(&u.AvailBytes)))
	runtime.KeepAlive(pathw)

	if ret == 0 {
		return Usage{}, err
	}

	return u, nil
}
