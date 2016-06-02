package filecache

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/missinggo"
)

func TestCache(t *testing.T) {
	td, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(td)
	c, err := NewCache(filepath.Join(td, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	assert.EqualValues(t, 0, c.Info().Filled)
	c.WalkItems(func(i ItemInfo) {})
	_, err = c.OpenFile("/", os.O_CREATE)
	assert.NotNil(t, err)
	_, err = c.OpenFile("", os.O_CREATE)
	assert.NotNil(t, err)
	c.WalkItems(func(i ItemInfo) {})
	require.Equal(t, 0, c.Info().NumItems)
	_, err = c.OpenFile("notexist", 0)
	assert.True(t, os.IsNotExist(err), err)
	_, err = c.OpenFile("/notexist", 0)
	assert.True(t, os.IsNotExist(err), err)
	_, err = c.OpenFile("/dir/notexist", 0)
	assert.True(t, os.IsNotExist(err), err)
	f, err := c.OpenFile("dir/blah", os.O_CREATE)
	require.NoError(t, err)
	defer f.Close()
	c.WalkItems(func(i ItemInfo) {})
	assert.True(t, missinggo.FilePathExists(filepath.Join(td, filepath.FromSlash("cache/dir/blah"))))
	assert.True(t, missinggo.FilePathExists(filepath.Join(td, filepath.FromSlash("cache/dir/"))))
	assert.Equal(t, 1, c.Info().NumItems)
	f.Remove()
	assert.False(t, missinggo.FilePathExists(filepath.Join(td, filepath.FromSlash("dir/blah"))))
	assert.False(t, missinggo.FilePathExists(filepath.Join(td, filepath.FromSlash("dir/"))))
	_, err = f.Read(nil)
	assert.NotEqual(t, io.EOF, err)
	a, err := c.OpenFile("/a", os.O_CREATE|os.O_WRONLY)
	defer a.Close()
	require.Nil(t, err)
	b, err := c.OpenFile("b", os.O_CREATE|os.O_WRONLY)
	defer b.Close()
	require.Nil(t, err)
	c.mu.Lock()
	assert.True(t, c.pathInfo("a").Accessed.Before(c.pathInfo("b").Accessed))
	c.mu.Unlock()
	n, err := a.Write([]byte("hello"))
	assert.Nil(t, err)
	assert.EqualValues(t, 5, n)
	assert.EqualValues(t, 5, c.Info().Filled)
	assert.True(t, c.pathInfo("b").Accessed.Before(c.pathInfo("a").Accessed))
	c.SetCapacity(5)
	n, err = a.Write([]byte(" world"))
	assert.NotNil(t, err)
	_, err = b.Write([]byte("boom!"))
	// "a" and "b" have been evicted.
	assert.NotNil(t, err)
	assert.EqualValues(t, 0, c.Info().Filled)
	assert.EqualValues(t, 0, c.Info().NumItems)
	_, err = a.Seek(0, os.SEEK_SET)
	assert.NotNil(t, err)
}

func TestSanitizePath(t *testing.T) {
	assert.Equal(t, "", sanitizePath("////"))
	assert.Equal(t, "", sanitizePath("/../.."))
	assert.Equal(t, "a", sanitizePath("/a//b/.."))
	assert.Equal(t, "a", sanitizePath("../a"))
	assert.Equal(t, "a", sanitizePath("./a"))
}
