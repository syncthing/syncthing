package model

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/calmh/syncthing/protocol"
)

func TestNewModel(t *testing.T) {
	m := NewModel("foo")

	if m == nil {
		t.Fatalf("NewModel returned nil")
	}

	if len(m.need) > 0 {
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
		Blocks:   []Block{{Offset: 0x0, Length: 0x7, Hash: []uint8{0xae, 0xc0, 0x70, 0x64, 0x5f, 0xe5, 0x3e, 0xe3, 0xb3, 0x76, 0x30, 0x59, 0x37, 0x61, 0x34, 0xf0, 0x58, 0xcc, 0x33, 0x72, 0x47, 0xc9, 0x78, 0xad, 0xd1, 0x78, 0xb6, 0xcc, 0xdf, 0xb0, 0x1, 0x9f}}},
	},
	"bar": File{
		Name:     "bar",
		Flags:    0,
		Modified: 0,
		Blocks:   []Block{{Offset: 0x0, Length: 0xa, Hash: []uint8{0x2f, 0x72, 0xcc, 0x11, 0xa6, 0xfc, 0xd0, 0x27, 0x1e, 0xce, 0xf8, 0xc6, 0x10, 0x56, 0xee, 0x1e, 0xb1, 0x24, 0x3b, 0xe3, 0x80, 0x5b, 0xf9, 0xa9, 0xdf, 0x98, 0xf9, 0x2f, 0x76, 0x36, 0xb0, 0x5c}}},
	},
	"baz/quux": File{
		Name:     "baz/quux",
		Flags:    0,
		Modified: 0,
		Blocks:   []Block{{Offset: 0x0, Length: 0x9, Hash: []uint8{0xc1, 0x54, 0xd9, 0x4e, 0x94, 0xba, 0x72, 0x98, 0xa6, 0xad, 0xb0, 0x52, 0x3a, 0xfe, 0x34, 0xd1, 0xb6, 0xa5, 0x81, 0xd6, 0xb8, 0x93, 0xa7, 0x63, 0xd4, 0x5d, 0xdc, 0x5e, 0x20, 0x9d, 0xcb, 0x83}}},
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
	m := NewModel("testdata")
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	if len(m.need) > 0 {
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
	m := NewModel("testdata")
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	newFile := protocol.FileInfo{
		Name:     "foo",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if l := len(m.need); l != 1 {
		t.Errorf("Model missing Need for one file (%d != 1)", l)
	}
}

func TestRemoteAddNew(t *testing.T) {
	m := NewModel("testdata")
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	newFile := protocol.FileInfo{
		Name:     "a new file",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if l1, l2 := len(m.need), 1; l1 != l2 {
		t.Errorf("Model len(m.need) incorrect (%d != %d)", l1, l2)
	}
}

func TestRemoteUpdateOld(t *testing.T) {
	m := NewModel("testdata")
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	oldTimeStamp := int64(1234)
	newFile := protocol.FileInfo{
		Name:     "foo",
		Modified: oldTimeStamp,
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if l1, l2 := len(m.need), 0; l1 != l2 {
		t.Errorf("Model len(need) incorrect (%d != %d)", l1, l2)
	}
}

func TestRemoteIndexUpdate(t *testing.T) {
	m := NewModel("testdata")
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

	if _, ok := m.need["foo"]; !ok {
		t.Error("Model doesn't need 'foo'")
	}

	m.IndexUpdate("42", []protocol.FileInfo{bar})

	if _, ok := m.need["foo"]; !ok {
		t.Error("Model doesn't need 'foo'")
	}
	if _, ok := m.need["bar"]; !ok {
		t.Error("Model doesn't need 'bar'")
	}
}

func TestDelete(t *testing.T) {
	m := NewModel("testdata")
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
	if ft := m.local["a new file"].Modified; ft != ot+1 {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot+1)
	}

	if m.global["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in global table")
	}
	if len(m.global["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in global")
	}
	if ft := m.local["a new file"].Modified; ft != ot+1 {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot+1)
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
	if ft := m.local["a new file"].Modified; ft != ot+1 {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot+1)
	}

	if m.global["a new file"].Flags&(1<<12) == 0 {
		t.Error("Unexpected deleted flag = 0 in global table")
	}
	if len(m.global["a new file"].Blocks) != 0 {
		t.Error("Unexpected non-zero blocks for deleted file in global")
	}
	if ft := m.local["a new file"].Modified; ft != ot+1 {
		t.Errorf("Unexpected time %d != %d for deleted file in local", ft, ot+1)
	}
}

func TestForgetNode(t *testing.T) {
	m := NewModel("testdata")
	fs, _ := m.Walk(false)
	m.ReplaceLocal(fs)

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs); l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.need), 0; l1 != l2 {
		t.Errorf("Model len(need) incorrect (%d != %d)", l1, l2)
	}

	newFile := protocol.FileInfo{
		Name:     "new file",
		Modified: time.Now().Unix(),
		Blocks:   []protocol.BlockInfo{{100, []byte("some hash bytes")}},
	}
	m.Index("42", []protocol.FileInfo{newFile})

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs)+1; l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.need), 1; l1 != l2 {
		t.Errorf("Model len(need) incorrect (%d != %d)", l1, l2)
	}

	m.Close("42", nil)

	if l1, l2 := len(m.local), len(fs); l1 != l2 {
		t.Errorf("Model len(local) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.global), len(fs); l1 != l2 {
		t.Errorf("Model len(global) incorrect (%d != %d)", l1, l2)
	}
	if l1, l2 := len(m.need), 0; l1 != l2 {
		t.Errorf("Model len(need) incorrect (%d != %d)", l1, l2)
	}
}
