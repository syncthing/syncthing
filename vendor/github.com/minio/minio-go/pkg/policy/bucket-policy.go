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

package policy

import (
	"reflect"
	"strings"

	"github.com/minio/minio-go/pkg/set"
)

// BucketPolicy - Bucket level policy.
type BucketPolicy string

// Different types of Policies currently supported for buckets.
const (
	BucketPolicyNone      BucketPolicy = "none"
	BucketPolicyReadOnly               = "readonly"
	BucketPolicyReadWrite              = "readwrite"
	BucketPolicyWriteOnly              = "writeonly"
)

// IsValidBucketPolicy - returns true if policy is valid and supported, false otherwise.
func (p BucketPolicy) IsValidBucketPolicy() bool {
	switch p {
	case BucketPolicyNone, BucketPolicyReadOnly, BucketPolicyReadWrite, BucketPolicyWriteOnly:
		return true
	}
	return false
}

// Resource prefix for all aws resources.
const awsResourcePrefix = "arn:aws:s3:::"

// Common bucket actions for both read and write policies.
var commonBucketActions = set.CreateStringSet("s3:GetBucketLocation")

// Read only bucket actions.
var readOnlyBucketActions = set.CreateStringSet("s3:ListBucket")

// Write only bucket actions.
var writeOnlyBucketActions = set.CreateStringSet("s3:ListBucketMultipartUploads")

// Read only object actions.
var readOnlyObjectActions = set.CreateStringSet("s3:GetObject")

// Write only object actions.
var writeOnlyObjectActions = set.CreateStringSet("s3:AbortMultipartUpload", "s3:DeleteObject", "s3:ListMultipartUploadParts", "s3:PutObject")

// Read and write object actions.
var readWriteObjectActions = readOnlyObjectActions.Union(writeOnlyObjectActions)

// All valid bucket and object actions.
var validActions = commonBucketActions.
	Union(readOnlyBucketActions).
	Union(writeOnlyBucketActions).
	Union(readOnlyObjectActions).
	Union(writeOnlyObjectActions)

var startsWithFunc = func(resource string, resourcePrefix string) bool {
	return strings.HasPrefix(resource, resourcePrefix)
}

// User - canonical users list.
type User struct {
	AWS           set.StringSet `json:"AWS,omitempty"`
	CanonicalUser set.StringSet `json:"CanonicalUser,omitempty"`
}

// Statement - minio policy statement
type Statement struct {
	Actions    set.StringSet `json:"Action"`
	Conditions ConditionMap  `json:"Condition,omitempty"`
	Effect     string
	Principal  User          `json:"Principal"`
	Resources  set.StringSet `json:"Resource"`
	Sid        string
}

// BucketAccessPolicy - minio policy collection
type BucketAccessPolicy struct {
	Version    string      // date in YYYY-MM-DD format
	Statements []Statement `json:"Statement"`
}

// isValidStatement - returns whether given statement is valid to process for given bucket name.
func isValidStatement(statement Statement, bucketName string) bool {
	if statement.Actions.Intersection(validActions).IsEmpty() {
		return false
	}

	if statement.Effect != "Allow" {
		return false
	}

	if statement.Principal.AWS == nil || !statement.Principal.AWS.Contains("*") {
		return false
	}

	bucketResource := awsResourcePrefix + bucketName
	if statement.Resources.Contains(bucketResource) {
		return true
	}

	if statement.Resources.FuncMatch(startsWithFunc, bucketResource+"/").IsEmpty() {
		return false
	}

	return true
}

