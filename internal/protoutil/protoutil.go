// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package protoutil

import (
	"errors"

	"google.golang.org/protobuf/proto"
)

var errBufferTooSmall = errors.New("buffer too small")

func MarshalTo(buf []byte, pb proto.Message) (int, error) {
	if sz := proto.Size(pb); len(buf) < sz {
		return 0, errBufferTooSmall
	} else if sz == 0 {
		return 0, nil
	}
	opts := proto.MarshalOptions{}
	bs, err := opts.MarshalAppend(buf[:0], pb)
	if err != nil {
		return 0, err
	}
	if &buf[0] != &bs[0] {
		panic("can't happen: slice was reallocated")
	}
	return len(bs), nil
}
