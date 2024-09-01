package backend

import "github.com/cockroachdb/pebble"

func OpenPebbleDB(location string) (Backend, error) {
	db, err := pebble.Open(location, nil)
	if err != nil {
		return nil, err
	}
	return newPebbleBackend(db, location), nil
}
