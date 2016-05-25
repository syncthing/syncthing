package httptoo

import (
	"encoding/gob"
	"io"
	"net/http"
	"net/url"
	"sync"
)

func deepCopy(dst, src interface{}) error {
	r, w := io.Pipe()
	e := gob.NewEncoder(w)
	d := gob.NewDecoder(r)
	var decErr, encErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		decErr = d.Decode(dst)
		r.Close()
	}()
	encErr = e.Encode(src)
	// Always returns nil.
	w.CloseWithError(encErr)
	wg.Wait()
	if encErr != nil {
		return encErr
	}
	return decErr
}

// Takes a request, and alters its destination fields, for proxying.
func RedirectedRequest(r *http.Request, newUrl string) (ret *http.Request, err error) {
	u, err := url.Parse(newUrl)
	if err != nil {
		return
	}
	ret = new(http.Request)
	*ret = *r
	ret.Header = nil
	err = deepCopy(&ret.Header, r.Header)
	if err != nil {
		return
	}
	ret.URL = u
	ret.RequestURI = ""
	return
}

func ForwardResponse(w http.ResponseWriter, r *http.Response) {
	for h, vs := range r.Header {
		for _, v := range vs {
			w.Header().Add(h, v)
		}
	}
	w.WriteHeader(r.StatusCode)
	// Errors frequently occur writing the body when the client hangs up.
	io.Copy(w, r.Body)
	r.Body.Close()
}

func ReverseProxy(w http.ResponseWriter, r *http.Request, originUrl string, client *http.Client) (err error) {
	if client == nil {
		client = http.DefaultClient
	}
	originRequest, err := RedirectedRequest(r, originUrl)
	if err != nil {
		return
	}
	// b, _ := httputil.DumpRequest(originRequest, false)
	// os.Stderr.Write(b)
	originResp, err := client.Do(originRequest)
	if err != nil {
		return
	}
	ForwardResponse(w, originResp)
	return
}
