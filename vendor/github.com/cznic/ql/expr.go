// Copyright 2014 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found pIn the LICENSE file.

package ql

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var (
	_ expression = (*binaryOperation)(nil)
	_ expression = (*call)(nil)
	_ expression = (*conversion)(nil)
	_ expression = (*ident)(nil)
	_ expression = (*indexOp)(nil)
	_ expression = (*isNull)(nil)
	_ expression = (*pIn)(nil)
	_ expression = (*pLike)(nil)
	_ expression = (*parameter)(nil)
	_ expression = (*pexpr)(nil)
	_ expression = (*slice)(nil)
	_ expression = (*unaryOperation)(nil)
	_ expression = value{}
)

type expression interface {
	clone(arg []interface{}, unqualify ...string) (expression, error)
	eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error)
	isStatic() bool
	String() string
}

func cloneExpressionList(arg []interface{}, list []expression, unqualify ...string) ([]expression, error) {
	r := make([]expression, len(list))
	var err error
	for i, v := range list {
		if r[i], err = v.clone(arg, unqualify...); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func isConstValue(v interface{}) interface{} {
	switch x := v.(type) {
	case value:
		return x.val
	case
		idealComplex,
		idealFloat,
		idealInt,
		idealRune,
		idealUint:
		return v
	default:
		return nil
	}
}

func isColumnExpression(v expression) (bool, string) {
	x, ok := v.(*ident)
	if ok {
		return true, x.s
	}

	c, ok := v.(*call)
	if !ok || c.f != "id" || len(c.arg) != 0 {
		return false, ""
	}

	return true, "id()"
}

func mentionedColumns0(e expression, q, nq bool, m map[string]struct{}) {
	switch x := e.(type) {
	case parameter,
		value:
		// nop
	case *binaryOperation:
		mentionedColumns0(x.l, q, nq, m)
		mentionedColumns0(x.r, q, nq, m)
	case *call:
		if x.f != "id" {
			for _, e := range x.arg {
				mentionedColumns0(e, q, nq, m)
			}
		}
	case *conversion:
		mentionedColumns0(x.val, q, nq, m)
	case *ident:
		if q && x.isQualified() {
			m[x.s] = struct{}{}
		}
		if nq && !x.isQualified() {
			m[x.s] = struct{}{}
		}
	case *indexOp:
		mentionedColumns0(x.expr, q, nq, m)
		mentionedColumns0(x.x, q, nq, m)
	case *isNull:
		mentionedColumns0(x.expr, q, nq, m)
	case *pexpr:
		mentionedColumns0(x.expr, q, nq, m)
	case *pIn:
		mentionedColumns0(x.expr, q, nq, m)
		for _, e := range x.list {
			mentionedColumns0(e, q, nq, m)
		}
	case *pLike:
		mentionedColumns0(x.expr, q, nq, m)
		mentionedColumns0(x.pattern, q, nq, m)
	case *slice:
		mentionedColumns0(x.expr, q, nq, m)
		if y := x.lo; y != nil {
			mentionedColumns0(*y, q, nq, m)
		}
		if y := x.hi; y != nil {
			mentionedColumns0(*y, q, nq, m)
		}
	case *unaryOperation:
		mentionedColumns0(x.v, q, nq, m)
	default:
		panic("internal error 052")
	}
}

func mentionedColumns(e expression) map[string]struct{} {
	m := map[string]struct{}{}
	mentionedColumns0(e, false, true, m)
	return m
}

func staticExpr(e expression) (expression, error) {
	if e.isStatic() {
		v, err := e.eval(nil, nil)
		if err != nil {
			return nil, err
		}

		if v == nil {
			return value{nil}, nil
		}

		return value{v}, nil
	}

	return e, nil
}

type (
	idealComplex complex128
	idealFloat   float64
	idealInt     int64
	idealRune    int32
	idealUint    uint64
)

type pexpr struct {
	expr expression
}

func (p *pexpr) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := p.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &pexpr{expr: expr}, nil
}

func (p *pexpr) isStatic() bool { return p.expr.isStatic() }

func (p *pexpr) String() string {
	return fmt.Sprintf("(%s)", p.expr)
}

func (p *pexpr) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	return p.expr.eval(execCtx, ctx)
}

//DONE newBetween
//LATER like newBetween, check all others have and use new*

func newBetween(expr, lo, hi interface{}, not bool) (expression, error) {
	e, err := staticExpr(expr.(expression))
	if err != nil {
		return nil, err
	}

	l, err := staticExpr(lo.(expression))
	if err != nil {
		return nil, err
	}

	h, err := staticExpr(hi.(expression))
	if err != nil {
		return nil, err
	}

	var a, b expression
	op := andand
	switch {
	case not: // e < l || e > h
		op = oror
		if a, err = newBinaryOperation('<', e, l); err != nil {
			return nil, err
		}

		if b, err = newBinaryOperation('>', e, h); err != nil {
			return nil, err
		}
	default: // e >= l && e <= h
		if a, err = newBinaryOperation(ge, e, l); err != nil {
			return nil, err
		}

		if b, err = newBinaryOperation(le, e, h); err != nil {
			return nil, err
		}
	}

	if a, err = staticExpr(a); err != nil {
		return nil, err
	}

	if b, err = staticExpr(b); err != nil {
		return nil, err
	}

	ret, err := newBinaryOperation(op, a, b)
	if err != nil {
		return nil, err
	}

	return staticExpr(ret)
}

