// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/juju/ratelimit"

	"github.com/syncthing/relaysrv/client"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

type relay struct {
	URL string `json:"url"`
	uri *url.URL
}

func (r relay) String() string {
	return r.URL
}

type request struct {
	relay  relay
	uri    *url.URL
	result chan result
}

type result struct {
	err      error
	eviction time.Duration
}

var (
	binDir         string
	testCert       tls.Certificate
	listen         string        = ":80"
	dir            string        = ""
	evictionTime   time.Duration = time.Hour
	debug          bool          = false
	getLRUSize     int           = 10 << 10
	getLimitBurst  int64         = 10
	getLimitAvg                  = 1
	postLRUSize    int           = 1 << 10
	postLimitBurst int64         = 2
	postLimitAvg                 = 1
	getLimit       time.Duration
	postLimit      time.Duration
	permRelaysFile string

	getMut      sync.RWMutex = sync.NewRWMutex()
	getLRUCache *lru.Cache

	postMut      sync.RWMutex = sync.NewRWMutex()
	postLRUCache *lru.Cache

	requests = make(chan request, 10)

	mut             sync.RWMutex           = sync.NewRWMutex()
	knownRelays     []relay                = make([]relay, 0)
	permanentRelays []relay                = make([]relay, 0)
	evictionTimers  map[string]*time.Timer = make(map[string]*time.Timer)
)

