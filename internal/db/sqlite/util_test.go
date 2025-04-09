package sqlite

import (
	"testing"

	"github.com/syncthing/syncthing/lib/protocol"
)

func TestDbvector(t *testing.T) {
	vec := protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 7}, {ID: 123456789, Value: 42424242}}}
	dbVec := dbVector{vec}
	val, err := dbVec.Value()
	if err != nil {
		t.Fatal(val)
	}

	var dbVec2 dbVector
	if err := dbVec2.Scan(val); err != nil {
		t.Fatal(err)
	}

	if !dbVec2.Vector.Equal(vec) {
		t.Log(vec)
		t.Log(dbVec2.Vector)
		t.Fatal("should match")
	}
}
