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
	err := hm.bucket.WriteAll(hm.ctx, hashToStringMapKey(hash), data, nil)
	if err != nil {
		log.Fatal(err)
		panic("writing to block storage failed!")
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
