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
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/pkg/credentials"
	"github.com/minio/minio-go/pkg/s3signer"
	"github.com/minio/minio-go/pkg/s3utils"
)

// Client implements Amazon S3 compatible methods.
type Client struct {
	///  Standard options.

	// Parsed endpoint url provided by the user.
	endpointURL url.URL

	// Holds various credential providers.
	credsProvider *credentials.Credentials

	// Custom signerType value overrides all credentials.
	overrideSignerType credentials.SignatureType

	// User supplied.
	appInfo struct {
		appName    string
		appVersion string
	}

	// Indicate whether we are using https or not
	secure bool

	// Needs allocation.
	httpClient     *http.Client
	bucketLocCache *bucketLocationCache

	// Advanced functionality.
	isTraceEnabled bool
	traceOutput    io.Writer

	// S3 specific accelerated endpoint.
	s3AccelerateEndpoint string

	// Region endpoint
	region string

	// Random seed.
	random *rand.Rand
}

// Global constants.
const (
	libraryName    = "minio-go"
	libraryVersion = "4.0.6"
)

// User Agent should always following the below style.
// Please open an issue to discuss any new changes here.
//
//       Minio (OS; ARCH) LIB/VER APP/VER
const (
	libraryUserAgentPrefix = "Minio (" + runtime.GOOS + "; " + runtime.GOARCH + ") "
	libraryUserAgent       = libraryUserAgentPrefix + libraryName + "/" + libraryVersion
)

// NewV2 - instantiate minio client with Amazon S3 signature version
// '2' compatibility.
func NewV2(endpoint string, accessKeyID, secretAccessKey string, secure bool) (*Client, error) {
	creds := credentials.NewStaticV2(accessKeyID, secretAccessKey, "")
	clnt, err := privateNew(endpoint, creds, secure, "")
	if err != nil {
		return nil, err
	}
	clnt.overrideSignerType = credentials.SignatureV2
	return clnt, nil
}

// NewV4 - instantiate minio client with Amazon S3 signature version
// '4' compatibility.
func NewV4(endpoint string, accessKeyID, secretAccessKey string, secure bool) (*Client, error) {
	creds := credentials.NewStaticV4(accessKeyID, secretAccessKey, "")
	clnt, err := privateNew(endpoint, creds, secure, "")
	if err != nil {
		return nil, err
	}
	clnt.overrideSignerType = credentials.SignatureV4
	return clnt, nil
}

// New - instantiate minio client, adds automatic verification of signature.
func New(endpoint, accessKeyID, secretAccessKey string, secure bool) (*Client, error) {
	creds := credentials.NewStaticV4(accessKeyID, secretAccessKey, "")
	clnt, err := privateNew(endpoint, creds, secure, "")
	if err != nil {
		return nil, err
	}
	// Google cloud storage should be set to signature V2, force it if not.
	if s3utils.IsGoogleEndpoint(clnt.endpointURL) {
		clnt.overrideSignerType = credentials.SignatureV2
	}
	// If Amazon S3 set to signature v4.
	if s3utils.IsAmazonEndpoint(clnt.endpointURL) {
		clnt.overrideSignerType = credentials.SignatureV4
	}
	return clnt, nil
}

// NewWithCredentials - instantiate minio client with credentials provider
// for retrieving credentials from various credentials provider such as
// IAM, File, Env etc.
func NewWithCredentials(endpoint string, creds *credentials.Credentials, secure bool, region string) (*Client, error) {
	return privateNew(endpoint, creds, secure, region)
}

// NewWithRegion - instantiate minio client, with region configured. Unlike New(),
// NewWithRegion avoids bucket-location lookup operations and it is slightly faster.
// Use this function when if your application deals with single region.
func NewWithRegion(endpoint, accessKeyID, secretAccessKey string, secure bool, region string) (*Client, error) {
	creds := credentials.NewStaticV4(accessKeyID, secretAccessKey, "")
	return privateNew(endpoint, creds, secure, region)
}