type pLike struct {
	expr    expression
	pattern expression
	re      *regexp.Regexp
	sexpr   *string
}

func (p *pLike) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := p.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	pattern, err := p.pattern.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &pLike{
		expr:    expr,
		pattern: pattern,
		re:      p.re,
		sexpr:   p.sexpr,
	}, nil
}

func (p *pLike) isStatic() bool { return p.expr.isStatic() && p.pattern.isStatic() }
func (p *pLike) String() string { return fmt.Sprintf("%s LIKE %s", p.expr, p.pattern) }

func (p *pLike) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	var sexpr string
	var ok bool
	switch {
	case p.sexpr != nil:
		sexpr = *p.sexpr
	default:
		expr, err := expand1(p.expr.eval(execCtx, ctx))
		if err != nil {
			return nil, err
		}

		if expr == nil {
			return nil, nil
		}

		sexpr, ok = expr.(string)
		if !ok {
			return nil, fmt.Errorf("non-string expression in LIKE: %v (value of type %T)", expr, expr)
		}

		if p.expr.isStatic() {
			p.sexpr = new(string)
			*p.sexpr = sexpr
		}
	}

	re := p.re
	if re == nil {
		pattern, err := expand1(p.pattern.eval(execCtx, ctx))
		if err != nil {
			return nil, err
		}

		if pattern == nil {
			return nil, nil
		}

		spattern, ok := pattern.(string)
		if !ok {
			return nil, fmt.Errorf("non-string pattern in LIKE: %v (value of type %T)", pattern, pattern)
		}

		if re, err = regexp.Compile(spattern); err != nil {
			return nil, err
		}

		if p.pattern.isStatic() {
			p.re = re
		}
	}

	return re.MatchString(sexpr), nil
}

type binaryOperation struct {
	op   int
	l, r expression
}

func newBinaryOperation0(op int, x, y interface{}) (v expression, err error) {
	if op == eq {
		if l, ok := x.(value); ok {
			if b, ok := l.val.(bool); ok {
				if b { // true == y: y
					return y.(expression), nil
				}

				// false == y: !y
				return newUnaryOperation('!', y)
			}
		}

		if r, ok := y.(value); ok {
			if b, ok := r.val.(bool); ok {
				if b { // x == true: x
					return x.(expression), nil
				}

				// x == false: !x
				return newUnaryOperation('!', x)
			}
		}
	}

	if op == neq {
		if l, ok := x.(value); ok {
			if b, ok := l.val.(bool); ok {
				if b { // true != y: !y
					return newUnaryOperation('!', y)
				}

				// false != y: y
				return y.(expression), nil
			}
		}

		if r, ok := y.(value); ok {
			if b, ok := r.val.(bool); ok {
				if b { // x != true: !x
					return newUnaryOperation('!', x)
				}

				// x != false: x
				return x.(expression), nil
			}
		}
	}

	b := binaryOperation{op, x.(expression), y.(expression)}
	var lv interface{}
	if e := b.l; e.isStatic() {
		if lv, err = e.eval(nil, nil); err != nil {
			return nil, err
		}

		b.l = value{lv}
	}

	if e := b.r; e.isStatic() {
		v, err := e.eval(nil, nil)
		if err != nil {
			return nil, err
		}

		if v == nil {
			return value{nil}, nil
		}

		if op == '/' || op == '%' {
			rb := binaryOperation{eq, e, value{idealInt(0)}}
			val, err := rb.eval(nil, nil)
			if err != nil {
				return nil, err
			}

			if val.(bool) {
				return nil, errDivByZero
			}
		}

		if b.l.isStatic() && lv == nil {
			return value{nil}, nil
		}

		b.r = value{v}
	}

	if !b.isStatic() {
		return &b, nil
	}

	val, err := b.eval(nil, nil)
	return value{val}, err
}

func newBinaryOperation(op int, x, y interface{}) (v expression, err error) {
	expr, err := newBinaryOperation0(op, x, y)
	if err != nil {
		return nil, err
	}

	b, ok := expr.(*binaryOperation)
	if !ok {
		return expr, nil
	}

	if _, ok := b.l.(*ident); ok {
		return expr, nil
	}

	if c, ok := b.l.(*call); ok && c.f == "id" {
		return expr, nil
	}

	var r expression
	if r, ok = b.r.(*ident); !ok {
		r1, ok := b.r.(*call)
		if !ok || r1.f != "id" || len(r1.arg) != 0 {
			return expr, nil
		}

		r = r1
	}

	// Normalize expr relOp indent: ident invRelOp expr
	switch b.op {
	case '<':
		return &binaryOperation{'>', r, b.l}, nil
	case le:
		return &binaryOperation{ge, r, b.l}, nil
	case '>':
		return &binaryOperation{'<', r, b.l}, nil
	case ge:
		return &binaryOperation{le, r, b.l}, nil
	case eq, neq:
		return &binaryOperation{b.op, r, b.l}, nil
	default:
		return expr, nil
	}
}

