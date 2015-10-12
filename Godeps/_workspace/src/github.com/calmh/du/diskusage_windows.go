package du

import (
	"syscall"
	"unsafe"
)

// Get returns the Usage of a given path, or an error if usage data is
// unavailable.
func Get(path string) (Usage, error) {
	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	var u Usage

	ret, _, err := c.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(path))),
		uintptr(unsafe.Pointer(&u.FreeBytes)),
		uintptr(unsafe.Pointer(&u.TotalBytes)),
		uintptr(unsafe.Pointer(&u.AvailBytes)))

	if ret == 0 {
		return u, err
	}

	return u, nil
}
