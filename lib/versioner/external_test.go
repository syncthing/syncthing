package versioner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExternalNoCommand(t *testing.T) {
	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/folder path", 0755)
	ioutil.WriteFile("testdata/folder path/long filename.txt", []byte("hello\n"), 0644)

	e := External{
		command:    "nonexistant command",
		folderPath: "testdata/folder path",
	}
	if err := e.Archive("testdata/folder path/long filename.txt"); err == nil {
		t.Error("Command should have failed")
	}
}

func TestExternal(t *testing.T) {
	cmd := "./_external_test/external.sh"
	if runtime.GOOS == "windows" {
		cmd = `.\_external_text\external.bat`
	}

	os.RemoveAll("testdata")
	defer os.RemoveAll("testdata")
	os.MkdirAll("testdata/folder path", 0755)
	ioutil.WriteFile("testdata/folder path/long filename.txt", []byte("hello\n"), 0644)

	e := External{
		command:    cmd,
		folderPath: filepath.FromSlash("testdata/folder path"),
	}
	if err := e.Archive("_external_test/folder path/long filename.txt"); err != nil {
		t.Fatal(err)
	}
}
