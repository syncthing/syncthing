package missinggo

import (
	"os"
	"syscall"
	"time"
)

func fileInfoAccessTime(fi os.FileInfo) time.Time {
	ts := fi.Sys().(syscall.Win32FileAttributeData).LastAccessTime
	return time.Unix(0, int64(ts.Nanoseconds()))
}
