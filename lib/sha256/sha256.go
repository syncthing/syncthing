// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sha256

import (
	cryptoSha256 "crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"math/rand"
	"os"
	"time"

	minioSha256 "github.com/minio/sha256-simd"
	"github.com/syncthing/syncthing/lib/logger"
)

var l = logger.DefaultLogger.NewFacility("sha256", "SHA256 hashing package")

const (
	benchmarkingIterations = 3
	benchmarkingDuration   = 150 * time.Millisecond
	defaultImpl            = "crypto/sha256"
	minioImpl              = "minio/sha256-simd"
	Size                   = cryptoSha256.Size
)

// May be switched out for another implementation
var (
	New    = cryptoSha256.New
	Sum256 = cryptoSha256.Sum256
)

var (
	selectedImpl = defaultImpl
	cryptoPerf   float64
	minioPerf    float64
)

func SelectAlgo() {
	switch os.Getenv("STHASHING") {
	case "":
		// When unset, probe for the fastest implementation.
		benchmark()
		if minioPerf > cryptoPerf {
			selectMinio()
		}

	case "minio":
		// When set to "minio", use that.
		selectMinio()

	default:
		// When set to anything else, such as "standard", use the default Go
		// implementation. Make sure not to touch the minio
		// implementation as it may be disabled for incompatibility reasons.
	}

	verifyCorrectness()
}

// Report prints a line with the measured hash performance rates for the
// selected and alternate implementation.
func Report() {
	var otherImpl string
	var selectedRate, otherRate float64

	switch selectedImpl {
	case defaultImpl:
		selectedRate = cryptoPerf
		otherRate = minioPerf
		otherImpl = minioImpl

	case minioImpl:
		selectedRate = minioPerf
		otherRate = cryptoPerf
		otherImpl = defaultImpl
	}

	if selectedRate == 0 {
		return
	}

	l.Infof("Single thread SHA256 performance is %s using %s (%s using %s).", formatRate(selectedRate), selectedImpl, formatRate(otherRate), otherImpl)
}

func selectMinio() {
	New = minioSha256.New
	Sum256 = minioSha256.Sum256
	selectedImpl = minioImpl
}

func benchmark() {
	// Interleave the tests to achieve some sort of fairness if the CPU is
	// just in the process of spinning up to full speed.
	for i := 0; i < benchmarkingIterations; i++ {
		if perf := cpuBenchOnce(benchmarkingDuration, cryptoSha256.New); perf > cryptoPerf {
			cryptoPerf = perf
		}
		if perf := cpuBenchOnce(benchmarkingDuration, minioSha256.New); perf > minioPerf {
			minioPerf = perf
		}
	}
}

func cpuBenchOnce(duration time.Duration, newFn func() hash.Hash) float64 {
	chunkSize := 100 * 1 << 10
	h := newFn()
	bs := make([]byte, chunkSize)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(bs)

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

func formatRate(rate float64) string {
	decimals := 0
	if rate < 1 {
		decimals = 2
	} else if rate < 10 {
		decimals = 1
	}
	return fmt.Sprintf("%.*f MB/s", decimals, rate)
}

func verifyCorrectness() {
	// The currently selected algo should in fact perform a SHA256 calculation.

	// $ echo "Syncthing Magic Testing Value" | openssl dgst -sha256 -hex
	correct := "87f6cfd24131724c6ec43495594c5c22abc7d2b86bcc134bc6f10b7ec3dda4ee"
	input := "Syncthing Magic Testing Value\n"

	h := New()
	h.Write([]byte(input))
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != correct {
		panic("sha256 is broken")
	}

	arr := Sum256([]byte(input))
	sum = hex.EncodeToString(arr[:])
	if sum != correct {
		panic("sha256 is broken")
	}
}
