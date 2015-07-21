// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"flag"
	"log"
	"net"
	"os"
	"time"

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
)

func main() {
	const (
		cleanIntv = 1 * time.Hour
		statsIntv = 5 * time.Minute
	)

	var listen string

	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	flag.StringVar(&listen, "listen", ":22027", "Listen address")
	flag.IntVar(&lruSize, "limit-cache", lruSize, "Limiter cache entries")
	flag.IntVar(&limitAvg, "limit-avg", limitAvg, "Allowed average package rate, per 10 s")
	flag.IntVar(&limitBurst, "limit-burst", limitBurst, "Allowed burst size, packets")
	flag.StringVar(&statsFile, "stats-file", statsFile, "File to write periodic operation stats to")
	flag.StringVar(&backend, "db-backend", backend, "Database backend to use")
	flag.StringVar(&dsn, "db-dsn", dsn, "Database DSN")
	flag.Parse()

	addr, _ := net.ResolveUDPAddr("udp", listen)

	var err error
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
		addr: addr,
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
