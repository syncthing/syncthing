// Copyright (C) 2012 Numerotron Inc.
// Use of this source code is governed by an MIT-style license
// that can be found in the LICENSE file.

// Copyright 2012 Numerotron Inc.
// Use of this source code is governed by an MIT-style license
// that can be found in the LICENSE file.
//
// Developed at www.stathat.com by Patrick Crosby
// Contact us on twitter with any questions:  twitter.com/stat_hat

// The stathat package makes it easy to post any values to your StatHat
// account.
package stathat

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const hostname = "api.stathat.com"

type statKind int

const (
	_                 = iota
	kcounter statKind = iota
	kvalue
)

func (sk statKind) classicPath() string {
	switch sk {
	case kcounter:
		return "/c"
	case kvalue:
		return "/v"
	}
	return ""
}

type apiKind int

const (
	_               = iota
	classic apiKind = iota
	ez
)

func (ak apiKind) path(sk statKind) string {
	switch ak {
	case ez:
		return "/ez"
	case classic:
		return sk.classicPath()
	}
	return ""
}

type statReport struct {
	StatKey   string
	UserKey   string
	Value     float64
	Timestamp int64
	statType  statKind
	apiType   apiKind
}

// Reporter describes an interface for communicating with the StatHat API
type Reporter interface {
	PostCount(statKey, userKey string, count int) error
	PostCountTime(statKey, userKey string, count int, timestamp int64) error
	PostCountOne(statKey, userKey string) error
	PostValue(statKey, userKey string, value float64) error
	PostValueTime(statKey, userKey string, value float64, timestamp int64) error
	PostEZCountOne(statName, ezkey string) error
	PostEZCount(statName, ezkey string, count int) error
	PostEZCountTime(statName, ezkey string, count int, timestamp int64) error
	PostEZValue(statName, ezkey string, value float64) error
	PostEZValueTime(statName, ezkey string, value float64, timestamp int64) error
	WaitUntilFinished(timeout time.Duration) bool
}

// BasicReporter is a StatHat client that can report stat values/counts to the servers.
type BasicReporter struct {
	reports chan *statReport
	done    chan bool
	client  *http.Client
	wg      *sync.WaitGroup
}

// NewReporter returns a new Reporter.  You must specify the channel bufferSize and the
// goroutine poolSize.  You can pass in nil for the transport and it will create an
// http transport with MaxIdleConnsPerHost set to the goroutine poolSize.  Note if you
// pass in your own transport, it's a good idea to have its MaxIdleConnsPerHost be set
// to at least the poolSize to allow for effective connection reuse.
func NewReporter(bufferSize, poolSize int, transport http.RoundTripper) Reporter {
	r := new(BasicReporter)
	if transport == nil {
		transport = &http.Transport{
			// Allow for an idle connection per goroutine.
			MaxIdleConnsPerHost: poolSize,
		}
	}
	r.client = &http.Client{Transport: transport}
	r.reports = make(chan *statReport, bufferSize)
	r.done = make(chan bool)
	r.wg = new(sync.WaitGroup)
	for i := 0; i < poolSize; i++ {
		r.wg.Add(1)
		go r.processReports()
	}
	return r
}

type statCache struct {
	counterStats map[string]int
	valueStats   map[string][]float64
}

