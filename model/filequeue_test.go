package model

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFileQueueAdd(t *testing.T) {
	q := NewFileQueue()
	q.Add("foo", nil, nil)
}

func TestFileQueueAddSorting(t *testing.T) {
	q := NewFileQueue()
	q.SetAvailable("zzz", []string{"nodeID"})
	q.SetAvailable("aaa", []string{"nodeID"})

	q.Add("zzz", []Block{{Offset: 0, Size: 128}, {Offset: 128, Size: 128}}, nil)
	q.Add("aaa", []Block{{Offset: 0, Size: 128}, {Offset: 128, Size: 128}}, nil)
	b, _ := q.Get("nodeID")
	if b.name != "aaa" {
		t.Errorf("Incorrectly sorted get: %+v", b)
	}

	q = NewFileQueue()
	q.SetAvailable("zzz", []string{"nodeID"})
	q.SetAvailable("aaa", []string{"nodeID"})

	q.Add("zzz", []Block{{Offset: 0, Size: 128}, {Offset: 128, Size: 128}}, nil)
	b, _ = q.Get("nodeID") // Start on zzzz
	if b.name != "zzz" {
		t.Errorf("Incorrectly sorted get: %+v", b)
	}
	q.Add("aaa", []Block{{Offset: 0, Size: 128}, {Offset: 128, Size: 128}}, nil)
	b, _ = q.Get("nodeID")
	if b.name != "zzz" {
		// Continue rather than starting a new file
		t.Errorf("Incorrectly sorted get: %+v", b)
	}
}

func TestFileQueueLen(t *testing.T) {
	q := NewFileQueue()
	q.Add("foo", nil, nil)
	q.Add("bar", nil, nil)

	if l := q.Len(); l != 2 {
		t.Errorf("Incorrect len %d != 2 after adds", l)
	}
}

func TestFileQueueGet(t *testing.T) {
	q := NewFileQueue()
	q.SetAvailable("foo", []string{"nodeID"})
	q.SetAvailable("bar", []string{"nodeID"})

	q.Add("foo", []Block{
		{Offset: 0, Size: 128, Hash: []byte("some foo hash bytes")},
		{Offset: 128, Size: 128, Hash: []byte("some other foo hash bytes")},
		{Offset: 256, Size: 128, Hash: []byte("more foo hash bytes")},
	}, nil)
	q.Add("bar", []Block{
		{Offset: 0, Size: 128, Hash: []byte("some bar hash bytes")},
		{Offset: 128, Size: 128, Hash: []byte("some other bar hash bytes")},
	}, nil)

	// First get should return the first block of the first file

	expected := queuedBlock{
		name: "bar",
		block: Block{
			Offset: 0,
			Size:   128,
			Hash:   []byte("some bar hash bytes"),
		},
	}
	actual, ok := q.Get("nodeID")

	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned (first)\n  E: %+v\n  A: %+v", expected, actual)
	}

	// Second get should return the next block of the first file

	expected = queuedBlock{
		name: "bar",
		block: Block{
			Offset: 128,
			Size:   128,
			Hash:   []byte("some other bar hash bytes"),
		},
		index: 1,
	}
	actual, ok = q.Get("nodeID")

	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned (second)\n  E: %+v\n  A: %+v", expected, actual)
	}

	// Third get should return the first block of the second file

	expected = queuedBlock{
		name: "foo",
		block: Block{
			Offset: 0,
			Size:   128,
			Hash:   []byte("some foo hash bytes"),
		},
	}
	actual, ok = q.Get("nodeID")

	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned (third)\n  E: %+v\n  A: %+v", expected, actual)
	}
}

