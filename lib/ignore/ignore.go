// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sync"
)

const (
	resultNotMatched Result = 0
	resultInclude    Result = 1 << iota
	resultDeletable         = 1 << iota
	resultFoldCase          = 1 << iota
)

type Pattern struct {
	pattern string
	match   glob.Glob
	result  Result
}

func (p Pattern) String() string {
	ret := p.pattern
	if p.result&resultInclude != resultInclude {
		ret = "!" + ret
	}
	if p.result&resultFoldCase == resultFoldCase {
		ret = "(?i)" + ret
	}
	if p.result&resultDeletable == resultDeletable {
		ret = "(?d)" + ret
	}
	return ret
}

type Result uint8

func (r Result) IsIgnored() bool {
	return r&resultInclude == resultInclude
}

func (r Result) IsDeletable() bool {
	return r.IsIgnored() && r&resultDeletable == resultDeletable
}

func (r Result) IsCaseFolded() bool {
	return r&resultFoldCase == resultFoldCase
}

// The ChangeDetector is responsible for determining if files have changed
// on disk. It gets told to Remember() files (name and modtime) and will
// then get asked if a file has been Seen() (i.e., Remember() has been
// called on it) and if any of the files have Changed(). To forget all
// files, call Reset().
type ChangeDetector interface {
	Remember(name string, modtime time.Time)
	Seen(name string) bool
	Changed() bool
	Reset()
}

type Matcher struct {
	lines          []string
	patterns       []Pattern
	withCache      bool
	matches        *cache
	curHash        string
	stop           chan struct{}
	changeDetector ChangeDetector
	mut            sync.Mutex
}

// An Option can be passed to New()
type Option func(*Matcher)

// WithCache enables or disables lookup caching. The default is disabled.
func WithCache(v bool) Option {
	return func(m *Matcher) {
		m.withCache = v
	}
}

// WithChangeDetector sets a custom ChangeDetector. The default is to simply
// use the on disk modtime for comparison.
func WithChangeDetector(cd ChangeDetector) Option {
	return func(m *Matcher) {
		m.changeDetector = cd
	}
}

