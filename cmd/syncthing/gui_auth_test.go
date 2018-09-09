// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestSimpleAuthOK(t *testing.T) {
	passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("pass"), 14)
	ok := authSimple("user", "pass", "user", string(passwordHashBytes))
	if !ok {
		t.Fatalf("should pass auth")
	}
}

func TestSimpleAuthUsernameFail(t *testing.T) {
	passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("pass"), 14)
	ok := authSimple("userWRONG", "pass", "user", string(passwordHashBytes))
	if ok {
		t.Fatalf("should fail auth")
	}
}

func TestSimpleAuthPasswordFail(t *testing.T) {
	passwordHashBytes, _ := bcrypt.GenerateFromPassword([]byte("passWRONG"), 14)
	ok := authSimple("user", "pass", "user", string(passwordHashBytes))
	if ok {
		t.Fatalf("should fail auth")
	}
}
