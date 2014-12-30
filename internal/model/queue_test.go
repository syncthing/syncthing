// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package model

import (
	"fmt"
	"testing"
)

func TestJobQueue(t *testing.T) {
	// Some random actions
	q := NewJobQueue()
	q.Push("f1")
	q.Push("f2")
	q.Push("f3")
	q.Push("f4")

	progress, queued := q.Jobs()
	if len(progress) != 0 || len(queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 1; i < 5; i++ {
		n, ok := q.Pop()
		if !ok || n != fmt.Sprintf("f%d", i) {
			t.Fatal("Wrong element")
		}
		progress, queued = q.Jobs()
		if len(progress) != 1 || len(queued) != 3 {
			t.Fatal("Wrong length")
		}

		q.Done(n)
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 3 {
			t.Fatal("Wrong length", len(progress), len(queued))
		}

		q.Push(n)
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}

		q.Done("f5") // Does not exist
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}
	}

	if len(q.progress) > 0 || len(q.queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 4; i > 0; i-- {
		progress, queued = q.Jobs()
		if len(progress) != 4-i || len(queued) != i {
			t.Fatal("Wrong length")
		}

		s := fmt.Sprintf("f%d", i)

		q.Bump(s)
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

	q.Done("f1")
	q.Done("f2")
	q.Done("f3")
	q.Done("f4")
	q.Done("f5") // Does not exist

	_, ok = q.Pop()
	if len(q.progress) != 0 || ok {
		t.Fatal("Wrong length")
	}

	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
	q.Bump("")
	q.Done("f5") // Does not exist
	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
}

/*
func BenchmarkJobQueuePush(b *testing.B) {
	files := genFiles(b.N)

	q := NewJobQueue()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(&files[i])
	}
}

func BenchmarkJobQueuePop(b *testing.B) {
	files := genFiles(b.N)

	q := NewJobQueue()
	for j := range files {
		q.Push(&files[j])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Pop()
	}
}

func BenchmarkJobQueuePopDone(b *testing.B) {
	files := genFiles(b.N)

	q := NewJobQueue()
	for j := range files {
		q.Push(&files[j])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := q.Pop()
		q.Done(n)
	}
}
*/

func BenchmarkJobQueueBump(b *testing.B) {
	files := genFiles(b.N)

	q := NewJobQueue()
	for _, f := range files {
		q.Push(f.Name)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Bump(files[i].Name)
	}
}

func BenchmarkJobQueuePushPopDone10k(b *testing.B) {
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := NewJobQueue()
		for _, f := range files {
			q.Push(f.Name)
		}
		for range files {
			n, _ := q.Pop()
			q.Done(n)
		}
	}

}
