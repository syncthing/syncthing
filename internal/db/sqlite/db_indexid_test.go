package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestIndexIDs(t *testing.T) {
	t.Parallel()

	db, err := OpenTemp()
	if err != nil {
		t.Fatal()
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LocalDeviceID", func(t *testing.T) {
		t.Parallel()

		localID, err := db.IndexIDGet("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if localID == 0 {
			t.Fatal("should have been generated")
		}

		again, err := db.IndexIDGet("foo", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if again != localID {
			t.Fatal("should get same again")
		}

		other, err := db.IndexIDGet("bar", protocol.LocalDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if other == localID {
			t.Fatal("should not get same for other folder")
		}
	})

	t.Run("OtherDeviceID", func(t *testing.T) {
		t.Parallel()

		localID, err := db.IndexIDGet("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if localID != 0 {
			t.Fatal("should have been zero")
		}

		newID := protocol.NewIndexID()
		if err := db.IndexIDSet("foo", protocol.DeviceID{42}, newID); err != nil {
			t.Fatal(err)
		}

		again, err := db.IndexIDGet("foo", protocol.DeviceID{42})
		if err != nil {
			t.Fatal(err)
		}
		if again != newID {
			t.Log(again, newID)
			t.Fatal("should get the ID we set")
		}
	})
}
