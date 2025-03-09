package sqlite

import (
	"testing"
	"time"
)

func TestMtimePairs(t *testing.T) {
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

	t0 := time.Now().Truncate(time.Second)
	t1 := t0.Add(1234567890)

	// Set a pair
	if err := db.MtimePut("foo", "bar", t0, t1); err != nil {
		t.Fatal(err)
	}

	// Check it
	gt0, gt1 := db.MtimeGet("foo", "bar")
	if !gt0.Equal(t0) || !gt1.Equal(t1) {
		t.Log(t0, gt0)
		t.Log(t1, gt1)
		t.Log("bad times")
	}

	// Delete it
	if err := db.MtimeDelete("foo", "bar"); err != nil {
		t.Fatal(err)
	}

	// Check it
	gt0, gt1 = db.MtimeGet("foo", "bar")
	if !gt0.IsZero() || !gt1.IsZero() {
		t.Log(gt0, gt1)
		t.Log("bad times")
	}
}
