package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitHostMaybePortNoPort(t *testing.T) {
	hmp := SplitHostMaybePort("some.domain")
	assert.Equal(t, "some.domain", hmp.Host)
	assert.True(t, hmp.NoPort)
	assert.NoError(t, hmp.Err)
}

func TestSplitHostMaybePortPort(t *testing.T) {
	hmp := SplitHostMaybePort("some.domain:123")
	assert.Equal(t, "some.domain", hmp.Host)
	assert.Equal(t, 123, hmp.Port)
	assert.False(t, hmp.NoPort)
	assert.NoError(t, hmp.Err)
}

func TestSplitHostMaybePortBadPort(t *testing.T) {
	hmp := SplitHostMaybePort("some.domain:wat")
	assert.Equal(t, "some.domain", hmp.Host)
	assert.Equal(t, -1, hmp.Port)
	assert.False(t, hmp.NoPort)
	assert.Error(t, hmp.Err)
}
