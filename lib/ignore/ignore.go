// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package ignore

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gobwas/glob"
	"github.com/syncthing/syncthing/lib/sync"
)

type Pattern struct {
	pattern  string
	match    glob.Glob
	include  bool
	foldCase bool
}

func (p Pattern) String() string {
	ret := p.pattern
	if !p.include {
		ret = "!" + ret
	}
	if p.foldCase {
		ret = "(?i)" + ret
	}
	return ret
}

type Matcher struct {
	patterns  []Pattern
	withCache bool
	matches   *cache
	curHash   string
	stop      chan struct{}
	mut       sync.Mutex
}

func New(withCache bool) *Matcher {
	m := &Matcher{
		withCache: withCache,
		stop:      make(chan struct{}),
		mut:       sync.NewMutex(),
	}
	if withCache {
		go m.clean(2 * time.Hour)
	}
	return m
}

func (m *Matcher) Load(file string) error {
	// No locking, Parse() does the locking

	fd, err := os.Open(file)
	if err != nil {
		// We do a parse with empty patterns to clear out the hash, cache etc.
		m.Parse(&bytes.Buffer{}, file)
		return err
	}
	defer fd.Close()

	return m.Parse(fd, file)
}

func (m *Matcher) Parse(r io.Reader, file string) error {
	m.mut.Lock()
	defer m.mut.Unlock()

	seen := map[string]bool{file: true}
	patterns, err := parseIgnoreFile(r, file, seen)
	// Error is saved and returned at the end. We process the patterns
	// (possibly blank) anyway.

	newHash := hashPatterns(patterns)
	if newHash == m.curHash {
		// We've already loaded exactly these patterns.
		return err
	}

	m.curHash = newHash
	m.patterns = patterns
	if m.withCache {
		m.matches = newCache(patterns)
	}

	return err
}

func (m *Matcher) Match(file string) (result bool) {
	if m == nil {
		return false
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	if len(m.patterns) == 0 {
		return false
	}

	if m.matches != nil {
		// Check the cache for a known result.
		res, ok := m.matches.get(file)
		if ok {
			return res
		}

		// Update the cache with the result at return time
		defer func() {
			m.matches.set(file, result)
		}()
	}

	// Check all the patterns for a match.
	file = filepath.ToSlash(file)
	var lowercaseFile string
	for _, pattern := range m.patterns {
		if pattern.foldCase {
			if lowercaseFile == "" {
				lowercaseFile = strings.ToLower(file)
			}
			if pattern.match.Match(lowercaseFile) {
				return pattern.include
			}
		} else {
			if pattern.match.Match(file) {
				return pattern.include
			}
		}
	}

	// Default to false.
	return false
}

// Patterns return a list of the loaded patterns, as they've been parsed
func (m *Matcher) Patterns() []string {
	if m == nil {
		return nil
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	patterns := make([]string, len(m.patterns))
	for i, pat := range m.patterns {
		patterns[i] = pat.String()
	}
	return patterns
}

func (m *Matcher) Hash() string {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.curHash
}

func (m *Matcher) Stop() {
	close(m.stop)
}

func (m *Matcher) clean(d time.Duration) {
	t := time.NewTimer(d / 2)
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.mut.Lock()
			if m.matches != nil {
				m.matches.clean(d)
			}
			t.Reset(d / 2)
			m.mut.Unlock()
		}
	}
}

func hashPatterns(patterns []Pattern) string {
	h := md5.New()
	for _, pat := range patterns {
		h.Write([]byte(pat.String()))
		h.Write([]byte("\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func loadIgnoreFile(file string, seen map[string]bool) ([]Pattern, error) {
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

func parseIgnoreFile(fd io.Reader, currentFile string, seen map[string]bool) ([]Pattern, error) {
	var patterns []Pattern

	addPattern := func(line string) error {
		pattern := Pattern{
			pattern:  line,
			include:  true,
			foldCase: runtime.GOOS == "darwin" || runtime.GOOS == "windows",
		}

		if strings.HasPrefix(line, "!") {
			line = line[1:]
			pattern.include = false
		}

		if strings.HasPrefix(line, "(?i)") {
			line = strings.ToLower(line[4:])
			pattern.foldCase = true
		}

		var err error
		if strings.HasPrefix(line, "/") {
			// Pattern is rooted in the current dir only
			pattern.match, err = glob.Compile(line[1:])
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file", line)
			}
			patterns = append(patterns, pattern)
		} else if strings.HasPrefix(line, "**/") {
			// Add the pattern as is, and without **/ so it matches in current dir
			pattern.match, err = glob.Compile(line)
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file", line)
			}
			patterns = append(patterns, pattern)

			pattern.match, err = glob.Compile(line[3:])
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file", line)
			}
			patterns = append(patterns, pattern)
		} else if strings.HasPrefix(line, "#include ") {
			includeRel := line[len("#include "):]
			includeFile := filepath.Join(filepath.Dir(currentFile), includeRel)
			includes, err := loadIgnoreFile(includeFile, seen)
			if err != nil {
				return fmt.Errorf("include of %q: %v", includeRel, err)
			}
			patterns = append(patterns, includes...)
		} else {
			// Path name or pattern, add it so it matches files both in
			// current directory and subdirs.
			pattern.match, err = glob.Compile(line)
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file", line)
			}
			patterns = append(patterns, pattern)

			pattern.match, err = glob.Compile("**/" + line)
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file", line)
			}
			patterns = append(patterns, pattern)
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
		}

		line = filepath.ToSlash(line)
		switch {
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

	return patterns, nil
}