// Returns new statements with bucket actions for given policy.
func newBucketStatement(policy BucketPolicy, bucketName string, prefix string) (statements []Statement) {
	statements = []Statement{}
	if policy == BucketPolicyNone || bucketName == "" {
		return statements
	}

	bucketResource := set.CreateStringSet(awsResourcePrefix + bucketName)

	statement := Statement{
		Actions:   commonBucketActions,
		Effect:    "Allow",
		Principal: User{AWS: set.CreateStringSet("*")},
		Resources: bucketResource,
		Sid:       "",
	}
	statements = append(statements, statement)

	if policy == BucketPolicyReadOnly || policy == BucketPolicyReadWrite {
		statement = Statement{
			Actions:   readOnlyBucketActions,
			Effect:    "Allow",
			Principal: User{AWS: set.CreateStringSet("*")},
			Resources: bucketResource,
			Sid:       "",
		}
		if prefix != "" {
			condKeyMap := make(ConditionKeyMap)
			condKeyMap.Add("s3:prefix", set.CreateStringSet(prefix))
			condMap := make(ConditionMap)
			condMap.Add("StringEquals", condKeyMap)
			statement.Conditions = condMap
		}
		statements = append(statements, statement)
	}

	if policy == BucketPolicyWriteOnly || policy == BucketPolicyReadWrite {
		statement = Statement{
			Actions:   writeOnlyBucketActions,
			Effect:    "Allow",
			Principal: User{AWS: set.CreateStringSet("*")},
			Resources: bucketResource,
			Sid:       "",
		}
		statements = append(statements, statement)
	}

	return statements
}

// Returns new statements contains object actions for given policy.
func newObjectStatement(policy BucketPolicy, bucketName string, prefix string) (statements []Statement) {
	statements = []Statement{}
	if policy == BucketPolicyNone || bucketName == "" {
		return statements
	}

	statement := Statement{
		Effect:    "Allow",
		Principal: User{AWS: set.CreateStringSet("*")},
		Resources: set.CreateStringSet(awsResourcePrefix + bucketName + "/" + prefix + "*"),
		Sid:       "",
	}

	if policy == BucketPolicyReadOnly {
		statement.Actions = readOnlyObjectActions
	} else if policy == BucketPolicyWriteOnly {
		statement.Actions = writeOnlyObjectActions
	} else if policy == BucketPolicyReadWrite {
		statement.Actions = readWriteObjectActions
	}

	statements = append(statements, statement)
	return statements
}

// Returns new statements for given policy, bucket and prefix.
func newStatements(policy BucketPolicy, bucketName string, prefix string) (statements []Statement) {
	statements = []Statement{}
	ns := newBucketStatement(policy, bucketName, prefix)
	statements = append(statements, ns...)

	ns = newObjectStatement(policy, bucketName, prefix)
	statements = append(statements, ns...)

	return statements
}

// Returns whether given bucket statements are used by other than given prefix statements.
func getInUsePolicy(statements []Statement, bucketName string, prefix string) (readOnlyInUse, writeOnlyInUse bool) {
	resourcePrefix := awsResourcePrefix + bucketName + "/"
	objectResource := awsResourcePrefix + bucketName + "/" + prefix + "*"

	for _, s := range statements {
		if !s.Resources.Contains(objectResource) && !s.Resources.FuncMatch(startsWithFunc, resourcePrefix).IsEmpty() {
			if s.Actions.Intersection(readOnlyObjectActions).Equals(readOnlyObjectActions) {
				readOnlyInUse = true
			}

			if s.Actions.Intersection(writeOnlyObjectActions).Equals(writeOnlyObjectActions) {
				writeOnlyInUse = true
			}
		}
		if readOnlyInUse && writeOnlyInUse {
			break
		}
	}

	return readOnlyInUse, writeOnlyInUse
}

// Removes object actions in given statement.
func removeObjectActions(statement Statement, objectResource string) Statement {
	if statement.Conditions == nil {
		if len(statement.Resources) > 1 {
			statement.Resources.Remove(objectResource)
		} else {
			statement.Actions = statement.Actions.Difference(readOnlyObjectActions)
			statement.Actions = statement.Actions.Difference(writeOnlyObjectActions)
		}
	}

	return statement
}

