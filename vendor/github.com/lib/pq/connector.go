// +build go1.10

package pq

import (
	"context"
	"database/sql/driver"
)

// Connector represents a fixed configuration for the pq driver with a given
// name. Connector satisfies the database/sql/driver Connector interface and
// can be used to create any number of DB Conn's via the database/sql OpenDB
// function.
//
// See https://golang.org/pkg/database/sql/driver/#Connector.
// See https://golang.org/pkg/database/sql/#OpenDB.
type connector struct {
	name string
}

// Connect returns a connection to the database using the fixed configuration
// of this Connector. Context is not used.
func (c *connector) Connect(_ context.Context) (driver.Conn, error) {
	return (&Driver{}).Open(c.name)
}

// Driver returnst the underlying driver of this Connector.
func (c *connector) Driver() driver.Driver {
	return &Driver{}
}

var _ driver.Connector = &connector{}

// NewConnector returns a connector for the pq driver in a fixed configuration
// with the given name. The returned connector can be used to create any number
// of equivalent Conn's. The returned connector is intended to be used with
// database/sql.OpenDB.
//
// See https://golang.org/pkg/database/sql/driver/#Connector.
// See https://golang.org/pkg/database/sql/#OpenDB.
func NewConnector(name string) (driver.Connector, error) {
	return &connector{name: name}, nil
}
