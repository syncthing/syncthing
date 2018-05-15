package db

import (
	"encoding/json"
	"io"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// writeJSONS serializes the database to a JSON stream that can be checked
// in to the repo and used for tests.
func writeJSONS(w io.Writer, db *leveldb.DB) {
	it := db.NewIterator(&util.Range{}, nil)
	defer it.Release()
	enc := json.NewEncoder(w)
	for it.Next() {
		enc.Encode(map[string][]byte{
			"k": it.Key(),
			"v": it.Value(),
		})
	}
}

// openJSONS reads a JSON stream file into a leveldb.DB
func openJSONS(file string) (*leveldb.DB, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(fd)

	db, _ := leveldb.Open(storage.NewMemStorage(), nil)

	for {
		var row map[string][]byte

		err := dec.Decode(&row)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		db.Put(row["k"], row["v"], nil)
	}

	return db, nil
}

// The commented out test below shows how to prepare a JSONS database file
// for future tests.

// func TestPrepareDBWithInvalidFile(t *testing.T) {
// 	db := OpenMemory()
// 	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
// 	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
// 		{ // invalid (ignored) file
// 			Name:    "foo",
// 			Type:    protocol.FileInfoTypeFile,
// 			Invalid: true,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1000}}},
// 		},
// 		{ // regular file
// 			Name:    "bar",
// 			Type:    protocol.FileInfoTypeFile,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
// 		},
// 	})
// 	fs.Update(protocol.DeviceID{42}, []protocol.FileInfo{
// 		{ // invalid file
// 			Name:    "baz",
// 			Type:    protocol.FileInfoTypeFile,
// 			Invalid: true,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1000}}},
// 		},
// 		{ // regular file
// 			Name:    "quux",
// 			Type:    protocol.FileInfoTypeFile,
// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1002}}},
// 		},
// 	})
// 	writeJSONS(os.Stdout, db.DB)
// }
