package flags

import (
	"fmt"
)

// ErrorType represents the type of error.
type ErrorType uint

const (
	// ErrUnknown indicates a generic error.
	ErrUnknown ErrorType = iota

	// ErrExpectedArgument indicates that an argument was expected.
	ErrExpectedArgument

	// ErrUnknownFlag indicates an unknown flag.
	ErrUnknownFlag

	// ErrUnknownGroup indicates an unknown group.
	ErrUnknownGroup

	// ErrMarshal indicates a marshalling error while converting values.
	ErrMarshal

	// ErrHelp indicates that the builtin help was shown (the error
	// contains the help message).
	ErrHelp

	// ErrNoArgumentForBool indicates that an argument was given for a
	// boolean flag (which don't not take any arguments).
	ErrNoArgumentForBool

	// ErrRequired indicates that a required flag was not provided.
	ErrRequired

	// ErrShortNameTooLong indicates that a short flag name was specified,
	// longer than one character.
	ErrShortNameTooLong

	// ErrDuplicatedFlag indicates that a short or long flag has been
	// defined more than once
	ErrDuplicatedFlag

	// ErrTag indicates an error while parsing flag tags.
	ErrTag
)

// String returns a string representation of the error type.
func (e ErrorType) String() string {
	switch e {
	case ErrUnknown:
		return "unknown"
	case ErrExpectedArgument:
		return "expected argument"
	case ErrUnknownFlag:
		return "unknown flag"
	case ErrUnknownGroup:
		return "unknown group"
	case ErrMarshal:
		return "marshal"
	case ErrHelp:
		return "help"
	case ErrNoArgumentForBool:
		return "no argument for bool"
	case ErrRequired:
		return "required"
	case ErrShortNameTooLong:
		return "short name too long"
	case ErrDuplicatedFlag:
		return "duplicated flag"
	case ErrTag:
		return "tag"
	}

	return "unknown"
}

// Error represents a parser error. The error returned from Parse is of this
// type. The error contains both a Type and Message.
type Error struct {
	// The type of error
	Type ErrorType

	// The error message
	Message string
}

// Error returns the error's message
func (e *Error) Error() string {
	return e.Message
}

func newError(tp ErrorType, message string) *Error {
	return &Error{
		Type:    tp,
		Message: message,
	}
}

func newErrorf(tp ErrorType, format string, args ...interface{}) *Error {
	return newError(tp, fmt.Sprintf(format, args...))
}

func wrapError(err error) *Error {
	ret, ok := err.(*Error)

	if !ok {
		return newError(ErrUnknown, err.Error())
	}

	return ret
}
