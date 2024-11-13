// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package config

type BlockPullOrder int32

const (
	BlockPullOrderStandard BlockPullOrder = 0
	BlockPullOrderRandom   BlockPullOrder = 1
	BlockPullOrderInOrder  BlockPullOrder = 2
)

func (o BlockPullOrder) String() string {
	switch o {
	case BlockPullOrderStandard:
		return "standard"
	case BlockPullOrderRandom:
		return "random"
	case BlockPullOrderInOrder:
		return "inOrder"
	default:
		return "unknown"
	}
}

func (o BlockPullOrder) MarshalText() ([]byte, error) {
	return []byte(o.String()), nil
}

func (o *BlockPullOrder) UnmarshalText(bs []byte) error {
	switch string(bs) {
	case "standard":
		*o = BlockPullOrderStandard
	case "random":
		*o = BlockPullOrderRandom
	case "inOrder":
		*o = BlockPullOrderInOrder
	default:
		*o = BlockPullOrderStandard
	}
	return nil
}
