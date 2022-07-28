// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package versioner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"

	"github.com/kballard/go-shellquote"
)

func init() {
	// Register the constructor for this type of versioner with the name "external"
	factories["external"] = newExternal
}

type external struct {
	command    string
	filesystem fs.Filesystem
}

func newExternal(cfg config.FolderConfiguration) Versioner {
	command := cfg.Versioning.Params["command"]

	if runtime.GOOS == "windows" {
		command = strings.ReplaceAll(command, `\`, `\\`)
	}

	s := external{
		command:    command,
		filesystem: cfg.Filesystem(nil),
	}

	l.Debugf("instantiated %#v", s)
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v external) Archive(filePath string) error {
	info, err := v.filesystem.Lstat(filePath)
	if fs.IsNotExist(err) {
		l.Debugln("not archiving nonexistent file", filePath)
		return nil
	} else if err != nil {
		return err
	}
	if info.IsSymlink() {
		panic("bug: attempting to version a symlink")
	}

	l.Debugln("archiving", filePath)

	if v.command == "" {
		return errors.New("command is empty, please enter a valid command")
	}

	words, err := shellquote.Split(v.command)
	if err != nil {
		return fmt.Errorf("command is invalid: %w", err)
	}

	context := map[string]string{
		"%FOLDER_FILESYSTEM%": v.filesystem.Type().String(),
		"%FOLDER_PATH%":       v.filesystem.URI(),
		"%FILE_PATH%":         filePath,
	}

	for i, word := range words {
		for key, val := range context {
			word = strings.ReplaceAll(word, key, val)
		}

		words[i] = word
	}

	cmd := exec.Command(words[0], words[1:]...)
	env := os.Environ()
	// filter STGUIAUTH and STGUIAPIKEY from environment variables
	filteredEnv := []string{}
	for _, x := range env {
		if !strings.HasPrefix(x, "STGUIAUTH=") && !strings.HasPrefix(x, "STGUIAPIKEY=") {
			filteredEnv = append(filteredEnv, x)
		}
	}
	cmd.Env = filteredEnv
	combinedOutput, err := cmd.CombinedOutput()
	l.Debugln("external command output:", string(combinedOutput))
	if err != nil {
		if eerr, ok := err.(*exec.ExitError); ok && len(eerr.Stderr) > 0 {
			return fmt.Errorf("%v: %v", err, string(eerr.Stderr))
		}
		return err
	}

	// return error if the file was not removed
	if _, err = v.filesystem.Lstat(filePath); fs.IsNotExist(err) {
		return nil
	}
	return errors.New("file was not removed by external script")
}

func (v external) GetVersions() (map[string][]FileVersion, error) {
	return nil, ErrRestorationNotSupported
}

func (v external) Restore(_ string, _ time.Time) error {
	return ErrRestorationNotSupported
}

func (v external) Clean(_ context.Context) error {
	return nil
}
