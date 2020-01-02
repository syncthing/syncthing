// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kingpin"
	"github.com/gogo/protobuf/proto"
	"github.com/syncthing/syncthing/lib/protocol"
)

func main() {
	folderID := kingpin.Flag("folder-id", "Folder ID").Required().String()
	folderPath := kingpin.Flag("folder-path", "Folder path").Required().String()
	password := kingpin.Flag("password", "Folder password").Required().String()
	destinationPath := kingpin.Flag("dest-path", "Destination path").Required().String()
	kingpin.Parse()

	folderKey := protocol.KeyFromPassword(*folderID, *password)

	err := filepath.Walk(*folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		encFi, err := fileInfoFor(path)
		if err != nil {
			log.Printf("Inspecting %s: %v", path, err)
			return nil
		}

		fi, err := protocol.DecryptFileInfo(encFi, folderKey)
		if err != nil {
			log.Printf("Decrypting metadata for %s: %v", path, err)
			return nil
		}

		fileKey := protocol.FileKey(fi.Name, folderKey)

		log.Println("Decrypting", fi.Name, "...")
		outPath := filepath.Join(*destinationPath, fi.Name)

		inFd, err := os.Open(path)
		if err != nil {
			log.Printf("Reading %s: %v", path, err)
			return nil
		}
		defer inFd.Close()

		verifyHashes := true
		if len(fi.Blocks) != len(encFi.Blocks) {
			log.Printf("Preparing %s: warning: block list mismatch (%d encrypted, %d plaintext), possible metadata out of sync?", outPath, len(encFi.Blocks), len(fi.Blocks))
			verifyHashes = false
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
			log.Printf("Preparing %s: %v", outPath, err)
			return nil
		}
		outFd, err := os.Create(outPath)
		if err != nil {
			log.Printf("Creating %s: %v", outPath, err)
			return nil
		}
		defer outFd.Close()

		var buffer []byte
		for i, eb := range encFi.Blocks {
			if cap(buffer) < int(eb.Size) {
				buffer = make([]byte, int(eb.Size))
			} else {
				buffer = buffer[:int(eb.Size)]
			}
			_, err := inFd.ReadAt(buffer, eb.Offset)
			if err != nil {
				log.Printf("Reading %s: %v", path, err)
				return nil
			}

			bs, err := protocol.DecryptBytes(buffer, fileKey)
			if err != nil {
				log.Printf("Decrypting %s: %v", path, err)
				return nil
			}

			if verifyHashes {
				hash := sha256.Sum256(bs)
				if !bytes.Equal(hash[:], fi.Blocks[i].Hash) {
					log.Printf("Decrypting %s: warning: decrypted data fails hash check", outPath)
				}
			}

			if _, err := outFd.Write(bs); err != nil {
				log.Printf("Writing %s: %v", outPath, err)
				return nil
			}
		}

		if err := outFd.Close(); err != nil {
			log.Printf("Writing %s: %v", outPath, err)
			return nil
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Walking %s: %v", *folderPath, err)
	}
}

func fileInfoFor(path string) (protocol.FileInfo, error) {
	fd, err := os.Open(path)
	if err != nil {
		return protocol.FileInfo{}, err
	}
	defer fd.Close()

	info, err := fd.Stat()
	if err != nil {
		return protocol.FileInfo{}, err
	}

	var sizeBs [4]byte
	if _, err := fd.ReadAt(sizeBs[:], info.Size()-4); err != nil {
		return protocol.FileInfo{}, err
	}

	size := int(binary.BigEndian.Uint32(sizeBs[:]))
	fiBs := make([]byte, size)
	if _, err := fd.ReadAt(fiBs, info.Size()-4-int64(size)); err != nil {
		return protocol.FileInfo{}, err
	}

	var encFi protocol.FileInfo
	if err := proto.Unmarshal(fiBs, &encFi); err != nil {
		return protocol.FileInfo{}, err
	}

	return encFi, nil
}
