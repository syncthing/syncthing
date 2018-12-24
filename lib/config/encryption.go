// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/nacl/secretbox"
)

var (
	encryptionKey [32]byte
)

// The encryption key must be set externally before loading or saving
// configs.
func SetEncryptionKey(key [32]byte) {
	copy(encryptionKey[:], key[:])
}

// An encryptedString is a string that obfuscates itself in serialization
// and deobfuscates in deserialization.
type encryptedString string

func (s encryptedString) String() string {
	return string(s)
}

func (s *encryptedString) Set(v string) {
	// simplifies assignments from string, avoids the assigner casting
	*s = encryptedString(v)
}

func (s *encryptedString) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(xmlEncryptedString{Encrypted: obfuscate(s.String())}, start)
}

func (s *encryptedString) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var es xmlEncryptedString
	if err := d.DecodeElement(&es, &start); err != nil {
		return err
	}
	if es.Encrypted != "" {
		clear, ok := deobfuscate(es.Encrypted)
		if !ok {
			return errors.New("bad encrypted string")
		}
		s.Set(clear)
		return nil
	}
	s.Set(es.Inner)
	return nil
}

func obfuscate(data string) string {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		panic(errors.New("randomness disaster: " + err.Error()))
	}
	out := secretbox.Seal(nil, []byte(data), &nonce, &encryptionKey)
	return fmt.Sprintf("%s/%s", base64.RawURLEncoding.EncodeToString(nonce[:]), base64.RawURLEncoding.EncodeToString(out))
}

func deobfuscate(data string) (string, bool) {
	parts := strings.Split(data, "/")
	if len(parts) != 2 {
		return "", false
	}

	var nonce [24]byte
	if _, err := base64.RawURLEncoding.Decode(nonce[:], []byte(parts[0])); err != nil {
		return "", false
	}

	box, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}

	msg, ok := secretbox.Open(nil, box, &nonce, &encryptionKey)
	if !ok {
		return "", false
	}
	return string(msg), true
}

type xmlEncryptedString struct {
	Encrypted string `xml:"encrypted,attr"`
	Inner     string `xml:",chardata"`
}
