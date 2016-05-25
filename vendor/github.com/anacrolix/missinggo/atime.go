package missinggo

import (
	"os"
	"time"
)

// Extracts the access time from the FileInfo internals.
func FileInfoAccessTime(fi os.FileInfo) time.Time {
	return fileInfoAccessTime(fi)
}
