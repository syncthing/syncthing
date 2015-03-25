// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"flag"
	"log"
	"net"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/thejerf/suture"
)

var (
	lruSize     = 10240
	limitAvg    = 5
	limitBurst  = 20
	dbConn      = getEnvDefault("DISCOSRV_DB", "postgres://user:password@localhost/discosrv")
	globalStats stats
)

func main() {
	const (
		cleanIntv = 1 * time.Hour
		statsIntv = 5 * time.Minute
	)

	var listen string

	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	flag.StringVar(&listen, "listen", ":22026", "Listen address")
	flag.IntVar(&lruSize, "limit-cache", lruSize, "Limiter cache entries")
	flag.IntVar(&limitAvg, "limit-avg", limitAvg, "Allowed average package rate, per 10 s")
	flag.IntVar(&limitBurst, "limit-burst", limitBurst, "Allowed burst size, packets")
	flag.Parse()

	addr, _ := net.ResolveUDPAddr("udp", listen)

	if !strings.Contains(dbConn, "sslmode=") {
		dbConn += "?sslmode=disable"
	}

	var err error
	db, err := sql.Open("postgres", dbConn)
	if err != nil {
		log.Fatalln("Setup:", err)
	}
	err = setupDB(db)
	if err != nil {
		log.Fatalln("Setup:", err)
	}

	prep, err := compileStatements(db)

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
