// +build darwin freebsd netbsd

package missinggo

import (
	"os"
	"syscall"
	"time"
)

func fileInfoAccessTime(fi os.FileInfo) time.Time {
	ts := fi.Sys().(*syscall.Stat_t).Atimespec
	return time.Unix(int64(ts.Sec), int64(ts.Nsec))
}
