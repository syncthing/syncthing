// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	mr "math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

type result struct {
	id       protocol.DeviceID
	priv     *ecdsa.PrivateKey
	derBytes []byte
}

func main() {
	flag.Parse()
	prefix := strings.ToUpper(strings.ReplaceAll(flag.Arg(0), "-", ""))
	if len(prefix) > 7 {
		prefix = prefix[:7] + "-" + prefix[7:]
	}

	found := make(chan result)
	stop := make(chan struct{})
	var count int64

	// Print periodic progress reports.
	go printProgress(prefix, &count)

	// Run one certificate generator per CPU core.
	var wg sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(-1); i++ {
		wg.Add(1)
		go func() {
			generatePrefixed(prefix, &count, found, stop)
			wg.Done()
		}()
	}

	// Save the result, when one has been found.
	res := <-found
	close(stop)
	wg.Wait()

	fmt.Println("Found", res.id)
	saveCert(res.priv, res.derBytes)
	fmt.Println("Saved to cert.pem, key.pem")
}

// Try certificates until one is found that has the prefix at the start of
// the resulting device ID. Increments count atomically, sends the result to
// found, returns when stop is closed.
func generatePrefixed(prefix string, count *int64, found chan<- result, stop <-chan struct{}) {
	notBefore := time.Now()
	notAfter := time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(mr.Int63()),
		Subject: pkix.Name{
			CommonName: "syncthing",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for {
		select {
		case <-stop:
			return
		default:
		}

		derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		id := protocol.NewDeviceID(derBytes)
		atomic.AddInt64(count, 1)

		if strings.HasPrefix(id.String(), prefix) {
			select {
			case found <- result{id, priv, derBytes}:
			case <-stop:
			}
			return
		}
	}
}

func printProgress(prefix string, count *int64) {
	started := time.Now()
	wantBits := 5 * len(prefix)
	if wantBits > 63 {
		fmt.Printf("Want %d bits for prefix %q, refusing to boil the ocean.\n", wantBits, prefix)
		os.Exit(1)
	}
	expectedIterations := float64(int(1) << uint(wantBits))
	fmt.Printf("Want %d bits for prefix %q, about %.2g certs to test (statistically speaking)\n", wantBits, prefix, expectedIterations)

	for range time.NewTicker(15 * time.Second).C {
		tried := atomic.LoadInt64(count)
		elapsed := time.Since(started)
		rate := float64(tried) / elapsed.Seconds()
		expected := timeStr(expectedIterations / rate)
		fmt.Printf("Trying %.0f certs/s, tested %d so far in %v, expect ~%s total time to complete\n", rate, tried, elapsed/time.Second*time.Second, expected)
	}
}

func saveCert(priv interface{}, derBytes []byte) {
	certOut, err := os.Create("cert.pem")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = certOut.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	keyOut, err := os.OpenFile("key.pem", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	block, err := pemBlockForKey(priv)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = pem.Encode(keyOut, block)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = keyOut.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func pemBlockForKey(priv interface{}) (*pem.Block, error) {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, errors.New("unknown key type")
	}
}

func timeStr(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%.0fm", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%.0fh", seconds/3600)
	}
	if seconds < 86400*365 {
		return fmt.Sprintf("%.0f days", seconds/3600)
	}
	return fmt.Sprintf("%.0f years", seconds/86400/365)
}
