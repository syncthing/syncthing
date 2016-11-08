package osutil

import (
	"os"
	"runtime"
)

func SyncFile(path string) error {
	flag := 0
	if runtime.GOOS == "windows" {
		flag = os.O_WRONLY
	}
	fd, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return err
	}
	defer fd.Close()
	if err := fd.Sync(); err != nil {
		return err
	}
	return nil
}

func SyncDir(path string) error {
	if runtime.GOOS == "windows" {
		// not supported by Windows
		return nil
	}
	return SyncFile(path)
}
