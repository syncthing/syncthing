// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobwas/glob"

	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/sync"
)

// A ParseError signifies an error with contents of an ignore file,
// including I/O errors on included files. An I/O error on the root level
// ignore file is not a ParseError.
type ParseError struct {
	inner error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error: %v", e.inner)
}

func (e *ParseError) Unwrap() error {
	return e.inner
}

func IsParseError(err error) bool {
	var e *ParseError
	return errors.As(err, &e)
}

func parseError(err error) error {
	if err == nil {
		return nil
	}
	return &ParseError{err}
}

type Pattern struct {
	pattern string
	match   glob.Glob
	result  ignoreresult.R
}

func (p Pattern) String() string {
	ret := p.pattern
	if !p.result.IsIgnored() {
		ret = "!" + ret
	}
	if p.result.IsCaseFolded() {
		ret = "(?i)" + ret
	}
	if p.result.IsDeletable() {
		ret = "(?d)" + ret
	}
	return ret
}

func (p Pattern) allowsSkippingIgnoredDirs() bool {
	if p.result.IsIgnored() {
		return true
	}
	if p.pattern[0] != '/' {
		return false
	}
	// A "/**" at the end is allowed and doesn't have any bearing on the
	// below checks; remove it before checking.
	pattern := strings.TrimSuffix(p.pattern, "/**")
	if len(pattern) == 0 {
		return true
	}
	if strings.Contains(pattern[1:], "/") {
		return false
	}
	// Double asterisk everywhere in the path except at the end is bad
	return !strings.Contains(strings.TrimSuffix(pattern, "**"), "**")
}

// The ChangeDetector is responsible for determining if files have changed
// on disk. It gets told to Remember() files (name and modtime) and will
// then get asked if a file has been Seen() (i.e., Remember() has been
// called on it) and if any of the files have Changed(). To forget all
// files, call Reset().
type ChangeDetector interface {
	Remember(fs fs.Filesystem, name string, modtime time.Time)
	Seen(fs fs.Filesystem, name string) bool
	Changed() bool
	Reset()
}

