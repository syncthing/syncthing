// Copyright (C) 2018 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
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
	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		panic("impossible error:" + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic("impossible error:" + err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		panic("impossible error:" + err.Error())
	}

	out := gcm.Seal(nonce, nonce, []byte(data), nil)
	return base64.RawURLEncoding.EncodeToString(out)
}

func deobfuscate(data string) (string, bool) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", false
	}

	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		return "", false
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", false
	}

	out, err := gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
	if err != nil {
		return "", false
	}
	return string(out), true
}

type xmlEncryptedString struct {
	Encrypted string `xml:"encrypted,attr"`
	Inner     string `xml:",chardata"`
}
