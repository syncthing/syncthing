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
