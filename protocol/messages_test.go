package protocol

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"testing"
	"testing/quick"
)

func TestIndex(t *testing.T) {
	idx := []FileInfo{
		{
			"Foo",
			0755,
			1234567890,
			[]BlockInfo{
				{12345678, []byte("hash hash hash")},
				{23456781, []byte("ash hash hashh")},
				{34567812, []byte("sh hash hashha")},
			},
		}, {
			"Quux/Quux",
			0644,
			2345678901,
			[]BlockInfo{
				{45678123, []byte("4321 hash hash hash")},
				{56781234, []byte("3214 ash hash hashh")},
				{67812345, []byte("2143 sh hash hashha")},
			},
		},
	}

	var buf = new(bytes.Buffer)
	var wr = marshalWriter{buf, 0, nil}
	wr.writeIndex(idx)

	var rd = marshalReader{buf, 0, nil}
	var idx2 = rd.readIndex()

	if !reflect.DeepEqual(idx, idx2) {
		t.Errorf("Index marshal error:\n%#v\n%#v\n", idx, idx2)
	}
}

func TestRequest(t *testing.T) {
	f := func(name string, offset uint64, size uint32, hash []byte) bool {
		var buf = new(bytes.Buffer)
		var req = request{name, offset, size, hash}
		var wr = marshalWriter{buf, 0, nil}
		wr.writeRequest(req)
		var rd = marshalReader{buf, 0, nil}
		var req2 = rd.readRequest()
		return req.name == req2.name &&
			req.offset == req2.offset &&
			req.size == req2.size &&
			bytes.Compare(req.hash, req2.hash) == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestResponse(t *testing.T) {
	f := func(data []byte) bool {
		var buf = new(bytes.Buffer)
		var wr = marshalWriter{buf, 0, nil}
		wr.writeResponse(data)
		var rd = marshalReader{buf, 0, nil}
		var read = rd.readResponse()
		return bytes.Compare(read, data) == 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func BenchmarkWriteIndex(b *testing.B) {
	idx := []FileInfo{
		{
			"Foo",
			0777,
			1234567890,
			[]BlockInfo{
				{12345678, []byte("hash hash hash")},
				{23456781, []byte("ash hash hashh")},
				{34567812, []byte("sh hash hashha")},
			},
		}, {
			"Quux/Quux",
			0644,
			2345678901,
			[]BlockInfo{
				{45678123, []byte("4321 hash hash hash")},
				{56781234, []byte("3214 ash hash hashh")},
				{67812345, []byte("2143 sh hash hashha")},
			},
		},
	}

	var wr = marshalWriter{ioutil.Discard, 0, nil}

	for i := 0; i < b.N; i++ {
		wr.writeIndex(idx)
	}
}

func BenchmarkWriteRequest(b *testing.B) {
	var req = request{"blah blah", 1231323, 13123123, []byte("hash hash hash")}
	var wr = marshalWriter{ioutil.Discard, 0, nil}

	for i := 0; i < b.N; i++ {
		wr.writeRequest(req)
	}
}
