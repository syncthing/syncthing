// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build darwin
// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

func makeNative(m rawModel) rawModel { return nativeModel{m} }

type nativeModel struct {
	rawModel
}

func (m nativeModel) Index(idx *Index) error {
	for i := range idx.Files {
		idx.Files[i].Name = norm.NFD.String(idx.Files[i].Name)
	}
	return m.rawModel.Index(idx)
}

func (m nativeModel) IndexUpdate(idxUp *IndexUpdate) error {
	for i := range idxUp.Files {
		idxUp.Files[i].Name = norm.NFD.String(idxUp.Files[i].Name)
	}
	return m.rawModel.IndexUpdate(idxUp)
}

func (m nativeModel) Request(req *Request) (RequestResponse, error) {
	req.Name = norm.NFD.String(req.Name)
	return m.rawModel.Request(req)
}