// lockedRandSource provides protected rand source, implements rand.Source interface.
type lockedRandSource struct {
	lk  sync.Mutex
	src rand.Source
}

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64.
func (r *lockedRandSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

// Seed uses the provided seed value to initialize the generator to a
// deterministic state.
func (r *lockedRandSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}

// getRegionFromURL - parse region from URL if present.
func getRegionFromURL(u url.URL) (region string) {
	region = ""
	if s3utils.IsGoogleEndpoint(u) {
		return
	} else if s3utils.IsAmazonChinaEndpoint(u) {
		// For china specifically we need to set everything to
		// cn-north-1 for now, there is no easier way until AWS S3
		// provides a cleaner compatible API across "us-east-1" and
		// China region.
		return "cn-north-1"
	} else if s3utils.IsAmazonGovCloudEndpoint(u) {
		// For us-gov specifically we need to set everything to
		// us-gov-west-1 for now, there is no easier way until AWS S3
		// provides a cleaner compatible API across "us-east-1" and
		// Gov cloud region.
		return "us-gov-west-1"
	}
	parts := s3utils.AmazonS3Host.FindStringSubmatch(u.Host)
	if len(parts) > 1 {
		region = parts[1]
	}
	return region
}

func privateNew(endpoint string, creds *credentials.Credentials, secure bool, region string) (*Client, error) {
	// construct endpoint.
	endpointURL, err := getEndpointURL(endpoint, secure)
	if err != nil {
		return nil, err
	}

	// instantiate new Client.
	clnt := new(Client)

	// Save the credentials.
	clnt.credsProvider = creds

	// Remember whether we are using https or not
	clnt.secure = secure

	// Save endpoint URL, user agent for future uses.
	clnt.endpointURL = *endpointURL

	// Instantiate http client and bucket location cache.
	clnt.httpClient = &http.Client{
		Transport: defaultMinioTransport,
	}

	// Sets custom region, if region is empty bucket location cache is used automatically.
	if region == "" {
		region = getRegionFromURL(clnt.endpointURL)
	}
	clnt.region = region

	// Instantiate bucket location cache.
	clnt.bucketLocCache = newBucketLocationCache()

	// Introduce a new locked random seed.
	clnt.random = rand.New(&lockedRandSource{src: rand.NewSource(time.Now().UTC().UnixNano())})

	// Return.
	return clnt, nil
}

// SetAppInfo - add application details to user agent.
func (c *Client) SetAppInfo(appName string, appVersion string) {
	// if app name and version not set, we do not set a new user agent.
	if appName != "" && appVersion != "" {
		c.appInfo = struct {
			appName    string
			appVersion string
		}{}
		c.appInfo.appName = appName
		c.appInfo.appVersion = appVersion
	}
}

// SetCustomTransport - set new custom transport.
func (c *Client) SetCustomTransport(customHTTPTransport http.RoundTripper) {
	// Set this to override default transport
	// ``http.DefaultTransport``.
	//
	// This transport is usually needed for debugging OR to add your
	// own custom TLS certificates on the client transport, for custom
	// CA's and certs which are not part of standard certificate
	// authority follow this example :-
	//
	//   tr := &http.Transport{
	//           TLSClientConfig:    &tls.Config{RootCAs: pool},
	//           DisableCompression: true,
	//   }
	//   api.SetTransport(tr)
	//
	if c.httpClient != nil {
		c.httpClient.Transport = customHTTPTransport
	}
}

// TraceOn - enable HTTP tracing.
func (c *Client) TraceOn(outputStream io.Writer) {
	// if outputStream is nil then default to os.Stdout.
	if outputStream == nil {
		outputStream = os.Stdout
	}
	// Sets a new output stream.
	c.traceOutput = outputStream

	// Enable tracing.
	c.isTraceEnabled = true
}

// TraceOff - disable HTTP tracing.
func (c *Client) TraceOff() {
	// Disable tracing.
	c.isTraceEnabled = false
}

// SetS3TransferAccelerate - turns s3 accelerated endpoint on or off for all your
// requests. This feature is only specific to S3 for all other endpoints this
// function does nothing. To read further details on s3 transfer acceleration
// please vist -
// http://docs.aws.amazon.com/AmazonS3/latest/dev/transfer-acceleration.html
func (c *Client) SetS3TransferAccelerate(accelerateEndpoint string) {
	if s3utils.IsAmazonEndpoint(c.endpointURL) {
		c.s3AccelerateEndpoint = accelerateEndpoint
	}
}

// Hash materials provides relevant initialized hash algo writers
// based on the expected signature type.
//
//  - For signature v4 request if the connection is insecure compute only sha256.
//  - For signature v4 request if the connection is secure compute only md5.
//  - For anonymous request compute md5.
func (c *Client) hashMaterials() (hashAlgos map[string]hash.Hash, hashSums map[string][]byte) {
	hashSums = make(map[string][]byte)
	hashAlgos = make(map[string]hash.Hash)
	if c.overrideSignerType.IsV4() {
		if c.secure {
			hashAlgos["md5"] = md5.New()
		} else {
			hashAlgos["sha256"] = sha256.New()
		}
	} else {
		if c.overrideSignerType.IsAnonymous() {
			hashAlgos["md5"] = md5.New()
		}
	}
	return hashAlgos, hashSums
}

// requestMetadata - is container for all the values to make a request.
type requestMetadata struct {
	// If set newRequest presigns the URL.
	presignURL bool

	// User supplied.
	bucketName   string
	objectName   string
	queryValues  url.Values
	customHeader http.Header
	expires      int64

	// Generated by our internal code.
	bucketLocation   string
	contentBody      io.Reader
	contentLength    int64
	contentMD5Base64 string // carries base64 encoded md5sum
	contentSHA256Hex string // carries hex encoded sha256sum
}

// dumpHTTP - dump HTTP request and response.
func (c Client) dumpHTTP(req *http.Request, resp *http.Response) error {
	// Starts http dump.
	_, err := fmt.Fprintln(c.traceOutput, "---------START-HTTP---------")
	if err != nil {
		return err
	}

	// Filter out Signature field from Authorization header.
	origAuth := req.Header.Get("Authorization")
	if origAuth != "" {
		req.Header.Set("Authorization", redactSignature(origAuth))
	}

	// Only display request header.
	reqTrace, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		return err
	}

	// Write request to trace output.
	_, err = fmt.Fprint(c.traceOutput, string(reqTrace))
	if err != nil {
		return err
	}

	// Only display response header.
	var respTrace []byte

	// For errors we make sure to dump response body as well.
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusPartialContent &&
		resp.StatusCode != http.StatusNoContent {
		respTrace, err = httputil.DumpResponse(resp, true)
		if err != nil {
			return err
		}
	} else {
		// WORKAROUND for https://github.com/golang/go/issues/13942.
		// httputil.DumpResponse does not print response headers for
		// all successful calls which have response ContentLength set
		// to zero. Keep this workaround until the above bug is fixed.
		if resp.ContentLength == 0 {
			var buffer bytes.Buffer
			if err = resp.Header.Write(&buffer); err != nil {
				return err
			}
			respTrace = buffer.Bytes()
			respTrace = append(respTrace, []byte("\r\n")...)
		} else {
			respTrace, err = httputil.DumpResponse(resp, false)
			if err != nil {
				return err
			}
		}
	}
	// Write response to trace output.
	_, err = fmt.Fprint(c.traceOutput, strings.TrimSuffix(string(respTrace), "\r\n"))
	if err != nil {
		return err
	}

	// Ends the http dump.
	_, err = fmt.Fprintln(c.traceOutput, "---------END-HTTP---------")
	if err != nil {
		return err
	}

	// Returns success.
	return nil
}

