// Copyright (c) 2012, Suryandaru Triandana <syndtr@gmail.com>
// All rights reserved.
//
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package leveldb

import "github.com/syndtr/goleveldb/leveldb/comparer"

type iComparer struct {
	cmp comparer.Comparer
}

func (p *iComparer) Name() string {
	return p.cmp.Name()
}

func (p *iComparer) Compare(a, b []byte) int {
	ia, ib := iKey(a), iKey(b)
	r := p.cmp.Compare(ia.ukey(), ib.ukey())
	if r == 0 {
		an, bn := ia.num(), ib.num()
		if an > bn {
			r = -1
		} else if an < bn {
			r = 1
		}
	}
	return r
}

func (p *iComparer) Separator(dst, a, b []byte) []byte {
	ua, ub := iKey(a).ukey(), iKey(b).ukey()
	dst = p.cmp.Separator(dst, ua, ub)
	if dst == nil {
		return nil
	}
	if len(dst) < len(ua) && p.cmp.Compare(ua, dst) < 0 {
		dst = append(dst, kMaxNumBytes...)
	} else {
		// Did not close possibilities that n maybe longer than len(ub).
		dst = append(dst, a[len(a)-8:]...)
	}
	return dst
}

func (p *iComparer) Successor(dst, b []byte) []byte {
	ub := iKey(b).ukey()
	dst = p.cmp.Successor(dst, ub)
	if dst == nil {
		return nil
	}
	if len(dst) < len(ub) && p.cmp.Compare(ub, dst) < 0 {
		dst = append(dst, kMaxNumBytes...)
	} else {
		// Did not close possibilities that n maybe longer than len(ub).
		dst = append(dst, b[len(b)-8:]...)
	}
	return dst
}
