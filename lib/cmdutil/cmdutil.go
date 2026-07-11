// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package cmdutil implements utilities for running external commands
package cmdutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/syncthing/syncthing/lib/build"
)

const unixSpecialChars = "`" + `"'<>;!#$&*? `

func TemplatedCommand(command string, context map[string]string) (*exec.Cmd, error) {
	if command == "" {
		return nil, errors.New("command is empty, please enter a valid command")
	}

	if build.IsWindows {
		command = strings.ReplaceAll(command, `\`, `\\`)
	}

	words, err := shellquote.Split(command)
	if err != nil {
		return nil, fmt.Errorf("command is invalid: %w", err)
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
				return nil, errors.New("unsafe external command; see https://docs.syncthing.net/users/versioning.html#external-file-versioning")
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