func (o *binaryOperation) isIdentRelOpVal() (bool, string, interface{}, error) {
	sid := ""
	id, ok := o.l.(*ident)
	if !ok {
		f, ok := o.l.(*call)
		if !ok || f.f != "id" || len(f.arg) != 0 {
			return false, "", nil, nil
		}

		sid = "id()"
	} else {
		if id.isQualified() {
			return false, "", nil, nil
		}

		sid = id.s
	}

	if v, ok := o.r.(value); ok {
		switch o.op {
		case '<',
			le,
			'>',
			ge,
			eq,
			neq:
			return true, sid, v.val, nil
		default:
			return false, "", nil, nil
		}
	}

	return false, "", nil, nil
}

func (o *binaryOperation) clone(arg []interface{}, unqualify ...string) (expression, error) {
	l, err := o.l.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	r, err := o.r.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return newBinaryOperation(o.op, l, r)
}

func (o *binaryOperation) isStatic() bool { return o.l.isStatic() && o.r.isStatic() }

func (o *binaryOperation) String() string {
	return fmt.Sprintf("%s %s %s", o.l, iop(o.op), o.r)
}

func (o *binaryOperation) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (r interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case error:
				r, err = nil, x
			default:
				r, err = nil, fmt.Errorf("%v", x)
			}
		}
	}()

	switch op := o.op; op {
	case andand:
		a, err := expand1(o.l.eval(execCtx, ctx))
		if err != nil {
			return nil, err
		}

		switch x := a.(type) {
		case nil:
			b, err := expand1(o.r.eval(execCtx, ctx))
			if err != nil {
				return nil, err
			}

			switch y := b.(type) {
			case nil:
				return nil, nil
			case bool:
				if !y {
					return false, nil
				}

				return nil, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			if !x {
				return false, nil
			}

			b, err := expand1(o.r.eval(execCtx, ctx))
			if err != nil {
				return nil, err
			}

			switch y := b.(type) {
			case nil:
				return nil, nil
			case bool:
				return y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return undOp(x, op)
		}
	case oror:
		a, err := expand1(o.l.eval(execCtx, ctx))
		if err != nil {
			return nil, err
		}

		switch x := a.(type) {
		case nil:
			b, err := expand1(o.r.eval(execCtx, ctx))
			if err != nil {
				return nil, err
			}

			switch y := b.(type) {
			case nil:
				return nil, nil
			case bool:
				if y {
					return y, nil
				}

				return nil, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			if x {
				return x, nil
			}

			b, err := expand1(o.r.eval(execCtx, ctx))
			if err != nil {
				return nil, err
			}

			switch y := b.(type) {
			case nil:
				return nil, nil
			case bool:
				return y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return undOp(x, op)
		}
	case '>':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			switch y := b.(type) {
			case float32:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) > 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) > 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x > y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return x.After(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '<':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			switch y := b.(type) {
			case float32:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) < 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) < 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x < y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return x.Before(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case le:
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			switch y := b.(type) {
			case float32:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) <= 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) <= 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x <= y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return x.Before(y) || x.Equal(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case ge:
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			switch y := b.(type) {
			case float32:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) >= 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) >= 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x >= y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return x.After(y) || x.Equal(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case neq:
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			switch y := b.(type) {
			case bool:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) != 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) != 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x != y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return !x.Equal(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case eq:
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			switch y := b.(type) {
			case bool:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				return x.Cmp(y) == 0, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				return x.Cmp(y) == 0, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x == y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Time:
				return x.Equal(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '+':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return idealComplex(complex64(x) + complex64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return idealFloat(float64(x) + float64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) + int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) + int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) + uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			switch y := b.(type) {
			case string:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x + y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.Add(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				var z big.Rat
				return z.Add(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x + y, nil
			case time.Time:
				return y.Add(x), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Duration:
				return x.Add(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '-':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return idealComplex(complex64(x) - complex64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return idealFloat(float64(x) - float64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) - int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) - int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) - uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.Sub(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				var z big.Rat
				return z.Sub(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x - y, nil
			default:
				return invOp2(x, y, op)
			}
		case time.Time:
			switch y := b.(type) {
			case time.Duration:
				return x.Add(-y), nil
			case time.Time:
				return x.Sub(y), nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case rsh:
		a, b := eval2(o.l, o.r, execCtx, ctx)
		if a == nil || b == nil {
			return
		}

		var cnt uint64
		switch y := b.(type) {
		//case nil:
		case idealComplex:
			return invShiftRHS(a, b)
		case idealFloat:
			return invShiftRHS(a, b)
		case idealInt:
			cnt = uint64(y)
		case idealRune:
			cnt = uint64(y)
		case idealUint:
			cnt = uint64(y)
		case bool:
			return invShiftRHS(a, b)
		case complex64:
			return invShiftRHS(a, b)
		case complex128:
			return invShiftRHS(a, b)
		case float32:
			return invShiftRHS(a, b)
		case float64:
			return invShiftRHS(a, b)
		case int8:
			return invShiftRHS(a, b)
		case int16:
			return invShiftRHS(a, b)
		case int32:
			return invShiftRHS(a, b)
		case int64:
			return invShiftRHS(a, b)
		case string:
			return invShiftRHS(a, b)
		case uint8:
			cnt = uint64(y)
		case uint16:
			cnt = uint64(y)
		case uint32:
			cnt = uint64(y)
		case uint64:
			cnt = y
		default:
			return invOp2(a, b, op)
		}

		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			return idealInt(int64(x) >> cnt), nil
		case idealRune:
			return idealRune(int64(x) >> cnt), nil
		case idealUint:
			return idealUint(uint64(x) >> cnt), nil
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			return x >> cnt, nil
		case int16:
			return x >> cnt, nil
		case int32:
			return x >> cnt, nil
		case int64:
			return x >> cnt, nil
		case string:
			return undOp2(a, b, op)
		case uint8:
			return x >> cnt, nil
		case uint16:
			return x >> cnt, nil
		case uint32:
			return x >> cnt, nil
		case uint64:
			return x >> cnt, nil
		case *big.Int:
			var z big.Int
			return z.Rsh(x, uint(cnt)), nil
		case time.Duration:
			return x >> cnt, nil
		default:
			return invOp2(a, b, op)
		}
	case lsh:
		a, b := eval2(o.l, o.r, execCtx, ctx)
		if a == nil || b == nil {
			return
		}

		var cnt uint64
		switch y := b.(type) {
		//case nil:
		case idealComplex:
			return invShiftRHS(a, b)
		case idealFloat:
			return invShiftRHS(a, b)
		case idealInt:
			cnt = uint64(y)
		case idealRune:
			cnt = uint64(y)
		case idealUint:
			cnt = uint64(y)
		case bool:
			return invShiftRHS(a, b)
		case complex64:
			return invShiftRHS(a, b)
		case complex128:
			return invShiftRHS(a, b)
		case float32:
			return invShiftRHS(a, b)
		case float64:
			return invShiftRHS(a, b)
		case int8:
			return invShiftRHS(a, b)
		case int16:
			return invShiftRHS(a, b)
		case int32:
			return invShiftRHS(a, b)
		case int64:
			return invShiftRHS(a, b)
		case string:
			return invShiftRHS(a, b)
		case uint8:
			cnt = uint64(y)
		case uint16:
			cnt = uint64(y)
		case uint32:
			cnt = uint64(y)
		case uint64:
			cnt = y
		default:
			return invOp2(a, b, op)
		}

		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			return idealInt(int64(x) << cnt), nil
		case idealRune:
			return idealRune(int64(x) << cnt), nil
		case idealUint:
			return idealUint(uint64(x) << cnt), nil
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			return x << cnt, nil
		case int16:
			return x << cnt, nil
		case int32:
			return x << cnt, nil
		case int64:
			return x << cnt, nil
		case string:
			return undOp2(a, b, op)
		case uint8:
			return x << cnt, nil
		case uint16:
			return x << cnt, nil
		case uint32:
			return x << cnt, nil
		case uint64:
			return x << cnt, nil
		case *big.Int:
			var z big.Int
			return z.Lsh(x, uint(cnt)), nil
		case time.Duration:
			return x << cnt, nil
		default:
			return invOp2(a, b, op)
		}
	case '&':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) & int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) & int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) & uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			switch y := b.(type) {
			case int8:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.And(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x & y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '|':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) | int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) | int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) | uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			switch y := b.(type) {
			case int8:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.Or(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x | y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case andnot:
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) &^ int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) &^ int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) &^ uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			switch y := b.(type) {
			case int8:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.AndNot(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x &^ y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '^':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) ^ int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) ^ int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) ^ uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			switch y := b.(type) {
			case int8:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.Xor(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x ^ y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '%':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp2(a, b, op)
		case idealFloat:
			return undOp2(a, b, op)
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) % int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) % int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) % uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			return undOp2(a, b, op)
		case complex128:
			return undOp2(a, b, op)
		case float32:
			return undOp2(a, b, op)
		case float64:
			return undOp2(a, b, op)
		case int8:
			switch y := b.(type) {
			case int8:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				if y.Sign() == 0 {
					return nil, errDivByZero
				}

				var z big.Int
				return z.Mod(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x % y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '/':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return idealComplex(complex64(x) / complex64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return idealFloat(float64(x) / float64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) / int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) / int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) / uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				if y.Sign() == 0 {
					return nil, errDivByZero
				}

				var z big.Int
				return z.Quo(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				if y.Sign() == 0 {
					return nil, errDivByZero
				}

				var z big.Rat
				return z.Quo(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x / y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	case '*':
		a, b := o.get2(execCtx, ctx)
		if a == nil || b == nil {
			return
		}
		switch x := a.(type) {
		//case nil:
		case idealComplex:
			switch y := b.(type) {
			case idealComplex:
				return idealComplex(complex64(x) * complex64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealFloat:
			switch y := b.(type) {
			case idealFloat:
				return idealFloat(float64(x) * float64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealInt:
			switch y := b.(type) {
			case idealInt:
				return idealInt(int64(x) * int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealRune:
			switch y := b.(type) {
			case idealRune:
				return idealRune(int64(x) * int64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case idealUint:
			switch y := b.(type) {
			case idealUint:
				return idealUint(uint64(x) * uint64(y)), nil
			default:
				return invOp2(x, y, op)
			}
		case bool:
			return undOp2(a, b, op)
		case complex64:
			switch y := b.(type) {
			case complex64:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case complex128:
			switch y := b.(type) {
			case complex128:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case float32:
			switch y := b.(type) {
			case float32:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case float64:
			switch y := b.(type) {
			case float64:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case int8:
			switch y := b.(type) {
			case int8:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case int16:
			switch y := b.(type) {
			case int16:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case int32:
			switch y := b.(type) {
			case int32:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case int64:
			switch y := b.(type) {
			case int64:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case string:
			return undOp2(a, b, op)
		case uint8:
			switch y := b.(type) {
			case uint8:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint16:
			switch y := b.(type) {
			case uint16:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint32:
			switch y := b.(type) {
			case uint32:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case uint64:
			switch y := b.(type) {
			case uint64:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Int:
			switch y := b.(type) {
			case *big.Int:
				var z big.Int
				return z.Mul(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case *big.Rat:
			switch y := b.(type) {
			case *big.Rat:
				var z big.Rat
				return z.Mul(x, y), nil
			default:
				return invOp2(x, y, op)
			}
		case time.Duration:
			switch y := b.(type) {
			case time.Duration:
				return x * y, nil
			default:
				return invOp2(x, y, op)
			}
		default:
			return invOp2(a, b, op)
		}
	default:
		panic("internal error 037")
	}
}

func (o *binaryOperation) get2(execCtx *execCtx, ctx map[interface{}]interface{}) (x, y interface{}) {
	x, y = eval2(o.l, o.r, execCtx, ctx)
	//dbg("get2 pIn     - ", x, y)
	//defer func() {dbg("get2 coerced ", x, y)}()
	return coerce(x, y)
}

type ident struct {
	s string
}

func (i *ident) clone(arg []interface{}, unqualify ...string) (expression, error) {
	x := strings.IndexByte(i.s, '.')
	if x < 0 {
		return &ident{s: i.s}, nil
	}

	q := i.s[:x]
	for _, v := range unqualify {
		if q == v {
			return &ident{i.s[x+1:]}, nil
		}
	}
	return &ident{s: i.s}, nil
}

func (i *ident) isQualified() bool { return strings.Contains(i.s, ".") }

func (i *ident) isStatic() bool { return false }

func (i *ident) String() string { return i.s }

func (i *ident) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	if _, ok := ctx["$agg0"]; ok {
		return int64(0), nil
	}

	//defer func() { dbg("ident %q -> %v %v", i.s, v, err) }()
	v, ok := ctx[i.s]
	if !ok {
		err = fmt.Errorf("unknown field %s", i.s)
	}
	return
}

type pInEval struct {
	m      map[interface{}]struct{} // IN (SELECT...) results
	sample interface{}
}

type pIn struct {
	expr expression
	list []expression
	not  bool
	sel  *selectStmt
}

func (n *pIn) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := n.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	list, err := cloneExpressionList(arg, n.list)
	if err != nil {
		return nil, err
	}

	return &pIn{
		expr: expr,
		list: list,
		not:  n.not,
		sel:  n.sel,
	}, nil
}

func (n *pIn) isStatic() bool {
	if !n.expr.isStatic() || n.sel != nil {
		return false
	}

	for _, v := range n.list {
		if !v.isStatic() {
			return false
		}
	}
	return true
}

//LATER newIn

func (n *pIn) String() string {
	if n.sel == nil {
		a := []string{}
		for _, v := range n.list {
			a = append(a, v.String())
		}
		if n.not {
			return fmt.Sprintf("%s NOT IN (%s)", n.expr, strings.Join(a, ","))
		}

		return fmt.Sprintf("%s IN (%s)", n.expr, strings.Join(a, ","))
	}

	if n.not {
		return fmt.Sprintf("%s NOT IN (%s)", n.expr, n.sel)
	}

	return fmt.Sprintf("%s IN (%s)", n.expr, n.sel)
}

func (n *pIn) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	lhs, err := expand1(n.expr.eval(execCtx, ctx))
	if err != nil {
		return nil, err
	}

	if lhs == nil {
		return nil, nil //TODO Add test for NULL LHS.
	}

	if n.sel == nil {
		for _, v := range n.list {
			b, err := newBinaryOperation(eq, value{lhs}, v)
			if err != nil {
				return nil, err
			}

			eval, err := b.eval(execCtx, ctx)
			if err != nil {
				return nil, err
			}

			if x, ok := eval.(bool); ok && x {
				return !n.not, nil
			}
		}
		return n.not, nil
	}

	var ev *pInEval
	ev0 := ctx[n]
	if ev0 == nil { // SELECT not yet evaluated.
		r, err := n.sel.plan(execCtx)
		if err != nil {
			return nil, err
		}

		if g, e := len(r.fieldNames()), 1; g != e {
			return false, fmt.Errorf("IN (%s): mismatched field count, have %d, need %d", n.sel, g, e)
		}

		ev = &pInEval{m: map[interface{}]struct{}{}}
		ctx[n] = ev
		m := ev.m
		typechecked := false
		if err := r.do(execCtx, func(id interface{}, data []interface{}) (more bool, err error) {
			if typechecked {
				if data[0] == nil {
					return true, nil
				}

				m[data[0]] = struct{}{}
			}

			if data[0] == nil {
				return true, nil
			}

			ev.sample = data[0]
			switch ev.sample.(type) {
			case bool, byte, complex128, complex64, float32,
				float64, int16, int32, int64, int8,
				string, uint16, uint32, uint64:
				typechecked = true
				m[ev.sample] = struct{}{}
				return true, nil
			default:
				return false, fmt.Errorf("IN (%s): invalid field type: %T", n.sel, data[0])
			}

		}); err != nil {
			return nil, err
		}
	} else {
		ev = ev0.(*pInEval)
	}

	if ev.sample == nil {
		return nil, nil
	}

	_, ok := ev.m[coerce1(lhs, ev.sample)]
	return ok != n.not, nil
}

type value struct {
	val interface{}
}

func (l value) clone(arg []interface{}, unqualify ...string) (expression, error) {
	return value{val: l.val}, nil
}

func (l value) isStatic() bool { return true }

func (l value) String() string {
	switch x := l.val.(type) {
	case nil:
		return "NULL"
	case idealComplex:
		s := fmt.Sprint(x)
		return s[1 : len(s)-1]
	case complex64:
		s := fmt.Sprint(x)
		return s[1 : len(s)-1]
	case complex128:
		s := fmt.Sprint(x)
		return s[1 : len(s)-1]
	case string:
		return fmt.Sprintf("%q", x)
	case time.Duration:
		return fmt.Sprintf("duration(%q)", l.val)
	case time.Time:
		y, m, d := x.Date()
		zone, _ := x.Zone()
		return fmt.Sprintf("date(%v, %v, %v, %v, %v, %v, %v, %v)", y, m, d, x.Hour(), x.Minute(), x.Second(), x.Nanosecond(), zone)
	case *big.Rat:
		return fmt.Sprintf("bigrat(%q)", l.val)
	case *big.Int:
		return fmt.Sprintf(`bigint("%v")`, l.val)
	default:
		return fmt.Sprintf("%v", l.val)
	}
}

func (l value) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (interface{}, error) {
	return l.val, nil
}

type conversion struct {
	typ int
	val expression
}

func (c *conversion) clone(arg []interface{}, unqualify ...string) (expression, error) {
	val, err := c.val.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &conversion{typ: c.typ, val: val}, nil
}

func (c *conversion) isStatic() bool {
	return c.val.isStatic()
}

//LATER newConversion or fake unary op

func (c *conversion) String() string {
	return fmt.Sprintf("%s(%s)", typeStr(c.typ), c.val)
}

func (c *conversion) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	val, err := expand1(c.val.eval(execCtx, ctx))
	if err != nil {
		return
	}

	return convert(val, c.typ)
}

type unaryOperation struct {
	op int
	v  expression
}

func newUnaryOperation(op int, x interface{}) (v expression, err error) {
	l, ok := x.(expression)
	if !ok {
		panic("internal error 038")
	}

	for {
		pe, ok := l.(*pexpr)
		if ok {
			l = pe.expr
			continue
		}

		break
	}

	if l.isStatic() {
		val, err := l.eval(nil, nil)
		if err != nil {
			return nil, err
		}

		l = value{val}
	}

	if op == '!' {
		b, ok := l.(*binaryOperation)
		if ok {
			switch b.op {
			case eq:
				b.op = neq
				return b, nil
			case neq:
				b.op = eq
				return b, nil
			case '>':
				b.op = le
				return b, nil
			case ge:
				b.op = '<'
				return b, nil
			case '<':
				b.op = ge
				return b, nil
			case le:
				b.op = '>'
				return b, nil
			}
		}

		u, ok := l.(*unaryOperation)
		if ok && u.op == '!' { // !!x: x
			return u.v, nil
		}
	}

	return &unaryOperation{op, l}, nil
}

func (u *unaryOperation) clone(arg []interface{}, unqualify ...string) (expression, error) {
	v, err := u.v.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &unaryOperation{op: u.op, v: v}, nil
}

func (u *unaryOperation) isStatic() bool { return u.v.isStatic() }

func (u *unaryOperation) String() string {
	switch u.v.(type) {
	case *binaryOperation:
		return fmt.Sprintf("%s(%s)", iop(u.op), u.v)
	default:
		return fmt.Sprintf("%s%s", iop(u.op), u.v)
	}
}

func (u *unaryOperation) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (r interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case error:
				r, err = nil, x
			default:
				r, err = nil, fmt.Errorf("%v", x)
			}
		}
	}()

	switch op := u.op; op {
	case '!':
		a := eval(u.v, execCtx, ctx)
		if a == nil {
			return
		}

		switch x := a.(type) {
		case bool:
			return !x, nil
		default:
			return undOp(a, op)
		}
	case '^':
		a := eval(u.v, execCtx, ctx)
		if a == nil {
			return
		}

		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return undOp(a, op)
		case idealFloat:
			return undOp(a, op)
		case idealInt:
			return ^x, nil
		case idealRune:
			return ^x, nil
		case idealUint:
			return ^x, nil
		case bool:
			return undOp(a, op)
		case complex64:
			return undOp(a, op)
		case complex128:
			return undOp(a, op)
		case float32:
			return undOp(a, op)
		case float64:
			return undOp(a, op)
		case int8:
			return ^x, nil
		case int16:
			return ^x, nil
		case int32:
			return ^x, nil
		case int64:
			return ^x, nil
		case string:
			return undOp(a, op)
		case uint8:
			return ^x, nil
		case uint16:
			return ^x, nil
		case uint32:
			return ^x, nil
		case uint64:
			return ^x, nil
		case *big.Int:
			var z big.Int
			return z.Not(x), nil
		case time.Duration:
			return ^x, nil
		default:
			return undOp(a, op)
		}
	case '+':
		a := eval(u.v, execCtx, ctx)
		if a == nil {
			return
		}

		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return +x, nil
		case idealFloat:
			return +x, nil
		case idealInt:
			return +x, nil
		case idealRune:
			return +x, nil
		case idealUint:
			return +x, nil
		case bool:
			return undOp(a, op)
		case complex64:
			return +x, nil
		case complex128:
			return +x, nil
		case float32:
			return +x, nil
		case float64:
			return +x, nil
		case int8:
			return +x, nil
		case int16:
			return +x, nil
		case int32:
			return +x, nil
		case int64:
			return +x, nil
		case string:
			return undOp(a, op)
		case uint8:
			return +x, nil
		case uint16:
			return +x, nil
		case uint32:
			return +x, nil
		case uint64:
			return +x, nil
		case *big.Int:
			var z big.Int
			return z.Set(x), nil
		case *big.Rat:
			var z big.Rat
			return z.Set(x), nil
		case time.Duration:
			return x, nil
		default:
			return undOp(a, op)
		}
	case '-':
		a := eval(u.v, execCtx, ctx)
		if a == nil {
			return
		}

		switch x := a.(type) {
		//case nil:
		case idealComplex:
			return -x, nil
		case idealFloat:
			return -x, nil
		case idealInt:
			return -x, nil
		case idealRune:
			return -x, nil
		case idealUint:
			return -x, nil
		case bool:
			return undOp(a, op)
		case complex64:
			return -x, nil
		case complex128:
			return -x, nil
		case float32:
			return -x, nil
		case float64:
			return -x, nil
		case int8:
			return -x, nil
		case int16:
			return -x, nil
		case int32:
			return -x, nil
		case int64:
			return -x, nil
		case string:
			return undOp(a, op)
		case uint8:
			return -x, nil
		case uint16:
			return -x, nil
		case uint32:
			return -x, nil
		case uint64:
			return -x, nil
		case *big.Int:
			var z big.Int
			return z.Neg(x), nil
		case *big.Rat:
			var z big.Rat
			return z.Neg(x), nil
		case time.Duration:
			return -x, nil
		default:
			return undOp(a, op)
		}
	default:
		panic("internal error 039")
	}
}

type call struct {
	f   string
	arg []expression
}

func newCall(f string, arg []expression) (v expression, isAgg bool, err error) {
	x := builtin[f]
	if x.f == nil {
		return nil, false, fmt.Errorf("undefined: %s", f)
	}

	isAgg = x.isAggregate
	if g, min, max := len(arg), x.minArgs, x.maxArgs; g < min || g > max {
		a := []interface{}{}
		for _, v := range arg {
			a = append(a, v)
		}
		return nil, false, badNArgs(min, f, a)
	}

	c := call{f: f}
	for _, val := range arg {
		if !val.isStatic() {
			c.arg = append(c.arg, val)
			continue
		}

		eval, err := val.eval(nil, nil)
		if err != nil {
			return nil, isAgg, err
		}

		c.arg = append(c.arg, value{eval})
	}

	return &c, isAgg, nil
}

func (c *call) clone(arg []interface{}, unqualify ...string) (expression, error) {
	list, err := cloneExpressionList(arg, c.arg)
	if err != nil {
		return nil, err
	}

	return &call{f: c.f, arg: list}, nil
}

func (c *call) isStatic() bool {
	v := builtin[c.f]
	if v.f == nil || !v.isStatic {
		return false
	}

	for _, v := range c.arg {
		if !v.isStatic() {
			return false
		}
	}
	return true
}

func (c *call) String() string {
	a := []string{}
	for _, v := range c.arg {
		a = append(a, v.String())
	}
	return fmt.Sprintf("%s(%s)", c.f, strings.Join(a, ", "))
}

func (c *call) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	f, ok := builtin[c.f]
	if !ok {
		return nil, fmt.Errorf("unknown function %s", c.f)
	}

	isID := c.f == "id"
	a := make([]interface{}, len(c.arg))
	for i, arg := range c.arg {
		if v, err = expand1(arg.eval(execCtx, ctx)); err != nil {
			if !isID {
				return nil, err
			}

			if _, ok := arg.(*ident); !ok {
				return nil, err
			}

			a[i] = arg
			continue
		}

		a[i] = v
	}

	if ctx != nil {
		ctx["$fn"] = c
	}
	return f.f(a, ctx)
}

type parameter struct {
	n int
}

func (p parameter) clone(arg []interface{}, unqualify ...string) (expression, error) {
	i := p.n - 1
	if i < len(arg) {
		return value{val: arg[i]}, nil
	}

	return nil, fmt.Errorf("missing %s", p)
}

func (parameter) isStatic() bool { return false }

func (p parameter) String() string { return fmt.Sprintf("$%d", p.n) }

func (p parameter) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	i := p.n - 1
	if i < len(execCtx.arg) {
		return execCtx.arg[i], nil
	}

	return nil, fmt.Errorf("missing %s", p)
}

//MAYBE make it an unary operation
type isNull struct {
	expr expression
	not  bool
}

//LATER newIsNull

func (is *isNull) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := is.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &isNull{expr: expr, not: is.not}, nil
}

func (is *isNull) isStatic() bool { return is.expr.isStatic() }

func (is *isNull) String() string {
	if is.not {
		return fmt.Sprintf("%s IS NOT NULL", is.expr)
	}

	return fmt.Sprintf("%s IS NULL", is.expr)
}

func (is *isNull) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	val, err := is.expr.eval(execCtx, ctx)
	if err != nil {
		return
	}

	return val == nil != is.not, nil
}

type indexOp struct {
	expr, x expression
}

func newIndex(sv, xv expression) (v expression, err error) {
	s, fs, i := "", false, uint64(0)
	x := indexOp{sv, xv}
	if x.expr.isStatic() {
		v, err := x.expr.eval(nil, nil)
		if err != nil {
			return nil, err
		}

		if v == nil {
			return value{nil}, nil
		}

		if s, fs = v.(string); !fs {
			return nil, invXOp(sv, xv)
		}

		x.expr = value{s}
	}

	if x.x.isStatic() {
		v, err := x.x.eval(nil, nil)
		if err != nil {
			return nil, err
		}

		if v == nil {
			return value{nil}, nil
		}

		var p *string
		if fs {
			p = &s
		}
		if i, err = indexExpr(p, v); err != nil {
			return nil, err
		}

		x.x = value{i}
	}

	return &x, nil
}

func (x *indexOp) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := x.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	x2, err := x.x.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	return &indexOp{expr: expr, x: x2}, nil
}

func (x *indexOp) isStatic() bool {
	return x.expr.isStatic() && x.x.isStatic()
}

func (x *indexOp) String() string { return fmt.Sprintf("%s[%s]", x.expr, x.x) }

func (x *indexOp) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	s0, err := x.expr.eval(execCtx, ctx)
	if err != nil {
		return nil, runErr(err)
	}

	s, ok := s0.(string)
	if !ok {
		return nil, runErr(invXOp(s0, x.x))
	}

	i0, err := x.x.eval(execCtx, ctx)
	if err != nil {
		return nil, runErr(err)
	}

	if i0 == nil {
		return nil, nil
	}

	i, err := indexExpr(&s, i0)
	if err != nil {
		return nil, runErr(err)
	}

	return s[i], nil
}

type slice struct {
	expr   expression
	lo, hi *expression
}

func newSlice(e expression, lo, hi *expression) (v expression, err error) {
	y := slice{e, lo, hi}
	var val interface{}
	if e := y.expr; e.isStatic() {
		if val, err = e.eval(nil, nil); err != nil {
			return nil, err
		}

		if val == nil {
			return value{nil}, nil
		}

		y.expr = value{val}
	}

	if p := y.lo; p != nil {
		if e := expr(*p); e.isStatic() {
			if val, err = e.eval(nil, nil); err != nil {
				return nil, err
			}

			if val == nil {
				return value{nil}, nil
			}

			v := expression(value{val})
			y.lo = &v
		}
	}

	if p := y.hi; p != nil {
		if e := expr(*p); e.isStatic() {
			if val, err = e.eval(nil, nil); err != nil {
				return nil, err
			}

			if val == nil {
				return value{nil}, nil
			}

			v := expression(value{val})
			y.hi = &v
		}
	}
	return &y, nil
}

func (s *slice) clone(arg []interface{}, unqualify ...string) (expression, error) {
	expr, err := s.expr.clone(arg, unqualify...)
	if err != nil {
		return nil, err
	}

	r := &slice{expr: expr, lo: s.lo, hi: s.hi}
	if s.lo != nil {
		e, err := (*s.lo).clone(arg, unqualify...)
		if err != nil {
			return nil, err
		}

		r.lo = &e
	}
	if s.hi != nil {
		e, err := (*s.hi).clone(arg, unqualify...)
		if err != nil {
			return nil, err
		}

		r.hi = &e
	}
	return r, nil
}

func (s *slice) eval(execCtx *execCtx, ctx map[interface{}]interface{}) (v interface{}, err error) {
	s0, err := s.expr.eval(execCtx, ctx)
	if err != nil {
		return
	}

	if s0 == nil {
		return
	}

	ss, ok := s0.(string)
	if !ok {
		return nil, runErr(invSOp(s0))
	}

	var iLo, iHi uint64
	if s.lo != nil {
		i, err := (*s.lo).eval(execCtx, ctx)
		if err != nil {
			return nil, err
		}

		if i == nil {
			return nil, err
		}

		if iLo, err = sliceExpr(&ss, i, 0); err != nil {
			return nil, err
		}
	}

	iHi = uint64(len(ss))
	if s.hi != nil {
		i, err := (*s.hi).eval(execCtx, ctx)
		if err != nil {
			return nil, err
		}

		if i == nil {
			return nil, err
		}

		if iHi, err = sliceExpr(&ss, i, 1); err != nil {
			return nil, err
		}
	}

	return ss[iLo:iHi], nil
}

func (s *slice) isStatic() bool {
	if !s.expr.isStatic() {
		return false
	}

	if p := s.lo; p != nil && !(*p).isStatic() {
		return false
	}

	if p := s.hi; p != nil && !(*p).isStatic() {
		return false
	}

	return false
}

func (s *slice) String() string {
	switch {
	case s.lo == nil && s.hi == nil:
		return fmt.Sprintf("%v[:]", s.expr)
	case s.lo == nil && s.hi != nil:
		return fmt.Sprintf("%v[:%v]", s.expr, *s.hi)
	case s.lo != nil && s.hi == nil:
		return fmt.Sprintf("%v[%v:]", s.expr, *s.lo)
	default: //case s.lo != nil && s.hi != nil:
		return fmt.Sprintf("%v[%v:%v]", s.expr, *s.lo, *s.hi)
	}
}
