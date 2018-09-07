// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"testing"
	"golang.org/x/crypto/bcrypt"

    "github.com/syncthing/syncthing/cmd/syncthing"

)

func TestSimpleAuthOK(t *testing.T) {
    passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("pass"), 14)
    ok, username := main.AuthSimple("user", "pass", "user", string(passwordHashBytes))
    if !ok {
        t.Fatalf("should pass auth")
    }
    if username != "user" {
        t.Fatalf(username)
    }
}

func TestSimpleAuthUsernameFail(t *testing.T) {
    passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("pass"), 14)
    ok, _ := main.AuthSimple("userWRONG", "pass", "user", string(passwordHashBytes))
    if ok {
        t.Fatalf("should fail auth")
    }
}

func TestSimpleAuthPasswordFail(t *testing.T) {
    passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("passWRONG"), 14)
    ok, _ := main.AuthSimple("user", "pass", "user", string(passwordHashBytes))
    if ok {
        t.Fatalf("should fail auth")
    }
}

