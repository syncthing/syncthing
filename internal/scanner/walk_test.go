// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

package scanner

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	rdebug "runtime/debug"
	"sort"
	"testing"

	"github.com/syncthing/syncthing/internal/ignore"
	"github.com/syncthing/syncthing/internal/protocol"
)

type testfile struct {
	name string
	size int
	hash string
}

type testfileList []testfile

var testdata = testfileList{
	{"afile", 4, "b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"},
	{"dir1", 128, ""},
	{filepath.Join("dir1", "dfile"), 5, "49ae93732fcf8d63fe1cce759664982dbd5b23161f007dba8561862adc96d063"},
	{"dir2", 128, ""},
	{filepath.Join("dir2", "cfile"), 4, "bf07a7fbb825fc0aae7bf4a1177b2b31fcf8a3feeaf7092761e18c859ee52a9c"},
	{"excludes", 37, "df90b52f0c55dba7a7a940affe482571563b1ac57bd5be4d8a0291e7de928e06"},
	{"further-excludes", 5, "7eb0a548094fa6295f7fd9200d69973e5f5ec5c04f2a86d998080ac43ecf89f1"},
}

var correctIgnores = map[string][]string{
	".": {".*", "quux"},
}

func init() {
	// This test runs the risk of entering infinite recursion if it fails.
	// Limit the stack size to 10 megs to creash early in that case instead of
	// potentially taking down the box...
	rdebug.SetMaxStack(10 * 1 << 20)
}

func TestWalkSub(t *testing.T) {
	ignores, err := ignore.Load("testdata/.stignore", false)
	if err != nil {
		t.Fatal(err)
	}

	w := Walker{
		Dir:       "testdata",
		Sub:       "dir2",
		BlockSize: 128 * 1024,
		Matcher:   ignores,
	}
	fchan, err := w.Walk()
	var files []protocol.FileInfo
	for f := range fchan {
		files = append(files, f)
	}
	if err != nil {
		t.Fatal(err)
	}

	// The directory contains two files, where one is ignored from a higher
	// level. We should see only the directory and one of the files.

	if len(files) != 2 {
		t.Fatalf("Incorrect length %d != 2", len(files))
	}
	if files[0].Name != "dir2" {
		t.Errorf("Incorrect file %v != dir2", files[0])
	}
	if files[1].Name != filepath.Join("dir2", "cfile") {
		t.Errorf("Incorrect file %v != dir2/cfile", files[1])
	}
}

func TestWalk(t *testing.T) {
	ignores, err := ignore.Load("testdata/.stignore", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(ignores)

	w := Walker{
		Dir:       "testdata",
		BlockSize: 128 * 1024,
		Matcher:   ignores,
	}

	fchan, err := w.Walk()
	if err != nil {
		t.Fatal(err)
	}

	var tmp []protocol.FileInfo
	for f := range fchan {
		tmp = append(tmp, f)
	}
	sort.Sort(fileList(tmp))
	files := fileList(tmp).testfiles()

	if !reflect.DeepEqual(files, testdata) {
		t.Errorf("Walk returned unexpected data\nExpected: %v\nActual: %v", testdata, files)
	}
}

func TestWalkError(t *testing.T) {
	w := Walker{
		Dir:       "testdata-missing",
		BlockSize: 128 * 1024,
	}
	_, err := w.Walk()

	if err == nil {
		t.Error("no error from missing directory")
	}

	w = Walker{
		Dir:       "testdata/bar",
		BlockSize: 128 * 1024,
	}
	_, err = w.Walk()

	if err == nil {
		t.Error("no error from non-directory")
	}
}

func TestVerify(t *testing.T) {
	blocksize := 16
	// data should be an even multiple of blocksize long
	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut e")
	buf := bytes.NewBuffer(data)

	blocks, err := Blocks(buf, blocksize, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exp := len(data) / blocksize; len(blocks) != exp {
		t.Fatalf("Incorrect number of blocks %d != %d", len(blocks), exp)
	}

	buf = bytes.NewBuffer(data)
	err = Verify(buf, blocksize, blocks)
	t.Log(err)
	if err != nil {
		t.Fatal("Unexpected verify failure", err)
	}

	buf = bytes.NewBuffer(append(data, '\n'))
	err = Verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}

	buf = bytes.NewBuffer(data[:len(data)-1])
	err = Verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}

	data[42] = 42
	buf = bytes.NewBuffer(data)
	err = Verify(buf, blocksize, blocks)
	t.Log(err)
	if err == nil {
		t.Fatal("Unexpected verify success")
	}
}

type fileList []protocol.FileInfo

func (f fileList) Len() int {
	return len(f)
}

func (f fileList) Less(a, b int) bool {
	return f[a].Name < f[b].Name
}

func (f fileList) Swap(a, b int) {
	f[a], f[b] = f[b], f[a]
}

func (l fileList) testfiles() testfileList {
	testfiles := make(testfileList, len(l))
	for i, f := range l {
		if len(f.Blocks) > 1 {
			panic("simple test case stuff only supports a single block per file")
		}
		testfiles[i] = testfile{name: f.Name, size: int(f.Size())}
		if len(f.Blocks) == 1 {
			testfiles[i].hash = fmt.Sprintf("%x", f.Blocks[0].Hash)
		}
	}
	return testfiles
}

func (l testfileList) String() string {
	var b bytes.Buffer
	b.WriteString("{\n")
	for _, f := range l {
		fmt.Fprintf(&b, "  %s (%d bytes): %s\n", f.name, f.size, f.hash)
	}
	b.WriteString("}")
	return b.String()
}
