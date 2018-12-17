// +build !windows,!netbsd,!openbsd,!solaris

package du

import (
	"path/filepath"
	"syscall"
)

// Get returns the Usage of a given path, or an error if usage data is
// unavailable.
func Get(path string) (Usage, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(filepath.Clean(path), &stat)
	if err != nil {
		return Usage{}, err
	}
	u := Usage{
		FreeBytes:  int64(stat.Bfree) * int64(stat.Bsize),
		TotalBytes: int64(stat.Blocks) * int64(stat.Bsize),
		AvailBytes: int64(stat.Bavail) * int64(stat.Bsize),
	}
	return u, nil
}
