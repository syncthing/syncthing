// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

var blockSize = protocol.BlockSize

func main() {
	flag.IntVar(&blockSize, "b", blockSize, "Block size")
	check := flag.Bool("check", false, "Check hashes")
	split := flag.Int("split", 0, "Split hashes")
	flag.Parse()

	if *check {
		// Check file containing hashes and paths for potential collisions.

		checkHashes()
	} else if *split > 0 {
		if err := splitHashes(*split); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		// Walk a directory and write a file describing all the hashesh.

		path := flag.Arg(0)
		if path == "" {
			fmt.Println("Need one argument: path to hash")
			os.Exit(1)
		}

		if err := filepath.Walk(path, walkFunc); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

type block struct {
	Hash string
	Path string
	Idx  int
}

func checkHashes() {
	// Read hashes as a JSON stream on stdin.
	hashes := make(map[string]block)
	dec := json.NewDecoder(os.Stdin)

	var b block
	i := 0
	for dec.Decode(&b) == nil {
		if cur, ok := hashes[b.Hash]; ok {
			// If we already have a matching hash in the map, check the two
			// blocks.
			if err := compare(b.Path, b.Idx, cur.Path, cur.Idx); err != nil {
				fmt.Println("Hash:", b.Hash)
				fmt.Println("Error:", err)
			}
		}

		hashes[b.Hash] = b
		i++
	}
	fmt.Println("Checked", i, "hashes")
}

func splitHashes(n int) error {
	// Read hashes as a JSON stream on stdin.
	files := make(map[string]*json.Encoder)
	dec := json.NewDecoder(os.Stdin)

	var b block
	for dec.Decode(&b) == nil {
		key := b.Hash[:n]
		enc, ok := files[key]
		if !ok {
			var err error
			fd, err := os.Create(key)
			if err != nil {
				return err
			}
			defer fd.Close()
			enc = json.NewEncoder(fd)
			files[key] = enc
		}
		enc.Encode(b)
	}
	return nil
}

func compare(path1 string, idx1 int, path2 string, idx2 int) error {
	b1, err := readBlock(path1, idx1)
	if err != nil {
		return err
	}

	b2, err := readBlock(path2, idx2)
	if err != nil {
		return err
	}

	if len(b1) != len(b2) {
		return fmt.Errorf("Block length mismatch, %d != %d", len(b1), len(b2))
	}
	if !bytes.Equal(b1, b2) {
		return fmt.Errorf("Block contents mismatch")
	}
	return nil
}

func readBlock(path string, idx int) ([]byte, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	fi, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	// Size & offset dance to handle the last block in a file, which may be
	// smaller than blockSize. ReadAt() returns an error if it can't read the
	// full block.
	offs := int64(idx * blockSize)
	size := int64(blockSize)
	if fi.Size() < offs+int64(blockSize) {
		size = fi.Size() - offs
	}

	buf := make([]byte, size)
	n, err := fd.ReadAt(buf, offs)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

func walkFunc(path string, fi os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if fi.Mode().IsRegular() {
		fd, err := os.Open(path)
		if err != nil {
			return err
		}

		bs, err := scanner.Blocks(scanner.Murmur3, fd, blockSize, fi.Size(), nil)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		for i, b := range bs {
			enc.Encode(block{fmt.Sprintf("%x", b.Hash), path, i})
		}
	}

	return nil
}
