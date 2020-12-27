// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Only for development testing, will be removed.

package db

import (
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

var (
	testDev1, _ = protocol.DeviceIDFromString("AEAQCAI-BAEAQCA-AIBAEAQ-CAIBAEC-AQCAIBA-EAQCAIA-BAEAQCA-IBAEAQC")
	testDev2, _ = protocol.DeviceIDFromString("AIBAEAQ-CAIBAEC-AQCAIBA-EAQCAIA-BAEAQCA-IBAEAQC-CAIBAEA-QCAIBA7")
	testDev3, _ = protocol.DeviceIDFromString("P56IOI7-MZJNU2Y-IQGDREY-DM2MGTI-MGL3BXN-PQ6W5BM-TBBZ4TJ-XZWICQ2")
	testDev4, _ = protocol.DeviceIDFromString("DOVII4U-SQEEESM-VZ2CVTC-CJM4YN5-QNV7DCU-5U3ASRL-YVFG6TH-W5DV5AA")
	testDev5, _ = protocol.DeviceIDFromString("YZJBJFX-RDBL7WY-6ZGKJ2D-4MJB4E7-ZATSDUY-LD6Y3L3-MLFUYWE-AEMXJAC")
	testDev6, _ = protocol.DeviceIDFromString("UYGDMA4-TPHOFO5-2VQYDCC-7CWX7XW-INZINQT-LE4B42N-4JUZTSM-IWCSXA4")
)

func (db *Lowlevel) CandidateLinksDummy() ([]CandidateLink, error) {
	res := []CandidateLink{
		{
			Introducer: testDev2,
			Folder:     "boggl-goggl",
			Candidate:  testDev1,
		},
		{
			Introducer: testDev2,
			Folder:     "sleep-wells",
			Candidate:  testDev1,
		},
		{
			Introducer: testDev2,
			Folder:     "damtn-omola",
			Candidate:  testDev1,
		},
	}
	return res, nil
}

func (db *Lowlevel) CandidateLinksDummyData() {
	l.Warnln(db.AddOrUpdateCandidateLink("cpkn4-57ysy", "Pics from Jane", testDev3, testDev4,
		&IntroducedDeviceDetails{
			CertName: "",
			Addresses: []string{
				"192.168.1.2:22000",
				"[2a02:8070::ff34:1234::aabb]:22000",
			},
			SuggestedName: "Jane",
		}))

	l.Warnln(db.AddOrUpdateCandidateLink("cpkn4-57ysy", "Pics of J & J", testDev3, testDev5,
		&IntroducedDeviceDetails{
			CertName: "",
			Addresses: []string{
				"192.168.1.2:22000",
				"[2a02:8070::ff34:1234::aabb]:22000",
			},
			SuggestedName: "Jane's Laptop",
		}))

	l.Warnln(db.AddOrUpdateCandidateLink("cpkn4-57ysy", "Family pics", testDev6, testDev5, nil))

	l.Warnln(db.AddOrUpdateCandidateLink("abcde-fghij", "Mighty nice folder", testDev6, testDev5, nil))

	l.Warnln(db.AddOrUpdateCandidateLink("cpkn4-57ysy", "Family pics", testDev6, testDev2, nil))

	l.Warnln(db.AddOrUpdateCandidateLink("cpkn4-57ysy", "Pictures from Joe", testDev4, testDev5, nil))
}

func (db *Lowlevel) CandidateDevicesDummy() (map[protocol.DeviceID]CandidateDevice, error) {
	res := map[protocol.DeviceID]CandidateDevice{
		testDev3: {
			IntroducedBy: map[protocol.DeviceID]candidateDeviceAttribution{
				testDev5: {
					// Should be the same for all folders, as they were all
					// mentioned in the most recent ClusterConfig
					Time: time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
					CommonFolders: map[string]string{
						"cpkn4-57ysy": "Pics of J & J",
					},
					// Only if the device ID is not known locally:
					SuggestedName: "Jane's Laptop",
				},
				testDev4: {
					Time: time.Date(2020, 3, 1, 10, 12, 13, 0, time.Local),
					CommonFolders: map[string]string{
						"cpkn4-57ysy": "Pics from Jane",
					},
					SuggestedName: "Jane",
				},
			},
			// Only if the device ID is not known locally:
			CertName: "",
			Addresses: []string{
				"192.168.1.2:22000",
				"[2a02:8070::ff34:1234::aabb]:22000",
			},
		},
		testDev6: {
			IntroducedBy: map[protocol.DeviceID]candidateDeviceAttribution{
				testDev5: {
					Time: time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
					CommonFolders: map[string]string{
						"cpkn4-57ysy": "Family pics",
						"abcde-fghij": "Mighty nice folder",
					},
				},
				testDev2: {
					Time: time.Date(2020, 2, 22, 14, 56, 0, 0, time.Local),
					CommonFolders: map[string]string{
						"cpkn4-57ysy": "Family pics",
					},
				},
			},
		},
		testDev4: {
			IntroducedBy: map[protocol.DeviceID]candidateDeviceAttribution{
				testDev5: {
					Time: time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
					CommonFolders: map[string]string{
						"cpkn4-57ysy": "Pictures from Joe",
					},
				},
			},
		},
	}
	return res, nil
}

func (db *Lowlevel) CandidateFoldersDummy() (map[string]CandidateFolder, error) {
	res := map[string]CandidateFolder{
		"abcde-fghij": {
			testDev6: {
				IntroducedBy: map[protocol.DeviceID]candidateFolderAttribution{
					testDev5: {
						Time:  time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
						Label: "Mighty nice folder",
					},
				},
			},
		},
		"cpkn4-57ysy": {
			testDev4: {
				IntroducedBy: map[protocol.DeviceID]candidateFolderAttribution{
					testDev5: {
						Time:  time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
						Label: "Pictures from Joe",
					},
				},
			},
			testDev6: {
				IntroducedBy: map[protocol.DeviceID]candidateFolderAttribution{
					testDev5: {
						Time:  time.Date(2020, 3, 18, 11, 43, 7, 0, time.Local),
						Label: "Family pics",
					},
					testDev2: {
						Time:  time.Date(2020, 11, 22, 14, 56, 0, 0, time.Local),
						Label: "Family pics",
					},
				},
			},
		},
	}
	return res, nil
}
