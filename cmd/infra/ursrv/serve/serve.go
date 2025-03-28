// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package serve

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/geoip"
	"github.com/syncthing/syncthing/lib/s3"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

type CLI struct {
	Listen          string        `env:"UR_LISTEN" help:"Usage reporting & metrics endpoint listen address" default:"0.0.0.0:8080"`
	ListenInternal  string        `env:"UR_LISTEN_INTERNAL" help:"Internal metrics endpoint listen address" default:"0.0.0.0:8082"`
	GeoIPLicenseKey string        `env:"UR_GEOIP_LICENSE_KEY"`
	GeoIPAccountID  int           `env:"UR_GEOIP_ACCOUNT_ID"`
	DumpFile        string        `env:"UR_DUMP_FILE" default:"reports.jsons.gz"`
	DumpInterval    time.Duration `env:"UR_DUMP_INTERVAL" default:"5m"`

	S3Endpoint    string `name:"s3-endpoint" hidden:"true" env:"UR_S3_ENDPOINT"`
	S3Region      string `name:"s3-region" hidden:"true" env:"UR_S3_REGION"`
	S3Bucket      string `name:"s3-bucket" hidden:"true" env:"UR_S3_BUCKET"`
	S3AccessKeyID string `name:"s3-access-key-id" hidden:"true" env:"UR_S3_ACCESS_KEY_ID"`
	S3SecretKey   string `name:"s3-secret-key" hidden:"true" env:"UR_S3_SECRET_KEY"`
}

var (
	compilerRe         = regexp.MustCompile(`\(([A-Za-z0-9()., -]+) \w+-\w+(?:| android| default)\) ([\w@.-]+)`)
	knownDistributions = []distributionMatch{
		// Maps well known builders to the official distribution method that
		// they represent

		{regexp.MustCompile(`\steamcity@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sjenkins@build\.syncthing\.net`), "GitHub"},
		{regexp.MustCompile(`\sbuilder@github\.syncthing\.net`), "GitHub"},

		{regexp.MustCompile(`\sdeb@build\.syncthing\.net`), "APT"},
		{regexp.MustCompile(`\sdebian@github\.syncthing\.net`), "APT"},

		{regexp.MustCompile(`\sdocker@syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@build.syncthing\.net`), "Docker Hub"},
		{regexp.MustCompile(`\sdocker@github.syncthing\.net`), "Docker Hub"},

		{regexp.MustCompile(`\sandroid-builder@github\.syncthing\.net`), "Google Play"},
		{regexp.MustCompile(`\sandroid-.*teamcity@build\.syncthing\.net`), "Google Play"},

		{regexp.MustCompile(`\sandroid-.*vagrant@basebox-stretch64`), "F-Droid"},
		{regexp.MustCompile(`\svagrant@bullseye`), "F-Droid"},
		{regexp.MustCompile(`\svagrant@bookworm`), "F-Droid"},

		{regexp.MustCompile(`Anwender@NET2017`), "Syncthing-Fork (3rd party)"},

		{regexp.MustCompile(`\sbuilduser@(archlinux|svetlemodry)`), "Arch (3rd party)"},
		{regexp.MustCompile(`\ssyncthing@archlinux`), "Arch (3rd party)"},
		{regexp.MustCompile(`@debian`), "Debian (3rd party)"},
		{regexp.MustCompile(`@fedora`), "Fedora (3rd party)"},
		{regexp.MustCompile(`\sbrew@`), "Homebrew (3rd party)"},
		{regexp.MustCompile(`\sroot@buildkitsandbox`), "LinuxServer.io (3rd party)"},
		{regexp.MustCompile(`\sports@freebsd`), "FreeBSD (3rd party)"},
		{regexp.MustCompile(`\snix@nix`), "Nix (3rd party)"},
		{regexp.MustCompile(`.`), "Others"},
	}
)

type distributionMatch struct {
	matcher      *regexp.Regexp
	distribution string
}

