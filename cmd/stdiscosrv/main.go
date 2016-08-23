// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"github.com/thejerf/suture"
)

const (
	minNegCache  = 60        // seconds
	maxNegCache  = 3600      // seconds
	maxDeviceAge = 7 * 86400 // one week, in seconds
)

var (
	Version    string
	BuildStamp string
	BuildUser  string
	BuildHost  string

	BuildDate   time.Time
	LongVersion string
)

func init() {
	stamp, _ := strconv.Atoi(BuildStamp)
	BuildDate = time.Unix(int64(stamp), 0)

	date := BuildDate.UTC().Format("2006-01-02 15:04:05 MST")
	LongVersion = fmt.Sprintf(`stdiscosrv %s (%s %s-%s) %s@%s %s`, Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, BuildUser, BuildHost, date)
}

var (
	lruSize     = 10240
	limitAvg    = 5
	limitBurst  = 20
	globalStats stats
	statsFile   string
	backend     = "ql"
	dsn         = getEnvDefault("STDISCOSRV_DB_DSN", "memory://stdiscosrv")
	certFile    = "cert.pem"
	keyFile     = "key.pem"
	debug       = false
	useHTTP     = false
)

func main() {
	const (
		cleanIntv = 1 * time.Hour
		statsIntv = 5 * time.Minute
	)

	var listen string

	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	flag.StringVar(&listen, "listen", ":8443", "Listen address")
	flag.IntVar(&lruSize, "limit-cache", lruSize, "Limiter cache entries")
	flag.IntVar(&limitAvg, "limit-avg", limitAvg, "Allowed average package rate, per 10 s")
	flag.IntVar(&limitBurst, "limit-burst", limitBurst, "Allowed burst size, packets")
	flag.StringVar(&statsFile, "stats-file", statsFile, "File to write periodic operation stats to")
	flag.StringVar(&backend, "db-backend", backend, "Database backend to use")
	flag.StringVar(&dsn, "db-dsn", dsn, "Database DSN")
	flag.StringVar(&certFile, "cert", certFile, "Certificate file")
	flag.StringVar(&keyFile, "key", keyFile, "Key file")
	flag.BoolVar(&debug, "debug", debug, "Debug")
	flag.BoolVar(&useHTTP, "http", useHTTP, "Listen on HTTP (behind an HTTPS proxy)")
	flag.Parse()

	log.Println(LongVersion)

	var cert tls.Certificate
	var err error
	if !useHTTP {
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Println("Failed to load keypair. Generating one, this might take a while...")
			cert, err = tlsutil.NewCertificate(certFile, keyFile, "stdiscosrv", 3072)
			if err != nil {
				log.Fatalln("Failed to generate X509 key pair:", err)
			}
		}

		devID := protocol.NewDeviceID(cert.Certificate[0])
		log.Println("Server device ID is", devID)
	}

	db, err := sql.Open(backend, dsn)
	if err != nil {
		log.Fatalln("sql.Open:", err)
	}
	prep, err := setup(backend, db)
	if err != nil {
		log.Fatalln("Setup:", err)
	}

	main := suture.NewSimple("main")

	main.Add(&querysrv{
		addr: listen,
		cert: cert,
		db:   db,
		prep: prep,
	})

	main.Add(&cleansrv{
		intv: cleanIntv,
		db:   db,
		prep: prep,
	})

	main.Add(&statssrv{
		intv: statsIntv,
		file: statsFile,
		db:   db,
	})

	globalStats.Reset()
	main.Serve()
}

func getEnvDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func next(intv time.Duration) time.Duration {
	t0 := time.Now()
	t1 := t0.Add(intv).Truncate(intv)
	return t1.Sub(t0)
}
