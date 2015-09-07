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

	"github.com/kardianos/osext"
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
	binDir       string
	testCert     []tls.Certificate
	listen       string
	dir          string
	evictionTime time.Duration
	debug        bool

	requests = make(chan request, 10)

	mut             sync.RWMutex           = sync.NewRWMutex()
	knownRelays     []relay                = make([]relay, 0)
	permanentRelays []relay                = make([]relay, 0)
	evictionTimers  map[string]*time.Timer = make(map[string]*time.Timer)
)

func main() {
	flag.StringVar(&listen, "listen", ":80", "Listen address")
	flag.StringVar(&dir, "keys", "", "Directory where http-cert.pem and http-key.pem is stored for TLS listening")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.DurationVar(&evictionTime, "eviction", time.Hour, "After how long the relay is evicted")

	flag.Parse()

	var listener net.Listener
	var err error

	binDir, err = osext.ExecutableFolder()
	if err != nil {
		log.Fatalln("Failed to locate executable directory")
	}

	loadPermanentRelays()
	loadOrCreateTestCertificate()
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

	// The client did not provide an IP address, work it out.
	if host == "" {
		rhost, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			if debug {
				log.Println("Failed to split remote address", r.RemoteAddr)
			}
			http.Error(w, err.Error(), 500)
			return
		}
		uri.Host = net.JoinHostPort(rhost, port)
		newRelay.URL = uri.String()
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

func loadPermanentRelays() {
	path, err := osext.ExecutableFolder()
	if err != nil {
		log.Println("Failed to locate executable directory")
		return
	}

	content, err := ioutil.ReadFile(filepath.Join(path, "relays"))
	if err != nil {
		return
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

func loadOrCreateTestCertificate() {
	certFile, keyFile := filepath.Join(binDir, "cert.pem"), filepath.Join(binDir, "key.pem")
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err == nil {
		testCert = []tls.Certificate{cert}
		return
	}

	cert, err = tlsutil.NewCertificate(certFile, keyFile, "relaypoolsrv", 3072)
	if err != nil {
		log.Fatalln("Failed to create test X509 key pair:", err)
	}
	testCert = []tls.Certificate{cert}
}

func requestProcessor() {
	for request := range requests {
		if debug {
			log.Println("Request for", request.relay)
		}
		if !client.TestRelay(request.uri, testCert, 250*time.Millisecond, 4) {
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