// Removes bucket actions for given policy in given statement.
func removeBucketActions(statement Statement, prefix string, bucketResource string, readOnlyInUse, writeOnlyInUse bool) Statement {
	removeReadOnly := func() {
		if !statement.Actions.Intersection(readOnlyBucketActions).Equals(readOnlyBucketActions) {
			return
		}

		if statement.Conditions == nil {
			statement.Actions = statement.Actions.Difference(readOnlyBucketActions)
			return
		}

		if prefix != "" {
			stringEqualsValue := statement.Conditions["StringEquals"]
			values := set.NewStringSet()
			if stringEqualsValue != nil {
				values = stringEqualsValue["s3:prefix"]
				if values == nil {
					values = set.NewStringSet()
				}
			}

			values.Remove(prefix)

			if stringEqualsValue != nil {
				if values.IsEmpty() {
					delete(stringEqualsValue, "s3:prefix")
				}
				if len(stringEqualsValue) == 0 {
					delete(statement.Conditions, "StringEquals")
				}
			}

			if len(statement.Conditions) == 0 {
				statement.Conditions = nil
				statement.Actions = statement.Actions.Difference(readOnlyBucketActions)
			}
		}
	}

	removeWriteOnly := func() {
		if statement.Conditions == nil {
			statement.Actions = statement.Actions.Difference(writeOnlyBucketActions)
		}
	}

	if len(statement.Resources) > 1 {
		statement.Resources.Remove(bucketResource)
	} else {
		if !readOnlyInUse {
			removeReadOnly()
		}

		if !writeOnlyInUse {
			removeWriteOnly()
		}
	}

	return statement
}

// Returns statements containing removed actions/statements for given
// policy, bucket name and prefix.
func removeStatements(statements []Statement, bucketName string, prefix string) []Statement {
	bucketResource := awsResourcePrefix + bucketName
	objectResource := awsResourcePrefix + bucketName + "/" + prefix + "*"
	readOnlyInUse, writeOnlyInUse := getInUsePolicy(statements, bucketName, prefix)

	out := []Statement{}
	readOnlyBucketStatements := []Statement{}
	s3PrefixValues := set.NewStringSet()

	for _, statement := range statements {
		if !isValidStatement(statement, bucketName) {
			out = append(out, statement)
			continue
		}

		if statement.Resources.Contains(bucketResource) {
			if statement.Conditions != nil {
				statement = removeBucketActions(statement, prefix, bucketResource, false, false)
			} else {
				statement = removeBucketActions(statement, prefix, bucketResource, readOnlyInUse, writeOnlyInUse)
			}
		} else if statement.Resources.Contains(objectResource) {
			statement = removeObjectActions(statement, objectResource)
		}

		if !statement.Actions.IsEmpty() {
			if statement.Resources.Contains(bucketResource) &&
				statement.Actions.Intersection(readOnlyBucketActions).Equals(readOnlyBucketActions) &&
				statement.Effect == "Allow" &&
				statement.Principal.AWS.Contains("*") {

				if statement.Conditions != nil {
					stringEqualsValue := statement.Conditions["StringEquals"]
					values := set.NewStringSet()
					if stringEqualsValue != nil {
						values = stringEqualsValue["s3:prefix"]
						if values == nil {
							values = set.NewStringSet()
						}
					}
					s3PrefixValues = s3PrefixValues.Union(values.ApplyFunc(func(v string) string {
						return bucketResource + "/" + v + "*"
					}))
				} else if !s3PrefixValues.IsEmpty() {
					readOnlyBucketStatements = append(readOnlyBucketStatements, statement)
					continue
				}
			}
			out = append(out, statement)
		}
	}

	skipBucketStatement := true
	resourcePrefix := awsResourcePrefix + bucketName + "/"
	for _, statement := range out {
		if !statement.Resources.FuncMatch(startsWithFunc, resourcePrefix).IsEmpty() &&
			s3PrefixValues.Intersection(statement.Resources).IsEmpty() {
			skipBucketStatement = false
			break
		}
	}

	for _, statement := range readOnlyBucketStatements {
		if skipBucketStatement &&
			statement.Resources.Contains(bucketResource) &&
			statement.Effect == "Allow" &&
			statement.Principal.AWS.Contains("*") &&
			statement.Conditions == nil {
			continue
		}

		out = append(out, statement)
	}

	if len(out) == 1 {
		statement := out[0]
		if statement.Resources.Contains(bucketResource) &&
			statement.Actions.Intersection(commonBucketActions).Equals(commonBucketActions) &&
			statement.Effect == "Allow" &&
			statement.Principal.AWS.Contains("*") &&
			statement.Conditions == nil {
			out = []Statement{}
		}
	}

	return out
}