/*
func TestFileQueueDone(t *testing.T) {
	ch := make(chan content)
	var recv sync.WaitGroup
	recv.Add(1)
	go func() {
		content := <-ch
		if bytes.Compare(content.data, []byte("first block bytes")) != 0 {
			t.Error("Incorrect data in first content block")
		}

		content = <-ch
		if bytes.Compare(content.data, []byte("second block bytes")) != 0 {
			t.Error("Incorrect data in second content block")
		}

		_, ok := <-ch
		if ok {
			t.Error("Content channel not closed")
		}

		recv.Done()
	}()

	q := FileQueue{resolver: fakeResolver{}}
	q.Add("foo", []Block{
		{Offset: 0, Length: 128, Hash: []byte("some foo hash bytes")},
		{Offset: 128, Length: 128, Hash: []byte("some other foo hash bytes")},
	}, ch)

	b0, _ := q.Get("nodeID")
	b1, _ := q.Get("nodeID")

	q.Done(b0.name, b0.block.Offset, []byte("first block bytes"))
	q.Done(b1.name, b1.block.Offset, []byte("second block bytes"))

	recv.Wait()

	// Queue should now have one file less

	if l := q.Len(); l != 0 {
		t.Error("Queue not empty")
	}

	_, ok := q.Get("nodeID")
	if ok {
		t.Error("Unexpected OK Get()")
	}
}
*/

func TestFileQueueGetNodeIDs(t *testing.T) {
	q := NewFileQueue()
	q.SetAvailable("a-foo", []string{"nodeID", "a"})
	q.SetAvailable("b-bar", []string{"nodeID", "b"})

	q.Add("a-foo", []Block{
		{Offset: 0, Size: 128, Hash: []byte("some foo hash bytes")},
		{Offset: 128, Size: 128, Hash: []byte("some other foo hash bytes")},
		{Offset: 256, Size: 128, Hash: []byte("more foo hash bytes")},
	}, nil)
	q.Add("b-bar", []Block{
		{Offset: 0, Size: 128, Hash: []byte("some bar hash bytes")},
		{Offset: 128, Size: 128, Hash: []byte("some other bar hash bytes")},
	}, nil)

	expected := queuedBlock{
		name: "b-bar",
		block: Block{
			Offset: 0,
			Size:   128,
			Hash:   []byte("some bar hash bytes"),
		},
	}
	actual, ok := q.Get("b")
	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned\n  E: %+v\n  A: %+v", expected, actual)
	}

	expected = queuedBlock{
		name: "a-foo",
		block: Block{
			Offset: 0,
			Size:   128,
			Hash:   []byte("some foo hash bytes"),
		},
	}
	actual, ok = q.Get("a")
	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned\n  E: %+v\n  A: %+v", expected, actual)
	}

	expected = queuedBlock{
		name: "a-foo",
		block: Block{
			Offset: 128,
			Size:   128,
			Hash:   []byte("some other foo hash bytes"),
		},
		index: 1,
	}
	actual, ok = q.Get("nodeID")
	if !ok {
		t.Error("Unexpected non-OK Get()")
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Incorrect block returned\n  E: %+v\n  A: %+v", expected, actual)
	}
}

func TestFileQueueThreadHandling(t *testing.T) {
	// This should pass with go test -race

	const n = 100
	var total int
	var blocks []Block
	for i := 1; i <= n; i++ {
		blocks = append(blocks, Block{Offset: int64(i), Size: 1})
		total += i
	}

	q := NewFileQueue()
	q.Add("foo", blocks, nil)
	q.SetAvailable("foo", []string{"nodeID"})

	var start = make(chan bool)
	var gotTot uint32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 1; i <= n; i++ {
		go func() {
			<-start
			b, _ := q.Get("nodeID")
			atomic.AddUint32(&gotTot, uint32(b.block.Offset))
			wg.Done()
		}()
	}

	close(start)
	wg.Wait()
	if int(gotTot) != total {
		t.Error("Total mismatch; %d != %d", gotTot, total)
	}
}

func TestDeleteAt(t *testing.T) {
	q := FileQueue{}

	for i := 0; i < 4; i++ {
		q.files = queuedFileList{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}}
		q.deleteAt(i)
		if l := len(q.files); l != 3 {
			t.Fatal("deleteAt(%d) failed; %d != 3", i, l)
		}
	}

	q.files = queuedFileList{{name: "a"}}
	q.deleteAt(0)
	if l := len(q.files); l != 0 {
		t.Fatal("deleteAt(only) failed; %d != 0", l)
	}
}
