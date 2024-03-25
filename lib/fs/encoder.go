// Copyright (C) 2021 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package fs

import (
	"fmt"
	"strings"

	"github.com/syncthing/syncthing/lib/sync"
)

const (
	DefaultFilesystemEncoderType = FilesystemEncoderTypePassthrough
	// The base of the Private Use Unicode characters we encode to.
	encoderBase     = 0xf000
	encoderBaseRune = rune(encoderBase)
	// The set of characters we may possibly encode (0-0xff).
	// See https://github.com/microsoft/WSL/issues/3200#issuecomment-389613611
	encoderNumChars = 0x100
	// The pattern characters to unencode, if encoded.
	encoderPatternChars = "*?" // `\` is not encoded
	// Path prefix defining the NT namespace on Windows.
	ntNamespacePrefix = `\\?\`
	// Valid drive letters on Windows.
	validDrives = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcedfghijklmnopqrstuvwxyz"
)

// encoder is an interface to allow file and directory names that are
// reserved by the underlying file or operating system, to be saved
// to disk.
type encoder interface {
	// CharsToEncode returns a string of the characters to encode.
	CharsToEncode() string
	// EncoderRunes returns an array of the encoded runes for characters
	// \x00 to \xff.
	EncoderRunes() [encoderNumChars]rune
	// EncoderType returns the encoder type.
	EncoderType() FilesystemEncoderType
	// AllowReservedFilenames returns true if the encoder can create Windows
	// reserved filesnames, such as CON, using the \\?\ prefix trick.
	AllowReservedFilenames() bool
	// init build the encoderRunes array from the string containing the
	// characters to encode.
	init(encoderType FilesystemEncoderType)
	// encode encodes the path, if it contains reserved characters, or
	// reserved filename, such as CON.
	// The encoder is passed so any subclass methods override the underlying superclass method.
	encode(path string) string
	// encodeName returns the encoded name as a string.
	encodeName(name string) string
	// encodeName returns the encoded name as an array of runes.
	encodeToRunes(name string) []rune
	// filter returns files after filtering out any files that should be
	// ignored.
	filter(files []string) []string
	// wouldEncode returns true if a path would be encoded.
	wouldEncode(path string) bool
}

type baseEncoder struct {
	encoder
	charsToEncode          string                // characters to encode
	encoderRunes           [encoderNumChars]rune // array of encoded chars for chars 0-0xff
	encoderType            FilesystemEncoderType // encoder type (int32)
	allowReservedFilenames bool                  // save files using \\?\ prefix trick
}

var (
	// encodedChars contains all the possible encoded characters (\uf000-\uf0ff).
	encodedChars string
	// encodedPatternChars contains the encoded * and ? (\uf02a & \uf03f).
	encodedPatternChars string
	// encoders holds the array of encoders. The passthrough encoder is nil,
	// as it doesn't encode or decode.
	encoders = make(map[FilesystemEncoderType]encoder)
	// EncoderOptions holds the array of the encoder Option to pass to NewFilesystem().
	encoderOptions = make(map[FilesystemEncoderType]Option)
	// encoderMutex is used to update the 2 maps above.
	encoderMutex = sync.NewMutex()
	// charsToIgnore are never encoded, as they are never valid in a file or directory name.
	charsToIgnore = "/"
)

// registerEncoder registers an encoder, and the Option passed to newBasicFilesystem().
func registerEncoder(encoderType FilesystemEncoderType, enc encoder, opt Option) {
	l.Debugf("Registering %v encoder", encoderType)

	if enc != nil {
		enc.init(encoderType)
	}
	encoderMutex.Lock()
	defer encoderMutex.Unlock()
	encoders[encoderType] = enc
	encoderOptions[encoderType] = opt
}

func init() {
	// Ignore the slash character (/) as no filesystem we support
	// allows this character in a file or directory name (including
	// Windows). On Windows, ignore the backslash character (\) as well.
	if !strings.Contains(charsToIgnore, pathSeparatorString) {
		charsToIgnore += pathSeparatorString
	}

	for _, r := range encoderPatternChars {
		encodedPatternChars += string(r | encoderBaseRune)
	}
}

// GetEncoder returns the encoder for the specified encoderType.
// The returned encoder can be nil for the passthrough encoder.
func GetEncoder(encoderType FilesystemEncoderType) encoder {
	enc, ok := encoders[encoderType]
	if !ok {
		panic(fmt.Sprintf("Unregistered encoder %q (%d)", encoderType, encoderType))
	}
	return enc
}

// GetEncoderOption returns the Option to pass to newBasicFilesystem()
// or NewFilesystem() for the specified encoderType.
func GetEncoderOption(encoderType FilesystemEncoderType) Option {
	opt, ok := encoderOptions[encoderType]
	if !ok {
		panic(fmt.Sprintf("Unregistered encoder %q (%d)", encoderType, encoderType))
	}
	return opt
}

// GetEncoders returns the map of all the registered encoders.
func GetEncoders() map[FilesystemEncoderType]encoder {
	return encoders
}

func (f *baseEncoder) init(encoderType FilesystemEncoderType) {
	f.encoderType = encoderType
	for r := 0; r < encoderNumChars; r++ {
		f.encoderRunes[r] = rune(r)
	}

	for _, r := range f.charsToEncode {
		if strings.ContainsRune(charsToIgnore, r) {
			l.Warnf("%s encoder: The %q character cannot be encoded (as it is ignored)", encoderType, string(r))
			continue
		}
		if r >= encoderNumChars {
			l.Warnf("%s encoder: The %q character cannot be encoded (as its Unicode value (%d) is above %d)", encoderType, string(r), r, encoderNumChars-1)
			continue
		}
		encoded := r | encoderBaseRune
		f.encoderRunes[r] = encoded

		if !strings.ContainsRune(encodedChars, encoded) {
			encodedChars += string(encoded)
		}
	}
}

func (f *baseEncoder) CharsToEncode() string {
	return f.charsToEncode
}

func (f *baseEncoder) EncoderRunes() [encoderNumChars]rune {
	return f.encoderRunes
}

func (f *baseEncoder) EncoderType() FilesystemEncoderType {
	return f.encoderType
}

func (f *baseEncoder) AllowReservedFilenames() bool {
	return f.allowReservedFilenames
}

// path is a complete path with pathSeparator characters.
func (f *baseEncoder) encode(path string) string {
	encoded := ""

	// We may want to use
	// strings.Split(filepath.Clean(path), pathSeparatorString)
	// instead.
	for i, part := range PathComponents(path) {
		if i > 0 {
			encoded += pathSeparatorString
		}
		if part != "" {
			encoded += f.encodeName(part)
		}
	}

	return encoded
}

// name is a file or directory name without pathSeparator characters.
func (f *baseEncoder) encodeName(name string) string {
	if name == "" || name == "." || name == ".." {
		return name
	}

	return string(f.encodeToRunes(name))
}

// name is a file or directory name without pathSeparator characters.
func (f *baseEncoder) encodeToRunes(name string) []rune {
	if name == "" || name == "." || name == ".." {
		return []rune(name)
	}

	runes := []rune(name)
	for i, r := range runes {
		if r >= 0 && r < encoderNumChars {
			runes[i] = f.encoderRunes[r]
		}
	}

	return runes
}

func (f *baseEncoder) filter(files []string) []string {
	// Only the standard encoder filters encoded files.
	if f.encoderType != FilesystemEncoderTypeStandard {
		return files
	}
	var filtered []string
	for _, path := range files {
		if isEncoded(path) {
			if l.ShouldDebug("encoder") {
				l.Debugf(filenameContainsSyncthingReservedChars, path)
			}
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
}

// path is a complete path with pathSeparator characters.
func (f *baseEncoder) wouldEncode(path string) bool {
	return f.encode(path) != path
}

// decode returns the decoded path. It doesn't matter what encoder is
// being used, as they all decode using the same algorithm.
// We only decode those characters that any encoder encodes.
//
// We could choose to decode all characters in the \uf000-\uf0ff space,
// as all of these characters are reserved for Syncthing's use, and
// future encoders may use any of these characters, but we don't
// currently. It seems it's best to only decode what we encode per
// https://github.com/microsoft/WSL/issues/3200#issuecomment-389613611
//
// path is a complete path with pathSeparator characters.
func decode(path string) string {
	if !strings.ContainsAny(path, encodedChars) {
		return path
	}
	return decodeChars(path, encodedChars)
}

func decodeChars(path, charsToDecode string) string {
	runes := []rune(path)

	for i, r := range runes {
		if strings.ContainsRune(charsToDecode, r) {
			runes[i] &^= encoderBaseRune
		}
	}

	return string(runes)
}

// decodePattern returns the path, with only the '*' and
// '?' characters decoded. It is only called by the Glob()
// function.
//
// path is a complete path with pathSeparator characters.
func decodePattern(path string) string {
	if !strings.ContainsAny(path, encodedPatternChars) {
		return path
	}
	return decodeChars(path, encodedPatternChars)
}

// isEncoded returns true if path contains any encoded characters.
//
// path is a complete path with pathSeparator characters.
func isEncoded(path string) bool {
	return strings.ContainsAny(path, encodedChars)
}
