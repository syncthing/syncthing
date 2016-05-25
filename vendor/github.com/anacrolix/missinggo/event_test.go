package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetEvent(t *testing.T) {
	var e Event
	e.Set()
}

func TestEventIsSet(t *testing.T) {
	var e Event
	assert.False(t, e.IsSet())
	e.Set()
	assert.True(t, e.IsSet())
}
