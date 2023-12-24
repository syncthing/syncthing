package blob

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const timeOut = time.Minute

type S3Config struct {
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
}

func (s *S3Config) isSet() bool {
	return s.AccessKey != "" && s.SecretKey != "" && s.Bucket != "" && s.Endpoint != ""
}

type S3 struct {
	client *s3.S3
	bucket string
}

func NewS3(config S3Config) (*S3, error) {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
		Endpoint:         aws.String("https://" + config.Endpoint),
		Region:           aws.String(config.Region),
		S3ForcePathStyle: aws.Bool(false),
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		return nil, err
	}
	s3Client := s3.New(newSession)

	return &S3{client: s3Client, bucket: config.Bucket}, nil
}

func (s *S3) Put(key string, data []byte) error {
	uploader := s3manager.NewUploaderWithClient(s.client)

	// Upload the file.
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (s *S3) Get(key string) ([]byte, error) {
	downloader := s3manager.NewDownloaderWithClient(s.client)
	buf := aws.NewWriteAtBuffer([]byte{})

	// Download the file.
	_, err := downloader.Download(buf, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *S3) Delete(key string) error {
	// Delete the object.
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}

	// Wait until the object is deleted.
	err = s.client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	return err
}

func (s *S3) Iterate(ctx context.Context, prefix string, fn func([]byte) bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeOut)
	defer cancel()

	downloadManager := s3manager.NewDownloaderWithClient(s.client)

	// ListObjectsV2 only supports up to 1000 keys per response. A response
	// indicates whether the result was truncated and if so returns a
	// continuation token which can be used to collect the remaining items.
	var nextContinuationToken *string
	for {
		// Obtain a list of objects based on a prefix.
		resp, err := s.client.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: aws.String(prefix), ContinuationToken: nextContinuationToken})
		if err != nil {
			return err
		}

		if len(resp.Contents) == 0 {
			// No keys were returned.
			return nil
		}

		// Convert the objects to a BatchDownloadObject.
		batch := make([]s3manager.BatchDownloadObject, *resp.KeyCount)
		for i, item := range resp.Contents {
			batch[i] = s3manager.BatchDownloadObject{
				Object: &s3.GetObjectInput{
					Bucket: aws.String(s.bucket),
					Key:    aws.String(*item.Key),
				},
				Writer: aws.NewWriteAtBuffer([]byte{}),
			}
		}

		// Download the requested items in a batch.
		err = downloadManager.DownloadWithIterator(ctx, &s3manager.DownloadObjectsIterator{Objects: batch})
		if err != nil {
			return err
		}

		for _, item := range batch {
			// Read the item's buffer.
			b, ok := item.Writer.(*aws.WriteAtBuffer)
			if !ok {
				continue
			}

			if !fn(b.Bytes()) {
				break
			}
		}

		if resp.IsTruncated != nil && *resp.IsTruncated {
			if resp.NextContinuationToken == nil || *resp.NextContinuationToken == "" {
				return errors.New("response was truncated but no continuation token was supplied")
			}

			nextContinuationToken = resp.NextContinuationToken
			continue
		}
		return nil
	}
}

func (s *S3) Count(prefix string) (int, error) {
	var total = 0

	// ListObjectsV2 only supports up to 1000 keys per response. A response
	// indicates whether the result was truncated and if so returns a
	// continuation token which can be used to collect the remaining items.
	var nextContinuationToken *string
	for {
		// Obtain a list of objects based on a prefix.
		resp, err := s.client.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: aws.String(prefix), ContinuationToken: nextContinuationToken})
		if err != nil {
			return 0, err
		}
		total += len(resp.Contents)
		if resp.IsTruncated != nil && *resp.IsTruncated {
			if resp.NextContinuationToken == nil || *resp.NextContinuationToken == "" {
				return 0, errors.New("response was truncated but no continuation token was supplied")
			}

			nextContinuationToken = resp.NextContinuationToken
			continue
		}
		break
	}

	return total, nil
}
