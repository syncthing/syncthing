// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/syncthing/syncthing/lib/geoip"
	"github.com/syncthing/syncthing/lib/httpcache"
	"github.com/syncthing/syncthing/lib/protocol"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncthing/syncthing/cmd/strelaypoolsrv/auto"
	"github.com/syncthing/syncthing/lib/assets"
	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/relay/client"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

type location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Continent string  `json:"continent"`
}

type relay struct {
	URL            string   `json:"url"`
	Location       location `json:"location"`
	uri            *url.URL
	Stats          *stats    `json:"stats"`
	StatsRetrieved time.Time `json:"statsRetrieved"`
}

type stats struct {
	StartTime          time.Time `json:"startTime"`
	UptimeSeconds      int       `json:"uptimeSeconds"`
	PendingSessionKeys int       `json:"numPendingSessionKeys"`
	ActiveSessions     int       `json:"numActiveSessions"`
	Connections        int       `json:"numConnections"`
	Proxies            int       `json:"numProxies"`
	BytesProxied       int       `json:"bytesProxied"`
	GoVersion          string    `json:"goVersion"`
	GoOS               string    `json:"goOS"`
	GoArch             string    `json:"goArch"`
	GoMaxProcs         int       `json:"goMaxProcs"`
	GoRoutines         int       `json:"goNumRoutine"`
	Rates              []int64   `json:"kbps10s1m5m15m30m60m"`
	Options            struct {
		NetworkTimeout int      `json:"network-timeout"`
		PintInterval   int      `json:"ping-interval"`
		MessageTimeout int      `json:"message-timeout"`
		SessionRate    int      `json:"per-session-rate"`
		GlobalRate     int      `json:"global-rate"`
		Pools          []string `json:"pools"`
		ProvidedBy     string   `json:"provided-by"`
	} `json:"options"`
}

func (r relay) String() string {
	return r.URL
}

type request struct {
	relay      *relay
	result     chan result
	queueTimer *prometheus.Timer
}

type result struct {
	err      error
	eviction time.Duration
}

var (
	testCert          tls.Certificate
	knownRelaysFile   = filepath.Join(os.TempDir(), "strelaypoolsrv_known_relays")
	listen            = ":80"
	dir               string
	evictionTime      = time.Hour
	debug             bool
	permRelaysFile    string
	ipHeader          string
	proto             string
	statsRefresh      = time.Minute
	requestQueueLen   = 64
	requestProcessors = 8
	geoipLicenseKey   string

	requests chan request

	mut             = sync.NewRWMutex()
	knownRelays     = make([]*relay, 0)
	permanentRelays = make([]*relay, 0)
	evictionTimers  = make(map[string]*time.Timer)
	globalBlocklist = newErrorTracker(1000)
)

const (
	httpStatusEnhanceYourCalm = 429
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Lshortfile)

	flag.StringVar(&listen, "listen", listen, "Listen address")
	flag.StringVar(&dir, "keys", dir, "Directory where http-cert.pem and http-key.pem is stored for TLS listening")
	flag.BoolVar(&debug, "debug", debug, "Enable debug output")
	flag.DurationVar(&evictionTime, "eviction", evictionTime, "After how long the relay is evicted")
	flag.StringVar(&permRelaysFile, "perm-relays", "", "Path to list of permanent relays")
	flag.StringVar(&knownRelaysFile, "known-relays", knownRelaysFile, "Path to list of current relays")
	flag.StringVar(&ipHeader, "ip-header", "", "Name of header which holds clients ip:port. Only meaningful when running behind a reverse proxy.")
	flag.StringVar(&proto, "protocol", "tcp", "Protocol used for listening. 'tcp' for IPv4 and IPv6, 'tcp4' for IPv4, 'tcp6' for IPv6")
	flag.DurationVar(&statsRefresh, "stats-refresh", statsRefresh, "Interval at which to refresh relay stats")
	flag.IntVar(&requestQueueLen, "request-queue", requestQueueLen, "Queue length for incoming test requests")
	flag.IntVar(&requestProcessors, "request-processors", requestProcessors, "Number of request processor routines")
	flag.StringVar(&geoipLicenseKey, "geoip-license-key", "", "License key for GeoIP database")

	flag.Parse()

	requests = make(chan request, requestQueueLen)
	geoip := geoip.NewGeoLite2CityProvider(geoipLicenseKey, os.TempDir())

	var listener net.Listener
	var err error

	if permRelaysFile != "" {
		permanentRelays = loadRelays(permRelaysFile, geoip)
	}

	testCert = createTestCertificate()

	for i := 0; i < requestProcessors; i++ {
		go requestProcessor(geoip)
	}

	// Load relays from cache in the background.
	// Load them in a serial fashion to make sure any genuine requests
	// are not dropped.
	go func() {
		for _, relay := range loadRelays(knownRelaysFile, geoip) {
			resultChan := make(chan result)
			requests <- request{relay, resultChan, nil}
			result := <-resultChan
			if result.err != nil {
				relayTestsTotal.WithLabelValues("failed").Inc()
			} else {
				relayTestsTotal.WithLabelValues("success").Inc()
			}
		}
		// Run the the stats refresher once the relays are loaded.
		statsRefresher(statsRefresh)
	}()

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
			ClientAuth:   tls.RequestClientCert,
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
	handler.Handle("/endpoint", httpcache.SinglePath(http.HandlerFunc(handleRequest), 15*time.Second))
	handler.HandleFunc("/metrics", handleMetrics)

	srv := http.Server{
		Handler:     handler,
		ReadTimeout: 10 * time.Second,
	}

	err = srv.Serve(listener)
	if err != nil {
		log.Fatalln("serve:", err)
	}
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metricsRequestsSeconds)
	// Acquire the mutex just to make sure we're not caught mid-way stats collection
	mut.RLock()
	promhttp.Handler().ServeHTTP(w, r)
	mut.RUnlock()
	timer.ObserveDuration()
}

func handleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")

	path := r.URL.Path[1:]
	if path == "" {
		path = "index.html"
	}

	as, ok := auto.Assets()[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	assets.Serve(w, r, as)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(apiRequestsSeconds.WithLabelValues(r.Method))

	w = NewLoggingResponseWriter(w)
	defer func() {
		timer.ObserveDuration()
		lw := w.(*loggingResponseWriter)
		apiRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(lw.statusCode)).Inc()
	}()

	if ipHeader != "" {
		hdr := r.Header.Get(ipHeader)
		fields := strings.Split(hdr, ",")
		if len(fields) > 0 {
			r.RemoteAddr = strings.TrimSpace(fields[len(fields)-1])
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	switch r.Method {
	case "GET":
		handleGetRequest(w, r)
	case "POST":
		handlePostRequest(w, r)
	default:
		if debug {
			log.Println("Unhandled HTTP method", r.Method)
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetRequest(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")

	mut.RLock()
	relays := make([]*relay, len(permanentRelays)+len(knownRelays))
	n := copy(relays, permanentRelays)
	copy(relays[n:], knownRelays)
	mut.RUnlock()

	// Shuffle
	rand.Shuffle(relays)

	_ = json.NewEncoder(rw).Encode(map[string][]*relay{
		"relays": relays,
	})
}

func handlePostRequest(w http.ResponseWriter, r *http.Request) {
	// Get the IP address of the client
	rhost := r.RemoteAddr
	if host, _, err := net.SplitHostPort(rhost); err == nil {
		rhost = host
	}

	// Check the black list. A client is blacklisted if their last 10
	// attempts to join have all failed. The "Unauthorized" status return
	// causes strelaysrv to cease attempting to join.
	if globalBlocklist.IsBlocked(rhost) {
		log.Println("Rejected blocked client", rhost)
		http.Error(w, "Too many errors", http.StatusUnauthorized)
		globalBlocklist.ClearErrors(rhost)
		return
	}

	var relayCert *x509.Certificate
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		relayCert = r.TLS.PeerCertificates[0]
		log.Printf("Got TLS cert from relay server")
	}

	var newRelay relay
	err := json.NewDecoder(r.Body).Decode(&newRelay)
	r.Body.Close()

	if err != nil {
		if debug {
			log.Println("Failed to parse payload")
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uri, err := url.Parse(newRelay.URL)
	if err != nil {
		if debug {
			log.Println("Failed to parse URI", newRelay.URL)
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Canonicalize the URL. In particular, parse and re-encode the query
	// string so that it's guaranteed to be valid.
	uri.RawQuery = uri.Query().Encode()
	newRelay.URL = uri.String()

	if relayCert != nil {
		advertisedId := uri.Query().Get("id")
		idFromCert := protocol.NewDeviceID(relayCert.Raw).String()
		if advertisedId != idFromCert {
			log.Println("Warning: Relay server requested to join with an ID different from the join request, rejecting")
			http.Error(w, "mismatched advertised id and join request cert", http.StatusBadRequest)
			return
		}
	}

	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil {
		if debug {
			log.Println("Failed to split URI", newRelay.URL)
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ip := net.ParseIP(host)
	// The client did not provide an IP address, use the IP address of the client.
	if ip == nil || ip.IsUnspecified() {
		uri.Host = net.JoinHostPort(rhost, port)
		newRelay.URL = uri.String()
	} else if host != rhost && relayCert == nil {
		if debug {
			log.Println("IP address advertised does not match client IP address", r.RemoteAddr, uri)
		}
		http.Error(w, fmt.Sprintf("IP advertised %s does not match client IP %s", host, rhost), http.StatusUnauthorized)
		return
	}

	newRelay.uri = uri

	for _, current := range permanentRelays {
		if current.uri.Host == newRelay.uri.Host {
			if debug {
				log.Println("Asked to add a relay", newRelay, "which exists in permanent list")
			}
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
	}

	reschan := make(chan result)

	select {
	case requests <- request{&newRelay, reschan, prometheus.NewTimer(relayTestActionsSeconds.WithLabelValues("queue"))}:
		result := <-reschan
		if result.err != nil {
			log.Println("Join from", r.RemoteAddr, "failed:", result.err)
			globalBlocklist.AddError(rhost)
			relayTestsTotal.WithLabelValues("failed").Inc()
			http.Error(w, result.err.Error(), http.StatusBadRequest)
			return
		}
		log.Println("Join from", r.RemoteAddr, "succeeded")
		globalBlocklist.ClearErrors(rhost)
		relayTestsTotal.WithLabelValues("success").Inc()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]time.Duration{
			"evictionIn": result.eviction,
		})

	default:
		relayTestsTotal.WithLabelValues("dropped").Inc()
		if debug {
			log.Println("Dropping request")
		}
		w.WriteHeader(httpStatusEnhanceYourCalm)
	}
}

func requestProcessor(geoip *geoip.Provider) {
	for request := range requests {
		if request.queueTimer != nil {
			request.queueTimer.ObserveDuration()
		}

		timer := prometheus.NewTimer(relayTestActionsSeconds.WithLabelValues("test"))
		handleRelayTest(request, geoip)
		timer.ObserveDuration()
	}
}

func handleRelayTest(request request, geoip *geoip.Provider) {
	if debug {
		log.Println("Request for", request.relay)
	}
	if err := client.TestRelay(context.TODO(), request.relay.uri, []tls.Certificate{testCert}, time.Second, 2*time.Second, 3); err != nil {
		if debug {
			log.Println("Test for relay", request.relay, "failed:", err)
		}
		request.result <- result{err, 0}
		return
	}

	stats := fetchStats(request.relay)
	location := getLocation(request.relay.uri.Host, geoip)

	mut.Lock()
	if stats != nil {
		updateMetrics(request.relay.uri.Host, *stats, location)
	}
	request.relay.Stats = stats
	request.relay.StatsRetrieved = time.Now().Truncate(time.Second)
	request.relay.Location = location

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

	if err := saveRelays(knownRelaysFile, knownRelays); err != nil {
		log.Println("Failed to write known relays: " + err.Error())
	}

	request.result <- result{nil, evictionTime}
}

func evict(relay *relay) func() {
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
				deleteMetrics(current.uri.Host)
			}
		}
		delete(evictionTimers, relay.uri.Host)
	}
}

func loadRelays(file string, geoip *geoip.Provider) []*relay {
	content, err := os.ReadFile(file)
	if err != nil {
		log.Println("Failed to load relays: " + err.Error())
		return nil
	}

	var relays []*relay
	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			continue
		}

		uri, err := url.Parse(line)
		if err != nil {
			if debug {
				log.Println("Skipping relay", line, "due to parse error", err)
			}
			continue

		}

		relays = append(relays, &relay{
			URL:      line,
			Location: getLocation(uri.Host, geoip),
			uri:      uri,
		})
		if debug {
			log.Println("Adding relay", line)
		}
	}
	return relays
}

func saveRelays(file string, relays []*relay) error {
	var content string
	for _, relay := range relays {
		content += relay.uri.String() + "\n"
	}
	return os.WriteFile(file, []byte(content), 0o777)
}

func createTestCertificate() tls.Certificate {
	tmpDir, err := os.MkdirTemp("", "relaypoolsrv")
	if err != nil {
		log.Fatal(err)
	}

	certFile, keyFile := filepath.Join(tmpDir, "cert.pem"), filepath.Join(tmpDir, "key.pem")
	cert, err := tlsutil.NewCertificate(certFile, keyFile, "relaypoolsrv", 20*365)
	if err != nil {
		log.Fatalln("Failed to create test X509 key pair:", err)
	}

	return cert
}

func getLocation(host string, geoip *geoip.Provider) location {
	timer := prometheus.NewTimer(locationLookupSeconds)
	defer timer.ObserveDuration()

	addr, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		return location{}
	}

	city, err := geoip.City(addr.IP)
	if err != nil {
		return location{}
	}

	return location{
		Longitude: city.Location.Longitude,
		Latitude:  city.Location.Latitude,
		City:      city.City.Names["en"],
		Country:   city.Country.IsoCode,
		Continent: city.Continent.Code,
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

type errorTracker struct {
	errors *lru.TwoQueueCache[string, *errorCounter]
}

type errorCounter struct {
	count atomic.Int32
}

func newErrorTracker(size int) *errorTracker {
	cache, err := lru.New2Q[string, *errorCounter](size)
	if err != nil {
		panic(err)
	}
	return &errorTracker{
		errors: cache,
	}
}

func (b *errorTracker) AddError(host string) {
	entry, ok := b.errors.Get(host)
	if !ok {
		entry = &errorCounter{}
		b.errors.Add(host, entry)
	}
	c := entry.count.Add(1)
	log.Printf("Error count for %s is now %d", host, c)
}

func (b *errorTracker) ClearErrors(host string) {
	b.errors.Remove(host)
}

func (b *errorTracker) IsBlocked(host string) bool {
	if be, ok := b.errors.Get(host); ok {
		return be.count.Load() > 10
	}
	return false
}
