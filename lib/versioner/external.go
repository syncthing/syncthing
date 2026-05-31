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
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/fs"

	"github.com/kballard/go-shellquote"
)

func init() {
	// Register the constructor for this type of versioner with the name "external"
	factories["external"] = newExternal
}

const unixSpecialChars = "`" + `"'<>;!#$&*? `

type external struct {
	command    string
	filesystem fs.Filesystem
}

func newExternal(cfg config.FolderConfiguration) Versioner {
	command := cfg.Versioning.Params["command"]

	if build.IsWindows {
		command = strings.ReplaceAll(command, `\`, `\\`)
	}

	s := external{
		command:    command,
		filesystem: cfg.Filesystem(),
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

	cmd, err := v.prepareCommand(filePath)
	if err != nil {
		return err
	}

	combinedOutput, err := cmd.CombinedOutput()
	l.Debugln("external command output:", string(combinedOutput))
	if err != nil {
		eerr := &exec.ExitError{}
		if errors.As(err, &eerr) && len(eerr.Stderr) > 0 {
			return fmt.Errorf("%w: %v", err, string(eerr.Stderr))
		}
		return err
	}

	// return error if the file was not removed
	if _, err = v.filesystem.Lstat(filePath); fs.IsNotExist(err) {
		return nil
	}
	return errors.New("file was not removed by external script")
}

func (external) GetVersions() (map[string][]FileVersion, error) {
	return nil, ErrRestorationNotSupported
}

func (external) Restore(_ string, _ time.Time) error {
	return ErrRestorationNotSupported
}

func (external) Clean(_ context.Context) error {
	return nil
}

// prepareCommand returns the command with environment for the given file
// path.
func (v external) prepareCommand(filePath string) (*exec.Cmd, error) {
	words, err := shellquote.Split(v.command)
	if err != nil {
		return nil, fmt.Errorf("command is invalid: %w", err)
	}

	context := map[string]string{
		"%FOLDER_FILESYSTEM%": string(v.filesystem.Type()),
		"%FOLDER_PATH%":       v.filesystem.URI(),
		"%FILE_PATH%":         filePath,
	}

	for i, word := range words {
		unsafe := strings.ContainsAny(word, unixSpecialChars)
		for key, val := range context {
			// If the parameter contains both an unsafe character and a
			// template placeholder, we consider it unsafe and reject it.
			// Note that the shell splitting will have already removed outer
			// quotes, so that a command like `foo "%FILE_PATH%"` is fine
			// here, despite the double quote being one of our unsafe
			// characters.
			if unsafe && strings.Contains(word, key) {
				return nil, errors.New("unsafe external versioning command; see https://docs.syncthing.net/users/versioning.html#external-file-versioning")
			}
			word = strings.ReplaceAll(word, key, val)
		}

		words[i] = word
	}

	// filter STGUIAUTH and STGUIAPIKEY from environment variables, and add
	// our folder info.
	env := os.Environ()
	var filteredEnv []string
	for _, x := range env {
		if !strings.HasPrefix(x, "STGUIAUTH=") && !strings.HasPrefix(x, "STGUIAPIKEY=") {
			filteredEnv = append(filteredEnv, x)
		}
	}
	for k, v := range context {
		k = strings.Trim(k, "%")
		filteredEnv = append(filteredEnv, fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.Command(words[0], words[1:]...)
	cmd.Env = filteredEnv
	return cmd, nil
}
