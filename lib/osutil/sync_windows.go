// +build windows

package osutil

import "os"

func SyncFile(path string) error {
	return syncFile(path, os.O_WRONLY)
}

func SyncDir(path string) error {
	// not supported by Windows
	return nil
}
