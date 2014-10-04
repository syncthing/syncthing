// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/syncthing/syncthing/internal/luhn"
)

type DeviceID [32]byte

var LocalDeviceID = DeviceID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// NewDeviceID generates a new device ID from the raw bytes of a certificate
func NewDeviceID(rawCert []byte) DeviceID {
	var n DeviceID
	hf := sha256.New()
	hf.Write(rawCert)
	hf.Sum(n[:0])
	return n
}

func DeviceIDFromString(s string) (DeviceID, error) {
	var n DeviceID
	err := n.UnmarshalText([]byte(s))
	return n, err
}

func DeviceIDFromBytes(bs []byte) DeviceID {
	var n DeviceID
	if len(bs) != len(n) {
		panic("incorrect length of byte slice representing device ID")
	}
	copy(n[:], bs)
	return n
}

// String returns the canonical string representation of the device ID
func (n DeviceID) String() string {
	id := base32.StdEncoding.EncodeToString(n[:])
	id = strings.Trim(id, "=")
	id, err := luhnify(id)
	if err != nil {
		// Should never happen
		panic(err)
	}
	id = chunkify(id)
	return id
}

func (n DeviceID) GoString() string {
	return n.String()
}

func (n DeviceID) Compare(other DeviceID) int {
	return bytes.Compare(n[:], other[:])
}

func (n DeviceID) Equals(other DeviceID) bool {
	return bytes.Compare(n[:], other[:]) == 0
}

func (n *DeviceID) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n *DeviceID) UnmarshalText(bs []byte) error {
	id := string(bs)
	id = strings.Trim(id, "=")
	id = strings.ToUpper(id)
	id = untypeoify(id)
	id = unchunkify(id)

	var err error
	switch len(id) {
	case 56:
		// New style, with check digits
		id, err = unluhnify(id)
		if err != nil {
			return err
		}
		fallthrough
	case 52:
		// Old style, no check digits
		dec, err := base32.StdEncoding.DecodeString(id + "====")
		if err != nil {
			return err
		}
		copy(n[:], dec)
		return nil
	default:
		return errors.New("device ID invalid: incorrect length")
	}
}

func luhnify(s string) (string, error) {
	if len(s) != 52 {
		panic("unsupported string length")
	}

	res := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		p := s[i*13 : (i+1)*13]
		l, err := luhn.Base32.Generate(p)
		if err != nil {
			return "", err
		}
		res = append(res, fmt.Sprintf("%s%c", p, l))
	}
	return res[0] + res[1] + res[2] + res[3], nil
}

func unluhnify(s string) (string, error) {
	if len(s) != 56 {
		return "", fmt.Errorf("unsupported string length %d", len(s))
	}

	res := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		p := s[i*14 : (i+1)*14-1]
		l, err := luhn.Base32.Generate(p)
		if err != nil {
			return "", err
		}
		if g := fmt.Sprintf("%s%c", p, l); g != s[i*14:(i+1)*14] {
			return "", errors.New("check digit incorrect")
		}
		res = append(res, p)
	}
	return res[0] + res[1] + res[2] + res[3], nil
}

func chunkify(s string) string {
	s = regexp.MustCompile("(.{7})").ReplaceAllString(s, "$1-")
	s = strings.Trim(s, "-")
	return s
}

func unchunkify(s string) string {
	s = strings.Replace(s, "-", "", -1)
	s = strings.Replace(s, " ", "", -1)
	return s
}

func untypeoify(s string) string {
	s = strings.Replace(s, "0", "O", -1)
	s = strings.Replace(s, "1", "I", -1)
	s = strings.Replace(s, "8", "B", -1)
	return s
}
