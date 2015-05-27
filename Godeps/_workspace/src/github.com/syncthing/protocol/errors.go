// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"errors"
)

const (
	ecNoError int32 = iota
	ecGeneric
	ecNoSuchFile
	ecInvalid
)

var (
	ErrNoError    error = nil
	ErrGeneric          = errors.New("generic error")
	ErrNoSuchFile       = errors.New("no such file")
	ErrInvalid          = errors.New("file is invalid")
)

var lookupError = map[int32]error{
	ecNoError:    ErrNoError,
	ecGeneric:    ErrGeneric,
	ecNoSuchFile: ErrNoSuchFile,
	ecInvalid:    ErrInvalid,
}

var lookupCode = map[error]int32{
	ErrNoError:    ecNoError,
	ErrGeneric:    ecGeneric,
	ErrNoSuchFile: ecNoSuchFile,
	ErrInvalid:    ecInvalid,
}

func codeToError(errcode int32) error {
	err, ok := lookupError[errcode]
	if !ok {
		return ErrGeneric
	}
	return err
}

func errorToCode(err error) int32 {
	code, ok := lookupCode[err]
	if !ok {
		return ecGeneric
	}
	return code
}
