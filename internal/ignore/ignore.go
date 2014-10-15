// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
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

package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/syncthing/syncthing/internal/fnmatch"
)

var caches = make(map[string]MatcherCache)

type Pattern struct {
	match   *regexp.Regexp
	include bool
}

type Matcher struct {
	patterns   []Pattern
	oldMatches map[string]bool

	newMatches map[string]bool
	mut        sync.Mutex
}

type MatcherCache struct {
	patterns []Pattern
	matches  *map[string]bool
}

func Load(file string, cache bool) (*Matcher, error) {
	seen := make(map[string]bool)
	matcher, err := loadIgnoreFile(file, seen)
	if !cache || err != nil {
		return matcher, err
	}

	// Get the current cache object for the given file
	cached, ok := caches[file]
	if !ok || !patternsEqual(cached.patterns, matcher.patterns) {
		// Nothing in cache or a cache mismatch, create a new cache which will
		// store matches for the given set of patterns.
		// Initialize oldMatches to indicate that we are interested in
		// caching.
		matcher.oldMatches = make(map[string]bool)
		matcher.newMatches = make(map[string]bool)
		caches[file] = MatcherCache{
			patterns: matcher.patterns,
			matches:  &matcher.newMatches,
		}
		return matcher, nil
	}

	// Patterns haven't changed, so we can reuse the old matches, create a new
	// matches map and update the pointer. (This prevents matches map from
	// growing indefinately, as we only cache whatever we've matched in the last
	// iteration, rather than through runtime history)
	matcher.oldMatches = *cached.matches
	matcher.newMatches = make(map[string]bool)
	cached.matches = &matcher.newMatches
	caches[file] = cached
	return matcher, nil
}

func Parse(r io.Reader, file string) (*Matcher, error) {
	seen := map[string]bool{
		file: true,
	}
	return parseIgnoreFile(r, file, seen)
}

func (m *Matcher) Match(file string) (result bool) {
	if len(m.patterns) == 0 {
		return false
	}

	// We have old matches map set, means we should do caching
	if m.oldMatches != nil {
		// Capture the result to the new matches regardless of who returns it
		defer func() {
			m.mut.Lock()
			m.newMatches[file] = result
			m.mut.Unlock()
		}()
		// Check perhaps we've seen this file before, and we already know
		// what the outcome is going to be.
		result, ok := m.oldMatches[file]
		if ok {
			return result
		}
	}

	for _, pattern := range m.patterns {
		if pattern.match.MatchString(file) {
			return pattern.include
		}
	}
	return false
}

func loadIgnoreFile(file string, seen map[string]bool) (*Matcher, error) {
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

func parseIgnoreFile(fd io.Reader, currentFile string, seen map[string]bool) (*Matcher, error) {
	var exps Matcher

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
			exps.patterns = append(exps.patterns, Pattern{exp, include})
		} else if strings.HasPrefix(line, "**/") {
			// Add the pattern as is, and without **/ so it matches in current dir
			exp, err := fnmatch.Convert(line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps.patterns = append(exps.patterns, Pattern{exp, include})

			exp, err = fnmatch.Convert(line[3:], fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps.patterns = append(exps.patterns, Pattern{exp, include})
		} else if strings.HasPrefix(line, "#include ") {
			includeFile := filepath.Join(filepath.Dir(currentFile), line[len("#include "):])
			includes, err := loadIgnoreFile(includeFile, seen)
			if err != nil {
				return err
			} else {
				exps.patterns = append(exps.patterns, includes.patterns...)
			}
		} else {
			// Path name or pattern, add it so it matches files both in
			// current directory and subdirs.
			exp, err := fnmatch.Convert(line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps.patterns = append(exps.patterns, Pattern{exp, include})

			exp, err = fnmatch.Convert("**/"+line, fnmatch.FNM_PATHNAME)
			if err != nil {
				return fmt.Errorf("Invalid pattern %q in ignore file", line)
			}
			exps.patterns = append(exps.patterns, Pattern{exp, include})
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

	return &exps, nil
}

func patternsEqual(a, b []Pattern) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].include != b[i].include || a[i].match.String() != b[i].match.String() {
			return false
		}
	}
	return true
}
