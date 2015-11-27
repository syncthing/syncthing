// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import "github.com/syndtr/goleveldb/leveldb/comparer"

type iComparer struct {
	ucmp comparer.Comparer
}

func (icmp *iComparer) uName() string {
	return icmp.ucmp.Name()
}

func (icmp *iComparer) uCompare(a, b []byte) int {
	return icmp.ucmp.Compare(a, b)
}

func (icmp *iComparer) uSeparator(dst, a, b []byte) []byte {
	return icmp.ucmp.Separator(dst, a, b)
}

func (icmp *iComparer) uSuccessor(dst, b []byte) []byte {
	return icmp.ucmp.Successor(dst, b)
}

func (icmp *iComparer) Name() string {
	return icmp.uName()
}

func (icmp *iComparer) Compare(a, b []byte) int {
	x := icmp.ucmp.Compare(iKey(a).ukey(), iKey(b).ukey())
	if x == 0 {
		if m, n := iKey(a).num(), iKey(b).num(); m > n {
			x = -1
		} else if m < n {
			x = 1
		}
	}
	return x
}

func (icmp *iComparer) Separator(dst, a, b []byte) []byte {
	ua, ub := iKey(a).ukey(), iKey(b).ukey()
	dst = icmp.ucmp.Separator(dst, ua, ub)
	if dst == nil {
		return nil
	}
	if len(dst) < len(ua) && icmp.uCompare(ua, dst) < 0 {
		dst = append(dst, kMaxNumBytes...)
	} else {
		// Did not close possibilities that n maybe longer than len(ub).
		dst = append(dst, a[len(a)-8:]...)
	}
	return dst
}

func (icmp *iComparer) Successor(dst, b []byte) []byte {
	ub := iKey(b).ukey()
	dst = icmp.ucmp.Successor(dst, ub)
	if dst == nil {
		return nil
	}
	if len(dst) < len(ub) && icmp.uCompare(ub, dst) < 0 {
		dst = append(dst, kMaxNumBytes...)
	} else {
		// Did not close possibilities that n maybe longer than len(ub).
		dst = append(dst, b[len(b)-8:]...)
	}
	return dst
}
