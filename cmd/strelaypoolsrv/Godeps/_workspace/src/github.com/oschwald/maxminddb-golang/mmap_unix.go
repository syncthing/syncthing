// +build !windows,!appengine

package maxminddb

import (
	"syscall"
)

func mmap(fd int, length int) (data []byte, err error) {
	return syscall.Mmap(fd, 0, length, syscall.PROT_READ, syscall.MAP_SHARED)
}

func munmap(b []byte) (err error) {
	return syscall.Munmap(b)
}
