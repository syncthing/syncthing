package flags

import (
	"fmt"
	"io"
	"os"
)

// IniError contains location information on where in the ini file an error
// occured.
type IniError struct {
	// The error message.
	Message    string

	// The filename of the file in which the error occurred.
	File       string

	// The line number at which the error occurred.
	LineNumber uint
}

// Error provides a "file:line: message" formatted message of the ini error.
func (x *IniError) Error() string {
	return fmt.Sprintf("%s:%d: %s",
		x.File,
		x.LineNumber,
		x.Message)
}

// IniOptions for writing ini files
type IniOptions uint

const (
	// IniNone indicates no options.
	IniNone IniOptions = 0

	// IniIncludeDefaults indicates that default values should be written
	// when writing options to an ini file.
	IniIncludeDefaults = 1 << iota

	// IniIncludeComments indicates that comments containing the description
	// of an option should be written when writing options to an ini file.
	IniIncludeComments

	// IniDefault provides a default set of options.
	IniDefault = IniIncludeComments
)

// IniParser is a utility to read and write flags options from and to ini
// files.
type IniParser struct {
	parser *Parser
}

// NewIniParser creates a new ini parser for a given Parser.
func NewIniParser(p *Parser) *IniParser {
	return &IniParser{
		parser: p,
	}
}

// IniParse is a convenience function to parse command line options with default
// settings from an ini file. The provided data is a pointer to a struct
// representing the default option group (named "Application Options"). For
// more control, use flags.NewParser.
func IniParse(filename string, data interface{}) error {
	p := NewParser(data, Default)
	return NewIniParser(p).ParseFile(filename)
}

// ParseFile parses flags from an ini formatted file. See Parse for more
// information on the ini file foramt. The returned errors can be of the type
// flags.Error or flags.IniError.
func (i *IniParser) ParseFile(filename string) error {
	i.parser.storeDefaults()

	ini, err := readIniFromFile(filename)

	if err != nil {
		return err
	}

	return i.parse(ini)
}

// Parse parses flags from an ini format. You can use ParseFile as a
// convenience function to parse from a filename instead of a general
// io.Reader.
//
// The format of the ini file is as follows:
//
//     [Option group name]
//     option = value
//
// Each section in the ini file represents an option group or command in the
// flags parser. The default flags parser option group (i.e. when using
// flags.Parse) is named 'Application Options'. The ini option name is matched
// in the following order:
//
//     1. Compared to the ini-name tag on the option struct field (if present)
//     2. Compared to the struct field name
//     3. Compared to the option long name (if present)
//     4. Compared to the option short name (if present)
//
// Sections for nested groups and commands can be addressed using a dot `.'
// namespacing notation (i.e [subcommand.Options]). Group section names are
// matched case insensitive.
//
// The returned errors can be of the type flags.Error or
// flags.IniError.
func (i *IniParser) Parse(reader io.Reader) error {
	i.parser.storeDefaults()

	ini, err := readIni(reader, "")

	if err != nil {
		return err
	}

	return i.parse(ini)
}

// WriteFile writes the flags as ini format into a file. See WriteIni
// for more information. The returned error occurs when the specified file
// could not be opened for writing.
func (i *IniParser) WriteFile(filename string, options IniOptions) error {
	file, err := os.Create(filename)

	if err != nil {
		return err
	}

	defer file.Close()
	i.Write(file, options)

	return nil
}

// Write writes the current values of all the flags to an ini format.
// See Parse for more information on the ini file format. You typically
// call this only after settings have been parsed since the default values of each
// option are stored just before parsing the flags (this is only relevant when
// IniIncludeDefaults is _not_ set in options).
func (i *IniParser) Write(writer io.Writer, options IniOptions) {
	writeIni(i, writer, options)
}
