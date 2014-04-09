package auth

import (
	"encoding/base64"
	"github.com/codegangsta/martini"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_BasicAuth(t *testing.T) {
	recorder := httptest.NewRecorder()

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("foo:bar"))

	m := martini.New()
	m.Use(Basic("foo", "bar"))
	m.Use(func(res http.ResponseWriter, req *http.Request) {
		res.Write([]byte("hello"))
	})

	r, _ := http.NewRequest("GET", "foo", nil)

	m.ServeHTTP(recorder, r)

	if recorder.Code != 401 {
		t.Error("Response not 401")
	}

	if recorder.Body.String() == "hello" {
		t.Error("Auth block failed")
	}

	recorder = httptest.NewRecorder()
	r.Header.Set("Authorization", auth)
	m.ServeHTTP(recorder, r)

	if recorder.Code == 401 {
		t.Error("Response is 401")
	}

	if recorder.Body.String() != "hello" {
		t.Error("Auth failed, got: ", recorder.Body.String())
	}
}
