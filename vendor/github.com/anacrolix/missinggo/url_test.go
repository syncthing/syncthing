package missinggo

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLOpaquePath(t *testing.T) {
	assert.Equal(t, "sqlite3://sqlite3.db", (&url.URL{Scheme: "sqlite3", Path: "sqlite3.db"}).String())
	u, err := url.Parse("sqlite3:sqlite3.db")
	assert.NoError(t, err)
	assert.Equal(t, "sqlite3.db", URLOpaquePath(u))
	assert.Equal(t, "sqlite3:sqlite3.db", (&url.URL{Scheme: "sqlite3", Opaque: "sqlite3.db"}).String())
	assert.Equal(t, "sqlite3:/sqlite3.db", (&url.URL{Scheme: "sqlite3", Opaque: "/sqlite3.db"}).String())
	u, err = url.Parse("sqlite3:/sqlite3.db")
	assert.NoError(t, err)
	assert.Equal(t, "/sqlite3.db", u.Path)
	assert.Equal(t, "/sqlite3.db", URLOpaquePath(u))
}
