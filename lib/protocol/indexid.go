// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/syncthing/syncthing/lib/rand"
)

type IndexID uint64

func (i IndexID) String() string {
	return fmt.Sprintf("0x%016X", uint64(i))
}

func (i IndexID) Marshal() ([]byte, error) {
	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(i))
	return bs, nil
}

func (i *IndexID) Unmarshal(bs []byte) error {
	if len(bs) != 8 {
		return errors.New("incorrect IndexID length")
	}
	*i = IndexID(binary.BigEndian.Uint64(bs))
	return nil
}

func NewIndexID() IndexID {
	return IndexID(rand.Uint64())
}
