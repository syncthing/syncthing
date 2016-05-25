package missinggo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func cryHeard() bool {
	return CryHeard()
}

func TestCrySameLocation(t *testing.T) {
	require.True(t, cryHeard())
	require.True(t, cryHeard())
	require.False(t, cryHeard())
	require.True(t, cryHeard())
	require.False(t, cryHeard())
	require.False(t, cryHeard())
	require.False(t, cryHeard())
	require.True(t, cryHeard())
}

func TestCryDifferentLocations(t *testing.T) {
	require.True(t, CryHeard())
	require.True(t, CryHeard())
	require.True(t, CryHeard())
	require.True(t, CryHeard())
	require.True(t, CryHeard())
}
