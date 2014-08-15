package files

import "code.google.com/p/go.text/unicode/norm"

func normalizedFilename(s string) string {
	return norm.NFC.String(s)
}

func nativeFilename(s string) string {
	return norm.NFD.String(s)
}
