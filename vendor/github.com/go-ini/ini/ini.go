// Copyright 2014 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

// Package ini provides INI file read and write functionality in Go.
package ini

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
)

const (
	// Name for default section. You can use this constant or the string literal.
	// In most of cases, an empty string is all you need to access the section.
	DEFAULT_SECTION = "DEFAULT"

	// Maximum allowed depth when recursively substituing variable names.
	_DEPTH_VALUES = 99
	_VERSION      = "1.32.0"
)

// Version returns current package version literal.
func Version() string {
	return _VERSION
}

var (
	// Delimiter to determine or compose a new line.
	// This variable will be changed to "\r\n" automatically on Windows
	// at package init time.
	LineBreak = "\n"

	// Variable regexp pattern: %(variable)s
	varPattern = regexp.MustCompile(`%\(([^\)]+)\)s`)

	// Indicate whether to align "=" sign with spaces to produce pretty output
	// or reduce all possible spaces for compact format.
	PrettyFormat = true

	// Explicitly write DEFAULT section header
	DefaultHeader = false

	// Indicate whether to put a line between sections
	PrettySection = true
)

func init() {
	if runtime.GOOS == "windows" {
		LineBreak = "\r\n"
	}
}

func inSlice(str string, s []string) bool {
	for _, v := range s {
		if str == v {
			return true
		}
	}
	return false
}

// dataSource is an interface that returns object which can be read and closed.
type dataSource interface {
	ReadCloser() (io.ReadCloser, error)
}

// sourceFile represents an object that contains content on the local file system.
type sourceFile struct {
	name string
}

func (s sourceFile) ReadCloser() (_ io.ReadCloser, err error) {
	return os.Open(s.name)
}

// sourceData represents an object that contains content in memory.
type sourceData struct {
	data []byte
}

func (s *sourceData) ReadCloser() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(s.data)), nil
}

// sourceReadCloser represents an input stream with Close method.
type sourceReadCloser struct {
	reader io.ReadCloser
}

func (s *sourceReadCloser) ReadCloser() (io.ReadCloser, error) {
	return s.reader, nil
}

func parseDataSource(source interface{}) (dataSource, error) {
	switch s := source.(type) {
	case string:
		return sourceFile{s}, nil
	case []byte:
		return &sourceData{s}, nil
	case io.ReadCloser:
		return &sourceReadCloser{s}, nil
	default:
		return nil, fmt.Errorf("error parsing data source: unknown type '%s'", s)
	}
}

type LoadOptions struct {
	// Loose indicates whether the parser should ignore nonexistent files or return error.
	Loose bool
	// Insensitive indicates whether the parser forces all section and key names to lowercase.
	Insensitive bool
	// IgnoreContinuation indicates whether to ignore continuation lines while parsing.
	IgnoreContinuation bool
	// IgnoreInlineComment indicates whether to ignore comments at the end of value and treat it as part of value.
	IgnoreInlineComment bool
	// AllowBooleanKeys indicates whether to allow boolean type keys or treat as value is missing.
	// This type of keys are mostly used in my.cnf.
	AllowBooleanKeys bool
	// AllowShadows indicates whether to keep track of keys with same name under same section.
	AllowShadows bool
	// AllowNestedValues indicates whether to allow AWS-like nested values.
	// Docs: http://docs.aws.amazon.com/cli/latest/topic/config-vars.html#nested-values
	AllowNestedValues bool
	// UnescapeValueDoubleQuotes indicates whether to unescape double quotes inside value to regular format
	// when value is surrounded by double quotes, e.g. key="a \"value\"" => key=a "value"
	UnescapeValueDoubleQuotes bool
	// UnescapeValueCommentSymbols indicates to unescape comment symbols (\# and \;) inside value to regular format
	// when value is NOT surrounded by any quotes.
	// Note: UNSTABLE, behavior might change to only unescape inside double quotes but may noy necessary at all.
	UnescapeValueCommentSymbols bool
	// Some INI formats allow group blocks that store a block of raw content that doesn't otherwise
	// conform to key/value pairs. Specify the names of those blocks here.
	UnparseableSections []string
}

func LoadSources(opts LoadOptions, source interface{}, others ...interface{}) (_ *File, err error) {
	sources := make([]dataSource, len(others)+1)
	sources[0], err = parseDataSource(source)
	if err != nil {
		return nil, err
	}
	for i := range others {
		sources[i+1], err = parseDataSource(others[i])
		if err != nil {
			return nil, err
		}
	}
	f := newFile(sources, opts)
	if err = f.Reload(); err != nil {
		return nil, err
	}
	return f, nil
}

// Load loads and parses from INI data sources.
// Arguments can be mixed of file name with string type, or raw data in []byte.
// It will return error if list contains nonexistent files.
func Load(source interface{}, others ...interface{}) (*File, error) {
	return LoadSources(LoadOptions{}, source, others...)
}

// LooseLoad has exactly same functionality as Load function
// except it ignores nonexistent files instead of returning error.
func LooseLoad(source interface{}, others ...interface{}) (*File, error) {
	return LoadSources(LoadOptions{Loose: true}, source, others...)
}

// InsensitiveLoad has exactly same functionality as Load function
// except it forces all section and key names to be lowercased.
func InsensitiveLoad(source interface{}, others ...interface{}) (*File, error) {
	return LoadSources(LoadOptions{Insensitive: true}, source, others...)
}

// InsensitiveLoad has exactly same functionality as Load function
// except it allows have shadow keys.
func ShadowLoad(source interface{}, others ...interface{}) (*File, error) {
	return LoadSources(LoadOptions{AllowShadows: true}, source, others...)
}
