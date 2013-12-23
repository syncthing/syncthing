package flags

import (
	"fmt"
	"reflect"
	"unicode/utf8"
)

// Option flag information. Contains a description of the option, short and
// long name as well as a default value and whether an argument for this
// flag is optional.
type Option struct {
	// The description of the option flag. This description is shown
	// automatically in the builtin help.
	Description string

	// The short name of the option (a single character). If not 0, the
	// option flag can be 'activated' using -<ShortName>. Either ShortName
	// or LongName needs to be non-empty.
	ShortName rune

	// The long name of the option. If not "", the option flag can be
	// activated using --<LongName>. Either ShortName or LongName needs
	// to be non-empty.
	LongName string

	// The default value of the option.
	Default []string

	// If true, specifies that the argument to an option flag is optional.
	// When no argument to the flag is specified on the command line, the
	// value of Default will be set in the field this option represents.
	// This is only valid for non-boolean options.
	OptionalArgument bool

	// The optional value of the option. The optional value is used when
	// the option flag is marked as having an OptionalArgument. This means
	// that when the flag is specified, but no option argument is given,
	// the value of the field this option represents will be set to
	// OptionalValue. This is only valid for non-boolean options.
	OptionalValue []string

	// If true, the option _must_ be specified on the command line. If the
	// option is not specified, the parser will generate an ErrRequired type
	// error.
	Required bool

	// A name for the value of an option shown in the Help as --flag [ValueName]
	ValueName string

	// A mask value to show in the help instead of the default value. This
	// is useful for hiding sensitive information in the help, such as
	// passwords.
	DefaultMask string

	// The struct field which the option represents.
	field reflect.StructField

	// The struct field value which the option represents.
	value reflect.Value

	defaultValue reflect.Value
	iniUsedName  string
	tag          multiTag
}

// String converts an option to a human friendly readable string describing the
// option.
func (option *Option) String() string {
	var s string
	var short string

	if option.ShortName != 0 {
		data := make([]byte, utf8.RuneLen(option.ShortName))
		utf8.EncodeRune(data, option.ShortName)
		short = string(data)

		if len(option.LongName) != 0 {
			s = fmt.Sprintf("%s%s, %s%s",
				string(defaultShortOptDelimiter), short,
				defaultLongOptDelimiter, option.LongName)
		} else {
			s = fmt.Sprintf("%s%s", string(defaultShortOptDelimiter), short)
		}
	} else if len(option.LongName) != 0 {
		s = fmt.Sprintf("%s%s", defaultLongOptDelimiter, option.LongName)
	}

	return s
}

// Value returns the option value as an interface{}.
func (option *Option) Value() interface{} {
	return option.value.Interface()
}
