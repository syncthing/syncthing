package blob

import (
	"context"
	"os"
	"path/filepath"
)

type Disk struct {
	path string
}

func NewDisk(path string) *Disk {
	return &Disk{path: path}
}

func (d *Disk) Put(key string, data []byte) error {
	path := filepath.Join(d.path, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (d *Disk) Get(key string) ([]byte, error) {
	path := filepath.Join(d.path, key)
	return os.ReadFile(path)
}

func (d *Disk) Delete(key string) error {
	path := filepath.Join(d.path, key)
	return os.Remove(path)
}

func (d *Disk) Iterate(_ context.Context, key string, fn func([]byte) bool) error {
	matches, err := filepath.Glob(filepath.Join(d.path, key+"*"))
	if err != nil {
		return err
	}
loop:
	for _, file := range matches {
		stat, err := os.Lstat(file)
		if err != nil {
			continue
		}
		if stat.IsDir() {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		if !fn(content) {
			break loop
		}
	}

	return err
}

func (d *Disk) Count(prefix string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(d.path, prefix+"*"))
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}
