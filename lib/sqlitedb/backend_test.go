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

	var v protocol.Vector
	err = db.Update("test", protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test", Size: 42, Version: v.Update(42)},
		{Name: "test2", Size: 42, Version: v.Update(42)},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update("test", protocol.LocalDeviceID, []protocol.FileInfo{
		{Name: "test3", Size: 42, Version: v.Update(42)},
		{Name: "test4", Size: 42, Version: v.Update(42)},
		{Name: "test", Size: 42, Version: v.Update(42)},
	})
	if err != nil {
		t.Fatal(err)
	}

	fi, ok, err := db.Get("test", protocol.LocalDeviceID, "test2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("not found")
	}
	t.Log(fi)

	fi, ok, err = db.GetGlobal("test", "test2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("not found")
	}
	t.Log(fi)

	// err = db.Drop(protocol.LocalDeviceID)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
