package files_test

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var items map[string][]byte
var keys map[string]string

const nItems = 10000

func setupMaps() {

	// Set up two simple maps, one "key" => data and one "indirect key" =>
	// "key".

	items = make(map[string][]byte, nItems)
	keys = make(map[string]string, nItems)

	for i := 0; i < nItems; i++ {
		k1 := fmt.Sprintf("key%d", i)
		data := make([]byte, 87)
		_, err := rand.Reader.Read(data)
		if err != nil {
			panic(err)
		}
		items[k1] = data

		k2 := fmt.Sprintf("indirect%d", i)
		keys[k2] = k1
	}
}

func makeK1(s string) []byte {
	k1 := make([]byte, 1+len(s))
	k1[0] = 1
	copy(k1[1:], []byte(s))
	return k1
}

func makeK2(s string) []byte {
	k2 := make([]byte, 1+len(s))
	k2[0] = 2 // Only difference from makeK1
	copy(k2[1:], []byte(s))
	return k2
}

func setItems(db *leveldb.DB) error {
	snap, err := db.GetSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	batch := &leveldb.Batch{}
	for k2, k1 := range keys {
		// Create k1 => item mapping first
		batch.Put(makeK1(k1), items[k1])
		// Then the k2 => k1 mapping
		batch.Put(makeK2(k2), makeK1(k1))
	}
	return db.Write(batch, nil)
}

func clearItems(db *leveldb.DB) error {
	snap, err := db.GetSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	// Iterate from the start of k2 space to the end
	it := snap.NewIterator(&util.Range{Start: []byte{2}, Limit: []byte{2, 0xff, 0xff, 0xff, 0xff}}, nil)
	defer it.Release()

	batch := &leveldb.Batch{}
	for it.Next() {
		k2 := it.Key()
		k1 := it.Value()

		// k1 should exist
		_, err := snap.Get(k1, nil)
		if err != nil {
			return err
		}

		// Delete the k2 => k1 mapping first
		batch.Delete(k2)
		// Then the k1 => key mapping
		batch.Delete(k1)
	}
	return db.Write(batch, nil)
}

func scanItems(db *leveldb.DB) error {
	snap, err := db.GetSnapshot()
	if err != nil {
		return err
	}
	defer snap.Release()

	// Iterate from the start of k2 space to the end
	it := snap.NewIterator(&util.Range{Start: []byte{2}, Limit: []byte{2, 0xff, 0xff, 0xff, 0xff}}, nil)
	defer it.Release()

	for it.Next() {
		// k2 => k1 => data
		k2 := it.Key()
		k1 := it.Value()
		_, err := snap.Get(k1, nil)
		if err != nil {
			log.Printf("k1: %q (%x)", k1, k1)
			log.Printf("k2: %q (%x)", k2, k2)
			return err
		}
	}
	return nil
}

func TestConcurrent(t *testing.T) {
	setupMaps()

	dur := 2 * time.Second
	t0 := time.Now()
	var wg sync.WaitGroup

	os.RemoveAll("testdata/global.db")
	db, err := leveldb.OpenFile("testdata/global.db", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("testdata/global.db")

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := setItems(db); err != nil {
				t.Fatal(err)
			}
			if err := clearItems(db); err != nil {
				t.Fatal(err)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Since(t0) < dur {
			if err := scanItems(db); err != nil {
				t.Fatal(err)
			}
		}
	}()

	wg.Wait()
	db.Close()
}
