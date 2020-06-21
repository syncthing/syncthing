// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build ursrv

package contract

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

type Int64Array pq.Int64Array

func (p IntMap) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *IntMap) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("Type assertion .([]byte) failed.")
	}

	var i map[string]int
	err := json.Unmarshal(source, &i)
	if err != nil {
		return err
	}

	*p = i
	return nil
}

func (r Report) Value() (driver.Value, error) {
	// This needs to be string, yet we read back bytes..
	bs, err := json.Marshal(r)
	return string(bs), err
}

func (r *Report) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &r)
}
