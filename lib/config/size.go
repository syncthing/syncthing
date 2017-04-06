// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"bytes"
	"fmt"
	"strconv"
)

type Size struct {
	value      float64
	percentage bool
	origValue  string
	origUnit   string
}

func (n *Size) UnmarshalText(s []byte) error {
	s = bytes.TrimSpace(s)
	if len(s) == 0 {
		n.value = 0
		n.percentage = false
		n.origValue = ""
		n.origUnit = ""
		return nil
	}

	var num, unit []byte
	for i := 0; i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ','); i++ {
		num = s[:i+1]
	}
	var i = len(num)
	for i < len(s) && s[i] == ' ' {
		i++
	}
	unit = s[i:]

	n.origValue = string(num)
	n.origUnit = string(unit)

	val, err := strconv.ParseFloat(n.origValue, 64)
	if err != nil {
		return err
	}

	switch n.origUnit {
	case "":
		n.value = val
		n.percentage = false
	case "%":
		n.value = val
		n.percentage = true
	case "k", "K":
		n.value = val * 1000
		n.percentage = false
	case "Ki":
		n.value = val * 1024
		n.percentage = false
	case "m", "M":
		n.value = val * 1000 * 1000
		n.percentage = false
	case "Mi":
		n.value = val * 1024 * 1024
		n.percentage = false
	case "g", "G":
		n.value = val * 1000 * 1000 * 1000
		n.percentage = false
	case "Gi":
		n.value = val * 1024 * 1024 * 1024
		n.percentage = false
	case "t", "T":
		n.value = val * 1000 * 1000 * 1000 * 1000
		n.percentage = false
	case "Ti":
		n.value = val * 1024 * 1024 * 1024 * 1024
		n.percentage = false
	default:
		return fmt.Errorf("unknown unit: %q", n.origUnit)
	}

	return nil
}

func (n Size) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n Size) Value() float64 {
	return n.value
}

func (n Size) Percentage() bool {
	return n.percentage
}

func (n Size) String() string {
	if n.origValue == "" && n.origUnit == "" {
		return ""
	}
	return n.origValue + " " + normalizeUnit(n.origUnit)
}

func (n Size) ParseDefault(s string) (interface{}, error) {
	err := n.UnmarshalText([]byte(s))
	return n, err
}

func normalizeUnit(unit string) string {
	switch unit {
	case "K":
		return "k"
	case "m":
		return "M"
	case "g":
		return "G"
	case "t":
		return "T"
	default:
		return unit
	}
}

func MustParseSize(s string) Size {
	var n Size
	if err := n.UnmarshalText([]byte(s)); err != nil {
		panic("must parse size: " + s)
	}
	return n
}
