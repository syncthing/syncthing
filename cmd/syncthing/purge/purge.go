// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package decrypt implements the `syncthing decrypt` subcommand.
package purge

import (
	"fmt"
	"log"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore"
)

type CLI struct {
	Path       string `arg:"" required:"1" help:"Path to folder"`
	IgnoreFile string `help:"Relative path, with respect to folder, to ignore file, if other than \".stignore\""`
	Continue   bool   `help:"Continue processing next file in case of error, instead of aborting"`
	Verbose    bool   `help:"Show verbose progress information"`
	DryRun     bool   `help:"Don't actually delete anything"`
}

func (c *CLI) Run() error {
	log.SetFlags(0)

	if c.IgnoreFile == "" {
		c.IgnoreFile = ".stignore"
	}

	if c.Verbose {
		log.Printf("Purging %q", c.Path)
	}

	return c.walk()
}

// walk finds and processes every file in the folder
func (c *CLI) walk() error {
	srcFs := fs.NewFilesystem(fs.FilesystemTypeBasic, c.Path)
	matcher := ignore.New(srcFs)
	if c.Verbose {
		log.Printf("Using ignore file %q", c.IgnoreFile)
	}
	matcher.Load(c.IgnoreFile)

	if c.Verbose {
		log.Printf("ignore patterns: %v", matcher.Patterns())
	}

	return srcFs.Walk(".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsRegular() {
			return nil
		}
		if fs.IsInternal(path) {
			return nil
		}

		return c.withContinue(c.process(srcFs, matcher, path))
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

func (c *CLI) process(srcFs fs.Filesystem, matcher *ignore.Matcher, path string) error {

	if c.Verbose {
		log.Printf("Processing %q", path)
	}

	// Check if the file is ignored
	result := matcher.Match(path)
	if result.IsDeletable() {
		if c.Verbose {
			log.Printf("Deleting %q", path)
		}

		if c.DryRun {
			log.Printf("Dry run: would delete %q", path)
			return nil
		}

		if err := srcFs.Remove(path); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	return nil
}
