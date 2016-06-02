package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJitterDuration(t *testing.T) {
	assert.Zero(t, JitterDuration(0, 0))
	assert.Panics(t, func() { JitterDuration(1, -1) })
}
