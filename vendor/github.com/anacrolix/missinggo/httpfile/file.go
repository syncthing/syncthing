package httpfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/httptoo"
)

type File struct {
	off    int64
	r      io.ReadCloser
	rOff   int64
	length int64
	url    string
	flags  int
	fs     *FS
}

func (me *File) headLength() (err error) {
	l, err := me.fs.GetLength(me.url)
	if err != nil {
		return
	}
	if l != -1 {
		me.length = l
	}
	return
}

func (me *File) prepareReader() (err error) {
	if me.r != nil && me.off != me.rOff {
		me.r.Close()
		me.r = nil
	}
	if me.r != nil {
		return nil
	}
	if me.flags&missinggo.O_ACCMODE == os.O_WRONLY {
		err = errors.New("read flags missing")
		return
	}
	req, err := http.NewRequest("GET", me.url, nil)
	if err != nil {
		return
	}
	if me.off != 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", me.off))
	}
	resp, err := me.fs.Client.Do(req)
	if err != nil {
		return
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		cr, ok := httptoo.ParseBytesContentRange(resp.Header.Get("Content-Range"))
		if !ok || cr.First != me.off {
			err = errors.New("bad response")
			resp.Body.Close()
			return
		}
		me.length = cr.Length
	case http.StatusOK:
		if me.off != 0 {
			err = errors.New("bad response")
			resp.Body.Close()
			return
		}
		if h := resp.Header.Get("Content-Length"); h != "" {
			var cl uint64
			cl, err = strconv.ParseUint(h, 10, 64)
			if err != nil {
				resp.Body.Close()
				return
			}
			me.length = int64(cl)
		}
	case http.StatusNotFound:
		err = ErrNotFound
		resp.Body.Close()
		return
	default:
		err = errors.New(resp.Status)
		resp.Body.Close()
		return
	}
	me.r = resp.Body
	me.rOff = me.off
	return
}

func (me *File) Read(b []byte) (n int, err error) {
	err = me.prepareReader()
	if err != nil {
		return
	}
	n, err = me.r.Read(b)
	me.off += int64(n)
	me.rOff += int64(n)
	return
}

func (me *File) Seek(offset int64, whence int) (ret int64, err error) {
	switch whence {
	case os.SEEK_SET:
		ret = offset
	case os.SEEK_CUR:
		ret = me.off + offset
	case os.SEEK_END:
		// Try to update the resource length.
		err = me.headLength()
		if err != nil {
			if me.length == -1 {
				// Don't even have an old value.
				return
			}
			err = nil
		}
		ret = me.length + offset
	default:
		err = fmt.Errorf("unhandled whence: %d", whence)
		return
	}
	me.off = ret
	return
}

func (me *File) Write(b []byte) (n int, err error) {
	if me.flags&(os.O_WRONLY|os.O_RDWR) == 0 || me.flags&os.O_CREATE == 0 {
		err = errors.New("cannot write without write and create flags")
		return
	}
	req, err := http.NewRequest("PATCH", me.url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Range", fmt.Sprintf("bytes=%d-", me.off))
	req.ContentLength = int64(len(b))
	resp, err := me.fs.Client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		err = errors.New(resp.Status)
		return
	}
	n = len(b)
	me.off += int64(n)
	return
}

func (me *File) Close() error {
	me.url = ""
	me.length = -1
	if me.r != nil {
		me.r.Close()
		me.r = nil
	}
	return nil
}
