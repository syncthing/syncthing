// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build go1.3

package util

import (
	"sync"
)

type Pool struct {
	sync.Pool
}

func NewPool(cap int) *Pool {
	return &Pool{}
}