// do - execute http request.
func (c Client) do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	// Do the request in a loop in case of 307 http is met since golang still doesn't
	// handle properly this situation (https://github.com/golang/go/issues/7912)
	for {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			// Handle this specifically for now until future Golang
			// versions fix this issue properly.
			urlErr, ok := err.(*url.Error)
			if ok && strings.Contains(urlErr.Err.Error(), "EOF") {
				return nil, &url.Error{
					Op:  urlErr.Op,
					URL: urlErr.URL,
					Err: errors.New("Connection closed by foreign host " + urlErr.URL + ". Retry again."),
				}
			}
			return nil, err
		}
		// Redo the request with the new redirect url if http 307 is returned, quit the loop otherwise
		if resp != nil && resp.StatusCode == http.StatusTemporaryRedirect {
			newURL, err := url.Parse(resp.Header.Get("Location"))
			if err != nil {
				break
			}
			req.URL = newURL
		} else {
			break
		}
	}

	// Response cannot be non-nil, report if its the case.
	if resp == nil {
		msg := "Response is empty. " + reportIssue
		return nil, ErrInvalidArgument(msg)
	}

	// If trace is enabled, dump http request and response.
	if c.isTraceEnabled {
		err = c.dumpHTTP(req, resp)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// List of success status.
var successStatus = []int{
	http.StatusOK,
	http.StatusNoContent,
	http.StatusPartialContent,
}

// executeMethod - instantiates a given method, and retries the
// request upon any error up to maxRetries attempts in a binomially
// delayed manner using a standard back off algorithm.
func (c Client) executeMethod(ctx context.Context, method string, metadata requestMetadata) (res *http.Response, err error) {
	var isRetryable bool     // Indicates if request can be retried.
	var bodySeeker io.Seeker // Extracted seeker from io.Reader.
	var reqRetry = MaxRetry  // Indicates how many times we can retry the request

	if metadata.contentBody != nil {
		// Check if body is seekable then it is retryable.
		bodySeeker, isRetryable = metadata.contentBody.(io.Seeker)
		switch bodySeeker {
		case os.Stdin, os.Stdout, os.Stderr:
			isRetryable = false
		}
		// Retry only when reader is seekable
		if !isRetryable {
			reqRetry = 1
		}

		// Figure out if the body can be closed - if yes
		// we will definitely close it upon the function
		// return.
		bodyCloser, ok := metadata.contentBody.(io.Closer)
		if ok {
			defer bodyCloser.Close()
		}
	}

	// Create a done channel to control 'newRetryTimer' go routine.
	doneCh := make(chan struct{}, 1)

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	// Blank indentifier is kept here on purpose since 'range' without
	// blank identifiers is only supported since go1.4
	// https://golang.org/doc/go1.4#forrange.
	for range c.newRetryTimer(reqRetry, DefaultRetryUnit, DefaultRetryCap, MaxJitter, doneCh) {
		// Retry executes the following function body if request has an
		// error until maxRetries have been exhausted, retry attempts are
		// performed after waiting for a given period of time in a
		// binomial fashion.
		if isRetryable {
			// Seek back to beginning for each attempt.
			if _, err = bodySeeker.Seek(0, 0); err != nil {
				// If seek failed, no need to retry.
				return nil, err
			}
		}

		// Instantiate a new request.
		var req *http.Request
		req, err = c.newRequest(method, metadata)
		if err != nil {
			errResponse := ToErrorResponse(err)
			if isS3CodeRetryable(errResponse.Code) {
				continue // Retry.
			}
			return nil, err
		}
		// Add context to request
		req = req.WithContext(ctx)

		// Initiate the request.
		res, err = c.do(req)
		if err != nil {
			// For supported network errors verify.
			if isNetErrorRetryable(err) {
				continue // Retry.
			}
			// For other errors, return here no need to retry.
			return nil, err
		}

		// For any known successful http status, return quickly.
		for _, httpStatus := range successStatus {
			if httpStatus == res.StatusCode {
				return res, nil
			}
		}

		// Read the body to be saved later.
		errBodyBytes, err := ioutil.ReadAll(res.Body)
		// res.Body should be closed
		closeResponse(res)
		if err != nil {
			return nil, err
		}

		// Save the body.
		errBodySeeker := bytes.NewReader(errBodyBytes)
		res.Body = ioutil.NopCloser(errBodySeeker)

		// For errors verify if its retryable otherwise fail quickly.
		errResponse := ToErrorResponse(httpRespToErrorResponse(res, metadata.bucketName, metadata.objectName))

		// Save the body back again.
		errBodySeeker.Seek(0, 0) // Seek back to starting point.
		res.Body = ioutil.NopCloser(errBodySeeker)

		// Bucket region if set in error response and the error
		// code dictates invalid region, we can retry the request
		// with the new region.
		//
		// Additionally we should only retry if bucketLocation and custom
		// region is empty.
		if metadata.bucketLocation == "" && c.region == "" {
			if errResponse.Code == "AuthorizationHeaderMalformed" || errResponse.Code == "InvalidRegion" {
				if metadata.bucketName != "" && errResponse.Region != "" {
					// Gather Cached location only if bucketName is present.
					if _, cachedLocationError := c.bucketLocCache.Get(metadata.bucketName); cachedLocationError != false {
						c.bucketLocCache.Set(metadata.bucketName, errResponse.Region)
						continue // Retry.
					}
				}
			}
		}

		// Verify if error response code is retryable.
		if isS3CodeRetryable(errResponse.Code) {
			continue // Retry.
		}

		// Verify if http status code is retryable.
		if isHTTPStatusRetryable(res.StatusCode) {
			continue // Retry.
		}

		// For all other cases break out of the retry loop.
		break
	}
	return res, err
}

// newRequest - instantiate a new HTTP request for a given method.
func (c Client) newRequest(method string, metadata requestMetadata) (req *http.Request, err error) {
	// If no method is supplied default to 'POST'.
	if method == "" {
		method = "POST"
	}

	location := metadata.bucketLocation
	if location == "" {
		if metadata.bucketName != "" {
			// Gather location only if bucketName is present.
			location, err = c.getBucketLocation(metadata.bucketName)
			if err != nil {
				if ToErrorResponse(err).Code != "AccessDenied" {
					return nil, err
				}
			}
			// Upon AccessDenied error on fetching bucket location, default
			// to possible locations based on endpoint URL. This can usually
			// happen when GetBucketLocation() is disabled using IAM policies.
		}
		if location == "" {
			location = getDefaultLocation(c.endpointURL, c.region)
		}
	}

	// Construct a new target URL.
	targetURL, err := c.makeTargetURL(metadata.bucketName, metadata.objectName, location, metadata.queryValues)
	if err != nil {
		return nil, err
	}

	// Initialize a new HTTP request for the method.
	req, err = http.NewRequest(method, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}

	// Get credentials from the configured credentials provider.
	value, err := c.credsProvider.Get()
	if err != nil {
		return nil, err
	}

	var (
		signerType      = value.SignerType
		accessKeyID     = value.AccessKeyID
		secretAccessKey = value.SecretAccessKey
		sessionToken    = value.SessionToken
	)

	// Custom signer set then override the behavior.
	if c.overrideSignerType != credentials.SignatureDefault {
		signerType = c.overrideSignerType
	}

	// If signerType returned by credentials helper is anonymous,
	// then do not sign regardless of signerType override.
	if value.SignerType == credentials.SignatureAnonymous {
		signerType = credentials.SignatureAnonymous
	}

	// Generate presign url if needed, return right here.
	if metadata.expires != 0 && metadata.presignURL {
		if signerType.IsAnonymous() {
			return nil, ErrInvalidArgument("Presigned URLs cannot be generated with anonymous credentials.")
		}
		if signerType.IsV2() {
			// Presign URL with signature v2.
			req = s3signer.PreSignV2(*req, accessKeyID, secretAccessKey, metadata.expires)
		} else if signerType.IsV4() {
			// Presign URL with signature v4.
			req = s3signer.PreSignV4(*req, accessKeyID, secretAccessKey, sessionToken, location, metadata.expires)
		}
		return req, nil
	}

	// Set 'User-Agent' header for the request.
	c.setUserAgent(req)

	// Set all headers.
	for k, v := range metadata.customHeader {
		req.Header.Set(k, v[0])
	}

	// Go net/http notoriously closes the request body.
	// - The request Body, if non-nil, will be closed by the underlying Transport, even on errors.
	// This can cause underlying *os.File seekers to fail, avoid that
	// by making sure to wrap the closer as a nop.
	if metadata.contentLength == 0 {
		req.Body = nil
	} else {
		req.Body = ioutil.NopCloser(metadata.contentBody)
	}

	// Set incoming content-length.
	req.ContentLength = metadata.contentLength
	if req.ContentLength <= -1 {
		// For unknown content length, we upload using transfer-encoding: chunked.
		req.TransferEncoding = []string{"chunked"}
	}

	// set md5Sum for content protection.
	if len(metadata.contentMD5Base64) > 0 {
		req.Header.Set("Content-Md5", metadata.contentMD5Base64)
	}

	// For anonymous requests just return.
	if signerType.IsAnonymous() {
		return req, nil
	}

	switch {
	case signerType.IsV2():
		// Add signature version '2' authorization header.
		req = s3signer.SignV2(*req, accessKeyID, secretAccessKey)
	case metadata.objectName != "" && method == "PUT" && metadata.customHeader.Get("X-Amz-Copy-Source") == "" && !c.secure:
		// Streaming signature is used by default for a PUT object request. Additionally we also
		// look if the initialized client is secure, if yes then we don't need to perform
		// streaming signature.
		req = s3signer.StreamingSignV4(req, accessKeyID,
			secretAccessKey, sessionToken, location, metadata.contentLength, time.Now().UTC())
	default:
		// Set sha256 sum for signature calculation only with signature version '4'.
		shaHeader := unsignedPayload
		if metadata.contentSHA256Hex != "" {
			shaHeader = metadata.contentSHA256Hex
		}
		req.Header.Set("X-Amz-Content-Sha256", shaHeader)

		// Add signature version '4' authorization header.
		req = s3signer.SignV4(*req, accessKeyID, secretAccessKey, sessionToken, location)
	}

	// Return request.
	return req, nil
}

// set User agent.
func (c Client) setUserAgent(req *http.Request) {
	req.Header.Set("User-Agent", libraryUserAgent)
	if c.appInfo.appName != "" && c.appInfo.appVersion != "" {
		req.Header.Set("User-Agent", libraryUserAgent+" "+c.appInfo.appName+"/"+c.appInfo.appVersion)
	}
}

// makeTargetURL make a new target url.
func (c Client) makeTargetURL(bucketName, objectName, bucketLocation string, queryValues url.Values) (*url.URL, error) {
	host := c.endpointURL.Host
	// For Amazon S3 endpoint, try to fetch location based endpoint.
	if s3utils.IsAmazonEndpoint(c.endpointURL) {
		if c.s3AccelerateEndpoint != "" && bucketName != "" {
			// http://docs.aws.amazon.com/AmazonS3/latest/dev/transfer-acceleration.html
			// Disable transfer acceleration for non-compliant bucket names.
			if strings.Contains(bucketName, ".") {
				return nil, ErrTransferAccelerationBucket(bucketName)
			}
			// If transfer acceleration is requested set new host.
			// For more details about enabling transfer acceleration read here.
			// http://docs.aws.amazon.com/AmazonS3/latest/dev/transfer-acceleration.html
			host = c.s3AccelerateEndpoint
		} else {
			// Do not change the host if the endpoint URL is a FIPS S3 endpoint.
			if !s3utils.IsAmazonFIPSGovCloudEndpoint(c.endpointURL) {
				// Fetch new host based on the bucket location.
				host = getS3Endpoint(bucketLocation)
			}
		}
	}

	// Save scheme.
	scheme := c.endpointURL.Scheme

	// Strip port 80 and 443 so we won't send these ports in Host header.
	// The reason is that browsers and curl automatically remove :80 and :443
	// with the generated presigned urls, then a signature mismatch error.
	if h, p, err := net.SplitHostPort(host); err == nil {
		if scheme == "http" && p == "80" || scheme == "https" && p == "443" {
			host = h
		}
	}

	urlStr := scheme + "://" + host + "/"
	// Make URL only if bucketName is available, otherwise use the
	// endpoint URL.
	if bucketName != "" {
		// Save if target url will have buckets which suppport virtual host.
		isVirtualHostStyle := s3utils.IsVirtualHostSupported(c.endpointURL, bucketName)

		// If endpoint supports virtual host style use that always.
		// Currently only S3 and Google Cloud Storage would support
		// virtual host style.
		if isVirtualHostStyle {
			urlStr = scheme + "://" + bucketName + "." + host + "/"
			if objectName != "" {
				urlStr = urlStr + s3utils.EncodePath(objectName)
			}
		} else {
			// If not fall back to using path style.
			urlStr = urlStr + bucketName + "/"
			if objectName != "" {
				urlStr = urlStr + s3utils.EncodePath(objectName)
			}
		}
	}

	// If there are any query values, add them to the end.
	if len(queryValues) > 0 {
		urlStr = urlStr + "?" + s3utils.QueryEncode(queryValues)
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	return u, nil
}
