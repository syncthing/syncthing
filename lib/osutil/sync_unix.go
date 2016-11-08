// +build !windows

package osutil

func SyncFile(path string) error {
	return syncFile(path, 0)
}

func SyncDir(path string) error {
	return syncFile(path, 0)
}
