package missinggo

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSectionReadSeekerReadBeyondEnd(t *testing.T) {
	base := bytes.NewReader([]byte{1, 2, 3})
	srs := NewSectionReadSeeker(base, 1, 1)
	dest := new(bytes.Buffer)
	n, err := io.Copy(dest, srs)
	assert.EqualValues(t, 1, n)
	assert.NoError(t, err)
}

func TestSectionReadSeekerSeekEnd(t *testing.T) {
	base := bytes.NewReader([]byte{1, 2, 3})
	srs := NewSectionReadSeeker(base, 1, 1)
	off, err := srs.Seek(0, os.SEEK_END)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, off)
}
