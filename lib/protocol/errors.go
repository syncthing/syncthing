// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"errors"
)

var (
	ErrNoError    error
	ErrGeneric    = errors.New("generic error")
	ErrNoSuchFile = errors.New("no such file")
	ErrInvalid    = errors.New("file is invalid")
)

var lookupError = map[ErrorCode]error{
	ErrorCodeNoError:     ErrNoError,
	ErrorCodeGeneric:     ErrGeneric,
	ErrorCodeNoSuchFile:  ErrNoSuchFile,
	ErrorCodeInvalidFile: ErrInvalid,
}

var lookupCode = map[error]ErrorCode{
	ErrNoError:    ErrorCodeNoError,
	ErrGeneric:    ErrorCodeGeneric,
	ErrNoSuchFile: ErrorCodeNoSuchFile,
	ErrInvalid:    ErrorCodeInvalidFile,
}

func codeToError(code ErrorCode) error {
	err, ok := lookupError[code]
	if !ok {
		return ErrGeneric
	}
	return err
}

func errorToCode(err error) ErrorCode {
	code, ok := lookupCode[err]
	if !ok {
		return ErrorCodeGeneric
	}
	return code
}
