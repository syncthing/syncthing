package itertools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIterator(t *testing.T) {
	const s = "AAAABBBCCDAABBB"
	si := StringIterator(s)
	for i := range s {
		require.True(t, si.Next())
		require.Equal(t, s[i], si.Value().(byte))
	}
	require.False(t, si.Next())
}
