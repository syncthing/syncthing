package martini

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_Recovery(t *testing.T) {
	buff := bytes.NewBufferString("")
	recorder := httptest.NewRecorder()

	m := New()
	// replace log for testing
	m.Map(log.New(buff, "[martini] ", 0))
	m.Use(Recovery())
	m.Use(func(res http.ResponseWriter, req *http.Request) {
		panic("here is a panic!")
	})
	m.ServeHTTP(recorder, (*http.Request)(nil))
	expect(t, recorder.Code, http.StatusInternalServerError)
	refute(t, len(buff.String()), 0)

}
