// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

//go:generate go run ../../script/genassets.go gui >auto/gui.go

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/oschwald/geoip2-golang"
	"github.com/syncthing/syncthing/cmd/strelaypoolsrv/auto"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"golang.org/x/time/rate"
)

type location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type relay struct {
	URL      string   `json:"url"`
	Location location `json:"location"`
	uri      *url.URL
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
	testCert       tls.Certificate
	listen         = ":80"
	dir            string
	evictionTime   = time.Hour
	debug          bool
	getLRUSize     = 10 << 10
	getLimitBurst  = 10
	getLimitAvg    = 1
	postLRUSize    = 1 << 10
	postLimitBurst = 2
	postLimitAvg   = 1
	getLimit       time.Duration
	postLimit      time.Duration
	permRelaysFile string
	ipHeader       string
	geoipPath      string
	proto          string

	getMut      = sync.NewRWMutex()
	getLRUCache *lru.Cache

	postMut      = sync.NewRWMutex()
	postLRUCache *lru.Cache

	requests = make(chan request, 10)

	mut             = sync.NewRWMutex()
	knownRelays     = make([]relay, 0)
	permanentRelays = make([]relay, 0)
	evictionTimers  = make(map[string]*time.Timer)
)

func main() {
	flag.StringVar(&listen, "listen", listen, "Listen address")
	flag.StringVar(&dir, "keys", dir, "Directory where http-cert.pem and http-key.pem is stored for TLS listening")
	flag.BoolVar(&debug, "debug", debug, "Enable debug output")
	flag.DurationVar(&evictionTime, "eviction", evictionTime, "After how long the relay is evicted")
	flag.IntVar(&getLRUSize, "get-limit-cache", getLRUSize, "Get request limiter cache size")
	flag.IntVar(&getLimitAvg, "get-limit-avg", 2, "Allowed average get request rate, per 10 s")
	flag.IntVar(&getLimitBurst, "get-limit-burst", getLimitBurst, "Allowed burst get requests")
	flag.IntVar(&postLRUSize, "post-limit-cache", postLRUSize, "Post request limiter cache size")
	flag.IntVar(&postLimitAvg, "post-limit-avg", 2, "Allowed average post request rate, per minute")
	flag.IntVar(&postLimitBurst, "post-limit-burst", postLimitBurst, "Allowed burst post requests")
	flag.StringVar(&permRelaysFile, "perm-relays", "", "Path to list of permanent relays")
	flag.StringVar(&ipHeader, "ip-header", "", "Name of header which holds clients ip:port. Only meaningful when running behind a reverse proxy.")
	flag.StringVar(&geoipPath, "geoip", "GeoLite2-City.mmdb", "Path to GeoLite2-City database")
	flag.StringVar(&proto, "protocol", "tcp", "Protocol used for listening. 'tcp' for IPv4 and IPv6, 'tcp4' for IPv4, 'tcp6' for IPv6")

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
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
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

		listener, err = tls.Listen(proto, listen, tlsCfg)
	} else {
		if debug {
			log.Println("Starting plain listener on", listen)
		}
		listener, err = net.Listen(proto, listen)
	}

	if err != nil {
		log.Fatalln("listen:", err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/", handleAssets)
	handler.HandleFunc("/endpoint", handleRequest)

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	err = srv.Serve(listener)
	if err != nil {
		log.Fatalln("serve:", err)
	}
}

func handleAssets(w http.ResponseWriter, r *http.Request) {
	assets := auto.Assets()
	path := r.URL.Path[1:]
	if path == "" {
		path = "index.html"
	}

	bs, ok := assets[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	mtype := mimeTypeForFile(path)
	if len(mtype) != 0 {
		w.Header().Set("Content-Type", mtype)
	}

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
	} else {
		// ungzip if browser not send gzip accepted header
		var gr *gzip.Reader
		gr, _ = gzip.NewReader(bytes.NewReader(bs))
		bs, _ = ioutil.ReadAll(gr)
		gr.Close()
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))

	w.Write(bs)
}

func mimeTypeForFile(file string) string {
	// We use a built in table of the common types since the system
	// TypeByExtension might be unreliable. But if we don't know, we delegate
	// to the system.
	ext := filepath.Ext(file)
	switch ext {
	case ".htm", ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".ttf":
		return "application/x-font-ttf"
	case ".woff":
		return "application/x-font-woff"
	case ".svg":
		return "image/svg+xml"
	default:
		return mime.TypeByExtension(ext)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if ipHeader != "" {
		r.RemoteAddr = r.Header.Get(ipHeader)
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	switch r.Method {
	case "GET":
		if limit(r.RemoteAddr, getLRUCache, getMut, getLimit, getLimitBurst) {
			w.WriteHeader(429)
			return
		}
		handleGetRequest(w, r)
	case "POST":
		if limit(r.RemoteAddr, postLRUCache, postMut, postLimit, postLimitBurst) {
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

	ip := net.ParseIP(host)
	// The client did not provide an IP address, use the IP address of the client.
	if ip == nil || ip.IsUnspecified() {
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
	newRelay.Location = getLocation(uri.Host)

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
		if !client.TestRelay(request.uri, []tls.Certificate{testCert}, time.Second, 2*time.Second, 3) {
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

		for i, current := range knownRelays {
			if current.uri.Host == request.relay.uri.Host {
				if debug {
					log.Println("Relay", request.relay, "already exists")
				}

				// Evict the old entry anyway, as configuration might have changed.
				last := len(knownRelays) - 1
				knownRelays[i] = knownRelays[last]
				knownRelays = knownRelays[:last]

				goto found
			}
		}

		if debug {
			log.Println("Adding new relay", request.relay)
		}

	found:

		knownRelays = append(knownRelays, request.relay)

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

func limit(addr string, cache *lru.Cache, lock sync.RWMutex, intv time.Duration, burst int) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	lock.RLock()
	bkt, ok := cache.Get(host)
	lock.RUnlock()
	if ok {
		bkt := bkt.(*rate.Limiter)
		if !bkt.Allow() {
			// Rate limit
			return true
		}
	} else {
		lock.Lock()
		cache.Add(host, rate.NewLimiter(rate.Every(intv), burst))
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
			URL:      line,
			Location: getLocation(uri.Host),
			uri:      uri,
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

func getLocation(host string) location {
	db, err := geoip2.Open(geoipPath)
	if err != nil {
		return location{}
	}
	defer db.Close()

	addr, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return location{}
	}

	city, err := db.City(addr.IP)
	if err != nil {
		return location{}
	}

	return location{
		Latitude:  city.Location.Latitude,
		Longitude: city.Location.Longitude,
	}
}