func (cli *CLI) Run() error {
	slog.Info("Starting", "version", build.Version)

	// Listening

	urListener, err := net.Listen("tcp", cli.Listen)
	if err != nil {
		slog.Error("Failed to listen (usage reports)", "error", err)
		return err
	}
	slog.Info("Listening (usage reports)", "address", urListener.Addr())

	internalListener, err := net.Listen("tcp", cli.ListenInternal)
	if err != nil {
		slog.Error("Failed to listen (internal)", "error", err)
		return err
	}
	slog.Info("Listening (internal)", "address", internalListener.Addr())

	var geo *geoip.Provider
	if cli.GeoIPAccountID != 0 && cli.GeoIPLicenseKey != "" {
		geo, err = geoip.NewGeoLite2CityProvider(context.Background(), cli.GeoIPAccountID, cli.GeoIPLicenseKey, os.TempDir())
		if err != nil {
			slog.Error("Failed to load GeoIP", "error", err)
			return err
		}
		go geo.Serve(context.TODO())
	}

	// s3

	var s3sess *s3.Session
	if cli.S3Endpoint != "" {
		s3sess, err = s3.NewSession(cli.S3Endpoint, cli.S3Region, cli.S3Bucket, cli.S3AccessKeyID, cli.S3SecretKey)
		if err != nil {
			slog.Error("Failed to create S3 session", "error", err)
			return err
		}
	}

	if _, err := os.Stat(cli.DumpFile); err != nil && s3sess != nil {
		if err := cli.downloadDumpFile(s3sess); err != nil {
			slog.Error("Failed to download dump file", "error", err)
		}
	}

	// server

	srv := &server{
		geo:     geo,
		reports: xsync.NewMapOf[string, *contract.Report](),
	}

	if fd, err := os.Open(cli.DumpFile); err == nil {
		gr, err := gzip.NewReader(fd)
		if err == nil {
			srv.load(gr)
		}
		fd.Close()
	}

	go func() {
		for range time.Tick(cli.DumpInterval) {
			if err := cli.saveDumpFile(srv, s3sess); err != nil {
				slog.Error("Failed to write dump file", "error", err)
			}
		}
	}()

	// The internal metrics endpoint just serves metrics about what the
	// server is doing.

	http.Handle("/metrics", promhttp.Handler())

	internalSrv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	go internalSrv.Serve(internalListener)

	// New external metrics endpoint accepts reports from clients and serves
	// aggregated usage reporting metrics.

	ms := newMetricsSet(srv)
	reg := prometheus.NewRegistry()
	reg.MustRegister(ms)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/newdata", srv.handleNewData)
	mux.HandleFunc("/ping", srv.handlePing)

	metricsSrv := http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		Handler:      mux,
	}

	slog.Info("Ready to serve")
	return metricsSrv.Serve(urListener)
}

func (cli *CLI) downloadDumpFile(s3sess *s3.Session) error {
	latestKey, err := s3sess.LatestKey()
	if err != nil {
		return fmt.Errorf("list latest S3 key: %w", err)
	}
	fd, err := os.Create(cli.DumpFile)
	if err != nil {
		return fmt.Errorf("create dump file: %w", err)
	}
	if err := s3sess.Download(fd, latestKey); err != nil {
		_ = fd.Close()
		return fmt.Errorf("download dump file: %w", err)
	}
	if err := fd.Close(); err != nil {
		return fmt.Errorf("close dump file: %w", err)
	}
	slog.Info("Dump file downloaded", "key", latestKey)
	return nil
}

func (cli *CLI) saveDumpFile(srv *server, s3sess *s3.Session) error {
	fd, err := os.Create(cli.DumpFile + ".tmp")
	if err != nil {
		return fmt.Errorf("creating dump file: %w", err)
	}
	gw := gzip.NewWriter(fd)
	if err := srv.save(gw); err != nil {
		return fmt.Errorf("saving dump file: %w", err)
	}
	if err := gw.Close(); err != nil {
		fd.Close()
		return fmt.Errorf("closing gzip writer: %w", err)
	}
	if err := fd.Close(); err != nil {
		return fmt.Errorf("closing dump file: %w", err)
	}
	if err := os.Rename(cli.DumpFile+".tmp", cli.DumpFile); err != nil {
		return fmt.Errorf("renaming dump file: %w", err)
	}
	slog.Info("Dump file saved")

	if s3sess != nil {
		key := fmt.Sprintf("reports-%s.jsons.gz", time.Now().UTC().Format("2006-01-02"))
		fd, err := os.Open(cli.DumpFile)
		if err != nil {
			return fmt.Errorf("opening dump file: %w", err)
		}
		if err := s3sess.Upload(fd, key); err != nil {
			return fmt.Errorf("uploading dump file: %w", err)
		}
		_ = fd.Close()
		slog.Info("Dump file uploaded")
	}

	return nil
}

