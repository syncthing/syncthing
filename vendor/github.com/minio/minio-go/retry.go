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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaxRetry is the maximum number of retries before stopping.
var MaxRetry = 5

// MaxJitter will randomize over the full exponential backoff time
const MaxJitter = 1.0

// NoJitter disables the use of jitter for randomizing the exponential backoff time
const NoJitter = 0.0

// DefaultRetryUnit - default unit multiplicative per retry.
// defaults to 1 second.
const DefaultRetryUnit = time.Second

// DefaultRetryCap - Each retry attempt never waits no longer than
// this maximum time duration.
const DefaultRetryCap = time.Second * 30

// newRetryTimer creates a timer with exponentially increasing
// delays until the maximum retry attempts are reached.
func (c Client) newRetryTimer(maxRetry int, unit time.Duration, cap time.Duration, jitter float64, doneCh chan struct{}) <-chan int {
	attemptCh := make(chan int)

	// computes the exponential backoff duration according to
	// https://www.awsarchitectureblog.com/2015/03/backoff.html
	exponentialBackoffWait := func(attempt int) time.Duration {
		// normalize jitter to the range [0, 1.0]
		if jitter < NoJitter {
			jitter = NoJitter
		}
		if jitter > MaxJitter {
			jitter = MaxJitter
		}

		//sleep = random_between(0, min(cap, base * 2 ** attempt))
		sleep := unit * time.Duration(1<<uint(attempt))
		if sleep > cap {
			sleep = cap
		}
		if jitter != NoJitter {
			sleep -= time.Duration(c.random.Float64() * float64(sleep) * jitter)
		}
		return sleep
	}

	go func() {
		defer close(attemptCh)
		for i := 0; i < maxRetry; i++ {
			select {
			// Attempts start from 1.
			case attemptCh <- i + 1:
			case <-doneCh:
				// Stop the routine.
				return
			}
			time.Sleep(exponentialBackoffWait(i))
		}
	}()
	return attemptCh
}

// isNetErrorRetryable - is network error retryable.
func isNetErrorRetryable(err error) bool {
	if err == nil {
		return false
	}
	switch err.(type) {
	case net.Error:
		switch err.(type) {
		case *net.DNSError, *net.OpError, net.UnknownNetworkError:
			return true
		case *url.Error:
			// For a URL error, where it replies back "connection closed"
			// retry again.
			if strings.Contains(err.Error(), "Connection closed by foreign host") {
				return true
			}
		default:
			if strings.Contains(err.Error(), "net/http: TLS handshake timeout") {
				// If error is - tlsHandshakeTimeoutError, retry.
				return true
			} else if strings.Contains(err.Error(), "i/o timeout") {
				// If error is - tcp timeoutError, retry.
				return true
			} else if strings.Contains(err.Error(), "connection timed out") {
				// If err is a net.Dial timeout, retry.
				return true
			}
		}
	}
	return false
}

// List of AWS S3 error codes which are retryable.
var retryableS3Codes = map[string]struct{}{
	"RequestError":          {},
	"RequestTimeout":        {},
	"Throttling":            {},
	"ThrottlingException":   {},
	"RequestLimitExceeded":  {},
	"RequestThrottled":      {},
	"InternalError":         {},
	"ExpiredToken":          {},
	"ExpiredTokenException": {},
	// Add more AWS S3 codes here.
}

// isS3CodeRetryable - is s3 error code retryable.
func isS3CodeRetryable(s3Code string) (ok bool) {
	_, ok = retryableS3Codes[s3Code]
	return ok
}

// List of HTTP status codes which are retryable.
var retryableHTTPStatusCodes = map[int]struct{}{
	429: {}, // http.StatusTooManyRequests is not part of the Go 1.5 library, yet
	http.StatusInternalServerError: {},
	http.StatusBadGateway:          {},
	http.StatusServiceUnavailable:  {},
	// Add more HTTP status codes here.
}

// isHTTPStatusRetryable - is HTTP error code retryable.
func isHTTPStatusRetryable(httpStatusCode int) (ok bool) {
	_, ok = retryableHTTPStatusCodes[httpStatusCode]
	return ok
}
