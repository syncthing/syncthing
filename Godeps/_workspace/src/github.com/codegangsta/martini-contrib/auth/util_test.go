package auth

import (
	"testing"
)

var comparetests = []struct {
	a   string
	b   string
	val bool
}{
	{"foo", "foo", true},
	{"bar", "bar", true},
	{"password", "password", true},
	{"Foo", "foo", false},
	{"foo", "foobar", false},
	{"password", "pass", false},
}

func Test_SecureCompare(t *testing.T) {
	for _, tt := range comparetests {
		if SecureCompare(tt.a, tt.b) != tt.val {
			t.Errorf("Expected SecureCompare(%v, %v) to return %v but did not", tt.a, tt.b, tt.val)
		}
	}
}
