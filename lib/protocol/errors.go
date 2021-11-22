// Copyright (C) 2014 The Protocol Authors.

package protocol

import "errors"

var (
	ErrGeneric    = errors.New("generic error")
	ErrNoSuchFile = errors.New("no such file")
	ErrInvalid    = errors.New("file is invalid")
)

func codeToError(code ErrorCode) error {
	switch code {
	case ErrorCodeNoError:
		return nil
	case ErrorCodeNoSuchFile:
		return ErrNoSuchFile
	case ErrorCodeInvalidFile:
		return ErrInvalid
	default:
		return ErrGeneric
	}
}

func errorToCode(err error) ErrorCode {
	switch err {
	case nil:
		return ErrorCodeNoError
	case ErrNoSuchFile:
		return ErrorCodeNoSuchFile
	case ErrInvalid:
		return ErrorCodeInvalidFile
	default:
		return ErrorCodeGeneric
	}
}
