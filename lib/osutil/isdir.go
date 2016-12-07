package osutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsDir returns true if base and every path component of name up to and
// including filepath.Join(base, name) is a directory (and not a symlink or
// similar). Base and name must both be clean and name must be relative to
// base.
func IsDir(base, name string) bool {
	defer func() {
		fmt.Println(base, name)
	}()
	path := base
	info, err := Lstat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}

	if name == "." {
		// The result of calling IsDir("some/where", filepath.Dir("foo"))
		return true
	}

	parts := strings.Split(name, string(os.PathSeparator))
	for _, part := range parts {
		path = filepath.Join(path, part)
		info, err := Lstat(path)
		if err != nil {
			return false
		}
		if !info.IsDir() {
			return false
		}
	}
	return true
}
