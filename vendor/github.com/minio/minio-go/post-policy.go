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
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// expirationDateFormat date format for expiration key in json policy.
const expirationDateFormat = "2006-01-02T15:04:05.999Z"

// policyCondition explanation:
// http://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-HTTPPOSTConstructPolicy.html
//
// Example:
//
//   policyCondition {
//       matchType: "$eq",
//       key: "$Content-Type",
//       value: "image/png",
//   }
//
type policyCondition struct {
	matchType string
	condition string
	value     string
}

// PostPolicy - Provides strict static type conversion and validation
// for Amazon S3's POST policy JSON string.
type PostPolicy struct {
	// Expiration date and time of the POST policy.
	expiration time.Time
	// Collection of different policy conditions.
	conditions []policyCondition
	// ContentLengthRange minimum and maximum allowable size for the
	// uploaded content.
	contentLengthRange struct {
		min int64
		max int64
	}

	// Post form data.
	formData map[string]string
}

// NewPostPolicy - Instantiate new post policy.
func NewPostPolicy() *PostPolicy {
	p := &PostPolicy{}
	p.conditions = make([]policyCondition, 0)
	p.formData = make(map[string]string)
	return p
}

// SetExpires - Sets expiration time for the new policy.
func (p *PostPolicy) SetExpires(t time.Time) error {
	if t.IsZero() {
		return ErrInvalidArgument("No expiry time set.")
	}
	p.expiration = t
	return nil
}

// SetKey - Sets an object name for the policy based upload.
func (p *PostPolicy) SetKey(key string) error {
	if strings.TrimSpace(key) == "" || key == "" {
		return ErrInvalidArgument("Object name is empty.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$key",
		value:     key,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["key"] = key
	return nil
}

// SetKeyStartsWith - Sets an object name that an policy based upload
// can start with.
func (p *PostPolicy) SetKeyStartsWith(keyStartsWith string) error {
	if strings.TrimSpace(keyStartsWith) == "" || keyStartsWith == "" {
		return ErrInvalidArgument("Object prefix is empty.")
	}
	policyCond := policyCondition{
		matchType: "starts-with",
		condition: "$key",
		value:     keyStartsWith,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["key"] = keyStartsWith
	return nil
}

// SetBucket - Sets bucket at which objects will be uploaded to.
func (p *PostPolicy) SetBucket(bucketName string) error {
	if strings.TrimSpace(bucketName) == "" || bucketName == "" {
		return ErrInvalidArgument("Bucket name is empty.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$bucket",
		value:     bucketName,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["bucket"] = bucketName
	return nil
}

// SetContentType - Sets content-type of the object for this policy
// based upload.
func (p *PostPolicy) SetContentType(contentType string) error {
	if strings.TrimSpace(contentType) == "" || contentType == "" {
		return ErrInvalidArgument("No content type specified.")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$Content-Type",
		value:     contentType,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["Content-Type"] = contentType
	return nil
}

// SetContentLengthRange - Set new min and max content length
// condition for all incoming uploads.
func (p *PostPolicy) SetContentLengthRange(min, max int64) error {
	if min > max {
		return ErrInvalidArgument("Minimum limit is larger than maximum limit.")
	}
	if min < 0 {
		return ErrInvalidArgument("Minimum limit cannot be negative.")
	}
	if max < 0 {
		return ErrInvalidArgument("Maximum limit cannot be negative.")
	}
	p.contentLengthRange.min = min
	p.contentLengthRange.max = max
	return nil
}

// SetSuccessStatusAction - Sets the status success code of the object for this policy
// based upload.
func (p *PostPolicy) SetSuccessStatusAction(status string) error {
	if strings.TrimSpace(status) == "" || status == "" {
		return ErrInvalidArgument("Status is empty")
	}
	policyCond := policyCondition{
		matchType: "eq",
		condition: "$success_action_status",
		value:     status,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData["success_action_status"] = status
	return nil
}

// SetUserMetadata - Set user metadata as a key/value couple.
// Can be retrieved through a HEAD request or an event.
func (p *PostPolicy) SetUserMetadata(key string, value string) error {
	if strings.TrimSpace(key) == "" || key == "" {
		return ErrInvalidArgument("Key is empty")
	}
	if strings.TrimSpace(value) == "" || value == "" {
		return ErrInvalidArgument("Value is empty")
	}
	headerName := fmt.Sprintf("x-amz-meta-%s", key)
	policyCond := policyCondition{
		matchType: "eq",
		condition: fmt.Sprintf("$%s", headerName),
		value:     value,
	}
	if err := p.addNewPolicy(policyCond); err != nil {
		return err
	}
	p.formData[headerName] = value
	return nil
}

// addNewPolicy - internal helper to validate adding new policies.
func (p *PostPolicy) addNewPolicy(policyCond policyCondition) error {
	if policyCond.matchType == "" || policyCond.condition == "" || policyCond.value == "" {
		return ErrInvalidArgument("Policy fields are empty.")
	}
	p.conditions = append(p.conditions, policyCond)
	return nil
}

// Stringer interface for printing policy in json formatted string.
func (p PostPolicy) String() string {
	return string(p.marshalJSON())
}

// marshalJSON - Provides Marshalled JSON in bytes.
func (p PostPolicy) marshalJSON() []byte {
	expirationStr := `"expiration":"` + p.expiration.Format(expirationDateFormat) + `"`
	var conditionsStr string
	conditions := []string{}
	for _, po := range p.conditions {
		conditions = append(conditions, fmt.Sprintf("[\"%s\",\"%s\",\"%s\"]", po.matchType, po.condition, po.value))
	}
	if p.contentLengthRange.min != 0 || p.contentLengthRange.max != 0 {
		conditions = append(conditions, fmt.Sprintf("[\"content-length-range\", %d, %d]",
			p.contentLengthRange.min, p.contentLengthRange.max))
	}
	if len(conditions) > 0 {
		conditionsStr = `"conditions":[` + strings.Join(conditions, ",") + "]"
	}
	retStr := "{"
	retStr = retStr + expirationStr + ","
	retStr = retStr + conditionsStr
	retStr = retStr + "}"
	return []byte(retStr)
}

// base64 - Produces base64 of PostPolicy's Marshalled json.
func (p PostPolicy) base64() string {
	return base64.StdEncoding.EncodeToString(p.marshalJSON())
}
