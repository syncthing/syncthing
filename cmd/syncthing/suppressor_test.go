package main

import (
	"testing"
	"time"
)

func TestSuppressor(t *testing.T) {
	s := suppressor{threshold: 10000}
	t0 := time.Now()

	t1 := t0
	sup, prev := s.suppress("foo", 10000, t1)
	if sup {
		t.Fatal("Never suppress first change")
	}
	if prev {
		t.Fatal("Incorrect prev status")
	}

	// bw is 10000 / 10 = 1000
	t1 = t0.Add(10 * time.Second)
	if bw := s.changes["foo"].bandwidth(t1); bw != 1000 {
		t.Errorf("Incorrect bw %d", bw)
	}
	sup, prev = s.suppress("foo", 10000, t1)
	if sup {
		t.Fatal("Should still be fine")
	}
	if prev {
		t.Fatal("Incorrect prev status")
	}

	// bw is (10000 + 10000) / 11 = 1818
	t1 = t0.Add(11 * time.Second)
	if bw := s.changes["foo"].bandwidth(t1); bw != 1818 {
		t.Errorf("Incorrect bw %d", bw)
	}
	sup, prev = s.suppress("foo", 100500, t1)
	if sup {
		t.Fatal("Should still be fine")
	}
	if prev {
		t.Fatal("Incorrect prev status")
	}

	// bw is (10000 + 10000 + 100500) / 12 = 10041
	t1 = t0.Add(12 * time.Second)
	if bw := s.changes["foo"].bandwidth(t1); bw != 10041 {
		t.Errorf("Incorrect bw %d", bw)
	}
	sup, prev = s.suppress("foo", 10000000, t1) // value will be ignored
	if !sup {
		t.Fatal("Should be over threshold")
	}
	if prev {
		t.Fatal("Incorrect prev status")
	}

	// bw is (10000 + 10000 + 100500) / 15 = 8033
	t1 = t0.Add(15 * time.Second)
	if bw := s.changes["foo"].bandwidth(t1); bw != 8033 {
		t.Errorf("Incorrect bw %d", bw)
	}
	sup, prev = s.suppress("foo", 10000000, t1)
	if sup {
		t.Fatal("Should be Ok")
	}
	if !prev {
		t.Fatal("Incorrect prev status")
	}
}

func TestHistory(t *testing.T) {
	h := changeHistory{}

	t0 := time.Now()
	h.append(40, t0)

	if l := len(h.changes); l != 1 {
		t.Errorf("Incorrect history length %d", l)
	}
	if s := h.changes[0].size; s != 40 {
		t.Errorf("Incorrect first record size %d", s)
	}

	for i := 1; i < MaxChangeHistory; i++ {
		h.append(int64(40+i), t0.Add(time.Duration(i)*time.Second))
	}

	if l := len(h.changes); l != MaxChangeHistory {
		t.Errorf("Incorrect history length %d", l)
	}
	if s := h.changes[0].size; s != 40 {
		t.Errorf("Incorrect first record size %d", s)
	}
	if s := h.changes[MaxChangeHistory-1].size; s != 40+MaxChangeHistory-1 {
		t.Errorf("Incorrect last record size %d", s)
	}

	h.append(999, t0.Add(time.Duration(999)*time.Second))

	if l := len(h.changes); l != MaxChangeHistory {
		t.Errorf("Incorrect history length %d", l)
	}
	if s := h.changes[0].size; s != 41 {
		t.Errorf("Incorrect first record size %d", s)
	}
	if s := h.changes[MaxChangeHistory-1].size; s != 999 {
		t.Errorf("Incorrect last record size %d", s)
	}

}
