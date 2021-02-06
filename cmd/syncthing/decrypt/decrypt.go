// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package decrypt implements the `syncthing decrypt` subcommand.
package decrypt

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/scanner"
)

type CLI struct {
	Path       string `arg:"" required:"1" help:"Path to encrypted folder"`
	To         string `xor:"mode" placeholder:"PATH" help:"Destination directory, when decrypting"`
	VerifyOnly bool   `xor:"mode" help:"Don't store decrypted files, only verify the hashes"`
	Password   string `help:"Folder password for decryption / verification" env:"FOLDER_PASSWORD"`
	FolderID   string `help:"Folder ID of the encrypted folder, if it cannot be determined automatically"`
	Continue   bool   `help:"Continue processing next file in case of error, instead of aborting"`
	Verbose    bool   `help:"Show verbose progress information"`
	TokenPath  string `placeholder:"PATH" help:"Path to the token file within the folder (used to determine folder ID)"`

	folderKey *[32]byte
}

type storedEncryptionToken struct {
	FolderID string
	Token    []byte
}

func (c *CLI) Run() error {
	log.SetFlags(0)

	if c.To == "" && !c.VerifyOnly {
		return fmt.Errorf("must set --to or --verify")
	}

	if c.TokenPath == "" {
		// This is a bit long to show as default in --help
		c.TokenPath = filepath.Join(config.DefaultMarkerName, config.EncryptionTokenName)
	}

	if c.FolderID == "" {
		// We should try to figure out the folder ID
		folderID, err := c.getFolderID()
		if err != nil {
			log.Println("No --folder-id given and couldn't read folder token")
			return fmt.Errorf("getting folder ID: %w", err)
		}

		c.FolderID = folderID
		if c.Verbose {
			log.Println("Found folder ID:", c.FolderID)
		}
	}

	c.folderKey = protocol.KeyFromPassword(c.FolderID, c.Password)

	return c.walk()
}

// walk finds and processes every file in the encrypted folder
func (c *CLI) walk() error {
	srcFs := fs.NewFilesystem(fs.FilesystemTypeBasic, c.Path)
	var dstFs fs.Filesystem
	if c.To != "" {
		dstFs = fs.NewFilesystem(fs.FilesystemTypeBasic, c.To)
	}

	return srcFs.Walk("/", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsRegular() {
			return nil
		}
		if fs.IsInternal(path) {
			return nil
		}

		return c.withContinue(c.process(srcFs, dstFs, path))
	})
}

// If --continue was set we just mention the error and return nil to
// continue processing.
func (c *CLI) withContinue(err error) error {
	if err == nil {
		return nil
	}
	if c.Continue {
		log.Println("Warning:", err)
		return nil
	}
	return err
}

// getFolderID returns the folder ID found in the encrypted token, or an
// error.
func (c *CLI) getFolderID() (string, error) {
	tokenPath := filepath.Join(c.Path, c.TokenPath)
	bs, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("reading folder token: %w", err)
	}

	var tok storedEncryptionToken
	if err := json.Unmarshal(bs, &tok); err != nil {
		return "", fmt.Errorf("parsing folder token: %w", err)
	}

	return tok.FolderID, nil
}

// process handles the file named path in srcFs, decrypting it into dstFs
// unless dstFs is nil.
func (c *CLI) process(srcFs fs.Filesystem, dstFs fs.Filesystem, path string) error {
	if c.Verbose {
		log.Printf("Processing %q", path)
	}

	encFd, err := srcFs.Open(path)
	if err != nil {
		return err
	}
	defer encFd.Close()

	encFi, err := c.loadEncryptedFileInfo(encFd)
	if err != nil {
		return fmt.Errorf("%s: loading metadata trailer: %w", path, err)
	}

	plainFi, err := protocol.DecryptFileInfo(*encFi, c.folderKey)
	if err != nil {
		return fmt.Errorf("%s: decrypting metadata: %w", path, err)
	}

	if c.Verbose {
		log.Printf("Plaintext filename is %q", plainFi.Name)
	}

	var plainFd fs.File
	if dstFs != nil {
		if err := dstFs.MkdirAll(filepath.Dir(plainFi.Name), 0700); err != nil {
			return fmt.Errorf("%s: %w", plainFi.Name, err)
		}

		plainFd, err = dstFs.Create(plainFi.Name)
		if err != nil {
			return fmt.Errorf("%s: %w", plainFi.Name, err)
		}
		defer plainFd.Close() // also closed explicitly in the return
	}

	if err := c.decryptFile(encFi, &plainFi, encFd, plainFd); err != nil {
		// Decrypting the file failed, leaving it in an inconsistent state.
		// Delete it. Even --continue currently doesn't mean "leave broken
		// stuff in place", it just means "try the next file instead of
		// aborting".
		if plainFd != nil {
			_ = dstFs.Remove(plainFd.Name())
		}
		return fmt.Errorf("%s: %w", plainFi.Name, err)
	} else if c.Verbose {
		log.Printf("Data verified for %q", plainFi.Name)
	}

	if plainFd != nil {
		return plainFd.Close()
	}
	return nil
}

