// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

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
	switch {
	case err == nil:
		return ErrorCodeNoError
	case errors.Is(err, ErrNoSuchFile):
		return ErrorCodeNoSuchFile
	case errors.Is(err, ErrInvalid):
		return ErrorCodeInvalidFile
	default:
		return ErrorCodeGeneric
	}
}
