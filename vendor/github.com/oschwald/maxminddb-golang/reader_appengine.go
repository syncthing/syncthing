// +build appengine

package maxminddb

import "io/ioutil"

// Open takes a string path to a MaxMind DB file and returns a Reader
// structure or an error. The database file is opened using a memory map,
// except on Google App Engine where mmap is not supported; there the database
// is loaded into memory. Use the Close method on the Reader object to return
// the resources to the system.
func Open(file string) (*Reader, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	return FromBytes(bytes)
}

// Close unmaps the database file from virtual memory and returns the
// resources to the system. If called on a Reader opened using FromBytes
// or Open on Google App Engine, this method does nothing.
func (r *Reader) Close() error {
	return nil
}
