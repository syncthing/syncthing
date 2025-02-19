package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestIndexIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal()
	}

	t.Run("LocalDeviceID", func(t *testing.T) {
		localID, err := db.IndexID("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if localID == 0 {
			t.Fatal("should have been generated")
		}

		again, err := db.IndexID("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if again != localID {
			t.Fatal("should get same again")
		}

		other, err := db.IndexID("bar", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if other == localID {
			t.Fatal("should not get same for other folder")
		}
	})

	t.Run("OtherDeviceID", func(t *testing.T) {
		localID, err := db.IndexID("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if localID != 0 {
			t.Fatal("should have been zero")
		}

		newID := protocol.NewIndexID()
		if err := db.SetIndexID("foo", protocol.DeviceID{42}, newID); err != nil {
			t.Fatal(err)
		}

		again, err := db.IndexID("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if again != newID {
			t.Fatal("should get the ID we set")
		}
	})
}
