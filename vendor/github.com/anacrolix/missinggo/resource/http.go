package resource

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Provides access to resources through a http.Client.
type HTTPProvider struct {
	Client *http.Client
}

var _ Provider = &HTTPProvider{}

func (me *HTTPProvider) NewInstance(urlStr string) (r Instance, err error) {
	_r := new(httpInstance)
	_r.URL, err = url.Parse(urlStr)
	if err != nil {
		return
	}
	_r.Client = me.Client
	if _r.Client == nil {
		_r.Client = http.DefaultClient
	}
	r = _r
	return
}

type httpInstance struct {
	Client *http.Client
	URL    *url.URL
}

var _ Instance = &httpInstance{}

func mustNewRequest(method, urlStr string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		panic(err)
	}
	return req
}

func responseError(r *http.Response) error {
	if r.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	return errors.New(r.Status)
}

func (me *httpInstance) Get() (ret io.ReadCloser, err error) {
	resp, err := me.Client.Get(me.URL.String())
	if err != nil {
		return
	}
	if resp.StatusCode == http.StatusOK {
		ret = resp.Body
		return
	}
	resp.Body.Close()
	err = responseError(resp)
	return
}

func (me *httpInstance) Put(r io.Reader) (err error) {
	resp, err := me.Client.Do(mustNewRequest("PUT", me.URL.String(), r))
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return
	}
	err = responseError(resp)
	return
}

func (me *httpInstance) ReadAt(b []byte, off int64) (n int, err error) {
	req := mustNewRequest("GET", me.URL.String(), nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(b))-1))
	resp, err := me.Client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		err = io.EOF
		return
	default:
		err = responseError(resp)
		return
	}
	// TODO: This will crash if ContentLength was not provided (-1). Do
	// something about that.
	b = b[:resp.ContentLength]
	return io.ReadFull(resp.Body, b)
}

func (me *httpInstance) WriteAt(b []byte, off int64) (n int, err error) {
	req := mustNewRequest("PATCH", me.URL.String(), bytes.NewReader(b))
	req.ContentLength = int64(len(b))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(b))-1))
	resp, err := me.Client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = responseError(resp)
	}
	n = len(b)
	return
}

func (me *httpInstance) Stat() (fi os.FileInfo, err error) {
	resp, err := me.Client.Head(me.URL.String())
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		err = os.ErrNotExist
		return
	}
	if resp.StatusCode != http.StatusOK {
		err = errors.New(resp.Status)
		return
	}
	var _fi httpFileInfo
	if h := resp.Header.Get("Last-Modified"); h != "" {
		_fi.lastModified, err = time.Parse(http.TimeFormat, h)
		if err != nil {
			err = fmt.Errorf("error parsing Last-Modified header: %s", err)
			return
		}
	}
	if h := resp.Header.Get("Content-Length"); h != "" {
		_fi.contentLength, err = strconv.ParseInt(h, 10, 64)
		if err != nil {
			err = fmt.Errorf("error parsing Content-Length header: %s", err)
			return
		}
	}
	fi = _fi
	return
}

func (me *httpInstance) Delete() (err error) {
	resp, err := me.Client.Do(mustNewRequest("DELETE", me.URL.String(), nil))
	if err != nil {
		return
	}
	err = responseError(resp)
	resp.Body.Close()
	return
}

type httpFileInfo struct {
	lastModified  time.Time
	contentLength int64
}

var _ os.FileInfo = httpFileInfo{}

func (fi httpFileInfo) IsDir() bool {
	return false
}

func (fi httpFileInfo) Mode() os.FileMode {
	return 0
}

func (fi httpFileInfo) Name() string {
	return ""
}

func (fi httpFileInfo) Size() int64 {
	return fi.contentLength
}

func (fi httpFileInfo) ModTime() time.Time {
	return fi.lastModified
}

func (fi httpFileInfo) Sys() interface{} {
	return nil
}
