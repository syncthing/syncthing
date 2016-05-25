package httptoo

import (
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helloWorld = "hello, world\n"

func helloWorldHandler(w http.ResponseWriter, r *http.Request) {
	// w.Header().Set("Content-Length", strconv.FormatInt(int64(len(helloWorld)), 10))
	w.Write([]byte(helloWorld))
}

func requestResponse(h http.Handler, r *http.Request) (*http.Response, error) {
	s := httptest.NewServer(h)
	defer s.Close()
	return http.DefaultClient.Do(r)
}

func TestGzipHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	helloWorldHandler(rr, nil)
	assert.EqualValues(t, helloWorld, rr.Body.String())

	rr = httptest.NewRecorder()
	GzipHandler(http.HandlerFunc(helloWorldHandler)).ServeHTTP(rr, new(http.Request))
	assert.EqualValues(t, helloWorld, rr.Body.String())

	rr = httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err)
	r.Header.Set("Accept-Encoding", "gzip")
	GzipHandler(http.HandlerFunc(helloWorldHandler)).ServeHTTP(rr, r)
	gr, err := gzip.NewReader(rr.Body)
	require.NoError(t, err)
	defer gr.Close()
	b, err := ioutil.ReadAll(gr)
	require.NoError(t, err)
	assert.EqualValues(t, helloWorld, b)

	s := httptest.NewServer(nil)
	s.Config.Handler = GzipHandler(http.HandlerFunc(helloWorldHandler))
	req, err := http.NewRequest("GET", s.URL, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	gr.Close()
	gr, err = gzip.NewReader(resp.Body)
	require.NoError(t, err)
	defer gr.Close()
	b, err = ioutil.ReadAll(gr)
	require.NoError(t, err)
	assert.EqualValues(t, helloWorld, b)
	assert.EqualValues(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.EqualValues(t, "gzip", resp.Header.Get("Content-Encoding"))
}
