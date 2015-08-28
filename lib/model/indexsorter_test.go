package model

import (
	"testing"

	"github.com/syncthing/protocol"
)

func TestInMemoryIndexSorter(t *testing.T) {
	s := inmemoryIndexSorter{}
	s.Enqueue(protocol.FileInfo{Name: "middle", LocalVersion: 10})
	s.Enqueue(protocol.FileInfo{Name: "last", LocalVersion: 12})
	s.Enqueue(protocol.FileInfo{Name: "first", LocalVersion: 9})

	if s.Size() != 3 {
		t.Fatal("wrong size", s.Size(), "!= 3")
	}

	c := s.GetChunk(2)
	if len(c) != 2 {
		t.Fatal("wrong chunk size", len(c), "!= 2")
	}
	if c[0].Name != "first" {
		t.Error("Incorrect first element:", c[0].Name)
	}
	if c[1].Name != "middle" {
		t.Error("Incorrect middle element:", c[1].Name)
	}

	c = s.GetChunk(2)
	if len(c) != 1 {
		t.Fatal("wrong chunk size", len(c), "!= 1")
	}
	if c[0].Name != "last" {
		t.Error("Incorrect last element:", c[0].Name)
	}

	c = s.GetChunk(2)
	if c != nil {
		t.Fatal("wrong chunk size", len(c), "!= 0")
	}
}
