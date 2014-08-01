package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/syncthing/syncthing/luhn"
)

type NodeID [32]byte

var LocalNodeID = NodeID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// NewNodeID generates a new node ID from the raw bytes of a certificate
func NewNodeID(rawCert []byte) NodeID {
	var n NodeID
	hf := sha256.New()
	hf.Write(rawCert)
	hf.Sum(n[:0])
	return n
}

func NodeIDFromString(s string) (NodeID, error) {
	var n NodeID
	err := n.UnmarshalText([]byte(s))
	return n, err
}

func NodeIDFromBytes(bs []byte) NodeID {
	var n NodeID
	if len(bs) != len(n) {
		panic("incorrect length of byte slice representing node ID")
	}
	copy(n[:], bs)
	return n
}

// String returns the canonical string representation of the node ID
func (n NodeID) String() string {
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

func (n NodeID) GoString() string {
	return n.String()
}

func (n NodeID) Compare(other NodeID) int {
	return bytes.Compare(n[:], other[:])
}

func (n NodeID) Equals(other NodeID) bool {
	return bytes.Compare(n[:], other[:]) == 0
}

func (n *NodeID) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n *NodeID) UnmarshalText(bs []byte) error {
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
		return errors.New("node ID invalid: incorrect length")
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
