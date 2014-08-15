package files

import (
	"path/filepath"

	"code.google.com/p/go.text/unicode/norm"
)

func normalizedFilename(s string) string {
	return norm.NFC.String(filepath.ToSlash(s))
}

func nativeFilename(s string) string {
	return filepath.FromSlash(s)
}
