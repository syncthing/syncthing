// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package model

import (
	"fmt"
	"iter"
	"math/rand"
	"slices"
	"testing"

	"github.com/d4l3k/messagediff"
	"github.com/syncthing/syncthing/lib/protocol"
)

func TestJobQueue(t *testing.T) {
	// Some random actions
	q := newFilenameJobQueue([]string{
		"f1",
		"f2",
		"f3",
		"f4",
	})

	progress, queued, _, _ := q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 4 {
		t.Fatal("Wrong length", len(progress), len(queued))
	}

	for i := 1; i < 5; i++ {
		n := fmt.Sprintf("f%d", i)
		q.Start(fmt.Sprintf("f%d", i))
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 1 || len(queued) != 3 {
			t.Log(progress)
			t.Log(queued)
			t.Fatal("Wrong length")
		}

		q.Done(n)
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 3 {
			t.Fatal("Wrong length on iteration", i, len(progress), len(queued))
		}

		q.add(n)
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}
	}

	if len(q.progress) > 0 || len(q.files) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 4; i > 0; i-- {
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		s := fmt.Sprintf("f%d", i)

		q.BringToFront(s)
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		n, ok := q.StartPrioritized()
		if !ok || n.Name != s {
			t.Fatalf("Wrong element, got %v, expected %v", n, s)
		}
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued, _, _ = q.Jobs(1, 100)
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}
	}

	_, ok := q.StartPrioritized()
	if len(q.progress) != 4 || ok {
		t.Fatal("Wrong length")
	}

	q.Done("f1")
	q.Done("f2")
	q.Done("f3")
	q.Done("f4")
	q.Done("f5") // Does not exist

	_, ok = q.StartPrioritized()
	if len(q.progress) != 0 || ok {
		t.Fatal("Wrong length")
	}

	progress, queued, _, _ = q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
	q.BringToFront("")
	q.Done("f5") // Does not exist
	progress, queued, _, _ = q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length", len(progress), len(queued))
	}
}

func TestBringToFront(t *testing.T) {
	q := newFilenameJobQueue([]string{
		"f1",
		"f2",
		"f3",
		"f4",
	})

	_, queued, _, _ := q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f1") // corner case: does nothing

	_, queued, _, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f1", "f2", "f3", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f3")

	_, queued, _, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f3", "f1", "f2", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f2")

	_, queued, _, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f2", "f3", "f1", "f4"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}

	q.BringToFront("f4") // corner case: last element

	_, queued, _, _ = q.Jobs(1, 100)
	if diff, equal := messagediff.PrettyDiff([]string{"f4", "f2", "f3", "f1"}, queued); !equal {
		t.Errorf("Order does not match. Diff:\n%s", diff)
	}
}

func BenchmarkJobQueueBump(b *testing.B) {
	files := genFiles(10000)

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	q := newFilenameJobQueue(names)

	rng := rand.New(rand.NewSource(int64(b.N)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rng.Intn(len(files))
		q.BringToFront(files[r].Name)
	}
}

func TestQueuePagination(t *testing.T) {
	// Ten random actions
	names := make([]string, 10)
	for i := 0; i < 10; i++ {
		names[i] = fmt.Sprint("f", i)
	}
	q := newFilenameJobQueue(names)

	progress, queued, _, skip := q.Jobs(1, 100)
	if len(progress) != 0 || len(queued) != 10 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	progress, queued, _, skip = q.Jobs(1, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[:5]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[:5])
	}

	progress, queued, _, skip = q.Jobs(2, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 5 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[5:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[5:])
	}

	progress, queued, _, skip = q.Jobs(2, 7)
	if len(progress) != 0 || len(queued) != 3 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[7:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[7:])
	}

	progress, queued, _, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	q.Start(names[0])

	progress, queued, _, skip = q.Jobs(1, 100)
	if len(progress) != 1 || len(queued) != 9 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	progress, queued, _, skip = q.Jobs(1, 5)
	if len(progress) != 1 || len(queued) != 4 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[:1]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[:1])
	} else if !slices.Equal(queued, names[1:5]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[1:5])
	}

	progress, queued, _, skip = q.Jobs(2, 5)
	if len(progress) != 0 || len(queued) != 5 || skip != 5 {
		t.Fatal("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[5:]) {
		t.Fatalf("Wrong elements in queued, got %v, expected %v", queued, names[5:])
	}

	progress, queued, _, skip = q.Jobs(2, 7)
	if len(progress) != 0 || len(queued) != 3 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(queued, names[7:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[7:])
	}

	progress, queued, _, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	for i := 1; i < 8; i++ {
		q.Start(names[i])
	}

	progress, queued, _, skip = q.Jobs(1, 100)
	if len(progress) != 8 || len(queued) != 2 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}

	l.Debugln("before fail")
	progress, queued, _, skip = q.Jobs(1, 5)
	if len(progress) != 5 || len(queued) != 0 || skip != 0 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[:5]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[:5])
	}

	progress, queued, _, skip = q.Jobs(2, 5)
	if len(progress) != 3 || len(queued) != 2 || skip != 5 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[5:8]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[5:8])
	} else if !slices.Equal(queued, names[8:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[8:])
	}

	progress, queued, _, skip = q.Jobs(2, 7)
	if len(progress) != 1 || len(queued) != 2 || skip != 7 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	} else if !slices.Equal(progress, names[7:8]) {
		t.Errorf("Wrong elements in progress, got %v, expected %v", progress, names[7:8])
	} else if !slices.Equal(queued, names[8:]) {
		t.Errorf("Wrong elements in queued, got %v, expected %v", queued, names[8:])
	}

	progress, queued, _, skip = q.Jobs(3, 5)
	if len(progress) != 0 || len(queued) != 0 || skip != 10 {
		t.Error("Wrong length", len(progress), len(queued), 0)
	}
}

type filenameJobQueue struct {
	*jobQueue
	files []protocol.FileInfo
}

func newFilenameJobQueue(filenames []string) *filenameJobQueue {
	q := &filenameJobQueue{
		files: make([]protocol.FileInfo, 0, len(filenames)),
	}
	for _, n := range filenames {
		q.add(n)
	}
	getNeeded := func(name string) (protocol.FileInfo, bool) {
		i := slices.IndexFunc(q.files, func(f protocol.FileInfo) bool { return f.Name == name})
		if i < 0 {
			return protocol.FileInfo{}, false
		}
		return q.files[i], true
	}
	iterFn := func() iter.Seq2[protocol.FileInfo, error] {
		return func(yield func(protocol.FileInfo, error) bool) {
			for _, file := range q.files {
				if !yield(file, nil) {
					break
				}
			}
		}
	}
	q.jobQueue = newJobQueue(getNeeded, iterFn)
	return q
}

func (q *filenameJobQueue) add(file string) {
	q.files = append(q.files, protocol.FileInfo{
		Name: file,
		Type: protocol.FileInfoTypeFile,
	})
}

func (q *filenameJobQueue) Done(file string) {
	l.Debugln("filenameJobQueue.Done", len(q.files))
	q.jobQueue.Done(file)
	q.files = slices.DeleteFunc(q.files, func(f protocol.FileInfo) bool { return f.Name == file})
	l.Debugln("filenameJobQueue.Done after", len(q.files))
}
