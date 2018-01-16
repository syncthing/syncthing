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

package minio

import (
	"encoding/xml"
	"reflect"
)

// NotificationEventType is a S3 notification event associated to the bucket notification configuration
type NotificationEventType string

// The role of all event types are described in :
// 	http://docs.aws.amazon.com/AmazonS3/latest/dev/NotificationHowTo.html#notification-how-to-event-types-and-destinations
const (
	ObjectCreatedAll                     NotificationEventType = "s3:ObjectCreated:*"
	ObjectCreatedPut                                           = "s3:ObjectCreated:Put"
	ObjectCreatedPost                                          = "s3:ObjectCreated:Post"
	ObjectCreatedCopy                                          = "s3:ObjectCreated:Copy"
	ObjectCreatedCompleteMultipartUpload                       = "s3:ObjectCreated:CompleteMultipartUpload"
	ObjectAccessedGet                                          = "s3:ObjectAccessed:Get"
	ObjectAccessedHead                                         = "s3:ObjectAccessed:Head"
	ObjectAccessedAll                                          = "s3:ObjectAccessed:*"
	ObjectRemovedAll                                           = "s3:ObjectRemoved:*"
	ObjectRemovedDelete                                        = "s3:ObjectRemoved:Delete"
	ObjectRemovedDeleteMarkerCreated                           = "s3:ObjectRemoved:DeleteMarkerCreated"
	ObjectReducedRedundancyLostObject                          = "s3:ReducedRedundancyLostObject"
)

// FilterRule - child of S3Key, a tag in the notification xml which
// carries suffix/prefix filters
type FilterRule struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

// S3Key - child of Filter, a tag in the notification xml which
// carries suffix/prefix filters
type S3Key struct {
	FilterRules []FilterRule `xml:"FilterRule,omitempty"`
}

// Filter - a tag in the notification xml structure which carries
// suffix/prefix filters
type Filter struct {
	S3Key S3Key `xml:"S3Key,omitempty"`
}

// Arn - holds ARN information that will be sent to the web service,
// ARN desciption can be found in http://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html
type Arn struct {
	Partition string
	Service   string
	Region    string
	AccountID string
	Resource  string
}

// NewArn creates new ARN based on the given partition, service, region, account id and resource
func NewArn(partition, service, region, accountID, resource string) Arn {
	return Arn{Partition: partition,
		Service:   service,
		Region:    region,
		AccountID: accountID,
		Resource:  resource}
}

// Return the string format of the ARN
func (arn Arn) String() string {
	return "arn:" + arn.Partition + ":" + arn.Service + ":" + arn.Region + ":" + arn.AccountID + ":" + arn.Resource
}

// NotificationConfig - represents one single notification configuration
// such as topic, queue or lambda configuration.
type NotificationConfig struct {
	ID     string                  `xml:"Id,omitempty"`
	Arn    Arn                     `xml:"-"`
	Events []NotificationEventType `xml:"Event"`
	Filter *Filter                 `xml:"Filter,omitempty"`
}

// NewNotificationConfig creates one notification config and sets the given ARN
func NewNotificationConfig(arn Arn) NotificationConfig {
	return NotificationConfig{Arn: arn}
}

// AddEvents adds one event to the current notification config
func (t *NotificationConfig) AddEvents(events ...NotificationEventType) {
	t.Events = append(t.Events, events...)
}

// AddFilterSuffix sets the suffix configuration to the current notification config
func (t *NotificationConfig) AddFilterSuffix(suffix string) {
	if t.Filter == nil {
		t.Filter = &Filter{}
	}
	newFilterRule := FilterRule{Name: "suffix", Value: suffix}
	// Replace any suffix rule if existing and add to the list otherwise
	for index := range t.Filter.S3Key.FilterRules {
		if t.Filter.S3Key.FilterRules[index].Name == "suffix" {
			t.Filter.S3Key.FilterRules[index] = newFilterRule
			return
		}
	}
	t.Filter.S3Key.FilterRules = append(t.Filter.S3Key.FilterRules, newFilterRule)
}

