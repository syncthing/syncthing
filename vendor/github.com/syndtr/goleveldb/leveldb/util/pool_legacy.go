// Copyright (c) 2014, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !go1.3

package util

type Pool struct {
	pool chan interface{}
}

func (p *Pool) Get() interface{} {
	select {
	case x := <-p.pool:
		return x
	default:
		return nil
	}
}

func (p *Pool) Put(x interface{}) {
	select {
	case p.pool <- x:
	default:
	}
}

func NewPool(cap int) *Pool {
	return &Pool{pool: make(chan interface{}, cap)}
}
