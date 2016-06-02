package httpfile

import (
	"net/http"
)

var DefaultFS = &FS{
	Client: http.DefaultClient,
}

// Returns the length of the resource in bytes.
func GetLength(url string) (ret int64, err error) {
	return DefaultFS.GetLength(url)
}
