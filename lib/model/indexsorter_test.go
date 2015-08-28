package model

import (
	"os"
	"testing"

	"github.com/syncthing/protocol"
)

func TestInMemoryIndexSorter(t *testing.T) {
	s := inMemoryIndexSorter{}
	s.Enqueue(protocol.FileInfo{Name: "middle", LocalVersion: 10})
	s.Enqueue(protocol.FileInfo{Name: "last", LocalVersion: 12})
	s.Enqueue(protocol.FileInfo{Name: "first", LocalVersion: 9})

	if s.size < 64 || s.size > 256 {
		t.Fatal("wrong size", s.size)
	}

	c := s.Batch()
	if len(c) != 3 {
		t.Fatal("wrong batch size", len(c), "!= 3")
	}
	if c[0].Name != "first" {
		t.Error("Incorrect first element:", c[0].Name)
	}
	if c[1].Name != "middle" {
		t.Error("Incorrect middle element:", c[1].Name)
	}
	if c[2].Name != "last" {
		t.Error("Incorrect last element:", c[2].Name)
	}

	c = s.Batch()
	if len(c) != 0 {
		t.Fatal("wrong batch size", len(c), "!= 0")
	}
}

func TestLeveldbIndexSorter(t *testing.T) {
	s, err := newLeveldbIndexSorter()
	if err != nil {
		t.Fatal(err)
	}
	s.Enqueue(protocol.FileInfo{Name: "middle", LocalVersion: 10})
	s.Enqueue(protocol.FileInfo{Name: "last", LocalVersion: 12})
	s.Enqueue(protocol.FileInfo{Name: "first", LocalVersion: 9})

	c := s.Batch()
	if len(c) != 3 {
		t.Fatal("wrong batch size", len(c), "!= 3")
	}
	if c[0].Name != "first" {
		t.Error("Incorrect first element:", c[0].Name)
	}
	if c[1].Name != "middle" {
		t.Error("Incorrect middle element:", c[1].Name)
	}
	if c[2].Name != "last" {
		t.Error("Incorrect last element:", c[2].Name)
	}

	c = s.Batch()
	if len(c) != 0 {
		t.Fatal("wrong batch size", len(c), "!= 0")
	}

	s.Close()

	if _, err := os.Stat(s.dbPath); err == nil {
		t.Error("db path should have been removed:", s.dbPath)
	}
}

func TestAdaptiveIndexSorter(t *testing.T) {
	s := newIndexSorter()
	defer s.Close()

	as := s.(*adaptiveIndexSorter)
	as.maxInMemorySize = 1

	s.Enqueue(protocol.FileInfo{Name: "middle", LocalVersion: 10})

	if _, ok := as.indexSorter.(*inMemoryIndexSorter); !ok {
		t.Fatal("should be an inMemoryIndexSorter to start with")
	}

	s.Enqueue(protocol.FileInfo{Name: "last", LocalVersion: 12})

	if _, ok := as.indexSorter.(*leveldbIndexSorter); !ok {
		t.Fatal("should have switched to a leveldbIndexSorter")
	}

	s.Enqueue(protocol.FileInfo{Name: "first", LocalVersion: 9})

	c := s.Batch()
	if len(c) != 3 {
		t.Fatal("wrong batch size", len(c), "!= 3")
	}
	if c[0].Name != "first" {
		t.Error("Incorrect first element:", c[0].Name)
	}
	if c[1].Name != "middle" {
		t.Error("Incorrect middle element:", c[1].Name)
	}
	if c[2].Name != "last" {
		t.Error("Incorrect last element:", c[2].Name)
	}

	c = s.Batch()
	if len(c) != 0 {
		t.Fatal("wrong batch size", len(c), "!= 0")
	}

}