type server struct {
	geo     *geoip.Provider
	reports *xsync.MapOf[string, *contract.Report]
}

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
}

func (s *server) handleNewData(w http.ResponseWriter, r *http.Request) {
	result := "fail"
	defer func() {
		// result is "accept" (new report), "replace" (existing report) or
		// "fail"
		metricReportsTotal.WithLabelValues(result).Inc()
	}()

	defer r.Body.Close()

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	addr := r.Header.Get("X-Forwarded-For")
	if addr != "" {
		addr = strings.Split(addr, ", ")[0]
	} else {
		addr = r.RemoteAddr
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}

	log := slog.With("addr", addr)

	if net.ParseIP(addr) == nil {
		addr = ""
	}

	var rep contract.Report

	lr := &io.LimitedReader{R: r.Body, N: 40 * 1024}
	bs, _ := io.ReadAll(lr)
	if err := json.Unmarshal(bs, &rep); err != nil {
		log.Error("Failed to decode JSON", "error", err)
		http.Error(w, "JSON Decode Error", http.StatusInternalServerError)
		return
	}

	rep.Received = time.Now()
	rep.Date = rep.Received.UTC().Format("20060102")
	rep.Address = addr

	if err := rep.Validate(); err != nil {
		log.Error("Failed to validate report", "error", err)
		http.Error(w, "Validation Error", http.StatusInternalServerError)
		return
	}

	if s.addReport(&rep) {
		result = "replace"
	} else {
		result = "accept"
	}
}

func (s *server) addReport(rep *contract.Report) bool {
	if s.geo != nil {
		if ip := net.ParseIP(rep.Address); ip != nil {
			if city, err := s.geo.City(ip); err == nil {
				rep.Country = city.Country.Names["en"]
				rep.CountryCode = city.Country.IsoCode
			}
		}
	}
	if rep.Country == "" {
		rep.Country = "Unknown"
	}
	if rep.CountryCode == "" {
		rep.CountryCode = "ZZ"
	}

	rep.Version = transformVersion(rep.Version)
	if strings.Contains(rep.Version, ".") {
		split := strings.SplitN(rep.Version, ".", 3)
		if len(split) == 3 {
			rep.MajorVersion = strings.Join(split[:2], ".")
		}
	}
	rep.OS, rep.Arch, _ = strings.Cut(rep.Platform, "-")

	if m := compilerRe.FindStringSubmatch(rep.LongVersion); len(m) == 3 {
		rep.Compiler = m[1]
		rep.Builder = m[2]
	}
	for _, d := range knownDistributions {
		if d.matcher.MatchString(rep.LongVersion) {
			rep.Distribution = d.distribution
			break
		}
	}

	_, loaded := s.reports.LoadAndStore(rep.UniqueID, rep)
	return loaded
}

func (s *server) save(w io.Writer) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	var err error
	s.reports.Range(func(k string, v *contract.Report) bool {
		err = enc.Encode(v)
		return err == nil
	})
	if err != nil {
		return err
	}
	return bw.Flush()
}

func (s *server) load(r io.Reader) {
	dec := json.NewDecoder(r)
	s.reports.Clear()
	for {
		var rep contract.Report
		if err := dec.Decode(&rep); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			slog.Error("Failed to load record", "error", err)
			break
		}
		s.addReport(&rep)
	}
	slog.Info("Loaded reports", "count", s.reports.Size())
}

var (
	plusRe  = regexp.MustCompile(`(\+.*|[.-]dev\..*)$`)
	plusStr = "-dev"
)

// transformVersion returns a version number formatted correctly, with all
// development versions aggregated into one.
func transformVersion(v string) string {
	if v == "unknown-dev" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	v = plusRe.ReplaceAllString(v, plusStr)

	return v
}
