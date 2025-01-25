// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/syncthing/syncthing/lib/ignore/ignoreresult"
)

const ShardExcludePrefixCommon = "#shard-exclude"
const ShardExcludePrefix = ShardExcludePrefixCommon + ":"
const ShardExcludePurgePrefix = ShardExcludePrefixCommon + "-purge:"

func HashIsBelowStart(hash, range_start []byte) bool {
	minLen := min(len(hash), len(range_start))
	for i := 0; i < minLen; i++ {
		if hash[i] < range_start[i] {
			return true
		}
		if hash[i] > range_start[i] {
			return false
		}
	}
	return false
}

func HashIsAboveEnd(hash, range_end []byte) bool {
	minLen := min(len(hash), len(range_end))
	for i := 0; i < minLen; i++ {
		if hash[i] > range_end[i] {
			return true
		}
		if hash[i] < range_end[i] {
			return false
		}
	}
	return false
}

type SingleShardRange struct {
	start_incl []byte
	end_incl   []byte
}

func (s SingleShardRange) IsInRange(hash []byte) bool {
	return !HashIsBelowStart(hash, s.start_incl) && !HashIsAboveEnd(hash, s.end_incl)
}

type ShardExclude struct {
	excludes []SingleShardRange
	ignoreR  ignoreresult.R
}

func (s ShardExclude) MatchHash(sha256 []byte) bool {
	excluded := false
	for _, ex := range s.excludes {
		excluded = excluded || ex.IsInRange(sha256)
	}
	return excluded
}

func (s ShardExclude) Match(file string) bool {
	sha256 := sha256.Sum256([]byte(file))
	return s.MatchHash(sha256[:])
}

func ShardRangeParseStartHex(hex_start string) ([]byte, error) {
	if len(hex_start)%2 != 0 {
		hex_start = hex_start + "0"
	}
	return hex.AppendDecode(nil, []byte(hex_start))
}

func ShardRangeParseEndHex(hex_end string) ([]byte, error) {
	if len(hex_end)%2 != 0 {
		hex_end = hex_end + "f"
	}
	return hex.AppendDecode(nil, []byte(hex_end))
}

func ShardExcludeParse(line string) (ShardExclude, error) {
	var se ShardExclude

	switch {
	case strings.HasPrefix(line, ShardExcludePrefix):
		line = line[len(ShardExcludePrefix):]
		se.ignoreR = ignoreresult.Ignored
	case strings.HasPrefix(line, ShardExcludePurgePrefix):
		line = line[len(ShardExcludePurgePrefix):]
		se.ignoreR = ignoreresult.IgnoredDeletable
	default:
		return se, errors.New("not a shard exclude line")
	}

	for _, part := range strings.Split(line, ",") {
		if part == "" {
			continue
		}
		var sse SingleShardRange
		parts := strings.Split(part, "-")
		if len(parts) != 2 {
			return se, errors.New("invalid shard exclude line")
		}
		var err1, err2 error
		sse.start_incl, err1 = ShardRangeParseStartHex(parts[0])
		sse.end_incl, err2 = ShardRangeParseEndHex(parts[1])
		if err1 != nil || err2 != nil {
			return se, errors.New("invalid shard exclude line")
		}

		se.excludes = append(se.excludes, sse)
	}
	return se, nil
}
