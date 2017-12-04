// Copyright (C) 2017 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/syncthing/syncthing/lib/fs"
)

type Size struct {
	Value float64 `json:"value" xml:",chardata"`
	Unit  string  `json:"unit" xml:"unit,attr"`
}

func ParseSize(s string) (Size, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return Size{}, nil
	}

	var num, unit string
	for i := 0; i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ','); i++ {
		num = s[:i+1]
	}
	var i = len(num)
	for i < len(s) && s[i] == ' ' {
		i++
	}
	unit = s[i:]

	val, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return Size{}, err
	}

	return Size{val, unit}, nil
}

func (s Size) BaseValue() float64 {
	unitPrefix := s.Unit
	if len(unitPrefix) > 1 {
		unitPrefix = unitPrefix[:1]
	}

	mult := 1.0
	switch unitPrefix {
	case "k", "K":
		mult = 1000
	case "m", "M":
		mult = 1000 * 1000
	case "g", "G":
		mult = 1000 * 1000 * 1000
	case "t", "T":
		mult = 1000 * 1000 * 1000 * 1000
	}

	return s.Value * mult
}

func (s Size) Percentage() bool {
	return strings.Contains(s.Unit, "%")
}

func (s Size) String() string {
	return fmt.Sprintf("%v %s", s.Value, s.Unit)
}

func (Size) ParseDefault(s string) (interface{}, error) {
	return ParseSize(s)
}

func checkFreeSpace(req Size, fs fs.Filesystem) error {
	val := req.BaseValue()
	if val <= 0 {
		return nil
	}

	usage, err := fs.Usage(".")
	if req.Percentage() {
		freePct := (float64(usage.Free) / float64(usage.Total)) * 100
		if err == nil && freePct < val {
			return fmt.Errorf("insufficient space in %v %v: %f %% < %v", fs.Type(), fs.URI(), freePct, req)
		}
	} else {
		if err == nil && float64(usage.Free) < val {
			return fmt.Errorf("insufficient space in %v %v: %v < %v", fs.Type(), fs.URI(), usage.Free, req)
		}
	}

	return nil
}
