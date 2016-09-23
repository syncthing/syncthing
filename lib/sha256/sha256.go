// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package sha256

import (
	"crypto/rand"
	cryptoSha256 "crypto/sha256"
	"hash"
	"os"
	"time"

	minioSha256 "github.com/minio/sha256-simd"
)

func init() {
	switch os.Getenv("STHASHING") {
	case "":
		// When unset, probe for the fastest implementation.
		selectFastest()

	case "minio":
		// When set to "minio", use that.
		selectMinio()

	default:
		// When set to anything else, such as "standard", use the default Go
		// implementation.
	}
}

const (
	benchmarkingIterations = 3
	benchmarkingDuration   = 75 * time.Millisecond
)

const (
	BlockSize = cryptoSha256.BlockSize
	Size      = cryptoSha256.Size
)

// May be switched out for another implementation
var (
	New    = cryptoSha256.New
	Sum256 = cryptoSha256.Sum256
)

// Exported variables to inspect the result of the selection process
var (
	Selected   = "crypto/sha256"
	CryptoPerf float64
	MinioPerf  float64
)

// selectFastest benchmarks both algos and selects minio if it's at least 5
// percent faster.
func selectFastest() {
	for i := 0; i < benchmarkingIterations; i++ {
		if perf := cpuBenchOnce(benchmarkingDuration, cryptoSha256.New); perf > CryptoPerf {
			CryptoPerf = perf
		}
		if perf := cpuBenchOnce(benchmarkingDuration, minioSha256.New); perf > MinioPerf {
			MinioPerf = perf
		}
	}

	if MinioPerf > 1.05*CryptoPerf {
		selectMinio()
	}
}

func selectMinio() {
	New = minioSha256.New
	Sum256 = minioSha256.Sum256
	Selected = "github.com/minio/sha256-simd"
}

func cpuBenchOnce(duration time.Duration, newFn func() hash.Hash) float64 {
	chunkSize := 100 * 1 << 10
	h := newFn()
	bs := make([]byte, chunkSize)
	rand.Reader.Read(bs)

	t0 := time.Now()
	b := 0
	for time.Since(t0) < duration {
		h.Write(bs)
		b += chunkSize
	}
	h.Sum(nil)
	d := time.Since(t0)
	return float64(int(float64(b)/d.Seconds()/(1<<20)*100)) / 100
}
