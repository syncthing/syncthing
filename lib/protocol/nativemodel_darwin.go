// Copyright (C) 2014 The Protocol Authors.

//go:build darwin
// +build darwin

package protocol

// Darwin uses NFD normalization

import "golang.org/x/text/unicode/norm"

func makeNative(m contextLessModel) contextLessModel { return nativeModel{m} }

type nativeModel struct {
	contextLessModel
}

func (m nativeModel) Index(folder string, files []FileInfo) error {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	return m.contextLessModel.Index(folder, files)
}

func (m nativeModel) IndexUpdate(folder string, files []FileInfo) error {
	for i := range files {
		files[i].Name = norm.NFD.String(files[i].Name)
	}
	return m.contextLessModel.IndexUpdate(folder, files)
}

func (m nativeModel) Request(folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	name = norm.NFD.String(name)
	return m.contextLessModel.Request(folder, name, blockNo, size, offset, hash, weakHash, fromTemporary)
}
