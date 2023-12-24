package blob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syncthing/syncthing/cmd/ursrv/report"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

const (
	USAGE_PREFIX      = "UR" // contract.Report
	AGGREGATED_PREFIX = "AR" // report.AggregatedReport
)

func NewBlobStorage(s3Config S3Config) Store {
	// If S3-compatible credentials are provided, use those.
	if s3Config.isSet() {
		s3, err := NewS3(s3Config)
		if err == nil {
			return s3
		}
	}

	// Fall back to local storage.
	dir, err := os.UserHomeDir()
	if err != nil {
		log.Println("Could not get user home directory", "error", err)
		dir = os.TempDir()
	}

	dir = filepath.Join(dir, ".ursrv", "blob")

	return NewDisk(dir)
}

type Store interface {
	Put(_ string, _ []byte) error
	Get(_ string) ([]byte, error)
	Delete(_ string) error
	Iterate(_ context.Context, _ string, _ func([]byte) bool) error
	Count(_ string) (int, error)
}

type UrsrvStore struct {
	Store
}

func NewUrsrvStore(s Store) *UrsrvStore {
	return &UrsrvStore{s}
}

func usageReportKey(when time.Time, uniqueId string) string {
	return fmt.Sprintf("%s/%s-%s.json", USAGE_PREFIX, when.UTC().Format("20060102"), uniqueId)
}

func aggregatedReportKey(when time.Time) string {
	return fmt.Sprintf("%s/%s.json", AGGREGATED_PREFIX, when.UTC().Format("20060102"))
}

func (s *UrsrvStore) PutUsageReport(rep contract.Report, received time.Time) error {
	key := usageReportKey(received, rep.UniqueID)

	// Check if we already have a report for this instance today.
	if data, err := s.Store.Get(key); err == nil && len(data) != 0 {
		return errors.New("already exists")
	}

	bs, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	return s.Store.Put(key, bs)
}

func (s *UrsrvStore) PutAggregatedReport(rep *report.AggregatedReport) error {
	key := aggregatedReportKey(rep.Date)
	bs, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	return s.Store.Put(key, bs)
}

func (s *UrsrvStore) ListUsageReportsForDate(when time.Time) ([]contract.Report, error) {
	ctx := context.Background()
	prefix, _ := strings.CutSuffix(usageReportKey(when, ""), ".json")

	var res []contract.Report
	var rep contract.Report

	err := s.Store.Iterate(ctx, prefix, func(b []byte) bool {
		err := json.Unmarshal(b, &rep)
		if err != nil {
			return true
		}
		res = append(res, rep)
		return true
	})

	return res, err
}

func (s *UrsrvStore) ListAggregatedReports() ([]report.AggregatedReport, error) {
	ctx := context.Background()
	prefix := AGGREGATED_PREFIX + "/"

	var res []report.AggregatedReport
	var rep report.AggregatedReport
	err := s.Store.Iterate(ctx, prefix, func(b []byte) bool {
		err := json.Unmarshal(b, &rep)
		if err != nil {
			return true
		}
		res = append(res, rep)
		return true
	})

	return res, err
}

func (s *UrsrvStore) LastAggregatedReport() (report.AggregatedReport, error) {
	var rep report.AggregatedReport

	// Requires an aggregated report of the day before, which in practise should
	// always be the case.
	date := time.Now().UTC().AddDate(0, 0, -1)
	key := aggregatedReportKey(date)
	data, err := s.Store.Get(key)
	if err != nil {
		// In practise this shouldn't happen, but we can look one day further
		// back.
		date := date.AddDate(0, 0, -1)
		key := aggregatedReportKey(date)
		data, err = s.Store.Get(key)
		if err != nil {
			return rep, errors.New("no aggregated report found")
		}
	}

	err = json.Unmarshal(data, &rep)

	return rep, err
}

func (s *UrsrvStore) CountAggregatedReports() (int, error) {
	prefix := AGGREGATED_PREFIX + "/"
	return s.Count(prefix)
}

func (s *UrsrvStore) Count(prefix string) (int, error) {
	return s.Store.Count(prefix)
}
