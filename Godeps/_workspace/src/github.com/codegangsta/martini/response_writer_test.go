package martini

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hijackableResponse struct {
	Hijacked bool
}

func newHijackableResponse() *hijackableResponse {
	return &hijackableResponse{}
}

func (h *hijackableResponse) Header() http.Header           { return nil }
func (h *hijackableResponse) Write(buf []byte) (int, error) { return 0, nil }
func (h *hijackableResponse) WriteHeader(code int)          {}
func (h *hijackableResponse) Flush()                        {}
func (h *hijackableResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.Hijacked = true
	return nil, nil, nil
}

func Test_ResponseWriter_WritingString(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.Write([]byte("Hello world"))

	expect(t, rec.Code, rw.Status())
	expect(t, rec.Body.String(), "Hello world")
	expect(t, rw.Status(), http.StatusOK)
	expect(t, rw.Size(), 11)
	expect(t, rw.Written(), true)
}

func Test_ResponseWriter_WritingStrings(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.Write([]byte("Hello world"))
	rw.Write([]byte("foo bar bat baz"))

	expect(t, rec.Code, rw.Status())
	expect(t, rec.Body.String(), "Hello worldfoo bar bat baz")
	expect(t, rw.Status(), http.StatusOK)
	expect(t, rw.Size(), 26)
}

func Test_ResponseWriter_WritingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	rw.WriteHeader(http.StatusNotFound)

	expect(t, rec.Code, rw.Status())
	expect(t, rec.Body.String(), "")
	expect(t, rw.Status(), http.StatusNotFound)
	expect(t, rw.Size(), 0)
}

func Test_ResponseWriter_Hijack(t *testing.T) {
	hijackable := newHijackableResponse()
	rw := NewResponseWriter(hijackable)
	hijacker, ok := rw.(http.Hijacker)
	expect(t, ok, true)
	_, _, err := hijacker.Hijack()
	if err != nil {
		t.Error(err)
	}
	expect(t, hijackable.Hijacked, true)
}
