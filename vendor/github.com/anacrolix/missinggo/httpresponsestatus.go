package missinggo

import "net/http"

// A http.ResponseWriter that tracks the status of the response. The status
// code, and number of bytes written for example.
type StatusResponseWriter struct {
	RW           http.ResponseWriter
	Code         int
	BytesWritten int64
	visible      interface {
		http.Hijacker
		http.CloseNotifier
	}
}

func (me *StatusResponseWriter) Base() interface{} { return me.RW }
func (me *StatusResponseWriter) Visible() interface{} {
	return &me.visible
}

var (
	_ http.ResponseWriter = &StatusResponseWriter{}
	_ Inheriter           = &StatusResponseWriter{}
)

func (me *StatusResponseWriter) Header() http.Header {
	return me.RW.Header()
}

func (me *StatusResponseWriter) Write(b []byte) (n int, err error) {
	if me.Code == 0 {
		me.Code = 200
	}
	n, err = me.RW.Write(b)
	me.BytesWritten += int64(n)
	return
}

func (me *StatusResponseWriter) WriteHeader(code int) {
	me.RW.WriteHeader(code)
	me.Code = code
}
