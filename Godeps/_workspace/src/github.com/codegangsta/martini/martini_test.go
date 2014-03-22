package martini

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

/* Test Helpers */
func expect(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Errorf("Expected %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}

func refute(t *testing.T, a interface{}, b interface{}) {
	if a == b {
		t.Errorf("Did not expect %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}

func Test_New(t *testing.T) {
	m := New()
	refute(t, m, nil)
}

func Test_Martini_ServeHTTP(t *testing.T) {
	result := ""
	response := httptest.NewRecorder()

	m := New()
	m.Use(func(c Context) {
		result += "foo"
		c.Next()
		result += "ban"
	})
	m.Use(func(c Context) {
		result += "bar"
		c.Next()
		result += "baz"
	})
	m.Action(func(res http.ResponseWriter, req *http.Request) {
		result += "bat"
		res.WriteHeader(http.StatusBadRequest)
	})

	m.ServeHTTP(response, (*http.Request)(nil))

	expect(t, result, "foobarbatbazban")
	expect(t, response.Code, http.StatusBadRequest)
}

func Test_Martini_Handlers(t *testing.T) {
	result := ""
	response := httptest.NewRecorder()

	batman := func(c Context) {
		result += "batman!"
	}

	m := New()
	m.Use(func(c Context) {
		result += "foo"
		c.Next()
		result += "ban"
	})
	m.Handlers(
		batman,
		batman,
		batman,
	)
	m.Action(func(res http.ResponseWriter, req *http.Request) {
		result += "bat"
		res.WriteHeader(http.StatusBadRequest)
	})

	m.ServeHTTP(response, (*http.Request)(nil))

	expect(t, result, "batman!batman!batman!bat")
	expect(t, response.Code, http.StatusBadRequest)
}

func Test_Martini_EarlyWrite(t *testing.T) {
	result := ""
	response := httptest.NewRecorder()

	m := New()
	m.Use(func(res http.ResponseWriter) {
		result += "foobar"
		res.Write([]byte("Hello world"))
	})
	m.Use(func() {
		result += "bat"
	})
	m.Action(func(res http.ResponseWriter) {
		result += "baz"
		res.WriteHeader(http.StatusBadRequest)
	})

	m.ServeHTTP(response, (*http.Request)(nil))

	expect(t, result, "foobar")
	expect(t, response.Code, http.StatusOK)
}
