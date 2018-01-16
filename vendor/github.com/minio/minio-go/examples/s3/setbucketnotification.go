// +build ignore

/*
 * Minio Go Library for Amazon S3 Compatible Cloud Storage
 * Copyright 2015-2017 Minio, Inc.
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

package main

import (
	"log"

	"github.com/minio/minio-go"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY and my-bucketname are
	// dummy values, please replace them with original values.

	// Requests are always secure (HTTPS) by default. Set secure=false to enable insecure (HTTP) access.
	// This boolean value is the last argument for New().

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	s3Client, err := minio.New("s3.amazonaws.com", "YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", true)
	if err != nil {
		log.Fatalln(err)
	}

	// s3Client.TraceOn(os.Stderr)

	// ARN represents a notification channel that needs to be created in your S3 provider
	//  (e.g. http://docs.aws.amazon.com/sns/latest/dg/CreateTopic.html)

	// An example of an ARN:
	//             arn:aws:sns:us-east-1:804064459714:UploadPhoto
	//                  ^   ^     ^           ^          ^
	//       Provider __|   |     |           |          |
	//                      |   Region    Account ID     |_ Notification Name
	//             Service _|
	//
	// You should replace YOUR-PROVIDER, YOUR-SERVICE, YOUR-REGION, YOUR-ACCOUNT-ID and YOUR-RESOURCE
	// with actual values that you receive from the S3 provider

	// Here you create a new Topic notification
	topicArn := minio.NewArn("YOUR-PROVIDER", "YOUR-SERVICE", "YOUR-REGION", "YOUR-ACCOUNT-ID", "YOUR-RESOURCE")
	topicConfig := minio.NewNotificationConfig(topicArn)
	topicConfig.AddEvents(minio.ObjectCreatedAll, minio.ObjectRemovedAll)
	topicConfig.AddFilterPrefix("photos/")
	topicConfig.AddFilterSuffix(".jpg")

	// Create a new Queue notification
	queueArn := minio.NewArn("YOUR-PROVIDER", "YOUR-SERVICE", "YOUR-REGION", "YOUR-ACCOUNT-ID", "YOUR-RESOURCE")
	queueConfig := minio.NewNotificationConfig(queueArn)
	queueConfig.AddEvents(minio.ObjectRemovedAll)

	// Create a new Lambda (CloudFunction)
	lambdaArn := minio.NewArn("YOUR-PROVIDER", "YOUR-SERVICE", "YOUR-REGION", "YOUR-ACCOUNT-ID", "YOUR-RESOURCE")
	lambdaConfig := minio.NewNotificationConfig(lambdaArn)
	lambdaConfig.AddEvents(minio.ObjectRemovedAll)
	lambdaConfig.AddFilterSuffix(".swp")

	// Now, set all previously created notification configs
	bucketNotification := minio.BucketNotification{}
	bucketNotification.AddTopic(topicConfig)
	bucketNotification.AddQueue(queueConfig)
	bucketNotification.AddLambda(lambdaConfig)

	err = s3Client.SetBucketNotification("YOUR-BUCKET", bucketNotification)
	if err != nil {
		log.Fatalln("Error: " + err.Error())
	}
	log.Println("Success")
}
