// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/syncthing/syncthing/lib/signature"
	"github.com/syncthing/syncthing/lib/upgrade"
	_ "go.uber.org/automaxprocs"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	flag.Parse()

	if flag.NArg() < 1 {
		log.Print(`Usage:
	stsigtool <command>

Where command is one of:

	gen
		- generate a new key pair

	sign <privkeyfile> [datafile]
		- sign a file

	verify <signaturefile> <datafile>
		- verify a signature, using the built in public key

	verify <signaturefile> <datafile> <pubkeyfile>
		- verify a signature, using the specified public key file

`)
	}

	switch flag.Arg(0) {
	case "gen":
		gen()
	case "sign":
		sign(flag.Arg(1), flag.Arg(2))
	case "verify":
		if flag.NArg() == 4 {
			verifyWithFile(flag.Arg(1), flag.Arg(2), flag.Arg(3))
		} else {
			verifyWithKey(flag.Arg(1), flag.Arg(2), upgrade.SigningKey)
		}
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
	privkey, err := os.ReadFile(keyname)
	if err != nil {
		log.Fatal(err)
	}

	var input io.Reader
	if dataname == "-" || dataname == "" {
		input = os.Stdin
	} else {
		fd, err := os.Open(dataname)
		if err != nil {
			log.Fatal(err)
		}
		defer fd.Close()
		input = fd
	}

	sig, err := signature.Sign(privkey, input)
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(sig)
}

func verifyWithFile(signame, dataname, keyname string) {
	pubkey, err := os.ReadFile(keyname)
	if err != nil {
		log.Fatal(err)
	}
	verifyWithKey(signame, dataname, pubkey)
}

func verifyWithKey(signame, dataname string, pubkey []byte) {
	sig, err := os.ReadFile(signame)
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

	log.Println("correct signature")
}
