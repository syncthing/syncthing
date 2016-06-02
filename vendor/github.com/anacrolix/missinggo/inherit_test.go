package missinggo

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type class struct {
	base    interface{}
	visible interface{}
}

func (me class) Base() interface{}    { return me.base }
func (me class) Visible() interface{} { return me.visible }

var _ Inheriter = class{}

func TestInheritNothingVisible(t *testing.T) {
	var i interface{}
	s := class{
		base:    nil,
		visible: nil,
	}
	Dispatch(&i, s)
	s = class{
		base:    nil,
		visible: struct{}{},
	}
	Dispatch(&i, s)
}

func TestInheritVisibleImplemented(t *testing.T) {
	var i io.Reader
	s := class{
		base:    &os.File{},
		visible: new(io.ReadCloser),
	}
	Dispatch(&i, s)
	assert.NotNil(t, i)
}

func TestInheritNotImplemented(t *testing.T) {
	var i io.Reader
	s := class{
		base:    struct{}{},
		visible: new(io.ReadCloser),
	}
	Dispatch(&i, s)
	assert.Nil(t, i)
}

func TestInheritNotVisible(t *testing.T) {
	var i io.Reader
	s := class{
		base:    &os.File{},
		visible: new(interface{}),
	}
	Dispatch(&i, s)
	assert.Nil(t, i)
}
