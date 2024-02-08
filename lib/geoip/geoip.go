// Copyright (C) 2024 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package geoip provides an automatically updating MaxMind GeoIP2 database
// provider.
package geoip

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maxmind/geoipupdate/v6/pkg/geoipupdate"
	"github.com/oschwald/geoip2-golang"
)

const maxDatabaseSize = 1 << 30 // 1 GiB, at the time of writing the database is about 95 MiB

type Provider struct {
	edition         string
	accountID       int
	licenseKey      string
	refreshInterval time.Duration
	directory       string

	mut        sync.Mutex
	db         *geoip2.Reader
	lastOpened time.Time
}

// NewGeoLite2CityProvider returns a new GeoIP2 database provider for the
// GeoLite2-City database. The database will be stored in the given
// directory (which should exist) and refreshed every 7 days.
func NewGeoLite2CityProvider(accountID int, licenseKey string, directory string) *Provider {
	return &Provider{
		edition:         "GeoLite2-City",
		accountID:       accountID,
		licenseKey:      licenseKey,
		refreshInterval: 7 * 24 * time.Hour,
		directory:       directory,
	}
}

func (p *Provider) City(ip net.IP) (*geoip2.City, error) {
	p.mut.Lock()

	if p.db != nil && time.Since(p.lastOpened) > p.refreshInterval/2 {
		p.db.Close()
		p.db = nil
	}
	if p.db == nil {
		var err error
		p.db, err = p.open(context.Background())
		if err != nil {
			p.mut.Unlock()
			return nil, err
		}
		p.lastOpened = time.Now()
	}
	db := p.db

	p.mut.Unlock()

	return db.City(ip)
}

// open returns a reader for the GeoIP2 database. If the database is not
// available locally, it will be downloaded. If the database is older than
// refreshInterval, it will be downloaded again. If the download fails, the
// existing database will be used. The returned reader must be closed by the
// caller in the normal manner.
func (p *Provider) open(ctx context.Context) (*geoip2.Reader, error) {
	if p.licenseKey == "" {
		return nil, errors.New("open: no license key set")
	}
	if p.edition == "" {
		return nil, errors.New("open: no edition set")
	}

	path := filepath.Join(p.directory, p.edition+".mmdb")
	info, err := os.Stat(path)
	if err != nil {
		// No file exists, download it
		err = p.download(ctx)
		if err != nil {
			return nil, fmt.Errorf("open: %w", err)
		}
	} else if time.Since(info.ModTime()) > p.refreshInterval {
		// File is too old, attempt to download it. If it fails, use the old
		// file.
		_ = p.download(ctx)
	}

	return geoip2.Open(path)
}

func (p *Provider) download(ctx context.Context) error {
	cfg := &geoipupdate.Config{
		URL:               "https://updates.maxmind.com",
		DatabaseDirectory: p.directory,
		LockFile:          filepath.Join(p.directory, "geoipupdate.lock"),
		RetryFor:          5 * time.Minute,
		Parallelism:       1,
		AccountID:         p.accountID,
		LicenseKey:        p.licenseKey,
		EditionIDs:        []string{p.edition},
	}

	if err := geoipupdate.NewClient(cfg).Run(ctx); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	return nil
}
