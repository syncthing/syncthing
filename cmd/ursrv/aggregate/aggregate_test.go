// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package aggregate

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/syncthing/syncthing/lib/ur"
	"github.com/syncthing/syncthing/lib/ur/contract"
)

func TestAggregateV2(t *testing.T) {
	geoip, err := geoip2.Open("/Users/ericp/Documents/dev-kastelo/dev/syncthing-debugging/GeoLite2-City.mmdb")
	if err != nil {
		log.Println("opening geoip db", err)
		geoip = nil
	} else {
		defer geoip.Close()
	}

	reps, err := generateUsageReports()
	if err != nil {
		t.Fatal("unable to read test-report", err)
	}

	date := time.Now().UTC()

	ap := aggregateUserReports(geoip, date, reps)
	for _, stat := range ap.Statistics {
		switch tf := stat.Statistic.(type) {
		case *ur.Statistic_Float:
			t.Log(fmt.Printf("float:\t\t%s => %v\n", stat.Key, tf.Float))
		case *ur.Statistic_Integer:
			t.Log(fmt.Printf("int:\t\t%s => %v\n", stat.Key, tf.Integer))
		case *ur.Statistic_Histogram:
			t.Log(fmt.Printf("histogram:\t%s => %v\n", stat.Key, tf.Histogram))
		case *ur.Statistic_MappedInteger:
			t.Log(fmt.Printf("mapped int:\t%s => %v\n", stat.Key, tf.MappedInteger))
		}
	}
	t.Log(fmt.Printf("reports:\t%d\n", ap.Count))
	t.Log(fmt.Printf("reportsV2:\t%d\n", ap.CountV2))
	t.Log(fmt.Printf("reportsV3:\t%d\n", ap.CountV3))

	// Imitate storing the report
	marshalled, err := ap.Marshal()
	if err != nil {
		t.Fatal("Marshall error")
	}

	// Imitate obtaining the report
	var nap ur.Aggregation
	err = nap.Unmarshal(marshalled)
	if err != nil {
		t.Fatal("Unmarshall error")
	}
}

func generateUsageReports() ([]contract.Report, error) {
	var reps []contract.Report
	rep, err := realTestReport("test_data/ur-01.json")
	if err != nil {
		return reps, err
	}
	// Add enough reports to trigger the percentiles
	for i := 0; i < 49; i++ {
		reps = append(reps, rep)
	}

	rep, err = realTestReport("test_data/ur-02.json")
	if err != nil {
		return reps, err
	}
	// Add enough reports to trigger the percentiles
	for i := 0; i < 51; i++ {
		reps = append(reps, rep)
	}

	return reps, nil
}

func realTestReport(file string) (contract.Report, error) {
	var rep contract.Report
	b, err := os.ReadFile(file)
	if err != nil {
		return rep, err
	}
	err = json.Unmarshal(b, &rep)
	return rep, err
}