// decryptFile reads, decrypts and verifies all the blocks in src, writing
// it to dst if dst is non-nil. (If dst is nil it just becomes a
// read-and-verify operation.)
func (c *CLI) decryptFile(encFi *protocol.FileInfo, plainFi *protocol.FileInfo, src io.ReaderAt, dst io.WriterAt) error {
	// The encrypted and plaintext files must consist of an equal number of blocks
	if len(encFi.Blocks) != len(plainFi.Blocks) {
		return fmt.Errorf("block count mismatch: encrypted %d != plaintext %d", len(encFi.Blocks), len(plainFi.Blocks))
	}

	fileKey := protocol.FileKey(plainFi.Name, c.folderKey)
	for i, encBlock := range encFi.Blocks {
		// Read the encrypted block
		buf := make([]byte, encBlock.Size)
		if _, err := src.ReadAt(buf, encBlock.Offset); err != nil {
			return fmt.Errorf("encrypted block %d (%d bytes): %w", i, encBlock.Size, err)
		}

		// Decrypt it
		dec, err := protocol.DecryptBytes(buf, fileKey)
		if err != nil {
			return fmt.Errorf("encrypted block %d (%d bytes): %w", i, encBlock.Size, err)
		}

		// Verify the block size against the expected plaintext
		plainBlock := plainFi.Blocks[i]
		if i == len(plainFi.Blocks)-1 && len(dec) > plainBlock.Size {
			// The last block might be padded, which is fine (we skip the padding)
			dec = dec[:plainBlock.Size]
		} else if len(dec) != plainBlock.Size {
			return fmt.Errorf("plaintext block %d size mismatch, actual %d != expected %d", i, len(dec), plainBlock.Size)
		}

		// Verify the hash against the plaintext block info
		if !scanner.Validate(dec, plainBlock.Hash, 0) {
			// The block decrypted correctly but fails the hash check. This
			// is odd and unexpected, but it it's still a valid block from
			// the source. The file might have changed while we pulled it?
			err := fmt.Errorf("plaintext block %d (%d bytes) failed validation after decryption", i, plainBlock.Size)
			if c.Continue {
				log.Printf("Warning: %s: %v", plainFi.Name, err)
			} else {
				return err
			}
		}

		// Write it to the destination, unless we're just verifying.
		if dst != nil {
			if _, err := dst.WriteAt(dec, plainBlock.Offset); err != nil {
				return err
			}
		}
	}

	return nil
}

// loadEncryptedFileInfo loads the encrypted FileInfo trailer from a file on
// disk.
func (c *CLI) loadEncryptedFileInfo(fd fs.File) (*protocol.FileInfo, error) {
	// Seek to the size of the trailer block
	if _, err := fd.Seek(-4, io.SeekEnd); err != nil {
		return nil, err
	}
	var bs [4]byte
	if _, err := io.ReadFull(fd, bs[:]); err != nil {
		return nil, err
	}
	size := int64(binary.BigEndian.Uint32(bs[:]))

	// Seek to the start of the trailer
	if _, err := fd.Seek(-(4 + size), io.SeekEnd); err != nil {
		return nil, err
	}
	trailer := make([]byte, size)
	if _, err := io.ReadFull(fd, trailer); err != nil {
		return nil, err
	}

	var encFi protocol.FileInfo
	if err := encFi.Unmarshal(trailer); err != nil {
		return nil, err
	}

	return &encFi, nil
}
