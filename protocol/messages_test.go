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
			FlagInvalid & FlagDeleted & 0755,
			1234567890,
			142,
			[]BlockInfo{
				{12345678, []byte("hash hash hash")},
				{23456781, []byte("ash hash hashh")},
				{34567812, []byte("sh hash hashha")},
			},
		}, {
			"Quux/Quux",
			0644,
			2345678901,
			232323232,
			[]BlockInfo{
				{45678123, []byte("4321 hash hash hash")},
				{56781234, []byte("3214 ash hash hashh")},
				{67812345, []byte("2143 sh hash hashha")},
			},
		},
	}

	var buf = new(bytes.Buffer)
	var wr = newMarshalWriter(buf)
	wr.writeIndex("default", idx)

	var rd = newMarshalReader(buf)
	var repo, idx2 = rd.readIndex()

	if repo != "default" {
		t.Error("Incorrect repo", repo)
	}

	if !reflect.DeepEqual(idx, idx2) {
		t.Errorf("Index marshal error:\n%#v\n%#v\n", idx, idx2)
	}
}

func TestRequest(t *testing.T) {
	f := func(repo, name string, offset int64, size uint32, hash []byte) bool {
		var buf = new(bytes.Buffer)
		var req = request{repo, name, offset, size, hash}
		var wr = newMarshalWriter(buf)
		wr.writeRequest(req)
		var rd = newMarshalReader(buf)
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
		var wr = newMarshalWriter(buf)
		wr.writeResponse(data)
		var rd = newMarshalReader(buf)
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
			424242,
			[]BlockInfo{
				{12345678, []byte("hash hash hash")},
				{23456781, []byte("ash hash hashh")},
				{34567812, []byte("sh hash hashha")},
			},
		}, {
			"Quux/Quux",
			0644,
			2345678901,
			323232,
			[]BlockInfo{
				{45678123, []byte("4321 hash hash hash")},
				{56781234, []byte("3214 ash hash hashh")},
				{67812345, []byte("2143 sh hash hashha")},
			},
		},
	}

	var wr = newMarshalWriter(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		wr.writeIndex("default", idx)
	}
}

func BenchmarkWriteRequest(b *testing.B) {
	var req = request{"default", "blah blah", 1231323, 13123123, []byte("hash hash hash")}
	var wr = newMarshalWriter(ioutil.Discard)

	for i := 0; i < b.N; i++ {
		wr.writeRequest(req)
	}
}

func TestOptions(t *testing.T) {
	opts := map[string]string{
		"foo":     "bar",
		"someKey": "otherValue",
		"hello":   "",
		"":        "42",
	}

	var buf = new(bytes.Buffer)
	var wr = newMarshalWriter(buf)
	wr.writeOptions(opts)

	var rd = newMarshalReader(buf)
	var ropts = rd.readOptions()

	if !reflect.DeepEqual(opts, ropts) {
		t.Error("Incorrect options marshal/demarshal")
	}
}
