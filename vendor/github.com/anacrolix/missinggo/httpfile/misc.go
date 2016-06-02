package httpfile

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/anacrolix/missinggo/httptoo"
)

var (
	ErrNotFound = os.ErrNotExist
)

// ok is false if the response just doesn't specify anything we handle.
func instanceLength(r *http.Response) (l int64, err error) {
	switch r.StatusCode {
	case http.StatusOK:
		l, err = strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
		return
	case http.StatusPartialContent:
		cr, parseOk := httptoo.ParseBytesContentRange(r.Header.Get("Content-Range"))
		l = cr.Length
		if !parseOk {
			err = errors.New("error parsing Content-Range")
		}
		return
	default:
		err = errors.New("unhandled status code")
		return
	}
}
