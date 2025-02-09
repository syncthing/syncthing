package db2

import (
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestOpen(t *testing.T) {
	db, err := Open(filepath.Join(".", "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	err = db.Update("test", protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test", Size: 42},
		{Name: "test2", Size: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update("test", protocol.DeviceID{42}, []protocol.FileInfo{
		{Name: "test3", Size: 42},
		{Name: "test4", Size: 42},
		{Name: "test", Size: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	// err = db.Drop(protocol.DeviceID{42})
	// if err != nil {
	// 	t.Fatal(err)
	// }
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
