// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/config"
)

func TestJobQueue(t *testing.T) {
	// Some random actions
	q := newJobQueue(config.OrderAlphabetic, "")
	defer q.Close()
	for i := 4; i > 0; i-- {
		if err := q.Push(fmt.Sprintf("f%d", i), 0, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	progress, queued := q.Jobs()
	if len(progress) != 0 || len(queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 1; i < 5; i++ {
		n, ok := q.Pop()
		if !ok || n != "f1" {
			t.Fatal("Wrong element")
		}

		if progress, queued = q.Jobs(); len(progress) != 1 || len(queued) != 3 {
			t.Log(progress)
			t.Log(queued)
			t.Fatal("Wrong length")
		}
		progress, queued = q.Jobs()
		q.Done(n)
		if progress, queued = q.Jobs(); len(progress) != 0 || len(queued) != 3 {
			t.Log(queued)
			t.Fatal("Wrong length", len(progress), len(queued))
		}

		t.Log(queued)
		if err := q.Push(n, 0, time.Time{}); err != nil {
			t.Fatal(err)
		}
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 4 {
			t.Log(progress, queued)
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}
	}

	if progress, queued = q.Jobs(); len(progress) > 0 || len(queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 4; i > 0; i-- {
		progress, queued = q.Jobs()
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		s := fmt.Sprintf("f%d", i)

		q.BringToFront(s)
		progress, queued = q.Jobs()
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		n, ok := q.Pop()
		if !ok || n != s {
			t.Fatal("Wrong element")
		}
		progress, queued = q.Jobs()
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued = q.Jobs()
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}
	}

	_, ok := q.Pop()
	if len(q.progress) != 4 || ok {
		t.Fatal("Wrong length")
	}

	for i := 1; i < 5; i++ {
		q.Done(fmt.Sprintf("f%d", i))
	}
	q.Done("f5") // Does not exist

	_, ok = q.Pop()
	if len(q.progress) != 0 || ok {
		t.Fatal("Wrong length")
	}

	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
	q.BringToFront("")
	q.Done("f9") // Does not exist
	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
}

func TestBringToFront(t *testing.T) {
	q := newJobQueue(config.OrderAlphabetic, "")
	defer q.Close()
	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), 0, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	_, queued := q.Jobs()
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f1") // corner case: does nothing

	_, queued = q.Jobs()
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f3")

	_, queued = q.Jobs()
	if diff, equal := messagediff.PrettyDiff([]string{"f3", "f1", "f2", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f2")

	_, queued = q.Jobs()
	if diff, equal := messagediff.PrettyDiff([]string{"f2", "f3", "f1", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f4") // corner case: last element

	_, queued = q.Jobs()
	if diff, equal := messagediff.PrettyDiff([]string{"f4", "f2", "f3", "f1"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}
}

func TestShuffle(t *testing.T) {
	q := newJobQueue(config.OrderRandom, "")
	defer q.Close()
	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), 0, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	// This test will fail once in eight million times (1 / (4!)^5) :)
	for i := 0; i < 5; i++ {
		_, queued := q.Jobs()
		if l := len(queued); l != 4 {
			t.Fatalf("Weird length %d returned from Jobs()", l)
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
	q := newJobQueue(config.OrderSmallestFirst, "")
	sizes := []int64{20, 40, 30, 10}
	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), sizes[i-1], time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	_, actual := q.Jobs()
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from Jobs()", l)
	}
	expected := []string{"f4", "f1", "f3", "f2"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortSmallestFirst() diff:\n%s", diff)
	}

	q.Close()
	q = newJobQueue(config.OrderLargestFirst, "")
	defer q.Close()

	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), sizes[i-1], time.Time{}); err != nil {
			t.Fatal(err)
		}
	}

	_, actual = q.Jobs()
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from Jobs()", l)
	}
	expected = []string{"f2", "f3", "f1", "f4"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		l.Infoln(expected)
		l.Infoln(actual)
		t.Errorf("SortLargestFirst() diff:\n%s", diff)
	}
}

func TestSortByAge(t *testing.T) {
	q := newJobQueue(config.OrderOldestFirst, "")
	times := []int64{20, 40, 30, 10}
	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), 0, time.Unix(times[i-1], 0)); err != nil {
			t.Fatal(err)
		}
	}

	_, actual := q.Jobs()
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from Jobs()", l)
	}
	expected := []string{"f4", "f1", "f3", "f2"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortOldestFirst() diff:\n%s", diff)
	}

	q.Close()
	q = newJobQueue(config.OrderNewestFirst, "")
	defer q.Close()

	for i := 1; i < 5; i++ {
		if err := q.Push(fmt.Sprintf("f%d", i), 0, time.Unix(times[i-1], 0)); err != nil {
			t.Fatal(err)
		}
	}

	_, actual = q.Jobs()
	if l := len(actual); l != 4 {
		t.Fatalf("Weird length %d returned from Jobs()", l)
	}
	expected = []string{"f2", "f3", "f1", "f4"}

	if diff, equal := messagediff.PrettyDiff(expected, actual); !equal {
		t.Errorf("SortNewestFirst() diff:\n%s", diff)
	}
}

func BenchmarkJobQueueBump(b *testing.B) {
	len := 1000

	files := genFiles(len)

	q := newJobQueue(config.OrderAlphabetic, "")
	defer q.Close()
	for _, f := range files {
		if err := q.Push(f.Name, 0, time.Time{}); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.BringToFront(files[i%len].Name)
	}
}

func BenchmarkJobQueuePushPopDone10k(b *testing.B) {
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := newJobQueue(config.OrderAlphabetic, "")
		for _, f := range files {
			if err := q.Push(f.Name, 0, time.Time{}); err != nil {
				b.Fatal(err)
			}
		}
		for range files {
			n, _ := q.Pop()
			q.Done(n)
		}
		q.Close()
	}

}