//  Appends given statement into statement list to have unique statements.
//  - If statement already exists in statement list, it ignores.
//  - If statement exists with different conditions, they are merged.
//  - Else the statement is appended to statement list.
func appendStatement(statements []Statement, statement Statement) []Statement {
	for i, s := range statements {
		if s.Actions.Equals(statement.Actions) &&
			s.Effect == statement.Effect &&
			s.Principal.AWS.Equals(statement.Principal.AWS) &&
			reflect.DeepEqual(s.Conditions, statement.Conditions) {
			statements[i].Resources = s.Resources.Union(statement.Resources)
			return statements
		} else if s.Resources.Equals(statement.Resources) &&
			s.Effect == statement.Effect &&
			s.Principal.AWS.Equals(statement.Principal.AWS) &&
			reflect.DeepEqual(s.Conditions, statement.Conditions) {
			statements[i].Actions = s.Actions.Union(statement.Actions)
			return statements
		}

		if s.Resources.Intersection(statement.Resources).Equals(statement.Resources) &&
			s.Actions.Intersection(statement.Actions).Equals(statement.Actions) &&
			s.Effect == statement.Effect &&
			s.Principal.AWS.Intersection(statement.Principal.AWS).Equals(statement.Principal.AWS) {
			if reflect.DeepEqual(s.Conditions, statement.Conditions) {
				return statements
			}
			if s.Conditions != nil && statement.Conditions != nil {
				if s.Resources.Equals(statement.Resources) {
					statements[i].Conditions = mergeConditionMap(s.Conditions, statement.Conditions)
					return statements
				}
			}
		}
	}

	if !(statement.Actions.IsEmpty() && statement.Resources.IsEmpty()) {
		return append(statements, statement)
	}

	return statements
}

// Appends two statement lists.
func appendStatements(statements []Statement, appendStatements []Statement) []Statement {
	for _, s := range appendStatements {
		statements = appendStatement(statements, s)
	}

	return statements
}

// Returns policy of given bucket statement.
func getBucketPolicy(statement Statement, prefix string) (commonFound, readOnly, writeOnly bool) {
	if !(statement.Effect == "Allow" && statement.Principal.AWS.Contains("*")) {
		return commonFound, readOnly, writeOnly
	}

	if statement.Actions.Intersection(commonBucketActions).Equals(commonBucketActions) &&
		statement.Conditions == nil {
		commonFound = true
	}

	if statement.Actions.Intersection(writeOnlyBucketActions).Equals(writeOnlyBucketActions) &&
		statement.Conditions == nil {
		writeOnly = true
	}

	if statement.Actions.Intersection(readOnlyBucketActions).Equals(readOnlyBucketActions) {
		if prefix != "" && statement.Conditions != nil {
			if stringEqualsValue, ok := statement.Conditions["StringEquals"]; ok {
				if s3PrefixValues, ok := stringEqualsValue["s3:prefix"]; ok {
					if s3PrefixValues.Contains(prefix) {
						readOnly = true
					}
				}
			} else if stringNotEqualsValue, ok := statement.Conditions["StringNotEquals"]; ok {
				if s3PrefixValues, ok := stringNotEqualsValue["s3:prefix"]; ok {
					if !s3PrefixValues.Contains(prefix) {
						readOnly = true
					}
				}
			}
		} else if prefix == "" && statement.Conditions == nil {
			readOnly = true
		} else if prefix != "" && statement.Conditions == nil {
			readOnly = true
		}
	}

	return commonFound, readOnly, writeOnly
}

// Returns policy of given object statement.
func getObjectPolicy(statement Statement) (readOnly bool, writeOnly bool) {
	if statement.Effect == "Allow" &&
		statement.Principal.AWS.Contains("*") &&
		statement.Conditions == nil {
		if statement.Actions.Intersection(readOnlyObjectActions).Equals(readOnlyObjectActions) {
			readOnly = true
		}
		if statement.Actions.Intersection(writeOnlyObjectActions).Equals(writeOnlyObjectActions) {
			writeOnly = true
		}
	}

	return readOnly, writeOnly
}

