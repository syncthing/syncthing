// Copyright 2015 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ql

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cznic/b"
	"github.com/cznic/strutil"
)

const (
	_               = iota
	indexEq         // [L]
	indexFalse      // [false]
	indexGe         // [L, ...)
	indexGt         // (L, ...)
	indexIsNotNull  // (NULL, ...)
	indexIsNull     // [NULL]
	indexIntervalCC // [L, H]
	indexIntervalCO // [L, H)
	indexIntervalOC // (L, H]
	indexIntervalOO // (L, H)
	indexLe         // (..., H]
	indexLt         // (..., H)
	indexNe         // (L)
	indexTrue       // [true]
)

// Note: All plans must have a pointer receiver. Enables planA == planB operation.
var (
	_ plan = (*crossJoinDefaultPlan)(nil)
	_ plan = (*distinctDefaultPlan)(nil)
	_ plan = (*explainDefaultPlan)(nil)
	_ plan = (*filterDefaultPlan)(nil)
	_ plan = (*fullJoinDefaultPlan)(nil)
	_ plan = (*groupByDefaultPlan)(nil)
	_ plan = (*indexPlan)(nil)
	_ plan = (*leftJoinDefaultPlan)(nil)
	_ plan = (*limitDefaultPlan)(nil)
	_ plan = (*nullPlan)(nil)
	_ plan = (*offsetDefaultPlan)(nil)
	_ plan = (*orderByDefaultPlan)(nil)
	_ plan = (*rightJoinDefaultPlan)(nil)
	_ plan = (*selectFieldsDefaultPlan)(nil)
	_ plan = (*selectFieldsGroupPlan)(nil)
	_ plan = (*selectIndexDefaultPlan)(nil)
	_ plan = (*sysColumnDefaultPlan)(nil)
	_ plan = (*sysIndexDefaultPlan)(nil)
	_ plan = (*sysTableDefaultPlan)(nil)
	_ plan = (*tableDefaultPlan)(nil)
	_ plan = (*tableNilPlan)(nil)
)

type plan interface {
	do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) error
	explain(w strutil.Formatter)
	fieldNames() []string
	filter(expr expression) (p plan, indicesSought []string, err error)
	hasID() bool
}

func isTableOrIndex(p plan) bool {
	switch p.(type) {
	case
		*indexPlan,
		*sysColumnDefaultPlan,
		*sysIndexDefaultPlan,
		*sysTableDefaultPlan,
		*tableDefaultPlan:
		return true
	default:
		return false
	}
}

// Invariants
// - All interval plans produce rows in ascending index value collating order.
// - L <= H
type indexPlan struct {
	src   *table
	cname string
	xname string
	x     btreeIndex
	kind  int         // See interval* consts.
	lval  interface{} // L, H: Ordered and comparable (lldb perspective).
	hval  interface{}
}

