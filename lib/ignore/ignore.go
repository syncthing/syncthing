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
	"regexp"
	"strings"
	"time"

	//"github.com/gobwas/glob"
	"github.com/bmatcuk/doublestar/v4"

	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/sha256"
	"github.com/syncthing/syncthing/lib/sync"
)

const (
	resultNotMatched Result = 0
	resultInclude    Result = 1 << iota
	resultDeletable         = 1 << iota
	resultFoldCase          = 1 << iota
)

var defaultResult Result = resultInclude

func init() {
	if build.IsDarwin || build.IsWindows {
		defaultResult |= resultFoldCase
	}
}

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
	match   Glob
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

func (p Pattern) allowsSkippingIgnoredDirs() bool {
	if p.result.IsIgnored() {
		return true
	}
	if p.pattern[0] != '/' {
		return false
	}
	if strings.Contains(p.pattern[1:], "/") {
		return false
	}
	// Double asterisk everywhere in the path except at the end is bad
	return !strings.Contains(strings.TrimSuffix(p.pattern, "**"), "**")
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
	Remember(fs fs.Filesystem, name string, modtime time.Time)
	Seen(fs fs.Filesystem, name string) bool
	Changed() bool
	Reset()
}

type Matcher struct {
	fs              fs.Filesystem
	lines           []string  // exact lines read from .stignore
	patterns        []Pattern // patterns including those from included files
	withCache       bool
	matches         *cache
	curHash         string
	stop            chan struct{}
	changeDetector  ChangeDetector
	skipIgnoredDirs bool
	mut             sync.Mutex
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
		fs:              fs,
		stop:            make(chan struct{}),
		mut:             sync.NewMutex(),
		skipIgnoredDirs: true,
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

	m.skipIgnoredDirs = true
	var previous string
	for _, p := range patterns {
		// We automatically add patterns with a /** suffix, which normally
		// means that we cannot skip directories. However if the same
		// pattern without the /** already exists (which is true for
		// automatically added patterns) we can skip.
		if l := len(p.pattern); l > 3 && p.pattern[:len(p.pattern)-3] == previous {
			continue
		}
		if !p.allowsSkippingIgnoredDirs() {
			m.skipIgnoredDirs = false
			break
		}
		previous = p.pattern
	}

	m.curHash = newHash
	m.patterns = patterns
	if m.withCache {
		m.matches = newCache(patterns)
	}

	return err
}

func (m *Matcher) Match(file string) (result Result) {
	if file == "." {
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
		} else if pattern.match.Match(file) {
			return pattern.result
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

// ShouldIgnore returns true when a file is temporary, internal or ignored
func (m *Matcher) ShouldIgnore(filename string) bool {
	switch {
	case fs.IsTemporary(filename):
		return true

	case fs.IsInternal(filename):
		return true

	case m.Match(filename).IsIgnored():
		return true
	}

	return false
}

func (m *Matcher) SkipIgnoredDirs() bool {
	m.mut.Lock()
	defer m.mut.Unlock()
	return m.skipIgnoredDirs
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
		return nil, parseError(fmt.Errorf("multiple include of ignore file %q", file))
	}

	fd, info, err := loadIgnoreFile(filesystem, file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	cd.Remember(filesystem, file, info.ModTime())

	_, patterns, err := parseIgnoreFile(filesystem, fd, file, cd, linesSeen)
	return patterns, err
}

func parseLine(line string) ([]Pattern, error) {
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
		pattern.match, err = Compile(line[1:], '/')
		return []Pattern{pattern}, parseError(err)
	}
	patterns := make([]Pattern, 2)
	if strings.HasPrefix(line, "**/") {
		// Add the pattern as is, and without **/ so it matches in current dir
		pattern.match, err = Compile(line, '/')
		if err != nil {
			return nil, parseError(err)
		}
		patterns[0] = pattern

		line = line[3:]
		pattern.pattern = line
		pattern.match, err = Compile(line, '/')
		if err != nil {
			return nil, parseError(err)
		}
		patterns[1] = pattern
		return patterns, nil
	}
	// Path name or pattern, add it so it matches files both in
	// current directory and subdirs.
	pattern.match, err = Compile(line, '/')
	if err != nil {
		return nil, parseError(err)
	}
	patterns[0] = pattern

	line = "**/" + line
	pattern.pattern = line
	pattern.match, err = Compile(line, '/')
	if err != nil {
		return nil, parseError(err)
	}
	patterns[1] = pattern
	return patterns, nil
}

func parseIgnoreFile(fs fs.Filesystem, fd io.Reader, currentFile string, cd ChangeDetector, linesSeen map[string]struct{}) ([]string, []Pattern, error) {
	var patterns []Pattern
	interDoubleStar, _ := regexp.Compile(`(\w+)\*\*(\w+)`)
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
			err = addPattern(line + "*")
		case interDoubleStar.MatchString(line):
			err = addPattern(interDoubleStar.ReplaceAllString(line, `$1*/**/*$2`))
			err = addPattern(strings.ReplaceAll(line, "**", "*"))
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

type Glob interface {
	Match(string) bool
}

type myGlob struct {
	pattern    string
	separators string
}

func (s *myGlob) Match(path string) bool {
	doesMatch, _ := doublestar.Match(s.pattern, path)
	return doesMatch
}

func Compile(pattern string, separators ...rune) (Glob, error) {
	didValidate := doublestar.ValidatePattern(pattern)
	var err error
	if didValidate {
		err = nil
	} else {
		err = doublestar.ErrBadPattern
	}
	return &myGlob{
		pattern,
		string(separators),
	}, err
}
