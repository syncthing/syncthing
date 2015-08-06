// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package versioner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/syncthing/syncthing/lib/osutil"
)

func init() {
	// Register the constructor for this type of versioner with the name "external"
	Factories["external"] = NewExternal
}

type External struct {
	command    string
	folderPath string
}

func NewExternal(folderID, folderPath string, params map[string]string) Versioner {
	command := params["command"]

	s := External{
		command:    command,
		folderPath: folderPath,
	}

	if debug {
		l.Debugf("instantiated %#v", s)
	}
	return s
}

// Archive moves the named file away to a version archive. If this function
// returns nil, the named file does not exist any more (has been archived).
func (v External) Archive(filePath string) error {
	_, err := osutil.Lstat(filePath)
	if os.IsNotExist(err) {
		if debug {
			l.Debugln("not archiving nonexistent file", filePath)
		}
		return nil
	} else if err != nil {
		return err
	}

	if debug {
		l.Debugln("archiving", filePath)
	}

	inFolderPath, err := filepath.Rel(v.folderPath, filePath)
	if err != nil {
		return err
	}

	if v.command == "" {
		return errors.New("Versioner: command is empty, please enter a valid command")
	}

	cmd := exec.Command(v.command, v.folderPath, inFolderPath)
	env := os.Environ()
	// filter STGUIAUTH and STGUIAPIKEY from environment variables
	filteredEnv := []string{}
	for _, x := range env {
		if !strings.HasPrefix(x, "STGUIAUTH=") && !strings.HasPrefix(x, "STGUIAPIKEY=") {
			filteredEnv = append(filteredEnv, x)
		}
	}
	cmd.Env = filteredEnv
	err = cmd.Run()
	if err != nil {
		return err
	}

	// return error if the file was not removed
	if _, err = osutil.Lstat(filePath); os.IsNotExist(err) {
		return nil
	}
	return errors.New("Versioner: file was not removed by external script")
}
