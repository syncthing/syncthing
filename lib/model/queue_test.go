// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"golang.org/x/exp/slices"
)

func TestJobQueue(t *testing.T) {
	// Some random actions
	q := newJobQueue()
	q.Push("f1", 0, time.Time{})
	q.Push("f2", 0, time.Time{})
	q.Push("f3", 0, time.Time{})
	q.Push("f4", 0, time.Time{})

	progress, queued, _ := q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 4 {
		t.Fatal("Wrong length", len(progress), len(queued))
	}

	for i := 1; i < 5; i++ {
		n, ok := q.Pop()
		if !ok || n != fmt.Sprintf("f%d", i) {
			t.Fatal("Wrong element")
		}
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 1 || len(queued) != 3 {
			t.Log(progress)
			t.Log(queued)
			t.Fatal("Wrong length")
		}

		q.Done(n)
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 3 {
			t.Fatal("Wrong length", len(progress), len(queued))
		}

		q.Push(n, 0, time.Time{})
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}
	}

	if len(q.progress) > 0 || len(q.queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 4; i > 0; i-- {
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		s := fmt.Sprintf("f%d", i)

		q.BringToFront(s)
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		n, ok := q.Pop()
		if !ok || n != s {
			t.Fatal("Wrong element")
		}
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued, _ = q.Jobs(1, 100)
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}
	}

	_, ok := q.Pop()
	if len(q.progress) != 4 || ok {
		t.Fatal("Wrong length")
	}

	q.Done("f1")
	q.Done("f2")
	q.Done("f3")
	q.Done("f4")
	q.Done("f5") // Does not exist

	_, ok = q.Pop()
	if len(q.progress) != 0 || ok {
		t.Fatal("Wrong length")
	}

	progress, queued, _ = q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
	q.BringToFront("")
	q.Done("f5") // Does not exist
	progress, queued, _ = q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
}

func TestBringToFront(t *testing.T) {
	q := newJobQueue()
	q.Push("f1", 0, time.Time{})
	q.Push("f2", 0, time.Time{})
	q.Push("f3", 0, time.Time{})
	q.Push("f4", 0, time.Time{})

	_, queued, _ := q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f1") // corner case: does nothing

	_, queued, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f3")

	_, queued, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f3", "f1", "f2", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f2")

	_, queued, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f2", "f3", "f1", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f4") // corner case: last element

	_, queued, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f4", "f2", "f3", "f1"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}
}

func TestShuffle(t *testing.T) {
	q := newJobQueue()
	q.Push("f1", 0, time.Time{})
	q.Push("f2", 0, time.Time{})
	q.Push("f3", 0, time.Time{})
	q.Push("f4", 0, time.Time{})

	// This test will fail once in eight million times (1 / (4!)^5) :)
	for i := 0; i < 5; i++ {
		q.Shuffle()
		_, queued, _ := q.Jobs(1, 100)
		if l := len(queued); l != 4 {
			t.Fatalf("Weird length %d returned from jobs(1, 100)", l)
		}

		t.Logf("%v", queued)
		if _, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
			// The queue was shuffled
			return
		}
	}

	t.Error("Queue was not shuffled after five attempts.")
}

func TestSortBySize(t *testing.T) {
	q := newJobQueue()
	q.Push("f1", 20, time.Time{})
	q.Push("f2", 40, time.Time{})
	q.Push("f3", 30, time.Time{})
	q.Push("f4", 10, time.Time{})

	q.SortSmallestFirst()

	_, actual, _ := q.Jobs(1, 100)
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from jobs(1, 100)", l)
	}
	expected := []string{"f4", "f1", "f3", "f2"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortSmallestFirst() diff:\n%s", diff)
	}

	q.SortLargestFirst()

	_, actual, _ = q.Jobs(1, 100)
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from jobs(1, 100)", l)
	}
	expected = []string{"f2", "f3", "f1", "f4"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortLargestFirst() diff:\n%s", diff)
	}
}

func TestSortByAge(t *testing.T) {
	q := newJobQueue()
	q.Push("f1", 0, time.Unix(20, 0))
	q.Push("f2", 0, time.Unix(40, 0))
	q.Push("f3", 0, time.Unix(30, 0))
	q.Push("f4", 0, time.Unix(10, 0))

	q.SortOldestFirst()

	_, actual, _ := q.Jobs(1, 100)
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from jobs(1, 100)", l)
	}
	expected := []string{"f4", "f1", "f3", "f2"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortOldestFirst() diff:\n%s", diff)
	}

	q.SortNewestFirst()

	_, actual, _ = q.Jobs(1, 100)
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from jobs(1, 100)", l)
	}
	expected = []string{"f2", "f3", "f1", "f4"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortNewestFirst() diff:\n%s", diff)
	}
}

func BenchmarkJobQueueBump(b *testing.B) {
	files := genFiles(10000)

	q := newJobQueue()
	for _, f := range files {
		q.Push(f.Name, 0, time.Time{})
	}

	rng := rand.New(rand.NewSource(int64(b.N)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rng.Intn(len(files))
		q.BringToFront(files[r].Name)
	}
}

func BenchmarkJobQueuePushPopDone10k(b *testing.B) {
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := newJobQueue()
		for _, f := range files {
			q.Push(f.Name, 0, time.Time{})
		}
		for range files {
			n, _ := q.Pop()
			q.Done(n)
		}
	}
}

func TestQueuePagination(t *testing.T) {
	q := newJobQueue()
	// Ten random actions
	names := make([]string, 10)
	for i := 0; i < 10; i++ {
		names[i] = fmt.Sprint("f", i)
		q.Push(names[i], 0, time.Time{})
	}

	progress, queued, skip := q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 10 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	progress, queued, skip = q.Jobs(1, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[:5]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[:5])
	}

	progress, queued, skip = q.Jobs(2, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 5 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[5:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[5:])
	}

	progress, queued, skip = q.Jobs(2, 7)
	if len(progress) != 0 || len(queued) != 3 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[7:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[7:])
	}

	progress, queued, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	n, ok := q.Pop()
	if !ok || n != names[0] {
		t.Fatal("Wrong element")
	}

	progress, queued, skip = q.Jobs(1, 100)
	if len(progress) != 1 || len(queued) != 9 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	progress, queued, skip = q.Jobs(1, 5)
	if len(progress) != 1 || len(queued) != 4 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[:1]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[:1])
	} else if !slices.Equal(queued, names[1:5]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[1:5])
	}

	progress, queued, skip = q.Jobs(2, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 5 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[5:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[5:])
	}

	progress, queued, skip = q.Jobs(2, 7)
	if len(progress) != 0 || len(queued) != 3 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[7:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[7:])
	}

	progress, queued, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	for i := 1; i < 8; i++ {
		n, ok := q.Pop()
		if !ok || n != names[i] {
			t.Fatal("Wrong element")
		}
	}

	progress, queued, skip = q.Jobs(1, 100)
	if len(progress) != 8 || len(queued) != 2 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	progress, queued, skip = q.Jobs(1, 5)
	if len(progress) != 5 || len(queued) != 0 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[:5]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[:5])
	}

	progress, queued, skip = q.Jobs(2, 5)
	if len(progress) != 3 || len(queued) != 2 || skip != 5 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[5:8]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[5:8])
	} else if !slices.Equal(queued, names[8:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[8:])
	}

	progress, queued, skip = q.Jobs(2, 7)
	if len(progress) != 1 || len(queued) != 2 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[7:8]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[7:8])
	} else if !slices.Equal(queued, names[8:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[8:])
	}

	progress, queued, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}
}
