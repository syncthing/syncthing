// Copyright (C) 2014 The Protocol Authors.

package protocol

import (
	"bytes"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/syncthing/syncthing/lib/sha256"
)

const DeviceIDLength = 32

type DeviceID [DeviceIDLength]byte
type ShortID uint64

var (
	LocalDeviceID  = repeatedDeviceID(0xff)
	GlobalDeviceID = repeatedDeviceID(0xf8)
	EmptyDeviceID  = DeviceID{ /* all zeroes */ }
)

func repeatedDeviceID(v byte) (d DeviceID) {
	for i := range d {
		d[i] = v
	}
	return
}

// NewDeviceID generates a new device ID from the raw bytes of a certificate
func NewDeviceID(rawCert []byte) DeviceID {
	return DeviceID(sha256.Sum256(rawCert))
}

func DeviceIDFromString(s string) (DeviceID, error) {
	var n DeviceID
	err := n.UnmarshalText([]byte(s))
	return n, err
}

func DeviceIDFromBytes(bs []byte) (DeviceID, error) {
	var n DeviceID
	if len(bs) != len(n) {
		return n, errors.New("incorrect length of byte slice representing device ID")
	}
	copy(n[:], bs)
	return n, nil
}

// String returns the canonical string representation of the device ID
func (n DeviceID) String() string {
	if n == EmptyDeviceID {
		return ""
	}
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
	return bytes.Equal(n[:], other[:])
}

// Short returns an integer representing bits 0-63 of the device ID.
func (n DeviceID) Short() ShortID {
	return ShortID(binary.BigEndian.Uint64(n[:]))
}

func (n DeviceID) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (s ShortID) String() string {
	if s == 0 {
		return ""
	}
	var bs [8]byte
	binary.BigEndian.PutUint64(bs[:], uint64(s))
	return base32.StdEncoding.EncodeToString(bs[:])[:7]
}

func (n *DeviceID) UnmarshalText(bs []byte) error {
	id := string(bs)
	id = strings.Trim(id, "=")
	id = strings.ToUpper(id)
	id = untypeoify(id)
	id = unchunkify(id)

	var err error
	switch len(id) {
	case 0:
		*n = EmptyDeviceID
		return nil
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
		return fmt.Errorf("%q: device ID invalid: incorrect length", bs)
	}
}

func (*DeviceID) ProtoSize() int {
	// Used by protobuf marshaller.
	return DeviceIDLength
}

func (n *DeviceID) MarshalTo(bs []byte) (int, error) {
	// Used by protobuf marshaller.
	if len(bs) < DeviceIDLength {
		return 0, errors.New("destination too short")
	}
	copy(bs, (*n)[:])
	return DeviceIDLength, nil
}

func (n *DeviceID) Unmarshal(bs []byte) error {
	// Used by protobuf marshaller.
	if len(bs) < DeviceIDLength {
		return fmt.Errorf("%q: not enough data", bs)
	}
	copy((*n)[:], bs)
	return nil
}

func luhnify(s string) (string, error) {
	if len(s) != 52 {
		panic("unsupported string length")
	}

	res := make([]byte, 4*(13+1))
	for i := 0; i < 4; i++ {
		p := s[i*13 : (i+1)*13]
		copy(res[i*(13+1):], p)
		l, err := luhn32(p)
		if err != nil {
			return "", err
		}
		res[(i+1)*(13)+i] = byte(l)
	}
	return string(res), nil
}

func unluhnify(s string) (string, error) {
	if len(s) != 56 {
		return "", fmt.Errorf("%q: unsupported string length %d", s, len(s))
	}

	res := make([]byte, 52)
	for i := 0; i < 4; i++ {
		p := s[i*(13+1) : (i+1)*(13+1)-1]
		copy(res[i*13:], p)
		l, err := luhn32(p)
		if err != nil {
			return "", err
		}
		if s[(i+1)*14-1] != byte(l) {
			return "", fmt.Errorf("%q: check digit incorrect", s)
		}
	}
	return string(res), nil
}

func chunkify(s string) string {
	chunks := len(s) / 7
	res := make([]byte, chunks*(7+1)-1)
	for i := 0; i < chunks; i++ {
		if i > 0 {
			res[i*(7+1)-1] = '-'
		}
		copy(res[i*(7+1):], s[i*7:(i+1)*7])
	}
	return string(res)
}

func unchunkify(s string) string {
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func untypeoify(s string) string {
	s = strings.ReplaceAll(s, "0", "O")
	s = strings.ReplaceAll(s, "1", "I")
	s = strings.ReplaceAll(s, "8", "B")
	return s
}
