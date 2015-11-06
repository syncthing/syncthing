// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"log"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/thejerf/suture"
)

var (
	lruSize     = 10240
	limitAvg    = 5
	limitBurst  = 20
	globalStats stats
	statsFile   string
	backend     = "ql"
	dsn         = getEnvDefault("DISCOSRV_DB_DSN", "memory://discosrv")
	certFile    = "cert.pem"
	keyFile     = "key.pem"
	debug       = false
	useHttp     = false
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
	flag.BoolVar(&useHttp, "http", useHttp, "Listen on HTTP (behind an HTTPS proxy)")
	flag.Parse()

	var cert tls.Certificate
	var err error
	if !useHttp {
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			log.Fatalln("Failed to load X509 key pair:", err)
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