func (sc *statCache) AverageValue(statName string) float64 {
	total := 0.0
	values := sc.valueStats[statName]
	if len(values) == 0 {
		return total
	}
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

// BatchReporter wraps an existing Reporter in order to implement sending stats
// to the StatHat server in batch. The flow is only available for the EZ API.
// The following describes how stats are sent:
// 1.) PostEZCountOne is called and adds the stat request to a queue.
// 2.) PostEZCountOne is called again on the same stat, the value in the queue is incremented.
// 3.) After batchInterval amount of time, all stat requests from the queue are
//     sent to the server.
type BatchReporter struct {
	sync.Mutex
	r               Reporter
	batchInterval   time.Duration
	caches          map[string]*statCache
	shutdownBatchCh chan struct{}
}

// DefaultReporter is the default instance of *Reporter.
var DefaultReporter = NewReporter(100000, 10, nil)

var testingEnv = false

type testPost struct {
	url    string
	values url.Values
}

var testPostChannel chan *testPost

// The Verbose flag determines if the package should write verbose output to stdout.
var Verbose = false

func setTesting() {
	testingEnv = true
	testPostChannel = make(chan *testPost)
}

func newEZStatCount(statName, ezkey string, count int) *statReport {
	return &statReport{StatKey: statName,
		UserKey:  ezkey,
		Value:    float64(count),
		statType: kcounter,
		apiType:  ez}
}

func newEZStatValue(statName, ezkey string, value float64) *statReport {
	return &statReport{StatKey: statName,
		UserKey:  ezkey,
		Value:    value,
		statType: kvalue,
		apiType:  ez}
}

func newClassicStatCount(statKey, userKey string, count int) *statReport {
	return &statReport{StatKey: statKey,
		UserKey:  userKey,
		Value:    float64(count),
		statType: kcounter,
		apiType:  classic}
}

func newClassicStatValue(statKey, userKey string, value float64) *statReport {
	return &statReport{StatKey: statKey,
		UserKey:  userKey,
		Value:    value,
		statType: kvalue,
		apiType:  classic}
}

func (sr *statReport) values() url.Values {
	switch sr.apiType {
	case ez:
		return sr.ezValues()
	case classic:
		return sr.classicValues()
	}

	return nil
}

func (sr *statReport) ezValues() url.Values {
	switch sr.statType {
	case kcounter:
		return sr.ezCounterValues()
	case kvalue:
		return sr.ezValueValues()
	}
	return nil
}

func (sr *statReport) classicValues() url.Values {
	switch sr.statType {
	case kcounter:
		return sr.classicCounterValues()
	case kvalue:
		return sr.classicValueValues()
	}
	return nil
}

func (sr *statReport) ezCommonValues() url.Values {
	result := make(url.Values)
	result.Set("stat", sr.StatKey)
	result.Set("ezkey", sr.UserKey)
	if sr.Timestamp > 0 {
		result.Set("t", sr.timeString())
	}
	return result
}

func (sr *statReport) classicCommonValues() url.Values {
	result := make(url.Values)
	result.Set("key", sr.StatKey)
	result.Set("ukey", sr.UserKey)
	if sr.Timestamp > 0 {
		result.Set("t", sr.timeString())
	}
	return result
}

func (sr *statReport) ezCounterValues() url.Values {
	result := sr.ezCommonValues()
	result.Set("count", sr.valueString())
	return result
}

func (sr *statReport) ezValueValues() url.Values {
	result := sr.ezCommonValues()
	result.Set("value", sr.valueString())
	return result
}

func (sr *statReport) classicCounterValues() url.Values {
	result := sr.classicCommonValues()
	result.Set("count", sr.valueString())
	return result
}

func (sr *statReport) classicValueValues() url.Values {
	result := sr.classicCommonValues()
	result.Set("value", sr.valueString())
	return result
}

func (sr *statReport) valueString() string {
	return strconv.FormatFloat(sr.Value, 'g', -1, 64)
}

func (sr *statReport) timeString() string {
	return strconv.FormatInt(sr.Timestamp, 10)
}

func (sr *statReport) path() string {
	return sr.apiType.path(sr.statType)
}

func (sr *statReport) url() string {
	return fmt.Sprintf("https://%s%s", hostname, sr.path())
}

// Using the classic API, posts a count to a stat using DefaultReporter.
func PostCount(statKey, userKey string, count int) error {
	return DefaultReporter.PostCount(statKey, userKey, count)
}

// Using the classic API, posts a count to a stat using DefaultReporter at a specific
// time.
func PostCountTime(statKey, userKey string, count int, timestamp int64) error {
	return DefaultReporter.PostCountTime(statKey, userKey, count, timestamp)
}

// Using the classic API, posts a count of 1 to a stat using DefaultReporter.
func PostCountOne(statKey, userKey string) error {
	return DefaultReporter.PostCountOne(statKey, userKey)
}

// Using the classic API, posts a value to a stat using DefaultReporter.
func PostValue(statKey, userKey string, value float64) error {
	return DefaultReporter.PostValue(statKey, userKey, value)
}

// Using the classic API, posts a value to a stat at a specific time using DefaultReporter.
func PostValueTime(statKey, userKey string, value float64, timestamp int64) error {
	return DefaultReporter.PostValueTime(statKey, userKey, value, timestamp)
}

// Using the EZ API, posts a count of 1 to a stat using DefaultReporter.
func PostEZCountOne(statName, ezkey string) error {
	return DefaultReporter.PostEZCountOne(statName, ezkey)
}

// Using the EZ API, posts a count to a stat using DefaultReporter.
func PostEZCount(statName, ezkey string, count int) error {
	return DefaultReporter.PostEZCount(statName, ezkey, count)
}

// Using the EZ API, posts a count to a stat at a specific time using DefaultReporter.
func PostEZCountTime(statName, ezkey string, count int, timestamp int64) error {
	return DefaultReporter.PostEZCountTime(statName, ezkey, count, timestamp)
}

// Using the EZ API, posts a value to a stat using DefaultReporter.
func PostEZValue(statName, ezkey string, value float64) error {
	return DefaultReporter.PostEZValue(statName, ezkey, value)
}

// Using the EZ API, posts a value to a stat at a specific time using DefaultReporter.
func PostEZValueTime(statName, ezkey string, value float64, timestamp int64) error {
	return DefaultReporter.PostEZValueTime(statName, ezkey, value, timestamp)
}

// Wait for all stats to be sent, or until timeout. Useful for simple command-
// line apps to defer a call to this in main()
func WaitUntilFinished(timeout time.Duration) bool {
	return DefaultReporter.WaitUntilFinished(timeout)
}

// Using the classic API, posts a count to a stat.
func (r *BasicReporter) PostCount(statKey, userKey string, count int) error {
	r.add(newClassicStatCount(statKey, userKey, count))
	return nil
}

// Using the classic API, posts a count to a stat at a specific time.
func (r *BasicReporter) PostCountTime(statKey, userKey string, count int, timestamp int64) error {
	x := newClassicStatCount(statKey, userKey, count)
	x.Timestamp = timestamp
	r.add(x)
	return nil
}

// Using the classic API, posts a count of 1 to a stat.
func (r *BasicReporter) PostCountOne(statKey, userKey string) error {
	return r.PostCount(statKey, userKey, 1)
}

// Using the classic API, posts a value to a stat.
func (r *BasicReporter) PostValue(statKey, userKey string, value float64) error {
	r.add(newClassicStatValue(statKey, userKey, value))
	return nil
}

// Using the classic API, posts a value to a stat at a specific time.
func (r *BasicReporter) PostValueTime(statKey, userKey string, value float64, timestamp int64) error {
	x := newClassicStatValue(statKey, userKey, value)
	x.Timestamp = timestamp
	r.add(x)
	return nil
}

// Using the EZ API, posts a count of 1 to a stat.
func (r *BasicReporter) PostEZCountOne(statName, ezkey string) error {
	return r.PostEZCount(statName, ezkey, 1)
}

// Using the EZ API, posts a count to a stat.
func (r *BasicReporter) PostEZCount(statName, ezkey string, count int) error {
	r.add(newEZStatCount(statName, ezkey, count))
	return nil
}

// Using the EZ API, posts a count to a stat at a specific time.
func (r *BasicReporter) PostEZCountTime(statName, ezkey string, count int, timestamp int64) error {
	x := newEZStatCount(statName, ezkey, count)
	x.Timestamp = timestamp
	r.add(x)
	return nil
}

// Using the EZ API, posts a value to a stat.
func (r *BasicReporter) PostEZValue(statName, ezkey string, value float64) error {
	r.add(newEZStatValue(statName, ezkey, value))
	return nil
}

// Using the EZ API, posts a value to a stat at a specific time.
func (r *BasicReporter) PostEZValueTime(statName, ezkey string, value float64, timestamp int64) error {
	x := newEZStatValue(statName, ezkey, value)
	x.Timestamp = timestamp
	r.add(x)
	return nil
}

func (r *BasicReporter) processReports() {
	for sr := range r.reports {
		if Verbose {
			log.Printf("posting stat to stathat: %s, %v", sr.url(), sr.values())
		}

		if testingEnv {
			if Verbose {
				log.Printf("in test mode, putting stat on testPostChannel")
			}
			testPostChannel <- &testPost{sr.url(), sr.values()}
			continue
		}

		resp, err := r.client.PostForm(sr.url(), sr.values())
		if err != nil {
			log.Printf("error posting stat to stathat: %s", err)
			continue
		}

		if Verbose {
			body, _ := ioutil.ReadAll(resp.Body)
			log.Printf("stathat post result: %s", body)
		} else {
			// Read the body even if we don't intend to use it. Otherwise golang won't pool the connection.
			// See also: http://stackoverflow.com/questions/17948827/reusing-http-connections-in-golang/17953506#17953506
			io.Copy(ioutil.Discard, resp.Body)
		}

		resp.Body.Close()
	}
	r.wg.Done()
}

func (r *BasicReporter) add(rep *statReport) {
	select {
	case r.reports <- rep:
	default:
	}
}

func (r *BasicReporter) finish() {
	close(r.reports)
	r.wg.Wait()
	r.done <- true
}

// Wait for all stats to be sent, or until timeout. Useful for simple command-
// line apps to defer a call to this in main()
func (r *BasicReporter) WaitUntilFinished(timeout time.Duration) bool {
	go r.finish()
	select {
	case <-r.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// NewBatchReporter creates a batching stat reporter. The interval parameter
// specifies how often stats should be posted to the StatHat server.
func NewBatchReporter(reporter Reporter, interval time.Duration) Reporter {

	br := &BatchReporter{
		r:               reporter,
		batchInterval:   interval,
		caches:          make(map[string]*statCache),
		shutdownBatchCh: make(chan struct{}),
	}

	go br.batchLoop()

	return br
}

func (br *BatchReporter) getEZCache(ezkey string) *statCache {
	var cache *statCache
	var ok bool

	// Fetch ezkey cache
	if cache, ok = br.caches[ezkey]; !ok {
		cache = &statCache{
			counterStats: make(map[string]int),
			valueStats:   make(map[string][]float64),
		}
		br.caches[ezkey] = cache
	}

	return cache
}

func (br *BatchReporter) PostEZCount(statName, ezkey string, count int) error {
	br.Lock()
	defer br.Unlock()

	// Increment stat by count
	br.getEZCache(ezkey).counterStats[statName] += count

	return nil
}

func (br *BatchReporter) PostEZCountOne(statName, ezkey string) error {
	return br.PostEZCount(statName, ezkey, 1)
}

func (br *BatchReporter) PostEZValue(statName, ezkey string, value float64) error {
	br.Lock()
	defer br.Unlock()

	// Update value cache
	cache := br.getEZCache(ezkey)
	cache.valueStats[statName] = append(cache.valueStats[statName], value)

	return nil
}

func (br *BatchReporter) batchPost() {

	// Copy and clear cache
	br.Lock()
	caches := br.caches
	br.caches = make(map[string]*statCache)
	br.Unlock()

	// Post stats
	for ezkey, cache := range caches {
		// Post counters
		for statName, count := range cache.counterStats {
			br.r.PostEZCount(statName, ezkey, count)
		}

		// Post values
		for statName := range cache.valueStats {
			br.r.PostEZValue(statName, ezkey, cache.AverageValue(statName))
		}
	}
}

func (br *BatchReporter) batchLoop() {
	for {
		select {
		case <-br.shutdownBatchCh:
			return
		case <-time.After(br.batchInterval):
			br.batchPost()
		}
	}
}

func (br *BatchReporter) PostCount(statKey, userKey string, count int) error {
	return br.r.PostCount(statKey, userKey, count)
}

func (br *BatchReporter) PostCountTime(statKey, userKey string, count int, timestamp int64) error {
	return br.r.PostCountTime(statKey, userKey, count, timestamp)
}

func (br *BatchReporter) PostCountOne(statKey, userKey string) error {
	return br.r.PostCountOne(statKey, userKey)
}

func (br *BatchReporter) PostValue(statKey, userKey string, value float64) error {
	return br.r.PostValue(statKey, userKey, value)
}

func (br *BatchReporter) PostValueTime(statKey, userKey string, value float64, timestamp int64) error {
	return br.r.PostValueTime(statKey, userKey, value, timestamp)
}

func (br *BatchReporter) PostEZCountTime(statName, ezkey string, count int, timestamp int64) error {
	return br.r.PostEZCountTime(statName, ezkey, count, timestamp)
}

func (br *BatchReporter) PostEZValueTime(statName, ezkey string, value float64, timestamp int64) error {
	return br.r.PostEZValueTime(statName, ezkey, value, timestamp)
}

func (br *BatchReporter) WaitUntilFinished(timeout time.Duration) bool {
	// Shut down batch loop
	close(br.shutdownBatchCh)

	// One last post
	br.batchPost()

	return br.r.WaitUntilFinished(timeout)
}