func main() {
	flag.StringVar(&listen, "listen", listen, "Listen address")
	flag.StringVar(&dir, "keys", dir, "Directory where http-cert.pem and http-key.pem is stored for TLS listening")
	flag.BoolVar(&debug, "debug", debug, "Enable debug output")
	flag.DurationVar(&evictionTime, "eviction", evictionTime, "After how long the relay is evicted")
	flag.IntVar(&getLRUSize, "get-limit-cache", getLRUSize, "Get request limiter cache size")
	flag.IntVar(&getLimitAvg, "get-limit-avg", 2, "Allowed average get request rate, per 10 s")
	flag.Int64Var(&getLimitBurst, "get-limit-burst", getLimitBurst, "Allowed burst get requests")
	flag.IntVar(&postLRUSize, "post-limit-cache", postLRUSize, "Post request limiter cache size")
	flag.IntVar(&postLimitAvg, "post-limit-avg", 2, "Allowed average post request rate, per minute")
	flag.Int64Var(&postLimitBurst, "post-limit-burst", postLimitBurst, "Allowed burst post requests")
	flag.StringVar(&permRelaysFile, "perm-relays", "", "Path to list of permanent relays")

	flag.Parse()

	getLimit = 10 * time.Second / time.Duration(getLimitAvg)
	postLimit = time.Minute / time.Duration(postLimitAvg)

	getLRUCache = lru.New(getLRUSize)
	postLRUCache = lru.New(postLRUSize)

	var listener net.Listener
	var err error

	if permRelaysFile != "" {
		loadPermanentRelays(permRelaysFile)
	}

	testCert = createTestCertificate()

	go requestProcessor()

	if dir != "" {
		if debug {
			log.Println("Starting TLS listener on", listen)
		}
		certFile, keyFile := filepath.Join(dir, "http-cert.pem"), filepath.Join(dir, "http-key.pem")
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalln("Failed to load HTTP X509 key pair:", err)
		}

		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS10, // No SSLv3
			CipherSuites: []uint16{
				// No RC4
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
				tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
			},
		}

		listener, err = tls.Listen("tcp", listen, tlsCfg)
	} else {
		if debug {
			log.Println("Starting plain listener on", listen)
		}
		listener, err = net.Listen("tcp", listen)
	}

	if err != nil {
		log.Fatalln("listen:", err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", handleRequest)

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	err = srv.Serve(listener)
	if err != nil {
		log.Fatalln("serve:", err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if limit(r.RemoteAddr, getLRUCache, getMut, getLimit, int64(getLimitBurst)) {
			w.WriteHeader(429)
			return
		}
		handleGetRequest(w, r)
	case "POST":
		if limit(r.RemoteAddr, postLRUCache, postMut, postLimit, int64(postLimitBurst)) {
			w.WriteHeader(429)
			return
		}
		handlePostRequest(w, r)
	default:
		if debug {
			log.Println("Unhandled HTTP method", r.Method)
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	mut.RLock()
	relays := append(permanentRelays, knownRelays...)
	mut.RUnlock()

	// Shuffle
	for i := range relays {
		j := rand.Intn(i + 1)
		relays[i], relays[j] = relays[j], relays[i]
	}

	json.NewEncoder(w).Encode(map[string][]relay{
		"relays": relays,
	})
}

func handlePostRequest(w http.ResponseWriter, r *http.Request) {
	var newRelay relay
	err := json.NewDecoder(r.Body).Decode(&newRelay)
	r.Body.Close()

	if err != nil {
		if debug {
			log.Println("Failed to parse payload")
		}
		http.Error(w, err.Error(), 500)
		return
	}

	uri, err := url.Parse(newRelay.URL)
	if err != nil {
		if debug {
			log.Println("Failed to parse URI", newRelay.URL)
		}
		http.Error(w, err.Error(), 500)
		return
	}

	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil {
		if debug {
			log.Println("Failed to split URI", newRelay.URL)
		}
		http.Error(w, err.Error(), 500)
		return
	}

	// Get the IP address of the client
	rhost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if debug {
			log.Println("Failed to split remote address", r.RemoteAddr)
		}
		http.Error(w, err.Error(), 500)
		return
	}

	// The client did not provide an IP address, use the IP address of the client.
	if host == "" {
		uri.Host = net.JoinHostPort(rhost, port)
		newRelay.URL = uri.String()
	} else if host != rhost {
		if debug {
			log.Println("IP address advertised does not match client IP address", r.RemoteAddr, uri)
		}
		http.Error(w, "IP address does not match client IP", http.StatusUnauthorized)
		return
	}
	newRelay.uri = uri

	for _, current := range permanentRelays {
		if current.uri.Host == newRelay.uri.Host {
			if debug {
				log.Println("Asked to add a relay", newRelay, "which exists in permanent list")
			}
			http.Error(w, "Invalid request", 500)
			return
		}
	}

	reschan := make(chan result)

	select {
	case requests <- request{newRelay, uri, reschan}:
		result := <-reschan
		if result.err != nil {
			http.Error(w, result.err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]time.Duration{
			"evictionIn": result.eviction,
		})

	default:
		if debug {
			log.Println("Dropping request")
		}
		w.WriteHeader(429)
	}
}

func requestProcessor() {
	for request := range requests {
		if debug {
			log.Println("Request for", request.relay)
		}
		if !client.TestRelay(request.uri, []tls.Certificate{testCert}, 250*time.Millisecond, 4) {
			if debug {
				log.Println("Test for relay", request.relay, "failed")
			}
			request.result <- result{fmt.Errorf("test failed"), 0}
			continue
		}

		mut.Lock()
		timer, ok := evictionTimers[request.relay.uri.Host]
		if ok {
			if debug {
				log.Println("Stopping existing timer for", request.relay)
			}
			timer.Stop()
		}

		for _, current := range knownRelays {
			if current.uri.Host == request.relay.uri.Host {
				if debug {
					log.Println("Relay", request.relay, "already exists")
				}
				goto found
			}
		}

		if debug {
			log.Println("Adding new relay", request.relay)
		}
		knownRelays = append(knownRelays, request.relay)

	found:
		evictionTimers[request.relay.uri.Host] = time.AfterFunc(evictionTime, evict(request.relay))
		mut.Unlock()
		request.result <- result{nil, evictionTime}
	}

}

func evict(relay relay) func() {
	return func() {
		mut.Lock()
		defer mut.Unlock()
		if debug {
			log.Println("Evicting", relay)
		}
		for i, current := range knownRelays {
			if current.uri.Host == relay.uri.Host {
				if debug {
					log.Println("Evicted", relay)
				}
				last := len(knownRelays) - 1
				knownRelays[i] = knownRelays[last]
				knownRelays = knownRelays[:last]
			}
		}
		delete(evictionTimers, relay.uri.Host)
	}
}

func limit(addr string, cache *lru.Cache, lock sync.RWMutex, rate time.Duration, burst int64) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	lock.RLock()
	bkt, ok := cache.Get(host)
	lock.RUnlock()
	if ok {
		bkt := bkt.(*ratelimit.Bucket)
		if bkt.TakeAvailable(1) != 1 {
			// Rate limit
			return true
		}
	} else {
		lock.Lock()
		cache.Add(host, ratelimit.NewBucket(rate, burst))
		lock.Unlock()
	}
	return false
}

func loadPermanentRelays(file string) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		if len(line) == 0 {
			continue
		}

		uri, err := url.Parse(line)
		if err != nil {
			if debug {
				log.Println("Skipping permanent relay", line, "due to parse error", err)
			}
			continue

		}

		permanentRelays = append(permanentRelays, relay{
			URL: line,
			uri: uri,
		})
		if debug {
			log.Println("Adding permanent relay", line)
		}
	}
}

func createTestCertificate() tls.Certificate {
	tmpDir, err := ioutil.TempDir("", "relaypoolsrv")
	if err != nil {
		log.Fatal(err)
	}

	certFile, keyFile := filepath.Join(tmpDir, "cert.pem"), filepath.Join(tmpDir, "key.pem")
	cert, err := tlsutil.NewCertificate(certFile, keyFile, "relaypoolsrv", 3072)
	if err != nil {
		log.Fatalln("Failed to create test X509 key pair:", err)
	}

	return cert
}
