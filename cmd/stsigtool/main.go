// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/syncthing/syncthing/lib/signature"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	flag.Parse()

	if flag.NArg() < 1 {
		log.Println(`Usage:
	stsigtool <command>

Where command is one of:

	gen
		- generate a new key pair

	sign <privkeyfile> <datafile>
		- sign a file

	verify <pubkeyfile> <signaturefile> <datafile>
		- verify a signature
`)
	}

	switch flag.Arg(0) {
	case "gen":
		gen()
	case "sign":
		sign(flag.Arg(1), flag.Arg(2))
	case "verify":
		verify(flag.Arg(1), flag.Arg(2), flag.Arg(3))
	}
}

func gen() {
	priv, pub, err := signature.GenerateKeys()
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(priv)
	os.Stdout.Write(pub)
}

func sign(keyname, dataname string) {
	privkey, err := ioutil.ReadFile(keyname)
	if err != nil {
		log.Fatal(err)
	}

	fd, err := os.Open(dataname)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	sig, err := signature.Sign(privkey, fd)
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(sig)
}

func verify(keyname, signame, dataname string) {
	pubkey, err := ioutil.ReadFile(keyname)
	if err != nil {
		log.Fatal(err)
	}

	sig, err := ioutil.ReadFile(signame)
	if err != nil {
		log.Fatal(err)
	}

	fd, err := os.Open(dataname)
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	err = signature.Verify(pubkey, sig, fd)
	if err != nil {
		log.Fatal(err)
	}
}
