// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// cmdutil implements utilities for running external commands
package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
)

func FormattedCommand(command string, keywords map[string]string) (*exec.Cmd, error) {
	if command == "" {
		return nil, errors.New("command is empty, please enter a valid command")
	}

	words, err := shellquote.Split(command)
	if err != nil {
		return nil, fmt.Errorf("command is invalid: %w", err)
	}

	for i, word := range words {
		for key, val := range keywords {
			word = strings.ReplaceAll(word, key, val)
		}

		words[i] = word
	}

	return commandWithFilteredEnv(words[0], words[1:]...), nil
}

func commandWithFilteredEnv(progname string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(),  progname, args...)
	env := os.Environ()
	// filter STGUIAUTH and STGUIAPIKEY from environment variables
	var filteredEnv []string

	for _, x := range env {
		if !strings.HasPrefix(x, "STGUIAUTH=") && !strings.HasPrefix(x, "STGUIAPIKEY=") {
			filteredEnv = append(filteredEnv, x)
		}
	}
	cmd.Env = filteredEnv

	return cmd
}