func New(opts ...Option) *Matcher {
	m := &Matcher{
		stop: make(chan struct{}),
		mut:  sync.NewMutex(),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.changeDetector == nil {
		m.changeDetector = newModtimeChecker()
	}
	if m.withCache {
		go m.clean(2 * time.Hour)
	}
	return m
}

func (m *Matcher) Load(file string) error {
	m.mut.Lock()
	defer m.mut.Unlock()

	if m.changeDetector.Seen(file) && !m.changeDetector.Changed() {
		return nil
	}

	fd, err := os.Open(file)
	if err != nil {
		m.parseLocked(&bytes.Buffer{}, file)
		return err
	}
	defer fd.Close()

	info, err := fd.Stat()
	if err != nil {
		m.parseLocked(&bytes.Buffer{}, file)
		return err
	}

	m.changeDetector.Reset()
	m.changeDetector.Remember(file, info.ModTime())

	return m.parseLocked(fd, file)
}

func (m *Matcher) Parse(r io.Reader, file string) error {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.parseLocked(r, file)
}

func (m *Matcher) parseLocked(r io.Reader, file string) error {
	lines, patterns, err := parseIgnoreFile(r, file, m.changeDetector)
	// Error is saved and returned at the end. We process the patterns
	// (possibly blank) anyway.

	newHash := hashPatterns(patterns)
	if newHash == m.curHash {
		// We've already loaded exactly these patterns.
		return err
	}

	m.curHash = newHash
	m.lines = lines
	m.patterns = patterns
	if m.withCache {
		m.matches = newCache(patterns)
	}

	return err
}

func (m *Matcher) Match(file string) (result Result) {
	if m == nil || file == "." {
		return resultNotMatched
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	if len(m.patterns) == 0 {
		return resultNotMatched
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
		if pattern.result.IsCaseFolded() {
			if lowercaseFile == "" {
				lowercaseFile = strings.ToLower(file)
			}
			if pattern.match.Match(lowercaseFile) {
				return pattern.result
			}
		} else {
			if pattern.match.Match(file) {
				return pattern.result
			}
		}
	}

	// Default to not matching.
	return resultNotMatched
}

// Lines return a list of the unprocessed lines in .stignore at last load
func (m *Matcher) Lines() []string {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.lines
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

// ShouldIgnore returns true when a file is temporary, internal or ignored
func (m *Matcher) ShouldIgnore(filename string) bool {
	switch {
	case IsTemporary(filename):
		return true

	case IsInternal(filename):
		return true

	case m.Match(filename).IsIgnored():
		return true
	}

	return false
}

func hashPatterns(patterns []Pattern) string {
	h := md5.New()
	for _, pat := range patterns {
		h.Write([]byte(pat.String()))
		h.Write([]byte("\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func loadIgnoreFile(file string, cd ChangeDetector) ([]string, []Pattern, error) {
	if cd.Seen(file) {
		return nil, nil, fmt.Errorf("multiple include of ignore file %q", file)
	}

	fd, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer fd.Close()

	info, err := fd.Stat()
	if err != nil {
		return nil, nil, err
	}

	cd.Remember(file, info.ModTime())

	return parseIgnoreFile(fd, file, cd)
}

func parseIgnoreFile(fd io.Reader, currentFile string, cd ChangeDetector) ([]string, []Pattern, error) {
	var lines []string
	var patterns []Pattern

	defaultResult := resultInclude
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		defaultResult |= resultFoldCase
	}

	addPattern := func(line string) error {
		pattern := Pattern{
			result: defaultResult,
		}

		// Allow prefixes to be specified in any order, but only once.
		var seenPrefix [3]bool

		for {
			if strings.HasPrefix(line, "!") && !seenPrefix[0] {
				seenPrefix[0] = true
				line = line[1:]
				pattern.result ^= resultInclude
			} else if strings.HasPrefix(line, "(?i)") && !seenPrefix[1] {
				seenPrefix[1] = true
				pattern.result |= resultFoldCase
				line = line[4:]
			} else if strings.HasPrefix(line, "(?d)") && !seenPrefix[2] {
				seenPrefix[2] = true
				pattern.result |= resultDeletable
				line = line[4:]
			} else {
				break
			}
		}

		if pattern.result.IsCaseFolded() {
			line = strings.ToLower(line)
		}

		pattern.pattern = line

		var err error
		if strings.HasPrefix(line, "/") {
			// Pattern is rooted in the current dir only
			pattern.match, err = glob.Compile(line[1:], '/')
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file (%v)", line, err)
			}
			patterns = append(patterns, pattern)
		} else if strings.HasPrefix(line, "**/") {
			// Add the pattern as is, and without **/ so it matches in current dir
			pattern.match, err = glob.Compile(line, '/')
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file (%v)", line, err)
			}
			patterns = append(patterns, pattern)

			line = line[3:]
			pattern.pattern = line
			pattern.match, err = glob.Compile(line, '/')
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file (%v)", line, err)
			}
			patterns = append(patterns, pattern)
		} else if strings.HasPrefix(line, "#include ") {
			includeRel := line[len("#include "):]
			includeFile := filepath.Join(filepath.Dir(currentFile), includeRel)
			includeLines, includePatterns, err := loadIgnoreFile(includeFile, cd)
			if err != nil {
				return fmt.Errorf("include of %q: %v", includeRel, err)
			}
			lines = append(lines, includeLines...)
			patterns = append(patterns, includePatterns...)
		} else {
			// Path name or pattern, add it so it matches files both in
			// current directory and subdirs.
			pattern.match, err = glob.Compile(line, '/')
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file (%v)", line, err)
			}
			patterns = append(patterns, pattern)

			line := "**/" + line
			pattern.pattern = line
			pattern.match, err = glob.Compile(line, '/')
			if err != nil {
				return fmt.Errorf("invalid pattern %q in ignore file (%v)", line, err)
			}
			patterns = append(patterns, pattern)
		}
		return nil
	}

	scanner := bufio.NewScanner(fd)
	var err error
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lines = append(lines, line)
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
			err = addPattern(line + "**")
		default:
			err = addPattern(line)
			if err == nil {
				err = addPattern(line + "/**")
			}
		}
		if err != nil {
			return nil, nil, err
		}
	}

	return lines, patterns, nil
}

// IsInternal returns true if the file, as a path relative to the folder
// root, represents an internal file that should always be ignored. The file
// path must be clean (i.e., in canonical shortest form).
func IsInternal(file string) bool {
	internals := []string{".stfolder", ".stignore", ".stversions"}
	pathSep := string(os.PathSeparator)
	for _, internal := range internals {
		if file == internal {
			return true
		}
		if strings.HasPrefix(file, internal+pathSep) {
			return true
		}
	}
	return false
}

// WriteIgnores is a convenience function to avoid code duplication
func WriteIgnores(path string, content []string) error {
	fd, err := osutil.CreateAtomic(path)
	if err != nil {
		return err
	}

	for _, line := range content {
		fmt.Fprintln(fd, line)
	}

	if err := fd.Close(); err != nil {
		return err
	}
	osutil.HideFile(path)

	return nil
}

// modtimeChecker is the default implementation of ChangeDetector
type modtimeChecker struct {
	modtimes map[string]time.Time
}

func newModtimeChecker() *modtimeChecker {
	return &modtimeChecker{
		modtimes: map[string]time.Time{},
	}
}

func (c *modtimeChecker) Remember(name string, modtime time.Time) {
	c.modtimes[name] = modtime
}

func (c *modtimeChecker) Seen(name string) bool {
	_, ok := c.modtimes[name]
	return ok
}

func (c *modtimeChecker) Reset() {
	c.modtimes = map[string]time.Time{}
}

func (c *modtimeChecker) Changed() bool {
	for name, modtime := range c.modtimes {
		info, err := os.Stat(name)
		if err != nil {
			return true
		}
		if !info.ModTime().Equal(modtime) {
			return true
		}
	}

	return false
}
