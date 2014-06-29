// Copyright (C) 2014 Jakob Borg and other contributors. All rights reserved.
// Use of this source code is governed by an MIT-style license that can be
// found in the LICENSE file.

package main

import (
	"crypto/tls"
	"path/filepath"
)

func loadCert(dir string) (tls.Certificate, error) {
	cf := filepath.Join(dir, "cert.pem")
	kf := filepath.Join(dir, "key.pem")
	return tls.LoadX509KeyPair(cf, kf)
}
