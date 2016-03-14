package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb/storage"
)

var (
	filename string
	child    bool
)

func init() {
	flag.StringVar(&filename, "filename", filepath.Join(os.TempDir(), "goleveldb_filelock_test"), "Filename used for testing")
	flag.BoolVar(&child, "child", false, "This is the child")
}

func runChild() error {
	var args []string
	args = append(args, os.Args[1:]...)
	args = append(args, "-child")
	cmd := exec.Command(os.Args[0], args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	r := bufio.NewReader(&out)
	for {
		line, _, e1 := r.ReadLine()
		if e1 != nil {
			break
		}
		fmt.Println("[Child]", string(line))
	}
	return err
}

func main() {
	flag.Parse()

	fmt.Printf("Using path: %s\n", filename)
	if child {
		fmt.Println("Child flag set.")
	}

	stor, err := storage.OpenFile(filename, false)
	if err != nil {
		fmt.Printf("Could not open storage: %s", err)
		os.Exit(10)
	}

	if !child {
		fmt.Println("Executing child -- first test (expecting error)")
		err := runChild()
		if err == nil {
			fmt.Println("Expecting error from child")
		} else if err.Error() != "exit status 10" {
			fmt.Println("Got unexpected error from child:", err)
		} else {
			fmt.Printf("Got error from child: %s (expected)\n", err)
		}
	}

	err = stor.Close()
	if err != nil {
		fmt.Printf("Error when closing storage: %s", err)
		os.Exit(11)
	}

	if !child {
		fmt.Println("Executing child -- second test")
		err := runChild()
		if err != nil {
			fmt.Println("Got unexpected error from child:", err)
		}
	}

	os.RemoveAll(filename)
}
