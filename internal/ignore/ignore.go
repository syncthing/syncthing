// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/syncthing/syncthing/internal/fnmatch"
)

type Pattern struct {
	match   *regexp.Regexp
	include bool
}

type Patterns []Pattern

func Load(file string) (Patterns, error) {
	seen := make(map[string]bool)
	return loadIgnoreFile(file, seen)
}

func Parse(r io.Reader, file string) (Patterns, error) {
	seen := map[string]bool{
		file: true,
	}
	return parseIgnoreFile(r, file, seen)
}

func (l Patterns) Match(file string) bool {
	for _, pattern := range l {
		if pattern.match.MatchString(file) {
			return pattern.include
		}
	}
	return false
}

func loadIgnoreFile(file string, seen map[string]bool) (Patterns, error) {
	if seen[file] {
		return nil, fmt.Errorf("Multiple include of ignore file %q", file)
	}
	seen[file] = true

	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	return parseIgnoreFile(fd, file, seen)
}

func parseIgnoreFile(fd io.Reader, currentFile string, seen map[string]bool) (Patterns, error) {
	var exps Patterns

	addPattern := func(line string) error {
		include := true
		if strings.HasPrefix(line, "!") {
			line = line[1:]
			include = false
		}

		if strings.HasPrefix(line, "/") {
			// Pattern is rooted in the current dir only
			exp, err := fnmatch.Convert(line[1:], fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps = append(exps, Pattern{exp, include})
		} else if strings.HasPrefix(line, "**/") {
			// Add the pattern as is, and without **/ so it matches in current dir
			exp, err := fnmatch.Convert(line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps = append(exps, Pattern{exp, include})

			exp, err = fnmatch.Convert(line[3:], fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps = append(exps, Pattern{exp, include})
		} else if strings.HasPrefix(line, "#include ") {
			includeFile := filepath.Join(filepath.Dir(currentFile), line[len("#include "):])
			includes, err := loadIgnoreFile(includeFile, seen)
			if err != nil {
				return err
			} else {
				exps = append(exps, includes...)
			}
		} else {
			// Path name or pattern, add it so it matches files both in
			// current directory and subdirs.
			exp, err := fnmatch.Convert(line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps = append(exps, Pattern{exp, include})

			exp, err = fnmatch.Convert("**/"+line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps = append(exps, Pattern{exp, include})
		}
		return nil
	}

	scanner := bufio.NewScanner(fd)
	var err error
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "//"):
			continue
		case strings.HasPrefix(line, "#"):
			err = addPattern(line)
		case strings.HasSuffix(line, "/**"):
			err = addPattern(line)
		case strings.HasSuffix(line, "/"):
			err = addPattern(line)
			if err == nil {
				err = addPattern(line + "**")
			}
		default:
			err = addPattern(line)
			if err == nil {
				err = addPattern(line + "/**")
			}
		}
		if err != nil {
			return nil, err
		}
	}

	return exps, nil
}
