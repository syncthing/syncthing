// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package blockstorage

import (
	"context"
	"fmt"
	"io"
	"log"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/memblob"
	_ "gocloud.dev/blob/s3blob"
)

type GoCloudUrlStorage struct {
	io.Closer

	ctx    context.Context
	bucket *blob.Bucket
}

func NewGoCloudUrlStorage(ctx context.Context, url string) *GoCloudUrlStorage {
	bucket, err := blob.OpenBucket(context.Background(), url)
	if err != nil {
		log.Fatal(err)
	}

	if err := bucket.WriteAll(ctx, "foo.txt", []byte("Go Cloud Development Kit"), nil); err != nil {
		log.Fatal(err)
	}
	b, err := bucket.ReadAll(ctx, "foo.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))

	return &GoCloudUrlStorage{
		ctx:    ctx,
		bucket: bucket,
	}
}

func (hm *GoCloudUrlStorage) Get(hash []byte) (data []byte, ok bool) {
	if len(hash) == 0 {
		return nil, false
	}

	data, err := hm.bucket.ReadAll(hm.ctx, hashToStringMapKey(hash))
	if gcerrors.Code(err) == gcerrors.NotFound {
		return nil, false
	}

	if err != nil {
		log.Fatal(err)
		panic("failed to get block from block storage")
	}

	return data, true
}

func (hm *GoCloudUrlStorage) Set(hash []byte, data []byte) {
	stringKey := hashToStringMapKey(hash)
	//existsAlready, err := hm.bucket.Exists(hm.ctx, stringKey)
	//if err != nil {
	//	log.Fatal(err)
	//	panic("writing to block storage failed! Pre-Check.")
	//}
	//if existsAlready {
	//	return // skip upload
	//}
	err := hm.bucket.WriteAll(hm.ctx, stringKey, data, nil)
	if err != nil {
		log.Fatal(err)
		panic("writing to block storage failed! Write.")
	}
}

func (hm *GoCloudUrlStorage) Delete(hash []byte) {
	err := hm.bucket.Delete(hm.ctx, hashToStringMapKey(hash))
	if err != nil {
		log.Fatal(err)
		panic("writing to block storage failed!")
	}
}

func (hm *GoCloudUrlStorage) Close() {
	hm.bucket.Close()
}
