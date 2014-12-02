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
	"time"

	"github.com/syncthing/syncthing/internal/fnmatch"
)

var caches = make(map[string]*MatcherCache)

type Pattern struct {
	match   *regexp.Regexp
	include bool
}

type Matcher struct {
	patterns []Pattern
	cache    *MatcherCache
	mut      sync.Mutex
}

type CacheEntry struct {
	matched bool
	at      time.Time
}

type MatcherCache struct {
	patterns []Pattern
	stop     chan struct{}

	matches map[string]CacheEntry
	mut     sync.RWMutex
}

// Periodically cleans up entries which haven't been touched for a while
func (c *MatcherCache) Serve(duration time.Duration) {
	ticker := time.NewTicker(duration)

	for {
		select {
		case <-ticker.C:
			c.mut.Lock()
			now := time.Now()
			for key, entry := range c.matches {
				if entry.at.Add(duration * 2).Before(now) {
					delete(c.matches, key)
				}
			}
			c.mut.Unlock()
		case <-c.stop:
			return
		}
	}
}

// Stops the cleanup timer (forcing the routine to exit)
func (c *MatcherCache) Stop() {
	close(c.stop)
}

// Retrieves the cached result for the given file if available.
func (c *MatcherCache) Get(file string) (bool, bool) {
	c.mut.RLock()
	defer c.mut.RUnlock()

	entry, ok := c.matches[file]
	if !ok {
		return false, false
	}
	return entry.matched, true
}

// Sets a cache entry for the given file.
func (c *MatcherCache) Set(file string, result bool) {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.matches[file] = CacheEntry{
		matched: result,
		at:      time.Now(),
	}
}

func Load(file string, duration time.Duration) (*Matcher, error) {
	seen := make(map[string]bool)
	matcher, err := loadIgnoreFile(file, seen)
	if duration == 0 || err != nil {
		return matcher, err
	}

	// Get the current cache object for the given ignore file
	cache, ok := caches[file]
	if !ok || !patternsEqual(cache.patterns, matcher.patterns) {
		// If there was an old cache, stop the cleanup timer
		if ok {
			cache.Stop()
		}
		// Create a new cache which will store matches for the given set of
		// patterns.
		newCache := &MatcherCache{
			patterns: matcher.patterns,
			matches:  make(map[string]CacheEntry),
			stop:     make(chan struct{}),
		}
		matcher.cache = newCache
		caches[file] = newCache
		go newCache.Serve(duration)
		return matcher, nil
	}

	matcher.cache = cache
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

	// We have a cache, means we should do caching
	if m.cache != nil {
		// Capture the result regardless, will force the old entry to get
		// replaced with a new one (bumping at time), or adding a new entry if
		// it doesn't exist.
		defer func() {
			m.cache.Set(file, result)
		}()

		// Check perhaps we've seen this file before, and we already know
		// what the outcome is going to be.
		result, ok := m.cache.Get(file)
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

// Patterns return a list of the loaded regexp patterns, as strings
func (m *Matcher) Patterns() []string {
	patterns := make([]string, len(m.patterns))
	for i, pat := range m.patterns {
		if pat.include {
			patterns[i] = pat.match.String()
		} else {
			patterns[i] = "(?exclude)" + pat.match.String()
		}
	}
	return patterns
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