// GetPolicy - Returns policy of given bucket name, prefix in given statements.
func GetPolicy(statements []Statement, bucketName string, prefix string) BucketPolicy {
	bucketResource := awsResourcePrefix + bucketName
	objectResource := awsResourcePrefix + bucketName + "/" + prefix + "*"

	bucketCommonFound := false
	bucketReadOnly := false
	bucketWriteOnly := false
	matchedResource := ""
	objReadOnly := false
	objWriteOnly := false

	for _, s := range statements {
		matchedObjResources := set.NewStringSet()
		if s.Resources.Contains(objectResource) {
			matchedObjResources.Add(objectResource)
		} else {
			matchedObjResources = s.Resources.FuncMatch(resourceMatch, objectResource)
		}

		if !matchedObjResources.IsEmpty() {
			readOnly, writeOnly := getObjectPolicy(s)
			for resource := range matchedObjResources {
				if len(matchedResource) < len(resource) {
					objReadOnly = readOnly
					objWriteOnly = writeOnly
					matchedResource = resource
				} else if len(matchedResource) == len(resource) {
					objReadOnly = objReadOnly || readOnly
					objWriteOnly = objWriteOnly || writeOnly
					matchedResource = resource
				}
			}
		} else if s.Resources.Contains(bucketResource) {
			commonFound, readOnly, writeOnly := getBucketPolicy(s, prefix)
			bucketCommonFound = bucketCommonFound || commonFound
			bucketReadOnly = bucketReadOnly || readOnly
			bucketWriteOnly = bucketWriteOnly || writeOnly
		}
	}

	policy := BucketPolicyNone
	if bucketCommonFound {
		if bucketReadOnly && bucketWriteOnly && objReadOnly && objWriteOnly {
			policy = BucketPolicyReadWrite
		} else if bucketReadOnly && objReadOnly {
			policy = BucketPolicyReadOnly
		} else if bucketWriteOnly && objWriteOnly {
			policy = BucketPolicyWriteOnly
		}
	}

	return policy
}

// GetPolicies - returns a map of policies rules of given bucket name, prefix in given statements.
func GetPolicies(statements []Statement, bucketName string) map[string]BucketPolicy {
	policyRules := map[string]BucketPolicy{}
	objResources := set.NewStringSet()
	// Search all resources related to objects policy
	for _, s := range statements {
		for r := range s.Resources {
			if strings.HasPrefix(r, awsResourcePrefix+bucketName+"/") {
				objResources.Add(r)
			}
		}
	}
	// Pretend that policy resource as an actual object and fetch its policy
	for r := range objResources {
		// Put trailing * if exists in asterisk
		asterisk := ""
		if strings.HasSuffix(r, "*") {
			r = r[:len(r)-1]
			asterisk = "*"
		}
		objectPath := r[len(awsResourcePrefix+bucketName)+1:]
		p := GetPolicy(statements, bucketName, objectPath)
		policyRules[bucketName+"/"+objectPath+asterisk] = p
	}
	return policyRules
}

// SetPolicy - Returns new statements containing policy of given bucket name and prefix are appended.
func SetPolicy(statements []Statement, policy BucketPolicy, bucketName string, prefix string) []Statement {
	out := removeStatements(statements, bucketName, prefix)
	// fmt.Println("out = ")
	// printstatement(out)
	ns := newStatements(policy, bucketName, prefix)
	// fmt.Println("ns = ")
	// printstatement(ns)

	rv := appendStatements(out, ns)
	// fmt.Println("rv = ")
	// printstatement(rv)

	return rv
}

// Match function matches wild cards in 'pattern' for resource.
func resourceMatch(pattern, resource string) bool {
	if pattern == "" {
		return resource == pattern
	}
	if pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return resource == pattern
	}
	tGlob := strings.HasSuffix(pattern, "*")
	end := len(parts) - 1
	if !strings.HasPrefix(resource, parts[0]) {
		return false
	}
	for i := 1; i < end; i++ {
		if !strings.Contains(resource, parts[i]) {
			return false
		}
		idx := strings.Index(resource, parts[i]) + len(parts[i])
		resource = resource[idx:]
	}
	return tGlob || strings.HasSuffix(resource, parts[end])
}