// AddFilterPrefix sets the prefix configuration to the current notification config
func (t *NotificationConfig) AddFilterPrefix(prefix string) {
	if t.Filter == nil {
		t.Filter = &Filter{}
	}
	newFilterRule := FilterRule{Name: "prefix", Value: prefix}
	// Replace any prefix rule if existing and add to the list otherwise
	for index := range t.Filter.S3Key.FilterRules {
		if t.Filter.S3Key.FilterRules[index].Name == "prefix" {
			t.Filter.S3Key.FilterRules[index] = newFilterRule
			return
		}
	}
	t.Filter.S3Key.FilterRules = append(t.Filter.S3Key.FilterRules, newFilterRule)
}

// TopicConfig carries one single topic notification configuration
type TopicConfig struct {
	NotificationConfig
	Topic string `xml:"Topic"`
}

// QueueConfig carries one single queue notification configuration
type QueueConfig struct {
	NotificationConfig
	Queue string `xml:"Queue"`
}

// LambdaConfig carries one single cloudfunction notification configuration
type LambdaConfig struct {
	NotificationConfig
	Lambda string `xml:"CloudFunction"`
}

// BucketNotification - the struct that represents the whole XML to be sent to the web service
type BucketNotification struct {
	XMLName       xml.Name       `xml:"NotificationConfiguration"`
	LambdaConfigs []LambdaConfig `xml:"CloudFunctionConfiguration"`
	TopicConfigs  []TopicConfig  `xml:"TopicConfiguration"`
	QueueConfigs  []QueueConfig  `xml:"QueueConfiguration"`
}

// AddTopic adds a given topic config to the general bucket notification config
func (b *BucketNotification) AddTopic(topicConfig NotificationConfig) {
	newTopicConfig := TopicConfig{NotificationConfig: topicConfig, Topic: topicConfig.Arn.String()}
	for _, n := range b.TopicConfigs {
		if reflect.DeepEqual(n, newTopicConfig) {
			// Avoid adding duplicated entry
			return
		}
	}
	b.TopicConfigs = append(b.TopicConfigs, newTopicConfig)
}

// AddQueue adds a given queue config to the general bucket notification config
func (b *BucketNotification) AddQueue(queueConfig NotificationConfig) {
	newQueueConfig := QueueConfig{NotificationConfig: queueConfig, Queue: queueConfig.Arn.String()}
	for _, n := range b.QueueConfigs {
		if reflect.DeepEqual(n, newQueueConfig) {
			// Avoid adding duplicated entry
			return
		}
	}
	b.QueueConfigs = append(b.QueueConfigs, newQueueConfig)
}

// AddLambda adds a given lambda config to the general bucket notification config
func (b *BucketNotification) AddLambda(lambdaConfig NotificationConfig) {
	newLambdaConfig := LambdaConfig{NotificationConfig: lambdaConfig, Lambda: lambdaConfig.Arn.String()}
	for _, n := range b.LambdaConfigs {
		if reflect.DeepEqual(n, newLambdaConfig) {
			// Avoid adding duplicated entry
			return
		}
	}
	b.LambdaConfigs = append(b.LambdaConfigs, newLambdaConfig)
}

// RemoveTopicByArn removes all topic configurations that match the exact specified ARN
func (b *BucketNotification) RemoveTopicByArn(arn Arn) {
	var topics []TopicConfig
	for _, topic := range b.TopicConfigs {
		if topic.Topic != arn.String() {
			topics = append(topics, topic)
		}
	}
	b.TopicConfigs = topics
}

// RemoveQueueByArn removes all queue configurations that match the exact specified ARN
func (b *BucketNotification) RemoveQueueByArn(arn Arn) {
	var queues []QueueConfig
	for _, queue := range b.QueueConfigs {
		if queue.Queue != arn.String() {
			queues = append(queues, queue)
		}
	}
	b.QueueConfigs = queues
}

// RemoveLambdaByArn removes all lambda configurations that match the exact specified ARN
func (b *BucketNotification) RemoveLambdaByArn(arn Arn) {
	var lambdas []LambdaConfig
	for _, lambda := range b.LambdaConfigs {
		if lambda.Lambda != arn.String() {
			lambdas = append(lambdas, lambda)
		}
	}
	b.LambdaConfigs = lambdas
}
