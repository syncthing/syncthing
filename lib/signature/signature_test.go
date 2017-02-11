// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package signature_test

import (
	"bytes"
	"testing"

	"github.com/syncthing/syncthing/lib/signature"
)

var (
	// A private key for signing
	privKey = []byte(`-----BEGIN EC PRIVATE KEY-----
MIHbAgEBBEFGXB1IgefFF6kSyE17xIAU7fDIn07sPnGf1kLOCVrEZyUbnAmNFk8u
lUt/knnvo+Gw1i9ucFjmtYtzDevrhSlG5aAHBgUrgQQAI6GBiQOBhgAEASlcbcgJ
4PN+TSnAYiMlA0I/PRtFrDCgrt27K7hR+U7Afjc4KqW+QYwoRLvxueNh7gUK+zc0
Aqrk3z+O1epiQTq8ACikHUXsx/bSzEFlPdMygUAAj3hChlgCL6/vOocuRUbtAqc6
Zr0L9px+J4L0K+uqhyhKya7y6QLJrYPovFq3A7AK
-----END EC PRIVATE KEY-----`)

	// The matching public key
	pubKey = []byte(`-----BEGIN EC PUBLIC KEY-----
MIGbMBAGByqGSM49AgEGBSuBBAAjA4GGAAQBKVxtyAng835NKcBiIyUDQj89G0Ws
MKCu3bsruFH5TsB+Nzgqpb5BjChEu/G542HuBQr7NzQCquTfP47V6mJBOrwAKKQd
RezH9tLMQWU90zKBQACPeEKGWAIvr+86hy5FRu0CpzpmvQv2nH4ngvQr66qHKErJ
rvLpAsmtg+i8WrcDsAo=
-----END EC PUBLIC KEY-----`)

	// A signature of "this is a string to sign" created with the private key
	// above
	exampleSig = []byte(`-----BEGIN SIGNATURE-----
MIGGAkFdHjdarlFOrtcnCqcb0BX7Mjjq/Sbgp4mopCxBwXmfamtCeRGhZJ5MikyD
VXScaJ2Dq2Ov7L4/gTcYj9fZwcrWgQJBc7+tcw5fpO0/y8DNq0t3g9bqt2MkmoNm
eSAM8Fze4usVXHEi+QeMuYM2IKeVPyAR3iyl5gflVul9NRXS3OPAH3A=
-----END SIGNATURE-----`)
)

func TestGenerateKeys(t *testing.T) {
	priv, pub, err := signature.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(priv, []byte("PRIVATE KEY")) {
		t.Fatal("should be a private key")
	}
	if !bytes.Contains(pub, []byte("PUBLIC KEY")) {
		t.Fatal("should be a private key")
	}
}

func TestSign(t *testing.T) {
	data := bytes.NewReader([]byte("this is a string to sign"))

	s, err := signature.Sign(privKey, data)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(s, []byte("SIGNATURE")) {
		t.Error("should be a signature")
	}
}

func TestVerify(t *testing.T) {
	data := bytes.NewReader([]byte("this is a string to sign"))
	err := signature.Verify(pubKey, exampleSig, data)
	if err != nil {
		t.Fatal(err)
	}

	data = bytes.NewReader([]byte("thus is a string to sign"))
	err = signature.Verify(pubKey, exampleSig, data)
	if err == nil {
		t.Fatal("signature should not match")
	}
}
