package model

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/calmh/syncthing/protocol"
)

func TestNewModel(t *testing.T) {
	m := NewModel("foo", 1e6)

	if m == nil {
		t.Fatalf("NewModel returned nil")
	}

	if fs, _ := m.NeedFiles(); len(fs) > 0 {
		t.Errorf("New model should have no Need")
	}

	if len(m.local) > 0 {
		t.Errorf("New model should have no Have")
	}
}

var testDataExpected = map[string]File{
	"foo": File{
		Name:     "foo",
		Flags:    0,
		Modified: 0,
		Blocks:   []Block{{Offset: 0x0, Size: 0x7, Hash: []uint8{0xae, 0xc0, 0x70, 0x64, 0x5f, 0xe5, 0x3e, 0xe3, 0xb3, 0x76, 0x30, 0x59, 0x37, 0x61, 0x34, 0xf0, 0x58, 0xcc, 0x33, 0x72, 0x47, 0xc9, 0x78, 0xad, 0xd1, 0x78, 0xb6, 0xcc, 0xdf, 0xb0, 0x1, 0x9f}}},
	},
	"empty": File{
		Name:     "empty",
		Flags:    0,
		Modified: 0,
		Blocks:   []Block{{Offset: 0x0, Size: 0x0, Hash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}}},
	},
	"bar": File{
		Name:     "bar",
		Flags:    0,
		Modified: 0,
		Blocks:   []Block{{Offset: 0x0, Size: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}},
	},
}

func init() {
	// Fix expected test data to match reality
	for n, f := range testDataExpected {
		fi, _ := os.Stat("testdata/" + n)
		f.Flags = uint32(fi.Mode())
		f.Modified = fi.ModTime().Unix()
		testDataExpected[n] = f
	}
}

func TestUpdateLocal(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	if fs, _ := m.NeedFiles(); len(fs) > 0 {
		t.Fatalf("Model with only local data should have no need")
	}

	if l1, l2 := len(m.local), len(testDataExpected); l1 != l2 {
		t.Fatalf("Model len(local) incorrect, %d != %d", l1, l2)
	}
	if l1, l2 := len(m.global), len(testDataExpected); l1 != l2 {
		t.Fatalf("Model len(global) incorrect, %d != %d", l1, l2)
	}
	for name, file := range testDataExpected {
		if f, ok := m.local[name]; ok {
			if !reflect.DeepEqual(f, file) {
				t.Errorf("Incorrect local\n%v !=\n%v\nfor file %q", f, file, name)
			}
		} else {
			t.Errorf("Missing file %q in local table", name)
		}
		if f, ok := m.global[name]; ok {
			if !reflect.DeepEqual(f, file) {
				t.Errorf("Incorrect global\n%v !=\n%v\nfor file %q", f, file, name)
			}
		} else {
			t.Errorf("Missing file %q in global table", name)
		}
	}

	for _, f := range fs {
		if hf, ok := m.local[f.Name]; !ok || hf.Modified != f.Modified {
			t.Fatalf("Incorrect local for %q", f.Name)
		}
		if cf, ok := m.global[f.Name]; !ok || cf.Modified != f.Modified {
			t.Fatalf("Incorrect global for %q", f.Name)
		}
	}
}

func TestRemoteUpdateExisting(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	newFile := protocol.FileInfo{
		Name:     "foo",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if fs, _ := m.NeedFiles(); len(fs) != 1 {
		t.Errorf("Model missing Need for one file (%d != 1)", len(fs))
	}
}

func TestRemoteAddNew(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	newFile := protocol.FileInfo{
		Name:     "a new file",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if fs, _ := m.NeedFiles(); len(fs) != 1 {
		t.Errorf("Model len(m.need) incorrect (%d != 1)", len(fs))
	}
}

func TestRemoteUpdateOld(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	oldTimeStamp := int64(1234)
	newFile := protocol.FileInfo{
		Name:     "foo",
		Modified: oldTimeStamp,
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if fs, _ := m.NeedFiles(); len(fs) != 0 {
		t.Errorf("Model len(need) incorrect (%d != 0)", len(fs))
	}
}

func TestRemoteIndexUpdate(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	foo := protocol.FileInfo{
		Name:     "foo",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}

	bar := protocol.FileInfo{
		Name:     "bar",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}

	m.Index("42", []protocol.FileInfo{foo})

	if fs, _ := m.NeedFiles(); fs[0].Name != "foo" {
		t.Error("Model doesn't need 'foo'")
	}

	m.IndexUpdate("42", []protocol.FileInfo{bar})

	if fs, _ := m.NeedFiles(); fs[0].Name != "foo" {
		t.Error("Model doesn't need 'foo'")
	}
	if fs, _ := m.NeedFiles(); fs[1].Name != "bar" {
		t.Error("Model doesn't need 'bar'")
	}
}

func TestDelete(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs); l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}

	ot := time.Now().Unix()
	newFile := File{
		Name:     "a new file",
		Modified: ot,
		Blocks:   []Block{{0, 100, []byte("some hash bytes")}},
	}
	m.updateLocal(newFile)

	if l1, l2 := len(m.local), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}

	// The deleted file is kept in the local and global tables and marked as deleted.

	m.ReplaceLocal(fs)

	if l1, l2 := len(m.local), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}

	if m.local["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in local table")
	}
	if len(m.local["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in local")
	}
	if ft := m.local["a new file"].Modified; ft != ot {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot+1)
	}
	if fv := m.local["a new file"].Version; fv != 1 {
		t.Errorf("Unexpected version %d != 1 for deleted file in local", fv)
	}

	if m.global["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in global table")
	}
	if len(m.global["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in global")
	}
	if ft := m.global["a new file"].Modified; ft != ot {
		t.Errorf("Unexpected time %d != %d for deleted file in global", ft, ot+1)
	}
	if fv := m.local["a new file"].Version; fv != 1 {
		t.Errorf("Unexpected version %d != 1 for deleted file in global", fv)
	}

	// Another update should change nothing

	m.ReplaceLocal(fs)

	if l1, l2 := len(m.local), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}

	if m.local["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in local table")
	}
	if len(m.local["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in local")
	}
	if ft := m.local["a new file"].Modified; ft != ot {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot)
	}
	if fv := m.local["a new file"].Version; fv != 1 {
		t.Errorf("Unexpected version %d != 1 for deleted file in local", fv)
	}

	if m.global["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in global table")
	}
	if len(m.global["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in global")
	}
	if ft := m.global["a new file"].Modified; ft != ot {
		t.Errorf("Unexpected time %d != %d for deleted file in global", ft, ot)
	}
	if fv := m.local["a new file"].Version; fv != 1 {
		t.Errorf("Unexpected version %d != 1 for deleted file in global", fv)
	}
}

