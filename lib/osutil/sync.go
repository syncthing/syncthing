package osutil

import "os"

func syncFile(path string, flag int) error {
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
