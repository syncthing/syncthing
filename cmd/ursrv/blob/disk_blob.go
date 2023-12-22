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

func (a *Disk) Put(key string, data []byte) error {
	path := filepath.Join(a.path, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (a *Disk) Get(key string) ([]byte, error) {
	path := filepath.Join(a.path, key)
	return os.ReadFile(path)
}

func (a *Disk) Delete(key string) error {
	path := filepath.Join(a.path, key)
	return os.Remove(path)
}

func (a *Disk) Iterate(_ context.Context, key string, fn func([]byte) bool) error {
	matches, err := filepath.Glob(filepath.Join(a.path, key+"*"))
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