func TestForgetNode(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs); l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}
	if fs, _ := m.NeedFiles(); len(fs) != 0 {
		t.Errorf("Model len(need) incorrect (%d != 0)", len(fs))
	}

	newFile := protocol.FileInfo{
		Name:     "new file",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	newFile = protocol.FileInfo{
		Name:     "new file 2",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("43", []protocol.FileInfo{newFile})

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+2; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}
	if fs, _ := m.NeedFiles(); len(fs) != 2 {
		t.Errorf("Model len(need) incorrect (%d != 2)", len(fs))
	}

	m.Close("42", nil)

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}

	if fs, _ := m.NeedFiles(); len(fs) != 1 {
		t.Errorf("Model len(need) incorrect (%d != 1)", len(fs))
	}
}

func TestRequest(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	bs, err := m.Request("some node", "foo", 0, 6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(bs, []byte("foobar")) != 0 {
		t.Errorf("Incorrect data from request: %q", string(bs))
	}

	bs, err = m.Request("some node", "../walk.go", 0, 6, nil)
	if err == nil {
		t.Error("Unexpected nil error on insecure file read")
	}
	if bs != nil {
		t.Errorf("Unexpected non nil data on insecure file read: %q", string(bs))
	}
}

func TestIgnoreWithUnknownFlags(t *testing.T) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	valid := protocol.FileInfo{
		Name:     "valid",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
		Flags:    protocol.FlagDeleted | 0755,
	}

	invalid := protocol.FileInfo{
		Name:     "invalid",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
		Flags:    1<<27 | protocol.FlagDeleted | 0755,
	}

	m.Index("42", []protocol.FileInfo{valid, invalid})

	if _, ok := m.global[valid.Name]; !ok {
		t.Error("Model should include", valid)
	}
	if _, ok := m.global[invalid.Name]; ok {
		t.Error("Model not should include", invalid)
	}
}

func genFiles(n int) []protocol.FileInfo {
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
		}
	}

	return files
}

func BenchmarkIndex10000(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)
	files := genFiles(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index("42", files)
	}
}

func BenchmarkIndex00100(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)
	files := genFiles(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Index("42", files)
	}
}

func BenchmarkIndexUpdate10000f10000(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)
	files := genFiles(10000)
	m.Index("42", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate("42", files)
	}
}

func BenchmarkIndexUpdate10000f00100(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)
	files := genFiles(10000)
	m.Index("42", files)

	ufiles := genFiles(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate("42", ufiles)
	}
}

func BenchmarkIndexUpdate10000f00001(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)
	files := genFiles(10000)
	m.Index("42", files)

	ufiles := genFiles(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.IndexUpdate("42", ufiles)
	}
}

type FakeConnection struct {
	id          string
	requestData []byte
}

func (FakeConnection) Close() error {
	return nil
}

func (f FakeConnection) ID() string {
	return string(f.id)
}

func (f FakeConnection) Option(string) string {
	return ""
}

func (FakeConnection) Index([]protocol.FileInfo) {}

func (f FakeConnection) Request(name string, offset int64, size uint32, hash []byte) ([]byte, error) {
	return f.requestData, nil
}

func (FakeConnection) Ping() bool {
	return true
}

func (FakeConnection) Statistics() protocol.Statistics {
	return protocol.Statistics{}
}

func BenchmarkRequest(b *testing.B) {
	m := NewModel("testdata", 1e6)
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	const n = 1000
	files := make([]protocol.FileInfo, n)
	t := time.Now().Unix()
	for i := 0; i < n; i++ {
		files[i] = protocol.FileInfo{
			Name:     fmt.Sprintf("file%d", i),
			Modified: t,
			Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
		}
	}

	fc := FakeConnection{
		id:          "42",
		requestData: []byte("some data to return"),
	}
	m.AddConnection(fc, fc)
	m.Index("42", files)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := m.requestGlobal("42", files[i%n].Name, 0, 32, nil)
		if err != nil {
			b.Error(err)
		}
		if data == nil {
			b.Error("nil data")
		}
	}
}
