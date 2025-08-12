// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/syncthing/syncthing/internal/slogutil"
	_ "github.com/syncthing/syncthing/lib/automaxprocs"
	"github.com/syncthing/syncthing/lib/httpcache"
	"github.com/syncthing/syncthing/lib/upgrade"
)

type cli struct {
	Listen        string        `default:":8080" help:"Listen address"`
	MetricsListen string        `default:":8082" help:"Listen address for metrics"`
	URL           string        `short:"u" default:"https://api.github.com/repos/syncthing/syncthing/releases?per_page=25" help:"GitHub releases url"`
	Forward       []string      `short:"f" help:"Forwarded pages, format: /path->https://example/com/url"`
	CacheTime     time.Duration `default:"15m" help:"Cache time"`
}

func main() {
	var params cli
	kong.Parse(&params)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := server(&params); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func server(params *cli) error {
	if params.MetricsListen != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metricsListen, err := net.Listen("tcp", params.MetricsListen)
		if err != nil {
			return fmt.Errorf("metrics: %w", err)
		}
		slog.Info("Metrics listener started", slogutil.Address(params.MetricsListen))
		go func() {
			if err := http.Serve(metricsListen, mux); err != nil {
				slog.Warn("Metrics server returned", slogutil.Error(err))
			}
		}()
	}

	cache := &cachedReleases{url: params.URL}
	if err := cache.Update(context.Background()); err != nil {
		return fmt.Errorf("initial cache update: %w", err)
	} else {
		slog.Info("Initial cache update done")
	}

	go func() {
		for range time.NewTicker(params.CacheTime).C {
			slog.Info("Refreshing cached releases", slogutil.URI(params.URL))
			if err := cache.Update(context.Background()); err != nil {
				slog.Error("Failed to refresh cached releases", slogutil.URI(params.URL), slogutil.Error(err))
			}
		}
	}()

	ghRels := &githubReleases{cache: cache}
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", ghRels.servePing)
	mux.HandleFunc("/meta.json", ghRels.serveReleases)

	for _, fwd := range params.Forward {
		path, url, ok := strings.Cut(fwd, "->")
		if !ok {
			return fmt.Errorf("invalid forward: %q", fwd)
		}
		slog.Info("Forwarding", "from", path, "to", url)
		name := strings.ReplaceAll(path, "/", "_")
		mux.Handle(path, httpcache.SinglePath(&proxy{name: name, url: url}, params.CacheTime))
	}

	srv := &http.Server{
		Addr:         params.Listen,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	srv.SetKeepAlivesEnabled(false)

	srvListener, err := net.Listen("tcp", params.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	slog.Info("Main listener started", slogutil.Address(params.Listen))

	return srv.Serve(srvListener)
}

type githubReleases struct {
	cache *cachedReleases
}

func (p *githubReleases) servePing(w http.ResponseWriter, req *http.Request) {
	rels := p.cache.Releases()

	if len(rels) == 0 {
		http.Error(w, "No releases available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Syncthing-Num-Releases", strconv.Itoa(len(rels)))
	w.WriteHeader(http.StatusOK)
}

func (p *githubReleases) serveReleases(w http.ResponseWriter, req *http.Request) {
	rels := p.cache.Releases()

	ua := req.Header.Get("User-Agent")
	osv := req.Header.Get("Syncthing-Os-Version")
	if ua != "" && osv != "" {
		// We should determine the compatibility of the releases.
		rels = filterForCompatibility(rels, ua, osv)
	} else {
		metricFilterCalls.WithLabelValues("no-ua-or-osversion").Inc()
	}

	rels = filterForLatest(rels)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Cache-Control", "public, max-age=900")
	w.Header().Set("Vary", "User-Agent, Syncthing-Os-Version")
	_ = json.NewEncoder(w).Encode(rels)

	metricUpgradeChecks.Inc()
}

type proxy struct {
	name string
	url  string
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req, err := http.NewRequestWithContext(req.Context(), http.MethodGet, p.url, nil)
	if err != nil {
		metricHTTPRequests.WithLabelValues(p.name, "error").Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		metricHTTPRequests.WithLabelValues(p.name, "error").Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	metricHTTPRequests.WithLabelValues(p.name, "success").Inc()

	ct := resp.Header.Get("Content-Type")
	w.Header().Set("Content-Type", ct)
	if resp.StatusCode == http.StatusOK {
		w.Header().Set("Cache-Control", "public, max-age=900")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
	}
	w.WriteHeader(resp.StatusCode)
	if strings.HasPrefix(ct, "application/json") {
		// Special JSON handling; clean it up a bit.
		var v interface{}
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	} else {
		_, _ = io.Copy(w, resp.Body)
	}
}

// filterForLatest returns the latest stable and prerelease only. If the
// stable version is newer (comes first in the list) there is no need to go
// looking for a prerelease at all.
func filterForLatest(rels []upgrade.Release) []upgrade.Release {
	var filtered []upgrade.Release
	havePre := make(map[string]bool)
	haveStable := make(map[string]bool)
	for _, rel := range rels {
		major, _, _ := strings.Cut(rel.Tag, ".")
		if !rel.Prerelease && !haveStable[major] {
			// Remember the first non-pre for each major
			filtered = append(filtered, rel)
			haveStable[major] = true
			continue
		}
		if rel.Prerelease && !havePre[major] && !haveStable[major] {
			// We remember the first prerelease we find, unless we've
			// already found a non-pre of the same major.
			filtered = append(filtered, rel)
			havePre[major] = true
		}
	}
	return filtered
}

var userAgentOSArchExp = regexp.MustCompile(`^syncthing.*\(.+ (\w+)-(\w+)\)$`)

func filterForCompatibility(rels []upgrade.Release, ua, osv string) []upgrade.Release {
	osArch := userAgentOSArchExp.FindStringSubmatch(ua)
	if len(osArch) != 3 {
		metricFilterCalls.WithLabelValues("bad-os-arch").Inc()
		return rels
	}
	os := osArch[1]

	var filtered []upgrade.Release
	for _, rel := range rels {
		if rel.Compatibility == nil {
			// No requirements means it's compatible with everything.
			filtered = append(filtered, rel)
			continue
		}

		req, ok := rel.Compatibility.Requirements[os]
		if !ok {
			// No entry for the current OS means it's compatible.
			filtered = append(filtered, rel)
			continue
		}

		if upgrade.CompareVersions(osv, req) >= 0 {
			filtered = append(filtered, rel)
			continue
		}
	}

	if len(filtered) != len(rels) {
		metricFilterCalls.WithLabelValues("filtered").Inc()
	} else {
		metricFilterCalls.WithLabelValues("unchanged").Inc()
	}

	return filtered
}

type cachedReleases struct {
	url                  string
	mut                  sync.RWMutex
	current              []upgrade.Release
	latestRel, latestPre string
}

func (c *cachedReleases) Releases() []upgrade.Release {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.current
}

func (c *cachedReleases) Update(ctx context.Context) error {
	rels, err := fetchGithubReleases(ctx, c.url)
	if err != nil {
		return err
	}
	latestRel, latestPre := "", ""
	for _, rel := range rels {
		if !rel.Prerelease && latestRel == "" {
			latestRel = rel.Tag
		}
		if rel.Prerelease && latestPre == "" {
			latestPre = rel.Tag
		}
		if latestRel != "" && latestPre != "" {
			break
		}
	}
	c.mut.Lock()
	c.current = rels
	if latestRel != c.latestRel || latestPre != c.latestPre {
		metricLatestReleaseInfo.DeleteLabelValues(c.latestRel, c.latestPre)
		metricLatestReleaseInfo.WithLabelValues(latestRel, latestPre).Set(1)
		c.latestRel = latestRel
		c.latestPre = latestPre
	}
	c.mut.Unlock()
	return nil
}

func fetchGithubReleases(ctx context.Context, url string) ([]upgrade.Release, error) {
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, url, nil)
	if err != nil {
		metricHTTPRequests.WithLabelValues("github-releases", "error").Inc()
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		metricHTTPRequests.WithLabelValues("github-releases", "error").Inc()
		return nil, err
	}
	defer resp.Body.Close()
	var rels []upgrade.Release
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		metricHTTPRequests.WithLabelValues("github-releases", "error").Inc()
		return nil, err
	}
	metricHTTPRequests.WithLabelValues("github-releases", "success").Inc()

	// Move the URL used for browser downloads to the URL field, and remove
	// the browser URL field. This avoids going via the GitHub API for
	// downloads, since Syncthing uses the URL field.
	for _, rel := range rels {
		for j, asset := range rel.Assets {
			rel.Assets[j].URL = asset.BrowserURL
			rel.Assets[j].BrowserURL = ""
		}
	}

	addReleaseCompatibility(ctx, rels)

	sort.Sort(upgrade.SortByRelease(rels))
	return rels, nil
}

func addReleaseCompatibility(ctx context.Context, rels []upgrade.Release) {
	for i := range rels {
		rel := &rels[i]
		for i, asset := range rel.Assets {
			if asset.Name != "compat.json" {
				continue
			}

			// Load compat.json into the Compatibility field
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
			if err != nil {
				metricHTTPRequests.WithLabelValues("compat-json", "error").Inc()
				break
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				metricHTTPRequests.WithLabelValues("compat-json", "error").Inc()
				break
			}
			if resp.StatusCode != http.StatusOK {
				metricHTTPRequests.WithLabelValues("compat-json", "error").Inc()
				resp.Body.Close()
				break
			}
			_ = json.NewDecoder(io.LimitReader(resp.Body, 10<<10)).Decode(&rel.Compatibility)
			metricHTTPRequests.WithLabelValues("compat-json", "success").Inc()
			resp.Body.Close()

			// Remove compat.json from the asset list since it's been processed
			rel.Assets = append(rel.Assets[:i], rel.Assets[i+1:]...)
			break
		}
	}
}
