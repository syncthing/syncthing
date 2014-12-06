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

	"github.com/syncthing/syncthing/internal/protocol"
)

var (
	f1 = &protocol.FileInfo{Name: "f1"}
	f2 = &protocol.FileInfo{Name: "f2"}
	f3 = &protocol.FileInfo{Name: "f3"}
	f4 = &protocol.FileInfo{Name: "f4"}
	f5 = &protocol.FileInfo{Name: "f5"}
)

func TestJobQueue(t *testing.T) {
	// Some random actions
	q := NewJobQueue()
	q.Push(f1)
	q.Push(f2)
	q.Push(f3)
	q.Push(f4)

	progress, queued := q.Jobs()
	if len(progress) != 0 || len(queued) != 4 {
		t.Fatal("Wrong length")
	}

	for i := 1; i < 5; i++ {
		n := q.Pop()
		if n == nil || n.Name != fmt.Sprintf("f%d", i) {
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

		q.Done(f5) // Does not exist
		progress, queued = q.Jobs()
		if len(progress) != 0 || len(queued) != 4 {
			t.Fatal("Wrong length")
		}
	}

	if len(q.progress) > 0 || len(q.lookup) != 4 || q.queued.Len() != 4 {
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

		n := q.Pop()
		if n == nil || n.Name != s {
			t.Fatal("Wrong element")
		}
		progress, queued = q.Jobs()
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}

		q.Done(f5) // Does not exist
		progress, queued = q.Jobs()
		if len(progress) != 5-i || len(queued) != i-1 {
			t.Fatal("Wrong length")
		}
	}

	if len(q.progress) != 4 || q.Pop() != nil || len(q.lookup) != 0 {
		t.Fatal("Wrong length")
	}

	q.Done(f1)
	q.Done(f2)
	q.Done(f3)
	q.Done(f4)
	q.Done(f5) // Does not exist

	if len(q.progress) != 0 || q.Pop() != nil || len(q.lookup) != 0 {
		t.Fatal("Wrong length")
	}

	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
	q.Bump("")
	q.Done(f5) // Does not exist
	progress, queued = q.Jobs()
	if len(progress) != 0 || len(queued) != 0 {
		t.Fatal("Wrong length")
	}
}
