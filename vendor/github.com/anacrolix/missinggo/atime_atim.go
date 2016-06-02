// +build linux dragonfly openbsd solaris

package missinggo

import (
	"os"
	"syscall"
	"time"
)

func fileInfoAccessTime(fi os.FileInfo) time.Time {
	ts := fi.Sys().(*syscall.Stat_t).Atim
	return time.Unix(int64(ts.Sec), int64(ts.Nsec))
}
