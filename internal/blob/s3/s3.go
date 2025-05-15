// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package s3

import (
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/syncthing/syncthing/internal/blob"
)

var _ blob.Store = (*Session)(nil)

type Session struct {
	bucket string
	s3sess *session.Session
}

type Object = s3.Object

func NewSession(endpoint, region, bucket, accessKeyID, secretKey string) (*Session, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(region),
		Endpoint:         aws.String(endpoint),
		Credentials:      credentials.NewStaticCredentials(accessKeyID, secretKey, ""),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	return &Session{
		bucket: bucket,
		s3sess: sess,
	}, nil
}

func (s *Session) Upload(_ context.Context, key string, r io.Reader) error {
	uploader := s3manager.NewUploader(s.s3sess)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

func (s *Session) Download(_ context.Context, key string, w blob.Writer) error {
	downloader := s3manager.NewDownloader(s.s3sess)
	_, err := downloader.Download(w, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *Session) LatestKey(_ context.Context) (string, error) {
	var latestKey string
	var lastModified time.Time
	if err := s.list(func(obj *Object) bool {
		if latestKey == "" || obj.LastModified.After(lastModified) {
			latestKey = *obj.Key
			lastModified = *obj.LastModified
		}
		return true
	}); err != nil {
		return "", err
	}
	return latestKey, nil
}

func (s *Session) list(fn func(*Object) bool) error {
	svc := s3.New(s.s3sess)

	opts := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	}
	for {
		resp, err := svc.ListObjectsV2(opts)
		if err != nil {
			return err
		}

		for _, item := range resp.Contents {
			if !fn(item) {
				return nil
			}
		}

		if resp.NextContinuationToken == nil || *resp.NextContinuationToken == "" {
			break
		}
		opts.ContinuationToken = resp.NextContinuationToken
	}

	return nil
}
