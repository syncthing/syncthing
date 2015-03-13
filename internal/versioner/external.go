// Copyright (C) 2014 The Syncthing Authors.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package versioner

import (
	"os"
	"os/exec"
	"path/filepath"
)

func init() {
	// Register the constructor for this type of versioner with the name "external"
	Factories["external"] = NewExternal
}

// The type holds our configuration
type External struct {
	command    string
	folderPath string
}

// The constructor function takes a map of parameters and creates the type.
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

// Move away the named file to a version archive. If this function returns
// nil, the named file does not exist any more (has been archived).
func (v External) Archive(filePath string) error {
	_, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			if debug {
				l.Debugln("not archiving nonexistent file", filePath)
			}
			return nil
		}
		return err
	}

	if debug {
		l.Debugln("archiving", filePath)
	}

	inFolderPath, err := filepath.Rel(v.folderPath, filePath)
	if err != nil {
		return err
	}
	if v.command != "" {
		cmd := exec.Command(v.command, v.folderPath, inFolderPath)
		cmd.Env = []string{""}
		err = cmd.Run()
		if err != nil {
			l.Warnln("Versioner:", err)
		}
	} else {
		l.Warnln("Versioner: command is empty, please enter a valid command")
	}

	// make sure that the file is removed even if the external versioner failed
	err = os.Remove(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		l.Warnf("Versioner: can't remove %q: %v", filePath, err)
	} else {
		l.Warnln("Versioner: file", filePath, "was not removed, check your command")
	}

	return nil
}