type Matcher struct {
	fs             fs.Filesystem
	lines          []string  // exact lines read from .stignore
	patterns       []Pattern // patterns including those from included files
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

func New(fs fs.Filesystem, opts ...Option) *Matcher {
	m := &Matcher{
		fs:   fs,
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

// Load and parse a file. The returned error may be of type *ParseError in
// which case a file was loaded from disk but the patterns could not be
// parsed. In this case the contents of the file are nonetheless available
// in the Lines() method.
func (m *Matcher) Load(file string) error {
	m.mut.Lock()
	defer m.mut.Unlock()

	if m.changeDetector.Seen(m.fs, file) && !m.changeDetector.Changed() {
		return nil
	}

	fd, info, err := loadIgnoreFile(m.fs, file)
	if err != nil {
		m.parseLocked(&bytes.Buffer{}, file)
		return err
	}
	defer fd.Close()

	m.changeDetector.Reset()

	err = m.parseLocked(fd, file)
	// If we failed to parse, don't cache, as next time Load is called
	// we'll pretend it's all good.
	if err == nil {
		m.changeDetector.Remember(m.fs, file, info.ModTime())
	}
	return err
}

// Load and parse an io.Reader. See Load() for notes on the returned error.
func (m *Matcher) Parse(r io.Reader, file string) error {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.parseLocked(r, file)
}

func (m *Matcher) parseLocked(r io.Reader, file string) error {
	lines, patterns, err := parseIgnoreFile(m.fs, r, file, m.changeDetector, make(map[string]struct{}))
	// Error is saved and returned at the end. We process the patterns
	// (possibly blank) anyway.

	m.lines = lines

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

// Match matches the patterns plus temporary and internal files.
func (m *Matcher) Match(file string) (result ignoreresult.R) {
	switch {
	case fs.IsTemporary(file):
		return ignoreresult.IgnoreAndSkip

	case fs.IsInternal(file):
		return ignoreresult.IgnoreAndSkip

	case file == ".":
		return ignoreresult.NotIgnored
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	if len(m.patterns) == 0 {
		return ignoreresult.NotIgnored
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

	// Check all the patterns for a match. Track wether the patterns so far
	// allow skipping matched directories or not. As soon as we hit an
	// exclude pattern (with some exceptions), we can't skip directories
	// anymore.
	file = filepath.ToSlash(file)
	var lowercaseFile string
	canSkipDir := true
	for _, pattern := range m.patterns {
		if canSkipDir && !pattern.allowsSkippingIgnoredDirs() {
			canSkipDir = false
		}

		res := pattern.result
		if canSkipDir {
			res = res.WithSkipDir()
		}
		if pattern.result.IsCaseFolded() {
			if lowercaseFile == "" {
				lowercaseFile = strings.ToLower(file)
			}
			if pattern.match.Match(lowercaseFile) {
				return res
			}
		} else if pattern.match.Match(file) {
			return res
		}
	}

	// Default to not matching.
	return ignoreresult.NotIgnored
}

// Lines return a list of the unprocessed lines in .stignore at last load
func (m *Matcher) Lines() []string {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.lines
}

// Patterns return a list of the loaded patterns, as they've been parsed
func (m *Matcher) Patterns() []string {
	m.mut.Lock()
	defer m.mut.Unlock()

	patterns := make([]string, len(m.patterns))
	for i, pat := range m.patterns {
		patterns[i] = pat.String()
	}
	return patterns
}

func (m *Matcher) String() string {
	return fmt.Sprintf("Matcher/%v@%p", m.Patterns(), m)
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
	h := sha256.New()
	for _, pat := range patterns {
		h.Write([]byte(pat.String()))
		h.Write([]byte("\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func loadIgnoreFile(fs fs.Filesystem, file string) (fs.File, fs.FileInfo, error) {
	fd, err := fs.Open(file)
	if err != nil {
		return fd, nil, err
	}

	info, err := fd.Stat()
	if err != nil {
		fd.Close()
	}

	return fd, info, err
}

func loadParseIncludeFile(filesystem fs.Filesystem, file string, cd ChangeDetector, linesSeen map[string]struct{}) ([]Pattern, error) {
	// Allow escaping the folders filesystem.
	// TODO: Deprecate, somehow?
	if filesystem.Type() == fs.FilesystemTypeBasic {
		uri := filesystem.URI()
		joined := filepath.Join(uri, file)
		if !fs.IsParent(joined, uri) {
			filesystem = fs.NewFilesystem(filesystem.Type(), filepath.Dir(joined))
			file = filepath.Base(joined)
		}
	}

	if cd.Seen(filesystem, file) {
		return nil, errors.New("multiple include")
	}

	fd, info, err := loadIgnoreFile(filesystem, file)
	if err != nil {
		// isNotExist is considered "ok" in a sense of that a folder doesn't have to act
		// upon it. This is because it is allowed for .stignore to not exist. However,
		// included ignore files are not allowed to be missing and these errors should be
		// acted upon on. So we don't preserve the error chain here and manually set an
		// error instead, if the file is missing.
		if fs.IsNotExist(err) {
			err = errors.New("file not found")
		}
		return nil, err
	}
	defer fd.Close()

	cd.Remember(filesystem, file, info.ModTime())

	_, patterns, err := parseIgnoreFile(filesystem, fd, file, cd, linesSeen)
	return patterns, err
}

func parseLine(line string) ([]Pattern, error) {
	pattern := Pattern{
		result: ignoreresult.Ignored,
	}

	// Allow prefixes to be specified in any order, but only once.
	var seenPrefix [3]bool

	for {
		if strings.HasPrefix(line, "!") && !seenPrefix[0] {
			seenPrefix[0] = true
			line = line[1:]
			pattern.result = pattern.result.ToggleIgnored()
		} else if strings.HasPrefix(line, "(?i)") && !seenPrefix[1] {
			seenPrefix[1] = true
			pattern.result = pattern.result.WithFoldCase()
			line = line[4:]
		} else if strings.HasPrefix(line, "(?d)") && !seenPrefix[2] {
			seenPrefix[2] = true
			pattern.result = pattern.result.WithDeletable()
			line = line[4:]
		} else {
			break
		}
	}

	if line == "" {
		return nil, parseError(errors.New("missing pattern"))
	}

	if pattern.result.IsCaseFolded() {
		line = strings.ToLower(line)
	}

	pattern.pattern = line

	var err error
	if strings.HasPrefix(line, "/") {
		// Pattern is rooted in the current dir only
		pattern.match, err = glob.Compile(line[1:], '/')
		return []Pattern{pattern}, parseError(err)
	}
	patterns := make([]Pattern, 2)
	if strings.HasPrefix(line, "**/") {
		// Add the pattern as is, and without **/ so it matches in current dir
		pattern.match, err = glob.Compile(line, '/')
		if err != nil {
			return nil, parseError(err)
		}
		patterns[0] = pattern

		line = line[3:]
		pattern.pattern = line
		pattern.match, err = glob.Compile(line, '/')
		if err != nil {
			return nil, parseError(err)
		}
		patterns[1] = pattern
		return patterns, nil
	}
	// Path name or pattern, add it so it matches files both in
	// current directory and subdirs.
	pattern.match, err = glob.Compile(line, '/')
	if err != nil {
		return nil, parseError(err)
	}
	patterns[0] = pattern

	line = "**/" + line
	pattern.pattern = line
	pattern.match, err = glob.Compile(line, '/')
	if err != nil {
		return nil, parseError(err)
	}
	patterns[1] = pattern
	return patterns, nil
}

func parseIgnoreFile(fs fs.Filesystem, fd io.Reader, currentFile string, cd ChangeDetector, linesSeen map[string]struct{}) ([]string, []Pattern, error) {
	var patterns []Pattern

	addPattern := func(line string) error {
		newPatterns, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("invalid pattern %q in ignore file: %w", line, err)
		}
		patterns = append(patterns, newPatterns...)
		return nil
	}

	scanner := bufio.NewScanner(fd)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	var err error
	for _, line := range lines {
		if _, ok := linesSeen[line]; ok {
			continue
		}
		linesSeen[line] = struct{}{}
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "//"):
			continue
		}

		line = filepath.ToSlash(line)
		switch {
		case strings.HasPrefix(line, "#include"):
			fields := strings.SplitN(line, " ", 2)
			if len(fields) != 2 {
				err = parseError(errors.New("failed to parse #include line: no file?"))
				break
			}

			includeRel := strings.TrimSpace(fields[1])
			if includeRel == "" {
				err = parseError(errors.New("failed to parse #include line: no file?"))
				break
			}

			includeFile := filepath.Join(filepath.Dir(currentFile), includeRel)
			var includePatterns []Pattern
			if includePatterns, err = loadParseIncludeFile(fs, includeFile, cd, linesSeen); err == nil {
				patterns = append(patterns, includePatterns...)
			} else {
				// Wrap the error, as if the include does not exist, we get a
				// IsNotExists(err) == true error, which we use to check
				// existence of the .stignore file, and just end up assuming
				// there is none, rather than a broken include.
				err = parseError(fmt.Errorf("failed to load include file %s: %w", includeFile, err))
			}
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
			return lines, nil, err
		}
	}

	return lines, patterns, nil
}

// WriteIgnores is a convenience function to avoid code duplication
func WriteIgnores(filesystem fs.Filesystem, path string, content []string) error {
	if len(content) == 0 {
		err := filesystem.Remove(path)
		if fs.IsNotExist(err) {
			return nil
		}
		return err
	}

	fd, err := osutil.CreateAtomicFilesystem(filesystem, path)
	if err != nil {
		return err
	}

	wr := osutil.LineEndingsWriter(fd)
	for _, line := range content {
		fmt.Fprintln(wr, line)
	}

	if err := fd.Close(); err != nil {
		return err
	}
	filesystem.Hide(path)

	return nil
}

type modtimeCheckerKey struct {
	fs   fs.Filesystem
	name string
}

// modtimeChecker is the default implementation of ChangeDetector
type modtimeChecker struct {
	modtimes map[modtimeCheckerKey]time.Time
}

func newModtimeChecker() *modtimeChecker {
	return &modtimeChecker{
		modtimes: map[modtimeCheckerKey]time.Time{},
	}
}

func (c *modtimeChecker) Remember(fs fs.Filesystem, name string, modtime time.Time) {
	c.modtimes[modtimeCheckerKey{fs, name}] = modtime
}

func (c *modtimeChecker) Seen(fs fs.Filesystem, name string) bool {
	_, ok := c.modtimes[modtimeCheckerKey{fs, name}]
	return ok
}

func (c *modtimeChecker) Reset() {
	c.modtimes = map[modtimeCheckerKey]time.Time{}
}

func (c *modtimeChecker) Changed() bool {
	for key, modtime := range c.modtimes {
		info, err := key.fs.Stat(key.name)
		if err != nil {
			return true
		}
		if !info.ModTime().Equal(modtime) {
			return true
		}
	}

	return false
}
