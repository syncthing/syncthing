// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package ignore

import (
	"encoding/hex"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestHashIsAboveRangeStart(t *testing.T) {

	testFunc := func(t *testing.T, hash string, start string, expected bool) {
		hashbytes, err := hex.DecodeString(hash)
		assert.NoError(t, err)
		start_bytes, err := ShardRangeParseStartHex(start)
		assert.NoError(t, err)
		assert.Equal(t, expected, HashIsBelowStart(hashbytes, start_bytes))
	}

	below := true

	testFunc(t, "0000", "0", !below)
	testFunc(t, "0000", "00", !below)
	testFunc(t, "0000", "000", !below)

	testFunc(t, "1111", "1", !below)
	testFunc(t, "1111", "11", !below)
	testFunc(t, "1111", "111", !below)

	testFunc(t, "ffff", "f", !below)
	testFunc(t, "ffff", "ff", !below)
	testFunc(t, "ffff", "fff", !below)

	testFunc(t, "1111", "2", below)
	testFunc(t, "1111", "12", below)
	testFunc(t, "1111", "112", below)
	testFunc(t, "1111", "1112", below)
}

func TestHashIsBelowRangeEnd(t *testing.T) {

	testFunc := func(t *testing.T, hash string, end string, expected bool) {
		hashbytes, err := hex.DecodeString(hash)
		assert.NoError(t, err)
		end_bytes, err := ShardRangeParseEndHex(end)
		assert.NoError(t, err)
		assert.Equal(t, expected, HashIsAboveEnd(hashbytes, end_bytes))
	}

	above := true

	testFunc(t, "ffff", "f", !above)
	testFunc(t, "ffff", "ff", !above)
	testFunc(t, "ffff", "fff", !above)
	testFunc(t, "ffff", "ffff", !above)
	testFunc(t, "efff", "e", !above)
	testFunc(t, "0001", "0", !above)
	testFunc(t, "10", "1", !above)

	testFunc(t, "01", "00", above)
	testFunc(t, "1000", "0", above)
	testFunc(t, "ffff", "fffe", above)
}

func TestShardExcludeAll(t *testing.T) {
	se, err := ShardExcludeParse("#shard-exclude:0-f")
	if err != nil {
		t.Fatalf("Failed to parse shard exclude: %v", err)
	}

	testFunc := func(t *testing.T, hash string, expected bool) {
		hashbytes, err := hex.DecodeString(hash)
		assert.NoError(t, err)
		assert.Equal(t, expected, se.MatchHash(hashbytes))
	}

	testFunc(t, "0000", true)
	testFunc(t, "1000", true)
	testFunc(t, "efff", true)
	testFunc(t, "ffff", true)
}

func TestShardExcludeFirstAndLast16th(t *testing.T) {
	se, err := ShardExcludeParse("#shard-exclude:0-0,f-f")
	if err != nil {
		t.Fatalf("Failed to parse shard exclude: %v", err)
	}

	testFunc := func(t *testing.T, hash string, expected bool) {
		hashbytes, err := hex.DecodeString(hash)
		assert.NoError(t, err)
		assert.Equal(t, expected, se.MatchHash(hashbytes))
	}

	testFunc(t, "0000", true)
	testFunc(t, "0fff", true)
	testFunc(t, "1000", false)
	testFunc(t, "5000", false)
	testFunc(t, "a000", false)
	testFunc(t, "efff", false)
	testFunc(t, "f000", true)
	testFunc(t, "ffff", true)
}

func TestShardExcludeFilenameBased(t *testing.T) {
	se, err := ShardExcludeParse("#shard-exclude:734-734")
	if err != nil {
		t.Fatalf("Failed to parse shard exclude: %v", err)
	}

	testFunc := func(t *testing.T, filename string, expected bool) {
		assert.Equal(t, expected, se.Match(filename))
	}

	testFunc(t, "hello.txt", true)
	testFunc(t, "world.txt", false)
}
