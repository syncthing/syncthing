package httpfile

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

type FS struct {
	Client *http.Client
}

func (fs *FS) Delete(urlStr string) (err error) {
	req, err := http.NewRequest("DELETE", urlStr, nil)
	if err != nil {
		return
	}
	resp, err := fs.Client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		err = ErrNotFound
		return
	}
	if resp.StatusCode != 200 {
		err = fmt.Errorf("response: %s", resp.Status)
	}
	return
}

func (fs *FS) GetLength(url string) (ret int64, err error) {
	resp, err := fs.Client.Head(url)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		err = ErrNotFound
		return
	}
	return instanceLength(resp)
}

func (fs *FS) OpenSectionReader(url string, off, n int64) (ret io.ReadCloser, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	resp, err := fs.Client.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode == http.StatusNotFound {
		err = ErrNotFound
		resp.Body.Close()
		return
	}
	if resp.StatusCode != http.StatusPartialContent {
		err = fmt.Errorf("bad response status: %s", resp.Status)
		resp.Body.Close()
		return
	}
	ret = resp.Body
	return
}

func (fs *FS) Open(url string, flags int) (ret *File, err error) {
	ret = &File{
		url:    url,
		flags:  flags,
		length: -1,
		fs:     fs,
	}
	if flags&os.O_CREATE == 0 {
		err = ret.headLength()
	}
	return
}
