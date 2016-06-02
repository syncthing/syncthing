package missinggo

import (
	"os"
	"syscall"
	"time"
)

func fileInfoAccessTime(fi os.FileInfo) time.Time {
	sec := fi.Sys().(*syscall.Dir).Atime
	return time.Unix(int64(sec), 0)
}