func (r *indexPlan) doGe(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ...
	// ---  ---  ---  ---  ---  +  +  +++  +++  +++
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	for {
		_, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doGt(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ...
	// ---  ---  ---  ---  ---  -  -  +++  +++  +++
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	var ok bool
	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if !ok {
			val, err := expand1(k[0], nil)
			if err != nil {
				return err
			}

			if collate1(val, r.lval) == 0 {
				continue
			}

			ok = true
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doInterval00(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  ---  ---  ---  -  -  +++  +++  +++  +++  +++  -  -  ---  ---  ---
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	var ok bool
	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if !ok {
			val, err := expand1(k[0], nil)
			if err != nil {
				return err
			}

			if collate1(val, r.lval) == 0 {
				continue
			}

			ok = true
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) == 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doIntervalOC(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  ---  ---  ---  -  -  +++  +++  +++  +++  +++  +  +  ---  ---  ---
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	var ok bool
	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if !ok {
			val, err := expand1(k[0], nil)
			if err != nil {
				return err
			}

			if collate1(val, r.lval) == 0 {
				continue
			}

			ok = true
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) > 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doIntervalCC(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  ---  ---  ---  +  +  +++  +++  +++  +++  +++  +  +  ---  ---  ---
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) > 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doIntervalCO(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  ---  ---  ---  +  +  +++  +++  +++  +++  +++  -  -  ---  ---  ---
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) >= 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doLe(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  +++  +++  +++  +  +  ---  ---
	t := r.src
	it, err := r.x.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if k == nil || k[0] == nil {
			continue
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) > 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doLt(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., H-1, H-1, H, H, H+1, H+1, ...
	// ---  ---  +++  +++  +++  -  -  ---  ---
	t := r.src
	it, err := r.x.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if k == nil || k[0] == nil {
			continue
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) >= 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doEq(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ...
	// ---  ---  ---  ---  ---  +  +  ---  ---  ---
	t := r.src
	it, _, err := r.x.Seek([]interface{}{r.lval})
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.lval) != 0 {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doNe(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ..., L-1, L-1, L, L, L+1, L+1, ...
	// ---  ---  +++  +++  +++  -  -  +++  +++  +++
	t := r.src
	it, err := r.x.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if k == nil || k[0] == nil {
			continue
		}

		val, err := expand1(k[0], nil)
		if err != nil {
			return err
		}

		if collate1(val, r.hval) == 0 {
			continue
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doIsNull(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ...
	// +++  +++  ---
	t := r.src
	it, err := r.x.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if k != nil && k[0] != nil {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doIsNotNull(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	// nil, nil, ...
	// ---  ---  +++
	t := r.src
	it, _, err := r.x.Seek([]interface{}{false}) // lldb collates false right after NULL.
	if err != nil {
		return noEOF(err)
	}

	for {
		_, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doFalse(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	t := r.src
	it, _, err := r.x.Seek([]interface{}{false})
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		b, ok := k[0].(bool)
		if !ok || b {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) doTrue(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	t := r.src
	it, _, err := r.x.Seek([]interface{}{true})
	if err != nil {
		return noEOF(err)
	}

	for {
		k, h, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		if _, ok := k[0].(bool); !ok {
			return nil
		}

		id, data, err := t.row(ctx, h)
		if err != nil {
			return err
		}

		if more, err := f(id, data); err != nil || !more {
			return err
		}
	}
}

func (r *indexPlan) do(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) error {
	switch r.kind {
	case indexEq:
		return r.doEq(ctx, f)
	case indexGe:
		return r.doGe(ctx, f)
	case indexGt:
		return r.doGt(ctx, f)
	case indexLe:
		return r.doLe(ctx, f)
	case indexLt:
		return r.doLt(ctx, f)
	case indexNe:
		return r.doNe(ctx, f)
	case indexIsNull:
		return r.doIsNull(ctx, f)
	case indexIsNotNull:
		return r.doIsNotNull(ctx, f)
	case indexFalse:
		return r.doFalse(ctx, f)
	case indexTrue:
		return r.doTrue(ctx, f)
	case indexIntervalOO:
		return r.doInterval00(ctx, f)
	case indexIntervalCC:
		return r.doIntervalCC(ctx, f)
	case indexIntervalOC:
		return r.doIntervalOC(ctx, f)
	case indexIntervalCO:
		return r.doIntervalCO(ctx, f)
	default:
		//dbg("", r.kind)
		panic("internal error 072")
	}
}

func (r *indexPlan) explain(w strutil.Formatter) {
	s := ""
	if r.kind == indexFalse {
		s = "!"
	}
	w.Format("┌Iterate all rows of table %q using index %q where %s%s", r.src.name, r.xname, s, r.cname)
	switch r.kind {
	case indexEq:
		w.Format(" == %v", value{r.lval})
	case indexGe:
		w.Format(" >= %v", value{r.lval})
	case indexGt:
		w.Format(" > %v", value{r.lval})
	case indexLe:
		w.Format(" <= %v", value{r.hval})
	case indexLt:
		w.Format(" < %v", value{r.hval})
	case indexNe:
		w.Format(" != %v", value{r.lval})
	case indexIsNull:
		w.Format(" IS NULL")
	case indexIsNotNull:
		w.Format(" IS NOT NULL")
	case indexFalse, indexTrue:
		// nop
	case indexIntervalOO:
		w.Format(" > %v && %s < %v", value{r.lval}, r.cname, value{r.hval})
	case indexIntervalCC:
		w.Format(" >= %v && %s <= %v", value{r.lval}, r.cname, value{r.hval})
	case indexIntervalCO:
		w.Format(" >= %v && %s < %v", value{r.lval}, r.cname, value{r.hval})
	case indexIntervalOC:
		w.Format(" > %v && %s <= %v", value{r.lval}, r.cname, value{r.hval})
	default:
		//dbg("", r.kind)
		panic("internal error 073")
	}
	w.Format("\n└Output field names %v\n", qnames(r.fieldNames()))
}

func (r *indexPlan) fieldNames() []string { return r.src.fieldNames() }

func (r *indexPlan) filterEq(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(r.lval, val) == 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		if collate1(r.lval, val) >= 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '>':
		if collate1(r.lval, val) > 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case le:
		if collate1(r.lval, val) <= 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '<':
		if collate1(r.lval, val) < 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case neq:
		if collate1(r.lval, val) != 0 {
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	}
	return nil, nil, nil
}

func (r *indexPlan) filterGe(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(r.lval, val) <= 0 {
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		if collate1(r.lval, val) < 0 {
			r.lval = val
		}
		return r, nil, nil
	case '>':
		if collate1(r.lval, val) <= 0 {
			r.lval = val
			r.kind = indexGt
		}
		return r, nil, nil
	case le:
		switch c := collate1(r.lval, val); {
		case c < 0:
			r.hval = val
			r.kind = indexIntervalCC
			return r, nil, nil
		case c == 0:
			r.kind = indexEq
			return r, nil, nil
		default: // c > 0
			return &nullPlan{r.fieldNames()}, nil, nil
		}
	case '<':
		if collate1(r.lval, val) < 0 {
			r.hval = val
			r.kind = indexIntervalCO
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case neq:
		switch c := collate1(r.lval, val); {
		case c < 0:
			//MAYBE ORed intervals
		case c == 0:
			r.kind = indexGt
			return r, nil, nil
		default: // c > 0
			return r, nil, nil
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterGt(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(r.lval, val) < 0 {
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		if collate1(r.lval, val) < 0 {
			r.lval = val
			r.kind = indexGe
		}
		return r, nil, nil
	case '>':
		if collate1(r.lval, val) < 0 {
			r.lval = val
		}
		return r, nil, nil
	case le:
		if collate1(r.lval, val) < 0 {
			r.hval = val
			r.kind = indexIntervalOC
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '<':
		if collate1(r.lval, val) < 0 {
			r.hval = val
			r.kind = indexIntervalOO
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case neq:
		if collate1(r.lval, val) >= 0 {
			return r, nil, nil
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterLe(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(r.hval, val) >= 0 {
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		switch c := collate1(r.hval, val); {
		case c < 0:
			return &nullPlan{r.fieldNames()}, nil, nil
		case c == 0:
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		default: // c > 0
			r.lval = val
			r.kind = indexIntervalCC
			return r, nil, nil
		}
	case '>':
		if collate1(r.hval, val) > 0 {
			r.lval = val
			r.kind = indexIntervalOC
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case le:
		if collate1(r.hval, val) > 0 {
			r.hval = val
		}
		return r, nil, nil
	case '<':
		if collate1(r.hval, val) >= 0 {
			r.hval = val
			r.kind = indexLt
		}
		return r, nil, nil
	case neq:
		switch c := collate1(r.hval, val); {
		case c < 0:
			return r, nil, nil
		case c == 0:
			r.kind = indexLt
			return r, nil, nil
		default: // c > 0
			//bop
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterLt(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(r.hval, val) > 0 {
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		if collate1(r.hval, val) > 0 {
			r.lval = val
			r.kind = indexIntervalCO
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '>':
		if collate1(r.hval, val) > 0 {
			r.lval = val
			r.kind = indexIntervalOO
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case le:
		if collate1(r.hval, val) > 0 {
			r.hval = val
			r.kind = indexLe
		}
		return r, nil, nil
	case '<':
		if collate1(r.hval, val) > 0 {
			r.hval = val
		}
		return r, nil, nil
	case neq:
		if collate1(r.hval, val) > 0 {
			return nil, nil, nil
		}

		return r, nil, nil
	}
	return nil, nil, nil
}

func (r *indexPlan) filterCC(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(val, r.lval) < 0 || collate1(val, r.hval) > 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		r.lval = val
		r.kind = indexEq
		return r, nil, nil
	case ge:
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		switch c := collate1(val, r.hval); {
		case c < 0:
			r.lval = val
			return r, nil, nil
		case c == 0:
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		default:
			return &nullPlan{r.fieldNames()}, nil, nil
		}
	case '>':
		switch c := collate1(val, r.lval); {
		case c < 0:
			return r, nil, nil
		case c == 0:
			r.kind = indexIntervalOC
			return r, nil, nil
		default:
			if collate1(val, r.hval) < 0 {
				r.lval = val
				r.kind = indexIntervalOC
				return r, nil, nil
			}

			return &nullPlan{r.fieldNames()}, nil, nil
		}
	case le:
		switch c := collate1(val, r.lval); {
		case c < 0:
			return &nullPlan{r.fieldNames()}, nil, nil
		case c == 0:
			r.kind = indexEq
			return r, nil, nil
		default:
			if collate1(val, r.hval) < 0 {
				r.hval = val
			}
			return r, nil, nil
		}
	case '<':
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		if collate1(val, r.hval) <= 0 {
			r.hval = val
			r.kind = indexIntervalCO
		}
		return r, nil, nil
	case neq:
		switch c := collate1(val, r.lval); {
		case c < 0:
			return r, nil, nil
		case c == 0:
			r.kind = indexIntervalOC
			return r, nil, nil
		default:
			switch c := collate1(val, r.hval); {
			case c == 0:
				r.kind = indexIntervalCO
				return r, nil, nil
			case c > 0:
				return r, nil, nil
			default:
				return nil, nil, nil
			}
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterOC(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(val, r.lval) <= 0 || collate1(val, r.hval) > 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		r.lval = val
		r.kind = indexEq
		return r, nil, nil
	case ge:
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		switch c := collate1(val, r.hval); {
		case c < 0:
			r.lval = val
			r.kind = indexIntervalCC
			return r, nil, nil
		case c == 0:
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		default:
			return &nullPlan{r.fieldNames()}, nil, nil
		}
	case '>':
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.lval = val
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case le:
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.hval = val
		}
		return r, nil, nil
	case '<':
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		switch c := collate1(val, r.hval); {
		case c < 0:
			r.hval = val
			r.kind = indexIntervalOO
		case c == 0:
			r.kind = indexIntervalOO
		}
		return r, nil, nil
	case neq:
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		switch c := collate1(val, r.hval); {
		case c < 0:
			return nil, nil, nil
		case c == 0:
			r.kind = indexIntervalOO
			return r, nil, nil
		default:
			return r, nil, nil
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterOO(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(val, r.lval) <= 0 || collate1(val, r.hval) >= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		r.lval = val
		r.kind = indexEq
		return r, nil, nil
	case ge:
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.lval = val
			r.kind = indexIntervalCO
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '>':
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.lval = val
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case le:
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.hval = val
			r.kind = indexIntervalOC
		}
		return r, nil, nil
	case '<':
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.hval = val
		}
		return r, nil, nil
	case neq:
		if collate1(val, r.lval) <= 0 || collate1(val, r.hval) >= 0 {
			return r, nil, nil
		}

		return nil, nil, nil
	}
	return nil, nil, nil
}

func (r *indexPlan) filterCO(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(val, r.lval) < 0 || collate1(val, r.hval) >= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		r.lval = val
		r.kind = indexEq
		return r, nil, nil
	case ge:
		if collate1(val, r.lval) <= 0 {
			return r, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.lval = val
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case '>':
		switch c := collate1(val, r.lval); {
		case c < 0:
			return r, nil, nil
		case c == 0:
			r.kind = indexIntervalOO
			return r, nil, nil
		default:
			if collate1(val, r.hval) < 0 {
				r.lval = val
				r.kind = indexIntervalOO
				return r, nil, nil
			}

			return &nullPlan{r.fieldNames()}, nil, nil
		}
	case le:
		switch c := collate1(val, r.lval); {
		case c < 0:
			return &nullPlan{r.fieldNames()}, nil, nil
		case c == 0:
			r.kind = indexEq
			return r, nil, nil
		default:
			if collate1(val, r.hval) < 0 {
				r.hval = val
				r.kind = indexIntervalCC
			}
			return r, nil, nil
		}
	case '<':
		if collate1(val, r.lval) <= 0 {
			return &nullPlan{r.fieldNames()}, nil, nil
		}

		if collate1(val, r.hval) < 0 {
			r.hval = val
		}
		return r, nil, nil
	case neq:
		switch c := collate1(val, r.lval); {
		case c < 0:
			return r, nil, nil
		case c == 0:
			r.kind = indexIntervalOO
			return r, nil, nil
		default:
			if collate1(val, r.hval) < 0 {
				return nil, nil, nil
			}

			return r, nil, nil
		}
	}
	return nil, nil, nil
}

func (r *indexPlan) filterNe(binOp2 int, val interface{}) (plan, []string, error) {
	switch binOp2 {
	case eq:
		if collate1(val, r.lval) != 0 {
			r.lval = val
			r.kind = indexEq
			return r, nil, nil
		}

		return &nullPlan{r.fieldNames()}, nil, nil
	case ge:
		switch c := collate1(val, r.lval); {
		case c < 0:
			return nil, nil, nil //TODO
		case c == 0:
			r.kind = indexGt
			return r, nil, nil
		default:
			r.lval = val
			r.kind = indexGe
			return r, nil, nil
		}
	case '>':
		if collate1(val, r.lval) < 0 {
			return nil, nil, nil //TODO
		}

		r.lval = val
		r.kind = indexGt
		return r, nil, nil
	case le:
		switch c := collate1(val, r.lval); {
		case c < 0:
			r.hval = val
			r.kind = indexLe
			return r, nil, nil
		case c == 0:
			r.kind = indexLt
			return r, nil, nil
		default:
			return nil, nil, nil //TODO
		}
	case '<':
		if collate1(val, r.lval) <= 0 {
			r.hval = val
			r.kind = indexLt
			return r, nil, nil
		}

		return nil, nil, nil //TODO
	case neq:
		if collate1(val, r.lval) == 0 {
			return r, nil, nil
		}

		return nil, nil, nil
	}
	return nil, nil, nil
}

func (r *indexPlan) filter(expr expression) (plan, []string, error) {
	switch x := expr.(type) {
	case *binaryOperation:
		ok, cname, val, err := x.isIdentRelOpVal()
		if err != nil {
			return nil, nil, err
		}

		if !ok || r.cname != cname {
			break
		}

		if val, err = typeCheck1(val, findCol(r.src.cols, cname)); err != nil {
			return nil, nil, err
		}

		switch r.kind {
		case indexEq: // [L]
			return r.filterEq(x.op, val)
		case indexGe: // [L, ...)
			return r.filterGe(x.op, val)
		case indexGt: // (L, ...)
			return r.filterGt(x.op, val)
		case indexIntervalCC: // [L, H]
			return r.filterCC(x.op, val)
		case indexIntervalCO: // [L, H)
			return r.filterCO(x.op, val)
		case indexIntervalOC: // (L, H]
			return r.filterOC(x.op, val)
		case indexIntervalOO: // (L, H)
			return r.filterOO(x.op, val)
		case indexLe: // (..., H]
			return r.filterLe(x.op, val)
		case indexLt: // (..., H)
			return r.filterLt(x.op, val)
		case indexNe: // (L)
			return r.filterNe(x.op, val)
		}
	case *ident:
		cname := x.s
		if r.cname != cname {
			break
		}

		switch r.kind {
		case indexFalse: // [false]
			return &nullPlan{r.fieldNames()}, nil, nil
		case indexTrue: // [true]
			return r, nil, nil
		}
	case *unaryOperation:
		if x.op != '!' {
			break
		}

		operand, ok := x.v.(*ident)
		if !ok {
			break
		}

		cname := operand.s
		if r.cname != cname {
			break
		}

		switch r.kind {
		case indexFalse: // [false]
			return r, nil, nil
		case indexTrue: // [true]
			return &nullPlan{r.fieldNames()}, nil, nil
		}
	}

	return nil, nil, nil
}

func (r *indexPlan) hasID() bool { return true }

type explainDefaultPlan struct {
	s stmt
}

func (r *explainDefaultPlan) hasID() bool { return false }

func (r *explainDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) error {
	var buf bytes.Buffer
	switch x := r.s.(type) {
	default:
		w := strutil.IndentFormatter(&buf, "│   ")
		x.explain(ctx, w)
	}

	a := bytes.Split(buf.Bytes(), []byte{'\n'})
	for _, v := range a[:len(a)-1] {
		if more, err := f(nil, []interface{}{string(v)}); !more || err != nil {
			return err
		}
	}
	return nil
}

func (r *explainDefaultPlan) explain(w strutil.Formatter) {
	return
}

func (r *explainDefaultPlan) fieldNames() []string {
	return []string{""}
}

func (r *explainDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

type filterDefaultPlan struct {
	plan
	expr expression
	is   []string
}

func (r *filterDefaultPlan) hasID() bool { return r.plan.hasID() }

func (r *filterDefaultPlan) explain(w strutil.Formatter) {
	r.plan.explain(w)
	w.Format("┌Filter on %v\n", r.expr)
	if len(r.is) != 0 {
		w.Format("│Possibly useful indices\n")
		m := map[string]bool{}
		for _, v := range r.is {
			if !m[v] {
				m[v] = true
				n := ""
				for _, r := range v {
					if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
						n += string(r)
						continue
					}

					n += "_"
				}
				for strings.Contains(n, "__") {
					n = strings.Replace(n, "__", "_", -1)
				}
				for strings.HasSuffix(n, "_") {
					n = n[:len(n)-1]
				}
				w.Format("│CREATE INDEX x%s ON %s;\n", n, v)
			}
		}
	}
	w.Format("└Output field names %v\n", qnames(r.plan.fieldNames()))
}

func (r *filterDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	m := map[interface{}]interface{}{}
	fields := r.plan.fieldNames()
	return r.plan.do(ctx, func(rid interface{}, data []interface{}) (bool, error) {
		for i, v := range fields {
			m[v] = data[i]
		}
		m["$id"] = rid
		val, err := r.expr.eval(ctx, m)
		if err != nil {
			return false, err
		}

		if val == nil {
			return true, nil
		}

		x, ok := val.(bool)
		if !ok {
			return false, fmt.Errorf("invalid boolean expression %s (value of type %T)", val, val)
		}

		if !x {
			return true, nil
		}

		return f(rid, data)
	})
}

type crossJoinDefaultPlan struct {
	rsets  []plan
	names  []string
	fields []string
}

func (r *crossJoinDefaultPlan) hasID() bool { return false }

func (r *crossJoinDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Compute Cartesian product of%i\n")
	for i, v := range r.rsets {
		sel := !isTableOrIndex(v)
		if sel {
			w.Format("┌Iterate all rows of virtual table %q%i\n", r.names[i])
		}
		v.explain(w)
		if sel {
			w.Format("%u└Output field names %v\n", qnames(v.fieldNames()))
		}
	}
	w.Format("%u└Output field names %v\n", qnames(r.fields))
}

func (r *crossJoinDefaultPlan) filter(expr expression) (plan, []string, error) {
	var is []string
	for i, v := range r.names {
		e2, err := expr.clone(nil, v)
		if err != nil {
			return nil, nil, err
		}

		p2, is2, err := r.rsets[i].filter(e2)
		is = append(is, is2...)
		if err != nil {
			return nil, nil, err
		}

		if p2 != nil {
			r.rsets[i] = p2
			return r, is, nil
		}
	}
	return nil, is, nil
}

func (r *crossJoinDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	if len(r.rsets) == 1 {
		return r.rsets[0].do(ctx, f)
	}

	ids := map[string]interface{}{}
	var g func([]interface{}, []plan, int) error
	g = func(prefix []interface{}, rsets []plan, x int) (err error) {
		return rsets[0].do(ctx, func(id interface{}, in []interface{}) (bool, error) {
			ids[r.names[x]] = id
			if len(rsets) > 1 {
				return true, g(append(prefix, in...), rsets[1:], x+1)
			}

			return f(ids, append(prefix, in...))
		})
	}
	return g(nil, r.rsets, 0)
}

func (r *crossJoinDefaultPlan) fieldNames() []string { return r.fields }

type distinctDefaultPlan struct {
	src    plan
	fields []string
}

func (r *distinctDefaultPlan) hasID() bool { return false }

func (r *distinctDefaultPlan) explain(w strutil.Formatter) {
	r.src.explain(w)
	w.Format("┌Compute distinct rows\n└Output field names %v\n", r.fields)
}

func (r *distinctDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *distinctDefaultPlan) fieldNames() []string { return r.fields }

func (r *distinctDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	t, err := ctx.db.store.CreateTemp(true)
	if err != nil {
		return
	}

	defer func() {
		if derr := t.Drop(); derr != nil && err == nil {
			err = derr
		}
	}()

	if err = r.src.do(ctx, func(id interface{}, in []interface{}) (bool, error) {
		if err = t.Set(in, nil); err != nil {
			return false, err
		}

		return true, nil
	}); err != nil {
		return
	}

	var data []interface{}
	more := true
	it, err := t.SeekFirst()
	for more && err == nil {
		data, _, err = it.Next()
		if err != nil {
			break
		}

		more, err = f(nil, data)
	}
	return noEOF(err)
}

type groupByDefaultPlan struct {
	colNames []string
	src      plan
	fields   []string
}

func (r *groupByDefaultPlan) hasID() bool { return false }

func (r *groupByDefaultPlan) explain(w strutil.Formatter) {
	r.src.explain(w)
	switch {
	case len(r.colNames) == 0: //TODO this case should not exist for this plan, should become tableDefaultPlan
		w.Format("┌Group by distinct rows")
	default:
		w.Format("┌Group by")
		for _, v := range r.colNames {
			w.Format(" %s,", v)
		}
	}
	w.Format("\n└Output field names %v\n", qnames(r.fields))
}

func (r *groupByDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *groupByDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	t, err := ctx.db.store.CreateTemp(true)
	if err != nil {
		return
	}

	defer func() {
		if derr := t.Drop(); derr != nil && err == nil {
			err = derr
		}
	}()

	var gcols []*col
	var cols []*col
	m := map[string]int{}
	for i, v := range r.src.fieldNames() {
		m[v] = i
	}
	for _, c := range r.colNames {
		i, ok := m[c]
		if !ok {
			return fmt.Errorf("unknown field %s", c)
		}

		gcols = append(gcols, &col{name: c, index: i})
	}
	k := make([]interface{}, len(r.colNames)) //TODO optimize when len(r.cols) == 0, should become tableDefaultPlan
	if err = r.src.do(ctx, func(rid interface{}, in []interface{}) (more bool, err error) {
		infer(in, &cols)
		for i, c := range gcols {
			k[i] = in[c.index]
		}
		h0, err := t.Get(k)
		if err != nil {
			return false, err
		}

		var h int64
		if len(h0) != 0 {
			h, _ = h0[0].(int64)
		}
		nh, err := t.Create(append([]interface{}{h, nil}, in...)...)
		if err != nil {
			return false, err
		}

		for i, c := range gcols {
			k[i] = in[c.index]
		}
		err = t.Set(k, []interface{}{nh})
		if err != nil {
			return false, err
		}

		return true, nil
	}); err != nil {
		return
	}

	for i, v := range r.src.fieldNames()[:len(cols)] {
		cols[i].name = v
		cols[i].index = i
	}
	if more, err := f(nil, []interface{}{t, cols}); !more || err != nil {
		return err
	}

	it, err := t.SeekFirst()
	more := true
	var data []interface{}
	for more && err == nil {
		if _, data, err = it.Next(); err != nil {
			break
		}

		more, err = f(nil, data)
	}
	return noEOF(err)
}

func (r *groupByDefaultPlan) fieldNames() []string { return r.fields }

type selectIndexDefaultPlan struct {
	nm string
	x  interface{}
}

func (r *selectIndexDefaultPlan) hasID() bool { return false }

func (r *selectIndexDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all values of index %q\n└Output field names N/A\n", r.nm)
}

func (r *selectIndexDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *selectIndexDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	var x btreeIndex
	switch ix := r.x.(type) {
	case *indexedCol:
		x = ix.x
	case *index2:
		x = ix.x
	default:
		panic("internal error 007")
	}

	en, err := x.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	var id int64
	for {
		k, _, err := en.Next()
		if err != nil {
			return noEOF(err)
		}

		id++
		if more, err := f(id, k); !more || err != nil {
			return err
		}
	}
}

func (r *selectIndexDefaultPlan) fieldNames() []string {
	return []string{r.nm}
}

type limitDefaultPlan struct {
	expr   expression
	src    plan
	fields []string
}

func (r *limitDefaultPlan) hasID() bool { return r.src.hasID() }

func (r *limitDefaultPlan) explain(w strutil.Formatter) {
	r.src.explain(w)
	w.Format("┌Pass first %v records\n└Output field names %v\n", r.expr, r.fields)
}

func (r *limitDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *limitDefaultPlan) fieldNames() []string { return r.fields }

func (r *limitDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) (err error) {
	m := map[interface{}]interface{}{}
	var eval bool
	var lim uint64
	return r.src.do(ctx, func(rid interface{}, in []interface{}) (more bool, err error) {
		if !eval {
			for i, fld := range r.fields {
				if fld != "" {
					m[fld] = in[i]
				}
			}
			m["$id"] = rid
			val, err := r.expr.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val == nil {
				return true, nil
			}

			if lim, err = limOffExpr(val); err != nil {
				return false, err
			}

			eval = true
		}
		switch lim {
		case 0:
			return false, nil
		default:
			lim--
			return f(rid, in)
		}
	})
}

type offsetDefaultPlan struct {
	expr   expression
	src    plan
	fields []string
}

func (r *offsetDefaultPlan) hasID() bool { return r.src.hasID() }

func (r *offsetDefaultPlan) explain(w strutil.Formatter) {
	r.src.explain(w)
	w.Format("┌Skip first %v records\n└Output field names %v\n", r.expr, qnames(r.fields))
}

func (r *offsetDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *offsetDefaultPlan) fieldNames() []string { return r.fields }

func (r *offsetDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	m := map[interface{}]interface{}{}
	var eval bool
	var off uint64
	return r.src.do(ctx, func(rid interface{}, in []interface{}) (bool, error) {
		if !eval {
			for i, fld := range r.fields {
				if fld != "" {
					m[fld] = in[i]
				}
			}
			m["$id"] = rid
			val, err := r.expr.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val == nil {
				return true, nil
			}

			if off, err = limOffExpr(val); err != nil {
				return false, err
			}

			eval = true
		}
		if off > 0 {
			off--
			return true, nil
		}

		return f(rid, in)
	})
}

type orderByDefaultPlan struct {
	asc    bool
	by     []expression
	src    plan
	fields []string
}

func (r *orderByDefaultPlan) hasID() bool { return r.src.hasID() }

func (r *orderByDefaultPlan) explain(w strutil.Formatter) {
	r.src.explain(w)
	w.Format("┌Order%s by", map[bool]string{false: " descending"}[r.asc])
	for _, v := range r.by {
		w.Format(" %s,", v)
	}
	w.Format("\n└Output field names %v\n", qnames(r.fields))
}

func (r *orderByDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *orderByDefaultPlan) fieldNames() []string { return r.fields }

func (r *orderByDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	t, err := ctx.db.store.CreateTemp(r.asc)
	if err != nil {
		return
	}

	defer func() {
		if derr := t.Drop(); derr != nil && err == nil {
			err = derr
		}
	}()

	m := map[interface{}]interface{}{}
	flds := r.fields
	k := make([]interface{}, len(r.by)+1)
	id := int64(-1)
	if err = r.src.do(ctx, func(rid interface{}, in []interface{}) (bool, error) {
		id++
		for i, fld := range flds {
			if fld != "" {
				m[fld] = in[i]
			}
		}
		m["$id"] = rid
		for i, expr := range r.by {
			val, err := expr.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val != nil {
				val, ordered, err := isOrderedType(val)
				if err != nil {
					return false, err
				}

				if !ordered {
					return false, fmt.Errorf("cannot order by %v (type %T)", val, val)

				}
			}

			k[i] = val
		}
		k[len(r.by)] = id
		if err = t.Set(k, in); err != nil {
			return false, err
		}

		return true, nil
	}); err != nil {
		return
	}

	it, err := t.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	var data []interface{}
	more := true
	for more && err == nil {
		if _, data, err = it.Next(); err != nil {
			break
		}

		more, err = f(nil, data)
	}
	return noEOF(err)
}

type selectFieldsDefaultPlan struct {
	flds   []*fld
	src    plan
	fields []string
}

func (r *selectFieldsDefaultPlan) hasID() bool { return r.src.hasID() }

func (r *selectFieldsDefaultPlan) explain(w strutil.Formatter) {
	//TODO check for non existing fields
	r.src.explain(w)
	w.Format("┌Evaluate")
	for _, v := range r.flds {
		w.Format(" %s as %s,", v.expr, fmt.Sprintf("%q", v.name))
	}
	w.Format("\n└Output field names %v\n", qnames(r.fields))
}

func (r *selectFieldsDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *selectFieldsDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	fields := r.src.fieldNames()
	m := map[interface{}]interface{}{}
	return r.src.do(ctx, func(rid interface{}, in []interface{}) (bool, error) {
		for i, nm := range fields {
			if nm != "" {
				m[nm] = in[i]
			}
		}
		m["$id"] = rid
		out := make([]interface{}, len(r.flds))
		for i, fld := range r.flds {
			var err error
			if out[i], err = fld.expr.eval(ctx, m); err != nil {
				return false, err
			}
		}
		return f(rid, out)
	})
}

func (r *selectFieldsDefaultPlan) fieldNames() []string { return r.fields }

type selectFieldsGroupPlan struct {
	flds   []*fld
	src    *groupByDefaultPlan
	fields []string
}

func (r *selectFieldsGroupPlan) hasID() bool { return false }

func (r *selectFieldsGroupPlan) explain(w strutil.Formatter) {
	//TODO check for non existing fields
	r.src.explain(w)
	w.Format("┌Evaluate")
	for _, v := range r.flds {
		w.Format(" %s as %s,", v.expr, fmt.Sprintf("%q", v.name))
	}
	w.Format("\n└Output field names %v\n", qnames(r.fields))
}

func (r *selectFieldsGroupPlan) fieldNames() []string { return r.fields }

func (r *selectFieldsGroupPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *selectFieldsGroupPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	var t temp
	var cols []*col
	var err error
	out := make([]interface{}, len(r.flds))
	ok := false
	rows := false
	if err = r.src.do(ctx, func(rid interface{}, in []interface{}) (bool, error) {
		if ok {
			h := in[0].(int64)
			m := map[interface{}]interface{}{}
			for h != 0 {
				in, err = t.Read(nil, h, cols...)
				if err != nil {
					return false, err
				}

				rec := in[2:]
				for i, c := range cols {
					if nm := c.name; nm != "" {
						m[nm] = rec[i]
					}
				}
				m["$id"] = rid
				for _, fld := range r.flds {
					if _, err = fld.expr.eval(ctx, m); err != nil {
						return false, err
					}
				}

				h = in[0].(int64)
			}
			m["$agg"] = true
			for i, fld := range r.flds {
				if out[i], err = fld.expr.eval(ctx, m); err != nil {
					return false, err
				}
			}
			rows = true
			return f(nil, out)
		}

		ok = true
		t = in[0].(temp)
		cols = in[1].([]*col)
		if len(r.flds) == 0 { // SELECT *
			r.flds = make([]*fld, len(cols))
			for i, v := range cols {
				r.flds[i] = &fld{expr: &ident{v.name}, name: v.name}
			}
			out = make([]interface{}, len(r.flds))
		}
		return true, nil
	}); err != nil {
		return err
	}

	if rows {
		return nil
	}

	m := map[interface{}]interface{}{"$agg0": true} // aggregate empty record set
	for i, fld := range r.flds {
		if out[i], err = fld.expr.eval(ctx, m); err != nil {
			return err
		}
	}
	_, err = f(nil, out)

	return err
}

type sysColumnDefaultPlan struct{}

func (r *sysColumnDefaultPlan) hasID() bool { return false }

func (r *sysColumnDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all rows of table \"__Column\"\n└Output field names %v\n", qnames(r.fieldNames()))
}

func (r *sysColumnDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *sysColumnDefaultPlan) fieldNames() []string {
	return []string{"TableName", "Ordinal", "Name", "Type"}
}

func (r *sysColumnDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	rec := make([]interface{}, 4)
	di, err := ctx.db.info()
	if err != nil {
		return err
	}

	var id int64
	for _, ti := range di.Tables {
		rec[0] = ti.Name
		var ix int64
		for _, ci := range ti.Columns {
			ix++
			rec[1] = ix
			rec[2] = ci.Name
			rec[3] = ci.Type.String()
			id++
			if more, err := f(id, rec); !more || err != nil {
				return err
			}
		}
	}
	return nil
}

type sysIndexDefaultPlan struct{}

func (r *sysIndexDefaultPlan) hasID() bool { return false }

func (r *sysIndexDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all rows of table \"__Index\"\n└Output field names %v\n", qnames(r.fieldNames()))
}

func (r *sysIndexDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *sysIndexDefaultPlan) fieldNames() []string {
	return []string{"TableName", "ColumnName", "Name", "IsUnique"}
}

func (r *sysIndexDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	rec := make([]interface{}, 4)
	di, err := ctx.db.info()
	if err != nil {
		return err
	}

	var id int64
	for _, xi := range di.Indices {
		rec[0] = xi.Table
		rec[1] = xi.Column
		rec[2] = xi.Name
		rec[3] = xi.Unique
		id++
		if more, err := f(id, rec); !more || err != nil {
			return err
		}
	}
	return nil
}

type sysTableDefaultPlan struct{}

func (r *sysTableDefaultPlan) hasID() bool { return false }

func (r *sysTableDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all rows of table \"__Table\"\n└Output field names %v\n", qnames(r.fieldNames()))
}

func (r *sysTableDefaultPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *sysTableDefaultPlan) fieldNames() []string { return []string{"Name", "Schema"} }

func (r *sysTableDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) error {
	rec := make([]interface{}, 2)
	di, err := ctx.db.info()
	if err != nil {
		return err
	}

	var id int64
	for _, ti := range di.Tables {
		rec[0] = ti.Name
		a := []string{}
		for _, ci := range ti.Columns {
			s := ""
			if ci.NotNull {
				s += " NOT NULL"
			}
			if c := ci.Constraint; c != "" {
				s += " " + c
			}
			if d := ci.Default; d != "" {
				s += " DEFAULT " + d
			}
			a = append(a, fmt.Sprintf("%s %s%s", ci.Name, ci.Type, s))
		}
		rec[1] = fmt.Sprintf("CREATE TABLE %s (%s);", ti.Name, strings.Join(a, ", "))
		id++
		if more, err := f(id, rec); !more || err != nil {
			return err
		}
	}
	return nil
}

type tableNilPlan struct {
	t *table
}

func (r *tableNilPlan) hasID() bool { return true }

func (r *tableNilPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all rows of table %q\n└Output field names %v\n", r.t.name, qnames(r.fieldNames()))
}

func (r *tableNilPlan) fieldNames() []string { return []string{} }

func (r *tableNilPlan) filter(expr expression) (plan, []string, error) {
	return nil, nil, nil
}

func (r *tableNilPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (bool, error)) (err error) {
	t := r.t
	h := t.head
	cols := t.cols
	for h > 0 {
		rec, err := t.store.Read(nil, h, cols...) // 0:next, 1:id, 2...: data
		if err != nil {
			return err
		}

		if m, err := f(rec[1], nil); !m || err != nil {
			return err
		}

		h = rec[0].(int64) // next
	}
	return nil
}

type tableDefaultPlan struct {
	t      *table
	fields []string
}

func (r *tableDefaultPlan) hasID() bool { return true }

func (r *tableDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate all rows of table %q\n└Output field names %v\n", r.t.name, qnames(r.fields))
}

func (r *tableDefaultPlan) filterBinOp(x *binaryOperation) (plan, []string, error) {
	ok, cn, rval, err := x.isIdentRelOpVal()
	if err != nil {
		return nil, nil, err
	}

	if !ok {
		return nil, nil, nil
	}

	t := r.t
	c, ix := t.findIndexByColName(cn)
	if ix == nil { // Column cn has no index.
		return nil, []string{fmt.Sprintf("%s(%s)", t.name, cn)}, nil
	}

	if rval, err = typeCheck1(rval, c); err != nil {
		return nil, nil, err
	}

	switch x.op {
	case eq:
		return &indexPlan{t, cn, ix.name, ix.x, indexEq, rval, rval}, nil, nil
	case '<':
		return &indexPlan{t, cn, ix.name, ix.x, indexLt, nil, rval}, nil, nil
	case le:
		return &indexPlan{t, cn, ix.name, ix.x, indexLe, nil, rval}, nil, nil
	case ge:
		return &indexPlan{t, cn, ix.name, ix.x, indexGe, rval, nil}, nil, nil
	case '>':
		return &indexPlan{t, cn, ix.name, ix.x, indexGt, rval, nil}, nil, nil
	case neq:
		return &indexPlan{t, cn, ix.name, ix.x, indexNe, rval, rval}, nil, nil
	default:
		panic("internal error 069")
	}
}

func (r *tableDefaultPlan) filterIdent(x *ident, trueValue bool) (plan, []string, error) {
	cn := x.s
	t := r.t
	for _, v := range t.cols {
		if v.name != cn {
			continue
		}

		if v.typ != qBool {
			return nil, nil, nil
		}

		xi := v.index + 1 // 0: id()
		if xi >= len(t.indices) {
			return nil, nil, nil
		}

		ix := t.indices[xi]
		if ix == nil { // Column cn has no index.
			return nil, []string{fmt.Sprintf("%s(%s)", t.name, cn)}, nil
		}

		kind := indexFalse
		if trueValue {
			kind = indexTrue
		}
		return &indexPlan{t, cn, ix.name, ix.x, kind, nil, nil}, nil, nil
	}
	return nil, nil, nil
}

func (r *tableDefaultPlan) filterIsNull(x *isNull) (plan, []string, error) {
	ok, cn := isColumnExpression(x.expr)
	if !ok {
		return nil, nil, nil
	}

	t := r.t
	_, ix := t.findIndexByColName(cn)
	if ix == nil { // Column cn has no index.
		return nil, []string{fmt.Sprintf("%s(%s)", t.name, cn)}, nil
	}

	switch {
	case x.not:
		return &indexPlan{t, cn, ix.name, ix.x, indexIsNotNull, nil, nil}, nil, nil
	default:
		return &indexPlan{t, cn, ix.name, ix.x, indexIsNull, nil, nil}, nil, nil
	}
}

func (r *tableDefaultPlan) filter(expr expression) (plan, []string, error) {
	cols := mentionedColumns(expr)
	for _, v := range r.fields {
		delete(cols, v)
	}
	for k := range cols {
		return nil, nil, fmt.Errorf("unknown field %s", k)
	}

	var is []string

	//TODO var sexpr string
	//TODO for _, ix := range t.indices2 {
	//TODO 	if len(ix.exprList) != 1 {
	//TODO 		continue
	//TODO 	}

	//TODO 	if sexpr == "" {
	//TODO 		sexpr = expr.String()
	//TODO 	}
	//TODO 	if ix.sources[0] != sexpr {
	//TODO 		continue
	//TODO 	}

	//TODO }

	switch x := expr.(type) {
	case *binaryOperation:
		return r.filterBinOp(x)
	case *ident:
		return r.filterIdent(x, true)
	case *isNull:
		return r.filterIsNull(x)
	case *unaryOperation:
		if x.op != '!' {
			break
		}

		if operand, ok := x.v.(*ident); ok {
			return r.filterIdent(operand, false)
		}
	default:
		//dbg("", expr)
		return nil, is, nil //TODO
	}

	return nil, is, nil
}

func (r *tableDefaultPlan) do(ctx *execCtx, f func(interface{}, []interface{}) (bool, error)) (err error) {
	t := r.t
	cols := t.cols
	h := t.head
	for h > 0 {
		rec, err := t.row0(ctx, h)
		if err != nil {
			return err
		}

		h = rec[0].(int64)
		id := rec[1].(int64)
		for i, c := range cols {
			rec[i] = rec[c.index+2]
		}
		if m, err := f(id, rec[:len(cols)]); !m || err != nil {
			return err
		}
	}
	return nil
}

func (r *tableDefaultPlan) fieldNames() []string { return r.fields }

type nullPlan struct {
	fields []string
}

func (r *nullPlan) hasID() bool { return false }

func (r *nullPlan) fieldNames() []string { return r.fields }

func (r *nullPlan) explain(w strutil.Formatter) {
	w.Format("┌Iterate no rows\n└Output field names %v\n", qnames(r.fields))
}

func (r *nullPlan) do(*execCtx, func(interface{}, []interface{}) (bool, error)) error {
	return nil
}

func (r *nullPlan) filter(expr expression) (plan, []string, error) {
	return r, nil, nil
}

type leftJoinDefaultPlan struct {
	on     expression
	rsets  []plan
	names  []string
	right  int
	fields []string
}

func (r *leftJoinDefaultPlan) hasID() bool { return false }

func (r *leftJoinDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Compute Cartesian product of%i\n")
	for i, v := range r.rsets {
		sel := !isTableOrIndex(v)
		if sel {
			w.Format("┌Iterate all rows of virtual table %q%i\n", r.names[i])
		}
		v.explain(w)
		if sel {
			w.Format("%u└Output field names %v\n", qnames(v.fieldNames()))
		}
	}
	w.Format("Extend the product with all NULL rows of %q when no match for %v%u\n", r.names[len(r.names)-1], r.on)
	w.Format("└Output field names %v\n", qnames(r.fields))
}

func (r *leftJoinDefaultPlan) filter(expr expression) (plan, []string, error) {
	var is []string
	for i, v := range r.names {
		e2, err := expr.clone(nil, v)
		if err != nil {
			return nil, nil, err
		}

		p2, is2, err := r.rsets[i].filter(e2)
		is = append(is, is2...)
		if err != nil {
			return nil, nil, err
		}

		if p2 != nil {
			r.rsets[i] = p2
			return r, is, nil
		}
	}
	return nil, is, nil
}

type rightJoinDefaultPlan struct {
	leftJoinDefaultPlan
}

func (r *rightJoinDefaultPlan) hasID() bool { return false }

func (r *rightJoinDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Compute Cartesian product of%i\n")
	for i, v := range r.rsets {
		sel := !isTableOrIndex(v)
		if sel {
			w.Format("┌Iterate all rows of virtual table %q%i\n", r.names[i])
		}
		v.explain(w)
		if sel {
			w.Format("%u└Output field names %v\n", qnames(v.fieldNames()))
		}
	}
	w.Format("Extend the product with all NULL rows of all but %q when no match for %v%u\n", r.names[len(r.names)-1], r.on)
	w.Format("└Output field names %v\n", qnames(r.fields))
}

func (r *rightJoinDefaultPlan) filter(expr expression) (plan, []string, error) {
	var is []string
	for i, v := range r.names {
		e2, err := expr.clone(nil, v)
		if err != nil {
			return nil, nil, err
		}

		p2, is2, err := r.rsets[i].filter(e2)
		is = append(is, is2...)
		if err != nil {
			return nil, nil, err
		}

		if p2 != nil {
			r.rsets[i] = p2
			return r, is, nil
		}
	}
	return nil, is, nil
}

type fullJoinDefaultPlan struct {
	leftJoinDefaultPlan
}

func (r *fullJoinDefaultPlan) hasID() bool { return false }

func (r *fullJoinDefaultPlan) explain(w strutil.Formatter) {
	w.Format("┌Compute Cartesian product of%i\n")
	for i, v := range r.rsets {
		sel := !isTableOrIndex(v)
		if sel {
			w.Format("┌Iterate all rows of virtual table %q%i\n", r.names[i])
		}
		v.explain(w)
		if sel {
			w.Format("%u└Output field names %v\n", qnames(v.fieldNames()))
		}
	}
	w.Format("Extend the product with all NULL rows of %q when no match for %v\n", r.names[len(r.names)-1], r.on)
	w.Format("Extend the product with all NULL rows of all but %q when no match for %v%u\n", r.names[len(r.names)-1], r.on)
	w.Format("└Output field names %v\n", qnames(r.fields))
}

func (r *fullJoinDefaultPlan) filter(expr expression) (plan, []string, error) {
	var is []string
	for i, v := range r.names {
		e2, err := expr.clone(nil, v)
		if err != nil {
			return nil, nil, err
		}

		p2, is2, err := r.rsets[i].filter(e2)
		is = append(is, is2...)
		if err != nil {
			return nil, nil, err
		}

		if p2 != nil {
			r.rsets[i] = p2
			return r, is, nil
		}
	}
	return nil, is, nil
}

func (r *leftJoinDefaultPlan) fieldNames() []string { return r.fields }

func (r *leftJoinDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) error {
	m := map[interface{}]interface{}{}
	ids := map[string]interface{}{}
	var g func([]interface{}, []plan, int) error
	var match bool
	g = func(prefix []interface{}, rsets []plan, x int) (err error) {
		return rsets[0].do(ctx, func(id interface{}, in []interface{}) (bool, error) {
			ids[r.names[x]] = id
			row := append(prefix, in...)
			if len(rsets) > 1 {
				if len(rsets) == 2 {
					match = false
				}
				if err = g(row, rsets[1:], x+1); err != nil {
					return false, err
				}

				if len(rsets) != 2 || match {
					return true, nil
				}

				ids[r.names[x+1]] = nil
				return f(ids, append(row, make([]interface{}, r.right)...))
			}

			for i, fld := range r.fields {
				if fld != "" {
					m[fld] = row[i]
				}
			}

			val, err := r.on.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val == nil {
				return true, nil
			}

			x, ok := val.(bool)
			if !ok {
				return false, fmt.Errorf("invalid ON expression %s (value of type %T)", val, val)
			}

			if !x {
				return true, nil
			}

			match = true
			return f(ids, row)
		})
	}
	return g(nil, r.rsets, 0)
}

func (r *rightJoinDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) error {
	right := r.right
	left := len(r.fields) - right
	n := len(r.rsets)
	m := map[interface{}]interface{}{}
	ids := map[string]interface{}{}
	var g func([]interface{}, []plan, int) error
	var match bool
	nf := len(r.fields)
	fields := append(append([]string(nil), r.fields[nf-right:]...), r.fields[:nf-right]...)
	g = func(prefix []interface{}, rsets []plan, x int) (err error) {
		return rsets[0].do(ctx, func(id interface{}, in []interface{}) (bool, error) {
			ids[r.names[x]] = id
			row := append(prefix, in...)
			if len(rsets) > 1 {
				if len(rsets) == n {
					match = false
				}
				if err = g(row, rsets[1:], x+1); err != nil {
					return false, err
				}

				if len(rsets) != n || match {
					return true, nil
				}

				for i := 0; i < n-1; i++ {
					ids[r.names[i]] = nil
				}

				// rigth, left -> left, right
				return f(ids, append(make([]interface{}, left), row[:right]...))
			}

			for i, fld := range fields {
				if fld != "" {
					m[fld] = row[i]
				}
			}

			val, err := r.on.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val == nil {
				return true, nil
			}

			x, ok := val.(bool)
			if !ok {
				return false, fmt.Errorf("invalid ON expression %s (value of type %T)", val, val)
			}

			if !x {
				return true, nil
			}

			match = true
			// rigth, left -> left, right
			return f(ids, append(append([]interface{}(nil), row[right:]...), row[:right]...))
		})
	}
	return g(nil, append([]plan{r.rsets[n-1]}, r.rsets[:n-1]...), 0)
}

func (r *fullJoinDefaultPlan) do(ctx *execCtx, f func(id interface{}, data []interface{}) (more bool, err error)) error {
	b3 := b.TreeNew(func(a, b interface{}) int {
		x := a.(int64)
		y := b.(int64)
		if x < y {
			return -1
		}

		if x == y {
			return 0
		}

		return 1
	})
	m := map[interface{}]interface{}{}
	ids := map[string]interface{}{}
	var g func([]interface{}, []plan, int) error
	var match bool
	var rid int64
	firstR := true
	g = func(prefix []interface{}, rsets []plan, x int) (err error) {
		return rsets[0].do(ctx, func(id interface{}, in []interface{}) (bool, error) {
			ids[r.names[x]] = id
			row := append(prefix, in...)
			if len(rsets) > 1 {
				if len(rsets) == 2 {
					match = false
					rid = 0
				}
				if err = g(row, rsets[1:], x+1); err != nil {
					return false, err
				}

				if len(rsets) == 2 {
					firstR = false
				}
				if len(rsets) != 2 || match {
					return true, nil
				}

				ids[r.names[x+1]] = nil
				return f(ids, append(row, make([]interface{}, r.right)...))
			}

			rid++
			if firstR {
				b3.Set(rid, in)
			}
			for i, fld := range r.fields {
				if fld != "" {
					m[fld] = row[i]
				}
			}

			val, err := r.on.eval(ctx, m)
			if err != nil {
				return false, err
			}

			if val == nil {
				return true, nil
			}

			x, ok := val.(bool)
			if !ok {
				return false, fmt.Errorf("invalid ON expression %s (value of type %T)", val, val)
			}

			if !x {
				return true, nil
			}

			match = true
			b3.Delete(rid)
			return f(ids, row)
		})
	}
	if err := g(nil, r.rsets, 0); err != nil {
		return err
	}

	it, err := b3.SeekFirst()
	if err != nil {
		return noEOF(err)
	}

	pref := make([]interface{}, len(r.fields)-r.right)
	for {
		_, v, err := it.Next()
		if err != nil {
			return noEOF(err)
		}

		more, err := f(nil, append(pref, v.([]interface{})...))
		if err != nil || !more {
			return err
		}
	}
}
