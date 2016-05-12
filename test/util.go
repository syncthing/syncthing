// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

// +build integration

package integration

import (
	"crypto/md5"
	cr "crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/rc"
	"github.com/syncthing/syncthing/lib/symlinks"
)

func init() {
	rand.Seed(42)
}

func ReadRand(bs []byte) (int, error) {
	var r uint32
	for i := range bs {
		if i%4 == 0 {
			r = uint32(rand.Int63())
		}
		bs[i] = byte(r >> uint((i%4)*8))
	}
	return len(bs), nil
}

func (i *inifiteReader) Read(bs []byte) (int, error) {
	n, err := i.rd.Read(bs)
	if err == io.EOF {
		err = nil
		i.rd.Seek(0, 0)
	}
	return n, err
}

func (f fileInfo) String() string {
	return fmt.Sprintf("%s %04o %d %x", f.name, f.mode, f.mod, f.hash)
}

func (l fileInfoList) Len() int {
	return len(l)
}

func (l fileInfoList) Less(a, b int) bool {
	return l[a].name < l[b].name
}

func (l fileInfoList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
