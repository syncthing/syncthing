/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2017 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package minio

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/pkg/s3utils"
)

// GetBucketNotification - get bucket notification at a given path.
func (c Client) GetBucketNotification(bucketName string) (bucketNotification BucketNotification, err error) {
	// Input validation.
	if err := s3utils.CheckValidBucketName(bucketName); err != nil {
		return BucketNotification{}, err
	}
	notification, err := c.getBucketNotification(bucketName)
	if err != nil {
		return BucketNotification{}, err
	}
	return notification, nil
}

// Request server for notification rules.
func (c Client) getBucketNotification(bucketName string) (BucketNotification, error) {
	urlValues := make(url.Values)
	urlValues.Set("notification", "")

	// Execute GET on bucket to list objects.
	resp, err := c.executeMethod(context.Background(), "GET", requestMetadata{
		bucketName:       bucketName,
		queryValues:      urlValues,
		contentSHA256Hex: emptySHA256Hex,
	})

	defer closeResponse(resp)
	if err != nil {
		return BucketNotification{}, err
	}
	return processBucketNotificationResponse(bucketName, resp)

}

// processes the GetNotification http response from the server.
func processBucketNotificationResponse(bucketName string, resp *http.Response) (BucketNotification, error) {
	if resp.StatusCode != http.StatusOK {
		errResponse := httpRespToErrorResponse(resp, bucketName, "")
		return BucketNotification{}, errResponse
	}
	var bucketNotification BucketNotification
	err := xmlDecoder(resp.Body, &bucketNotification)
	if err != nil {
		return BucketNotification{}, err
	}
	return bucketNotification, nil
}

// Indentity represents the user id, this is a compliance field.
type identity struct {
	PrincipalID string `json:"principalId"`
}

// Notification event bucket metadata.
type bucketMeta struct {
	Name          string   `json:"name"`
	OwnerIdentity identity `json:"ownerIdentity"`
	ARN           string   `json:"arn"`
}

// Notification event object metadata.
type objectMeta struct {
	Key       string `json:"key"`
	Size      int64  `json:"size,omitempty"`
	ETag      string `json:"eTag,omitempty"`
	VersionID string `json:"versionId,omitempty"`
	Sequencer string `json:"sequencer"`
}

// Notification event server specific metadata.
type eventMeta struct {
	SchemaVersion   string     `json:"s3SchemaVersion"`
	ConfigurationID string     `json:"configurationId"`
	Bucket          bucketMeta `json:"bucket"`
	Object          objectMeta `json:"object"`
}

// sourceInfo represents information on the client that
// triggered the event notification.
type sourceInfo struct {
	Host      string `json:"host"`
	Port      string `json:"port"`
	UserAgent string `json:"userAgent"`
}

// NotificationEvent represents an Amazon an S3 bucket notification event.
type NotificationEvent struct {
	EventVersion      string            `json:"eventVersion"`
	EventSource       string            `json:"eventSource"`
	AwsRegion         string            `json:"awsRegion"`
	EventTime         string            `json:"eventTime"`
	EventName         string            `json:"eventName"`
	UserIdentity      identity          `json:"userIdentity"`
	RequestParameters map[string]string `json:"requestParameters"`
	ResponseElements  map[string]string `json:"responseElements"`
	S3                eventMeta         `json:"s3"`
	Source            sourceInfo        `json:"source"`
}

// NotificationInfo - represents the collection of notification events, additionally
// also reports errors if any while listening on bucket notifications.
type NotificationInfo struct {
	Records []NotificationEvent
	Err     error
}

// ListenBucketNotification - listen on bucket notifications.
func (c Client) ListenBucketNotification(bucketName, prefix, suffix string, events []string, doneCh <-chan struct{}) <-chan NotificationInfo {
	notificationInfoCh := make(chan NotificationInfo, 1)
	// Only success, start a routine to start reading line by line.
	go func(notificationInfoCh chan<- NotificationInfo) {
		defer close(notificationInfoCh)

		// Validate the bucket name.
		if err := s3utils.CheckValidBucketName(bucketName); err != nil {
			notificationInfoCh <- NotificationInfo{
				Err: err,
			}
			return
		}

		// Check ARN partition to verify if listening bucket is supported
		if s3utils.IsAmazonEndpoint(c.endpointURL) || s3utils.IsGoogleEndpoint(c.endpointURL) {
			notificationInfoCh <- NotificationInfo{
				Err: ErrAPINotSupported("Listening for bucket notification is specific only to `minio` server endpoints"),
			}
			return
		}

		// Continuously run and listen on bucket notification.
		// Create a done channel to control 'ListObjects' go routine.
		retryDoneCh := make(chan struct{}, 1)

		// Indicate to our routine to exit cleanly upon return.
		defer close(retryDoneCh)

		// Wait on the jitter retry loop.
		for range c.newRetryTimerContinous(time.Second, time.Second*30, MaxJitter, retryDoneCh) {
			urlValues := make(url.Values)
			urlValues.Set("prefix", prefix)
			urlValues.Set("suffix", suffix)
			urlValues["events"] = events

			// Execute GET on bucket to list objects.
			resp, err := c.executeMethod(context.Background(), "GET", requestMetadata{
				bucketName:       bucketName,
				queryValues:      urlValues,
				contentSHA256Hex: emptySHA256Hex,
			})
			if err != nil {
				notificationInfoCh <- NotificationInfo{
					Err: err,
				}
				return
			}

			// Validate http response, upon error return quickly.
			if resp.StatusCode != http.StatusOK {
				errResponse := httpRespToErrorResponse(resp, bucketName, "")
				notificationInfoCh <- NotificationInfo{
					Err: errResponse,
				}
				return
			}

			// Initialize a new bufio scanner, to read line by line.
			bio := bufio.NewScanner(resp.Body)

			// Close the response body.
			defer resp.Body.Close()

			// Unmarshal each line, returns marshalled values.
			for bio.Scan() {
				var notificationInfo NotificationInfo
				if err = json.Unmarshal(bio.Bytes(), &notificationInfo); err != nil {
					continue
				}
				// Send notifications on channel only if there are events received.
				if len(notificationInfo.Records) > 0 {
					select {
					case notificationInfoCh <- notificationInfo:
					case <-doneCh:
						return
					}
				}
			}
			// Look for any underlying errors.
			if err = bio.Err(); err != nil {
				// For an unexpected connection drop from server, we close the body
				// and re-connect.
				if err == io.ErrUnexpectedEOF {
					resp.Body.Close()
				}
			}
		}
	}(notificationInfoCh)

	// Returns the notification info channel, for caller to start reading from.
	return notificationInfoCh
}
