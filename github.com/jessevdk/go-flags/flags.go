// Copyright 2012 Jesse van den Kieboom. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package flags provides an extensive command line option parser.
// The flags package is similar in functionality to the go builtin flag package
// but provides more options and uses reflection to provide a convenient and
// succinct way of specifying command line options.
//
// Supported features:
//     Options with short names (-v)
//     Options with long names (--verbose)
//     Options with and without arguments (bool v.s. other type)
//     Options with optional arguments and default values
//     Multiple option groups each containing a set of options
//     Generate and print well-formatted help message
//     Passing remaining command line arguments after -- (optional)
//     Ignoring unknown command line options (optional)
//     Supports -I/usr/include -I=/usr/include -I /usr/include option argument specification
//     Supports multiple short options -aux
//     Supports all primitive go types (string, int{8..64}, uint{8..64}, float)
//     Supports same option multiple times (can store in slice or last option counts)
//     Supports maps
//     Supports function callbacks
//
// Additional features specific to Windows:
//     Options with short names (/v)
//     Options with long names (/verbose)
//     Windows-style options with arguments use a colon as the delimiter
//     Modify generated help message with Windows-style / options
//
// The flags package uses structs, reflection and struct field tags
// to allow users to specify command line options. This results in very simple
// and consise specification of your application options. For example:
//
//     type Options struct {
//         Verbose []bool `short:"v" long:"verbose" description:"Show verbose debug information"`
//     }
//
// This specifies one option with a short name -v and a long name --verbose.
// When either -v or --verbose is found on the command line, a 'true' value
// will be appended to the Verbose field. e.g. when specifying -vvv, the
// resulting value of Verbose will be {[true, true, true]}.
//
// Slice options work exactly the same as primitive type options, except that
// whenever the option is encountered, a value is appended to the slice.
//
// Map options from string to primitive type are also supported. On the command
// line, you specify the value for such an option as key:value. For example
//
//     type Options struct {
//         AuthorInfo string[string] `short:"a"`
//     }
//
// Then, the AuthorInfo map can be filled with something like
// -a name:Jesse -a "surname:van den Kieboom".
//
// Finally, for full control over the conversion between command line argument
// values and options, user defined types can choose to implement the Marshaler
// and Unmarshaler interfaces.
//
// Available field tags:
//     short:          the short name of the option (single character)
//     long:           the long name of the option
//     description:    the description of the option (optional)
//     optional:       whether an argument of the option is optional (optional)
//     optional-value: the value of an optional option when the option occurs
//                     without an argument. This tag can be specified multiple
//                     times in the case of maps or slices (optional)
//     default:        the default value of an option. This tag can be specified
//                     multiple times in the case of slices or maps (optional).
//     default-mask:   when specified, this value will be displayed in the help
//                     instead of the actual default value. This is useful
//                     mostly for hiding otherwise sensitive information from
//                     showing up in the help. If default-mask takes the special
//                     value "-", then no default value will be shown at all
//                     (optional)
//     required:       whether an option is required to appear on the command
//                     line. If a required option is not present, the parser
//                     will return ErrRequired.
//     base:           a base (radix) used to convert strings to integer values,
//                     the default base is 10 (i.e. decimal) (optional)
//     value-name:     the name of the argument value (to be shown in the help,
//                     (optional)
//     group:          when specified on a struct field, makes the struct field
//                     a separate group with the given name (optional).
//     command:        when specified on a struct field, makes the struct field
//                     a (sub)command with the given name (optional).
//
// Either short: or long: must be specified to make the field eligible as an
// option.
//
//
// Option groups:
//
// Option groups are a simple way to semantically separate your options. The
// only real difference is in how your options will appear in the builtin
// generated help. All options in a particular group are shown together in the
// help under the name of the group.
//
// There are currently three ways to specify option groups.
//
//     1. Use NewNamedParser specifying the various option groups.
//     2. Use AddGroup to add a group to an existing parser.
//     3. Add a struct field to the toplevel options annotated with the
//        group:"group-name" tag.
//
//
//
// Commands:
//
// The flags package also has basic support for commands. Commands are often
// used in monolithic applications that support various commands or actions.
// Take git for example, all of the add, commit, checkout, etc. are called
// commands. Using commands you can easily separate multiple functions of your
// application.
//
// There are currently two ways to specifiy a command.
//
//     1. Use AddCommand on an existing parser.
//     2. Add a struct field to your options struct annotated with the
//        command:"command-name" tag.
//
// The most common, idiomatic way to implement commands is to define a global
// parser instance and implement each command in a separate file. These
// command files should define a go init function which calls AddCommand on
// the global parser.
//
// When parsing ends and there is an active command and that command implements
// the Commander interface, then its Execute method will be run with the
// remaining command line arguments.
//
// Command structs can have options which become valid to parse after the
// command has been specified on the command line. It is currently not valid
// to specify options from the parent level of the command after the command
// name has occurred. Thus, given a toplevel option "-v" and a command "add":
//
//     Valid:   ./app -v add
//     Invalid: ./app add -v
//
package flags
