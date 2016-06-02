package httptoo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseHTTPContentRange(t *testing.T) {
	for _, _case := range []struct {
		h  string
		cr *BytesContentRange
	}{
		{"", nil},
		{"1-2/*", nil},
		{"bytes=1-2/3", &BytesContentRange{1, 2, 3}},
		{"bytes=12-34/*", &BytesContentRange{12, 34, -1}},
		{" bytes=12-34/*", &BytesContentRange{12, 34, -1}},
		{"  bytes 12-34/56", &BytesContentRange{12, 34, 56}},
		{"  bytes=*/56", &BytesContentRange{-1, -1, 56}},
	} {
		ret, ok := ParseBytesContentRange(_case.h)
		assert.Equal(t, _case.cr != nil, ok)
		if _case.cr != nil {
			assert.Equal(t, *_case.cr, ret)
		}
	}
}
