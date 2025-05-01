// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package azureblob

import (
	"context"
	"io"
	"time"

	stblob "github.com/syncthing/syncthing/internal/blob"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

var _ stblob.Store = (*BlobStore)(nil)

type BlobStore struct {
	client    *azblob.Client
	container string
}

func NewBlobStore(accountName, accountKey, containerName string) (*BlobStore, error) {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}
	url := "https://" + accountName + ".blob.core.windows.net/"
	sc, err := azblob.NewClientWithSharedKeyCredential(url, credential, &azblob.ClientOptions{})
	if err != nil {
		return nil, err
	}
	// This errors when the container already exists, which we ignore.
	_, _ = sc.CreateContainer(context.Background(), containerName, &container.CreateOptions{})
	return &BlobStore{
		client:    sc,
		container: containerName,
	}, nil
}

func (a *BlobStore) Upload(ctx context.Context, key string, data io.Reader) error {
	_, err := a.client.UploadStream(ctx, a.container, key, data, &blockblob.UploadStreamOptions{})
	return err
}

func (a *BlobStore) Download(ctx context.Context, key string, w stblob.Writer) error {
	resp, err := a.client.DownloadStream(ctx, a.container, key, &blob.DownloadStreamOptions{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

func (a *BlobStore) LatestKey(ctx context.Context) (string, error) {
	opts := &azblob.ListBlobsFlatOptions{}
	pager := a.client.NewListBlobsFlatPager(a.container, opts)
	var latest string
	var lastModified time.Time
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, blob := range page.Segment.BlobItems {
			if latest == "" || blob.Properties.LastModified.After(lastModified) {
				latest = *blob.Name
				lastModified = *blob.Properties.LastModified
			}
		}
	}
	return latest, nil
}
