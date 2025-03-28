// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import "github.com/syncthing/syncthing/internal/gen/bep"

type ErrorCode = bep.ErrorCode

const (
	ErrorCodeNoError     = bep.ErrorCode_ERROR_CODE_NO_ERROR
	ErrorCodeGeneric     = bep.ErrorCode_ERROR_CODE_GENERIC
	ErrorCodeNoSuchFile  = bep.ErrorCode_ERROR_CODE_NO_SUCH_FILE
	ErrorCodeInvalidFile = bep.ErrorCode_ERROR_CODE_INVALID_FILE
)

type Request struct {
	ID            int
	Folder        string
	Name          string
	Offset        int64
	Size          int
	Hash          []byte
	FromTemporary bool
	WeakHash      uint32
	BlockNo       int
}

func (r *Request) toWire() *bep.Request {
	return &bep.Request{
		Id:            int32(r.ID),
		Folder:        r.Folder,
		Name:          r.Name,
		Offset:        r.Offset,
		Size:          int32(r.Size),
		Hash:          r.Hash,
		FromTemporary: r.FromTemporary,
		WeakHash:      r.WeakHash,
		BlockNo:       int32(r.BlockNo),
	}
}

func requestFromWire(w *bep.Request) *Request {
	return &Request{
		ID:            int(w.Id),
		Folder:        w.Folder,
		Name:          w.Name,
		Offset:        w.Offset,
		Size:          int(w.Size),
		Hash:          w.Hash,
		FromTemporary: w.FromTemporary,
		WeakHash:      w.WeakHash,
		BlockNo:       int(w.BlockNo),
	}
}

type Response struct {
	ID   int
	Data []byte
	Code ErrorCode
}

func (r *Response) toWire() *bep.Response {
	return &bep.Response{
		Id:   int32(r.ID),
		Data: r.Data,
		Code: r.Code,
	}
}

func responseFromWire(w *bep.Response) *Response {
	return &Response{
		ID:   int(w.Id),
		Data: w.Data,
		Code: w.Code,
	}
}
