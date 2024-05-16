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
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maxmind/geoipupdate/v6/pkg/geoipupdate"
	"github.com/oschwald/geoip2-golang"
)

type Provider struct {
	edition         string
	accountID       int
	licenseKey      string
	refreshInterval time.Duration
	directory       string

	mut          sync.Mutex
	currentDBDir string
	db           *geoip2.Reader
}

// NewGeoLite2CityProvider returns a new GeoIP2 database provider for the
// GeoLite2-City database. The database will be stored in the given
// directory (which should exist) and refreshed every 7 days.
func NewGeoLite2CityProvider(ctx context.Context, accountID int, licenseKey string, directory string) (*Provider, error) {
	p := &Provider{
		edition:         "GeoLite2-City",
		accountID:       accountID,
		licenseKey:      licenseKey,
		refreshInterval: 7 * 24 * time.Hour,
		directory:       directory,
	}

	if err := p.download(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Provider) City(ip net.IP) (*geoip2.City, error) {
	p.mut.Lock()
	defer p.mut.Unlock()

	if p.db == nil {
		return nil, errors.New("database not open")
	}

	return p.db.City(ip)
}

// Serve downloads the GeoIP2 database and keeps it up to date. It will return
// when the context is canceled.
func (p *Provider) Serve(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.After(p.refreshInterval):
			if err := p.download(ctx); err != nil {
				return err
			}
		}
	}
}

func (p *Provider) download(ctx context.Context) error {
	newSubdir, err := os.MkdirTemp(p.directory, "geoipupdate")
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	log.Println("Downloading GeoIP2 database to", newSubdir)

	cfg := &geoipupdate.Config{
		URL:               "https://updates.maxmind.com",
		DatabaseDirectory: newSubdir,
		LockFile:          filepath.Join(newSubdir, "geoipupdate.lock"),
		RetryFor:          5 * time.Minute,
		Parallelism:       1,
		AccountID:         p.accountID,
		LicenseKey:        p.licenseKey,
		EditionIDs:        []string{p.edition},
	}

	if err := geoipupdate.NewClient(cfg).Run(ctx); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	dbPath := filepath.Join(newSubdir, p.edition+".mmdb")
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open downloaded db: %w", err)
	}

	p.mut.Lock()
	prevDBDir := p.currentDBDir
	if p.db != nil {
		p.db.Close()
	}
	p.currentDBDir = newSubdir
	p.db = db
	p.mut.Unlock()

	if prevDBDir != "" {
		log.Println("Removing old GeoIP2 database", prevDBDir)
		_ = os.RemoveAll(p.currentDBDir)
	}

	log.Println("Downloaded GeoIP2 database to", dbPath)
	return nil
}
