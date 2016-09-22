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
		// When set to anything else, such as "default", use the default Go implementation.
	}
}

const BlockSize = cryptoSha256.BlockSize
const Size = cryptoSha256.Size
const Size224 = cryptoSha256.Size224

// May be switched out for another implementation
var New func() hash.Hash = cryptoSha256.New
var Sum256 func(data []byte) [Size]byte = cryptoSha256.Sum256

func New224() hash.Hash {
	// Will be inlined
	return cryptoSha256.New224()
}

func Sum224(data []byte) (sum224 [Size224]byte) {
	// Will be inlined
	return cryptoSha256.Sum224(data)
}

var Selected = "crypto/sha256"
var CryptoPerf float64
var MinioPerf float64

func selectFastest() {
	CryptoPerf = cpuBench(cryptoSha256.New)
	MinioPerf = cpuBench(minioSha256.New)

	if MinioPerf > 1.05*CryptoPerf {
		selectMinio()
	}
}

func selectMinio() {
	New = minioSha256.New
	Sum256 = minioSha256.Sum256
	Selected = "github.com/minio/sha256-simd"
}

func cpuBench(newFn func() hash.Hash) float64 {
	const iterations = 3
	const duration = 75 * time.Millisecond
	var perf float64
	for i := 0; i < iterations; i++ {
		if v := cpuBenchOnce(duration, newFn); v > perf {
			perf = v
		}
	}
	return perf
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
