// +build ignore

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

package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"io/ioutil"
	"log"

	minio "github.com/minio/minio-go"
)

func main() {
	// Note: YOUR-ACCESSKEYID, YOUR-SECRETACCESSKEY, my-testfile, my-bucketname and
	// my-objectname are dummy values, please replace them with original values.

	// New returns an Amazon S3 compatible client object. API compatibility (v2 or v4) is automatically
	// determined based on the Endpoint value.
	minioClient, err := minio.New("s3.amazonaws.com", "YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", true)
	if err != nil {
		log.Fatalln(err)
	}

	content := bytes.NewReader([]byte("Hello again"))
	key := []byte("32byteslongsecretkeymustprovided")
	h := md5.New()
	h.Write(key)
	encryptionKey := base64.StdEncoding.EncodeToString(key)
	encryptionKeyMD5 := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Amazon S3 does not store the encryption key you provide.
	// Instead S3 stores a randomly salted HMAC value of the
	// encryption key in order to validate future requests.
	// The salted HMAC value cannot be used to derive the value
	// of the encryption key or to decrypt the contents of the
	// encrypted object. That means, if you lose the encryption
	// key, you lose the object.
	var metadata = map[string]string{
		"x-amz-server-side-encryption-customer-algorithm": "AES256",
		"x-amz-server-side-encryption-customer-key":       encryptionKey,
		"x-amz-server-side-encryption-customer-key-MD5":   encryptionKeyMD5,
	}

	// minioClient.TraceOn(os.Stderr) // Enable to debug.
	_, err = minioClient.PutObject("mybucket", "my-encrypted-object.txt", content, 11, minio.PutObjectOptions{UserMetadata: metadata})
	if err != nil {
		log.Fatalln(err)
	}

	opts := minio.GetObjectOptions{}
	for k, v := range metadata {
		opts.Set(k, v)
	}
	coreClient := minio.Core{minioClient}
	reader, _, err := coreClient.GetObject("mybucket", "my-encrypted-object.txt", opts)
	if err != nil {
		log.Fatalln(err)
	}
	defer reader.Close()

	decBytes, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Fatalln(err)
	}
	if !bytes.Equal(decBytes, []byte("Hello again")) {
		log.Fatalln("Expected \"Hello, world\", got %s", string(decBytes))
	}
}
