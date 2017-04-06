// Copyright 2014 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//LATER profile mem
//LATER profile cpu
//LATER coverage

package ql

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cznic/strutil"
)

const (
	crossJoin = iota
	leftJoin
	rightJoin
	fullJoin
)

// NOTE: all rset implementations must be safe for concurrent use by multiple
// goroutines.  If the do method requires any execution domain local data, they
// must be held out of the implementing instance.
var (
	_ rset = (*distinctRset)(nil)
	_ rset = (*groupByRset)(nil)
	_ rset = (*joinRset)(nil)
	_ rset = (*limitRset)(nil)
	_ rset = (*offsetRset)(nil)
	_ rset = (*orderByRset)(nil)
	_ rset = (*selectRset)(nil)
	_ rset = (*selectStmt)(nil)
	_ rset = (*tableRset)(nil)
	_ rset = (*whereRset)(nil)

	isTesting bool // enables test hook: select from an index
)

type rset interface {
	plan(ctx *execCtx) (plan, error)
}

type recordset struct {
	ctx *execCtx
	plan
	tx *TCtx
}

func (r recordset) fieldNames() []interface{} {
	f := r.plan.fieldNames()
	a := make([]interface{}, len(f))
	for i, v := range f {
		a[i] = v
	}
	return a
}

// Do implements Recordset.
func (r recordset) Do(names bool, f func(data []interface{}) (bool, error)) error {
	if names {
		if more, err := f(r.fieldNames()); err != nil || !more {
			return err
		}
	}
	return r.ctx.db.do(r, f)
}

// Fields implements Recordset.
func (r recordset) Fields() (names []string, err error) {
	return r.plan.fieldNames(), nil
}

// FirstRow implements Recordset.
func (r recordset) FirstRow() (row []interface{}, err error) {
	rows, err := r.Rows(1, 0)
	if err != nil {
		return nil, err
	}

	if len(rows) != 0 {
		return rows[0], nil
	}

	return nil, nil
}

// Rows implements Recordset.
func (r recordset) Rows(limit, offset int) ([][]interface{}, error) {
	var rows [][]interface{}
	if err := r.Do(false, func(row []interface{}) (bool, error) {
		if offset > 0 {
			offset--
			return true, nil
		}

		switch {
		case limit < 0:
			rows = append(rows, row)
			return true, nil
		case limit == 0:
			return false, nil
		default: // limit > 0
			rows = append(rows, row)
			limit--
			return limit > 0, nil
		}
	}); err != nil {
		return nil, err
	}

	return rows, nil
}

// List represents a group of compiled statements.
type List struct {
	l      []stmt
	params int
}

// String implements fmt.Stringer
func (l List) String() string {
	var b bytes.Buffer
	f := strutil.IndentFormatter(&b, "\t")
	for _, s := range l.l {
		switch s.(type) {
		case beginTransactionStmt:
			f.Format("%s\n%i", s)
		case commitStmt, rollbackStmt:
			f.Format("%u%s\n", s)
		default:
			f.Format("%s\n", s)
		}
	}
	return b.String()
}

// IsExplainStmt reports whether l is a single EXPLAIN statement or a single EXPLAIN
// statement enclosed in a transaction.
func (l List) IsExplainStmt() bool {
	switch len(l.l) {
	case 1:
		_, ok := l.l[0].(*explainStmt)
		return ok
	case 3:
		if _, ok := l.l[0].(beginTransactionStmt); !ok {
			return false
		}
		if _, ok := l.l[1].(*explainStmt); !ok {
			return false
		}
		_, ok := l.l[2].(commitStmt)
		return ok
	default:
		return false
	}
}

type groupByRset struct {
	colNames []string
	src      plan
}

func (r *groupByRset) plan(ctx *execCtx) (plan, error) {
	fields := r.src.fieldNames()
	for _, v := range r.colNames {
		found := false
		for _, v2 := range fields {
			if v == v2 {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown field %s", v)
		}
	}
	return &groupByDefaultPlan{colNames: r.colNames, src: r.src, fields: fields}, nil
}

// TCtx represents transaction context. It enables to execute multiple
// statement lists in the same context. The same context guarantees the state
// of the DB cannot change in between the separated executions.
//
// LastInsertID
//
// LastInsertID is updated by INSERT INTO statements. The value considers
// performed ROLLBACK statements, if any, even though roll backed IDs are not
// reused. QL clients should treat the field as read only.
//
// RowsAffected
//
// RowsAffected is updated by INSERT INTO, DELETE FROM and UPDATE statements.
// The value does not (yet) consider any ROLLBACK statements involved.  QL
// clients should treat the field as read only.
type TCtx struct {
	LastInsertID int64
	RowsAffected int64
}

// NewRWCtx returns a new read/write transaction context.  NewRWCtx is safe for
// concurrent use by multiple goroutines, every one of them will get a new,
// unique context.
func NewRWCtx() *TCtx { return &TCtx{} }

// Recordset is a result of a select statement. It can call a user function for
// every row (record) in the set using the Do method.
//
// Recordsets can be safely reused. Evaluation of the rows is performed lazily.
// Every invocation of Do will see the current, potentially actualized data.
//
// Do
//
// Do will call f for every row (record) in the Recordset.
//
// If f returns more == false or err != nil then f will not be called for any
// remaining rows in the set and the err value is returned from Do.
//
// If names == true then f is firstly called with a virtual row
// consisting of field (column) names of the RecordSet.
//
// Do is executed in a read only context and performs a RLock of the
// database.
//
// Do is safe for concurrent use by multiple goroutines.
//
// Fields
//
// Fields return a slice of field names of the recordset. The result is computed
// without actually computing the recordset rows.
//
// FirstRow
//
// FirstRow will return the first row of the RecordSet or an error, if any. If
// the Recordset has no rows the result is (nil, nil).
//
// Rows
//
// Rows will return rows in Recordset or an error, if any. The semantics of
// limit and offset are the same as of the LIMIT and OFFSET clauses of the
// SELECT statement. To get all rows pass limit < 0. If there are no rows to
// return the result is (nil, nil).
type Recordset interface {
	Do(names bool, f func(data []interface{}) (more bool, err error)) error
	Fields() (names []string, err error)
	FirstRow() (row []interface{}, err error)
	Rows(limit, offset int) (rows [][]interface{}, err error)
}

type assignment struct {
	colName string
	expr    expression
}

func (a *assignment) String() string {
	return fmt.Sprintf("%s=%s", a.colName, a.expr)
}

type distinctRset struct {
	src plan
}

func (r *distinctRset) plan(ctx *execCtx) (plan, error) {
	return &distinctDefaultPlan{src: r.src, fields: r.src.fieldNames()}, nil
}

type orderByRset struct {
	asc bool
	by  []expression
	src plan
}

func (r *orderByRset) String() string {
	a := make([]string, len(r.by))
	for i, v := range r.by {
		a[i] = v.String()
	}
	s := strings.Join(a, ", ")
	if !r.asc {
		s += " DESC"
	}
	return s
}

func (r *orderByRset) plan(ctx *execCtx) (plan, error) {
	if _, ok := r.src.(*nullPlan); ok {
		return r.src, nil
	}

	var by []expression
	fields := r.src.fieldNames()
	for _, e := range r.by {
		cols := mentionedColumns(e)
		for k := range cols {
			found := false
			for _, v := range fields {
				if k == v {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown field %s", k)
			}
		}
		if len(cols) == 0 {
			v, err := e.eval(ctx, nil)
			if err != nil {
				by = append(by, e)
				continue
			}

			if isConstValue(v) != nil {
				continue
			}
		}

		by = append(by, e)
	}
	return &orderByDefaultPlan{asc: r.asc, by: by, src: r.src, fields: fields}, nil
}

type whereRset struct {
	expr expression
	src  plan
}

func (r *whereRset) planBinOp(x *binaryOperation) (plan, error) {
	p := r.src
	ok, cn := isColumnExpression(x.l)
	if ok && cn == "id()" {
		if v := isConstValue(x.r); v != nil {
			v, err := typeCheck1(v, idCol)
			if err != nil {
				return nil, err
			}

			rv := v.(int64)
			switch {
			case p.hasID():
				switch x.op {
				case '<':
					if rv <= 1 {
						return &nullPlan{p.fieldNames()}, nil
					}
				case '>':
					if rv <= 0 {
						return p, nil
					}
				case ge:
					if rv >= 1 {
						return p, nil
					}
				case neq:
					if rv <= 0 {
						return p, nil
					}
				case eq:
					if rv <= 0 {
						return &nullPlan{p.fieldNames()}, nil
					}
				case le:
					if rv <= 0 {
						return &nullPlan{p.fieldNames()}, nil
					}
				}
			}
		}
	}

	var err error
	var p2 plan
	var is []string
	switch x.op {
	case eq, ge, '>', le, '<', neq:
		if p2, is, err = p.filter(x); err != nil {
			return nil, err
		}

		if p2 != nil {
			return p2, nil
		}
	case andand:
		var in []expression
		var f func(expression)
		f = func(e expression) {
			b, ok := e.(*binaryOperation)
			if !ok || b.op != andand {
				in = append(in, e)
				return
			}

			f(b.l)
			f(b.r)
		}
		f(x)
		out := []expression{}
		p := r.src
		isNewPlan := false
		for _, e := range in {
			p2, is2, err := p.filter(e)
			if err != nil {
				return nil, err
			}

			if p2 == nil {
				is = append(is, is2...)
				out = append(out, e)
				continue
			}

			p = p2
			isNewPlan = true
		}

		if !isNewPlan {
			break
		}

		if len(out) == 0 {
			return p, nil
		}

		for len(out) > 1 {
			n := len(out)
			e, err := newBinaryOperation(andand, out[n-2], out[n-1])
			if err != nil {
				return nil, err
			}

			out = out[:n-1]
			out[n-2] = e
		}

		return &filterDefaultPlan{p, out[0], is}, nil
	}

	return &filterDefaultPlan{p, x, is}, nil
}

func (r *whereRset) planIdent(x *ident) (plan, error) {
	p := r.src
	p2, is, err := p.filter(x)
	if err != nil {
		return nil, err
	}

	if p2 != nil {
		return p2, nil
	}

	return &filterDefaultPlan{p, x, is}, nil
}

func (r *whereRset) planIsNull(x *isNull) (plan, error) {
	p := r.src
	ok, cn := isColumnExpression(x.expr)
	if !ok {
		return &filterDefaultPlan{p, x, nil}, nil
	}

	if cn == "id()" {
		switch {
		case p.hasID():
			switch {
			case x.not: // IS NOT NULL
				return p, nil
			default: // IS NULL
				return &nullPlan{p.fieldNames()}, nil
			}
		default:
			switch {
			case x.not: // IS NOT NULL
				return &nullPlan{p.fieldNames()}, nil
			default: // IS NULL
				return p, nil
			}
		}
	}

	p2, is, err := p.filter(x)
	if err != nil {
		return nil, err
	}

	if p2 != nil {
		return p2, nil
	}

	return &filterDefaultPlan{p, x, is}, nil
}

func (r *whereRset) planUnaryOp(x *unaryOperation) (plan, error) {
	p := r.src
	p2, is, err := p.filter(x)
	if err != nil {
		return nil, err
	}

	if p2 != nil {
		return p2, nil
	}

	return &filterDefaultPlan{p, x, is}, nil
}

func (r *whereRset) plan(ctx *execCtx) (plan, error) {
	expr, err := r.expr.clone(ctx.arg)
	if err != nil {
		return nil, err
	}

	switch r.src.(type) {
	case *leftJoinDefaultPlan, *rightJoinDefaultPlan, *fullJoinDefaultPlan:
		return &filterDefaultPlan{r.src, expr, nil}, nil
	}

	switch x := expr.(type) {
	case *binaryOperation:
		return r.planBinOp(x)
	case *ident:
		return r.planIdent(x)
	case *isNull:
		return r.planIsNull(x)
	case *pIn:
		//TODO optimize
		//TODO show plan
	case *pLike:
		//TODO optimize
	case *unaryOperation:
		return r.planUnaryOp(x)
	}

	return &filterDefaultPlan{r.src, expr, nil}, nil
}

type offsetRset struct {
	expr expression
	src  plan
}

func (r *offsetRset) plan(ctx *execCtx) (plan, error) {
	return &offsetDefaultPlan{expr: r.expr, src: r.src, fields: r.src.fieldNames()}, nil
}

type limitRset struct {
	expr expression
	src  plan
}

func (r *limitRset) plan(ctx *execCtx) (plan, error) {
	return &limitDefaultPlan{expr: r.expr, src: r.src, fields: r.src.fieldNames()}, nil
}

type selectRset struct {
	flds []*fld
	src  plan
}

func (r *selectRset) plan(ctx *execCtx) (plan, error) {
	var flds2 []*fld
	if len(r.flds) != 0 {
		m := map[string]struct{}{}
		for _, v := range r.flds {
			mentionedColumns0(v.expr, true, true, m)
		}
		for _, v := range r.src.fieldNames() {
			delete(m, v)
		}
		for k := range m {
			return nil, fmt.Errorf("unknown field %s", k)
		}

		flds2 = append(flds2, r.flds...)
	}

	if x, ok := r.src.(*groupByDefaultPlan); ok {
		if len(r.flds) == 0 {
			fields := x.fieldNames()
			flds := make([]*fld, len(fields))
			for i, v := range fields {
				flds[i] = &fld{&ident{v}, v}
			}
			return &selectFieldsGroupPlan{flds: flds, src: x, fields: fields}, nil
		}

		p := &selectFieldsGroupPlan{flds: flds2, src: x}
		for _, v := range r.flds {
			p.fields = append(p.fields, v.name)
		}
		return p, nil
	}

	if len(r.flds) == 0 {
		return r.src, nil
	}

	f0 := r.src.fieldNames()
	if len(f0) == len(flds2) {
		match := true
		for i, v := range flds2 {
			if x, ok := v.expr.(*ident); ok && x.s == f0[i] && v.name == f0[i] {
				continue
			}

			match = false
			break
		}

		if match {
			return r.src, nil
		}
	}

	src := r.src
	if x, ok := src.(*tableDefaultPlan); ok {
		isconst := true
		for _, v := range flds2 {
			if isConstValue(v.expr) == nil {
				isconst = false
				break
			}
		}
		if isconst { // #250
			src = &tableNilPlan{x.t}
		}
	}

	p := &selectFieldsDefaultPlan{flds: flds2, src: src}
	for _, v := range r.flds {
		p.fields = append(p.fields, v.name)
	}
	return p, nil
}

type tableRset string

func (r tableRset) plan(ctx *execCtx) (plan, error) {
	switch r {
	case "__Table":
		return &sysTableDefaultPlan{}, nil
	case "__Column":
		return &sysColumnDefaultPlan{}, nil
	case "__Index":
		return &sysIndexDefaultPlan{}, nil
	}

	t, ok := ctx.db.root.tables[string(r)]
	if !ok && isTesting {
		if _, x0 := ctx.db.root.findIndexByName(string(r)); x0 != nil {
			return &selectIndexDefaultPlan{nm: string(r), x: x0}, nil
		}
	}

	if !ok {
		return nil, fmt.Errorf("table %s does not exist", r)
	}

	rs := &tableDefaultPlan{t: t}
	for _, col := range t.cols {
		rs.fields = append(rs.fields, col.name)
	}
	return rs, nil
}

func findFld(fields []*fld, name string) (f *fld) {
	for _, f = range fields {
		if f.name == name {
			return
		}
	}

	return nil
}

type col struct {
	index      int
	name       string
	typ        int
	constraint *constraint
	dflt       expression
}

var idCol = &col{name: "id()", typ: qInt64}

func findCol(cols []*col, name string) (c *col) {
	for _, c = range cols {
		if c.name == name {
			return
		}
	}

	return nil
}

func (f *col) clone() *col {
	var r col
	r = *f
	r.constraint = f.constraint.clone()
	if f.dflt != nil {
		r.dflt, _ = r.dflt.clone(nil)
	}
	return &r
}

func (f *col) typeCheck(x interface{}) (ok bool) { //NTYPE
	switch x.(type) {
	case nil:
		return true
	case bool:
		return f.typ == qBool
	case complex64:
		return f.typ == qComplex64
	case complex128:
		return f.typ == qComplex128
	case float32:
		return f.typ == qFloat32
	case float64:
		return f.typ == qFloat64
	case int8:
		return f.typ == qInt8
	case int16:
		return f.typ == qInt16
	case int32:
		return f.typ == qInt32
	case int64:
		return f.typ == qInt64
	case string:
		return f.typ == qString
	case uint8:
		return f.typ == qUint8
	case uint16:
		return f.typ == qUint16
	case uint32:
		return f.typ == qUint32
	case uint64:
		return f.typ == qUint64
	case []byte:
		return f.typ == qBlob
	case *big.Int:
		return f.typ == qBigInt
	case *big.Rat:
		return f.typ == qBigRat
	case time.Time:
		return f.typ == qTime
	case time.Duration:
		return f.typ == qDuration
	case chunk:
		return true // was checked earlier
	}
	return
}

func cols2meta(f []*col) (s string) {
	a := []string{}
	for _, f := range f {
		a = append(a, string(f.typ)+f.name)
	}
	return strings.Join(a, "|")
}

// DB represent the database capable of executing QL statements.
type DB struct {
	cc          *TCtx // Current transaction context
	exprCache   map[string]expression
	exprCacheMu sync.Mutex
	hasIndex2   int // 0: nope, 1: in progress, 2: yes.
	isMem       bool
	mu          sync.Mutex
	queue       []chan struct{}
	root        *root
	rw          bool // DB FSM
	rwmu        sync.RWMutex
	store       storage
	tnl         int // Transaction nesting level
}

var selIndex2Expr = MustCompile("select Expr from __Index2_Expr where Index2_ID == $1")

func newDB(store storage) (db *DB, err error) {
	db0 := &DB{
		exprCache: map[string]expression{},
		store:     store,
	}
	if db0.root, err = newRoot(store); err != nil {
		return
	}

	ctx := &execCtx{db: db0}
	for _, t := range db0.root.tables {
		if err := t.constraintsAndDefaults(ctx); err != nil {
			return nil, err
		}
	}

	if !db0.hasAllIndex2() {
		return db0, nil
	}

	db0.hasIndex2 = 2
	rss, _, err := db0.Run(nil, "select id(), TableName, IndexName, IsUnique, Root from __Index2 where !IsSimple")
	if err != nil {
		return nil, err
	}

	rows, err := rss[0].Rows(-1, 0)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		defer func() {
			if e := recover(); e != nil {
				err = fmt.Errorf("error loading DB indices: %v", e)
			}
		}()

		id := row[0].(int64)
		tn := row[1].(string)
		xn := row[2].(string)
		unique := row[3].(bool)
		xroot := row[4].(int64)

		t := db0.root.tables[tn]
		if t == nil {
			return nil, fmt.Errorf("DB index refers to nonexistent table: %s", tn)
		}

		x, err := store.OpenIndex(unique, xroot)
		if err != nil {
			return nil, err
		}

		if v := t.indices2[xn]; v != nil {
			return nil, fmt.Errorf("duplicate DB index: %s", xn)
		}

		ix := &index2{
			unique: unique,
			x:      x,
			xroot:  xroot,
		}

		rss, _, err := db0.Execute(nil, selIndex2Expr, id)
		if err != nil {
			return nil, err
		}

		rows, err := rss[0].Rows(-1, 0)
		if err != nil {
			return nil, err
		}

		if len(rows) == 0 {
			return nil, fmt.Errorf("index has no expression: %s", xn)
		}

		var sources []string
		var list []expression
		for _, row := range rows {
			src, ok := row[0].(string)
			if !ok {
				return nil, fmt.Errorf("index %s: expression of type %T", xn, row[0])
			}

			expr, err := db0.str2expr(src)
			if err != nil {
				return nil, fmt.Errorf("index %s: expression error: %v", xn, err)
			}

			sources = append(sources, src)
			list = append(list, expr)
		}

		ix.sources = sources
		ix.exprList = list
		if t.indices2 == nil {
			t.indices2 = map[string]*index2{}
		}
		t.indices2[xn] = ix
	}
	return db0, nil
}

func (db *DB) deleteIndex2ByIndexName(nm string) error {
	for _, s := range deleteIndex2ByIndexName.l {
		if _, err := s.exec(&execCtx{db: db, arg: []interface{}{nm}}); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) deleteIndex2ByTableName(nm string) error {
	for _, s := range deleteIndex2ByTableName.l {
		if _, err := s.exec(&execCtx{db: db, arg: []interface{}{nm}}); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) createIndex2() error {
	if db.hasIndex2 != 0 {
		return nil
	}

	db.hasIndex2 = 1
	ctx := execCtx{db: db}
	for _, s := range createIndex2.l {
		if _, err := s.exec(&ctx); err != nil {
			db.hasIndex2 = 0
			return err
		}
	}

	for t := db.root.thead; t != nil; t = t.tnext {
		for i, index := range t.indices {
			if index == nil {
				continue
			}

			expr := "id()"
			if i != 0 {
				expr = t.cols[i-1].name
			}

			if err := db.insertIndex2(t.name, index.name, []string{expr}, index.unique, true, index.xroot); err != nil {
				db.hasIndex2 = 0
				return err
			}
		}
	}

	db.hasIndex2 = 2
	return nil
}

func (db *DB) insertIndex2(tableName, indexName string, expr []string, unique, isSimple bool, h int64) error {
	ctx := execCtx{db: db}
	ctx.arg = []interface{}{
		tableName,
		indexName,
		unique,
		isSimple,
		h,
	}
	if _, err := insertIndex2.l[0].exec(&ctx); err != nil {
		return err
	}

	id := db.root.lastInsertID
	for _, e := range expr {
		ctx.arg = []interface{}{id, e}
		if _, err := insertIndex2Expr.l[0].exec(&ctx); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) hasAllIndex2() bool {
	t := db.root.tables
	if _, ok := t["__Index2"]; !ok {
		return false
	}

	_, ok := t["__Index2_Expr"]
	return ok
}

func (db *DB) str2expr(expr string) (expression, error) {
	db.exprCacheMu.Lock()
	e := db.exprCache[expr]
	db.exprCacheMu.Unlock()
	if e != nil {
		return e, nil
	}

	e, err := compileExpr(expr)
	if err != nil {
		return nil, err
	}

	db.exprCacheMu.Lock()
	for k := range db.exprCache {
		if len(db.exprCache) < 1000 {
			break
		}

		delete(db.exprCache, k)
	}
	db.exprCache[expr] = e
	db.exprCacheMu.Unlock()
	return e, nil
}

// Name returns the name of the DB.
func (db *DB) Name() string { return db.store.Name() }

// Run compiles and executes a statement list.  It returns, if applicable, a
// RecordSet slice and/or an index and error.
//
// For more details please see DB.Execute
//
// Run is safe for concurrent use by multiple goroutines.
func (db *DB) Run(ctx *TCtx, ql string, arg ...interface{}) (rs []Recordset, index int, err error) {
	l, err := Compile(ql)
	if err != nil {
		return nil, -1, err
	}

	return db.Execute(ctx, l, arg...)
}

func (db *DB) run(ctx *TCtx, ql string, arg ...interface{}) (rs []Recordset, index int, err error) {
	l, err := compile(ql)
	if err != nil {
		return nil, -1, err
	}

	return db.Execute(ctx, l, arg...)
}

// Compile parses the ql statements from src and returns a compiled list for
// DB.Execute or an error if any.
//
// Compile is safe for concurrent use by multiple goroutines.
func Compile(src string) (List, error) {
	l := newLexer(src)
	if yyParse(l) != 0 {
		return List{}, l.errs[0]
	}

	return List{l.list, l.params}, nil
}

func compileExpr(src string) (expression, error) {
	l := newLexer(src)
	l.inj = parseExpression
	if yyParse(l) != 0 {
		return nil, l.errs[0]
	}

	return l.expr, nil
}

func compile(src string) (List, error) {
	l := newLexer(src)
	l.root = true
	if yyParse(l) != 0 {
		return List{}, l.errs[0]
	}

	return List{l.list, l.params}, nil
}

// MustCompile is like Compile but panics if the ql statements in src cannot be
// compiled. It simplifies safe initialization of global variables holding
// compiled statement lists for DB.Execute.
//
// MustCompile is safe for concurrent use by multiple goroutines.
func MustCompile(src string) List {
	list, err := Compile(src)
	if err != nil {
		panic("ql: Compile(" + strconv.Quote(src) + "): " + err.Error()) // panic ok here
	}

	return list
}

func mustCompile(src string) List {
	list, err := compile(src)
	if err != nil {
		panic("ql: compile(" + strconv.Quote(src) + "): " + err.Error()) // panic ok here
	}

	return list
}

// Execute executes statements in a list while substituting QL parameters from
// arg.
//
// The resulting []Recordset corresponds to the SELECT FROM statements in the
// list.
//
// If err != nil then index is the zero based index of the failed QL statement.
// Empty statements do not count.
//
// The FSM STT describing the relations between DB states, statements and the
// ctx parameter.
//
//  +-----------+---------------------+------------------+------------------+------------------+
//  |\  Event   |                     |                  |                  |                  |
//  | \-------\ |     BEGIN           |                  |                  |    Other         |
//  |   State  \|     TRANSACTION     |      COMMIT      |     ROLLBACK     |    statement     |
//  +-----------+---------------------+------------------+------------------+------------------+
//  | RD        | if PC == nil        | return error     | return error     | DB.RLock         |
//  |           |     return error    |                  |                  | Execute(1)       |
//  | CC == nil |                     |                  |                  | DB.RUnlock       |
//  | TNL == 0  | DB.Lock             |                  |                  |                  |
//  |           | CC = PC             |                  |                  |                  |
//  |           | TNL++               |                  |                  |                  |
//  |           | DB.BeginTransaction |                  |                  |                  |
//  |           | State = WR          |                  |                  |                  |
//  +-----------+---------------------+------------------+------------------+------------------+
//  | WR        | if PC == nil        | if PC != CC      | if PC != CC      | if PC == nil     |
//  |           |     return error    |     return error |     return error |     DB.Rlock     |
//  | CC != nil |                     |                  |                  |     Execute(1)   |
//  | TNL != 0  | if PC != CC         | DB.Commit        | DB.Rollback      |     RUnlock      |
//  |           |     DB.Lock         | TNL--            | TNL--            | else if PC != CC |
//  |           |     CC = PC         | if TNL == 0      | if TNL == 0      |     return error |
//  |           |                     |     CC = nil     |     CC = nil     | else             |
//  |           | TNL++               |     State = RD   |     State = RD   |     Execute(2)   |
//  |           | DB.BeginTransaction |     DB.Unlock    |     DB.Unlock    |                  |
//  +-----------+---------------------+------------------+------------------+------------------+
//  CC: Curent transaction context
//  PC: Passed transaction context
//  TNL: Transaction nesting level
//
// Lock, Unlock, RLock, RUnlock semantics above are the same as in
// sync.RWMutex.
//
// (1): Statement list is executed outside of a transaction. Attempts to update
// the DB will fail, the execution context is read-only. Other statements with
// read only context will execute concurrently. If any statement fails, the
// execution of the statement list is aborted.
//
// Note that the RLock/RUnlock surrounds every single "other" statement when it
// is executed outside of a transaction. If read consistency is required by a
// list of more than one statement then an explicit BEGIN TRANSACTION / COMMIT
// or ROLLBACK wrapper must be provided. Otherwise the state of the DB may
// change in between executing any two out-of-transaction statements.
//
// (2): Statement list is executed inside an isolated transaction. Execution of
// statements can update the DB, the execution context is read-write. If any
// statement fails, the execution of the statement list is aborted and the DB
// is automatically rolled back to the TNL which was active before the start of
// execution of the statement list.
//
// Execute is safe for concurrent use by multiple goroutines, but one must
// consider the blocking issues as discussed above.
//
// ACID
//
// Atomicity: Transactions are atomic. Transactions can be nested. Commit or
// rollbacks work on the current transaction level. Transactions are made
// persistent only on the top level commit. Reads made from within an open
// transaction are dirty reads.
//
// Consistency: Transactions bring the DB from one structurally consistent
// state to other structurally consistent state.
//
// Isolation: Transactions are isolated. Isolation is implemented by
// serialization.
//
// Durability: Transactions are durable. A two phase commit protocol and a
// write ahead log is used. Database is recovered after a crash from the write
// ahead log automatically on open.
func (db *DB) Execute(ctx *TCtx, l List, arg ...interface{}) (rs []Recordset, index int, err error) {
	// Sanitize args
	for i, v := range arg {
		switch x := v.(type) {
		case nil, bool, complex64, complex128, float32, float64, string,
			int8, int16, int32, int64, int,
			uint8, uint16, uint32, uint64, uint,
			*big.Int, *big.Rat, []byte, time.Duration, time.Time:
		case big.Int:
			arg[i] = &x
		case big.Rat:
			arg[i] = &x
		default:
			return nil, 0, fmt.Errorf("cannot use arg[%d] (type %T):unsupported type", i, v)
		}
	}

	tnl0 := -1
	if ctx != nil {
		ctx.LastInsertID, ctx.RowsAffected = 0, 0
	}

	list := l.l
	for _, s := range list {
		r, tnla, tnl, err := db.run1(ctx, s, arg...)
		if tnl0 < 0 {
			tnl0 = tnla
		}
		if err != nil {
			for tnl > tnl0 {
				var e2 error
				if _, _, tnl, e2 = db.run1(ctx, rollbackStmt{}); e2 != nil {
					err = e2
				}
			}
			return rs, index, err
		}

		if r != nil {
			if x, ok := r.(recordset); ok {
				x.tx = ctx
				r = x
			}
			rs = append(rs, r)
		}
	}
	return
}

func (db *DB) muUnlock() {
	if n := len(db.queue); n != 0 {
		db.queue[0] <- struct{}{}
		copy(db.queue, db.queue[1:])
		db.queue = db.queue[:n-1]
	}
	db.mu.Unlock()
}

func (db *DB) run1(pc *TCtx, s stmt, arg ...interface{}) (rs Recordset, tnla, tnlb int, err error) {
	db.mu.Lock()
	tnla = db.tnl
	tnlb = db.tnl
	switch db.rw {
	case false:
		switch s.(type) {
		case beginTransactionStmt:
			defer db.muUnlock()
			if pc == nil {
				return nil, tnla, tnlb, errors.New("BEGIN TRANSACTION: cannot start a transaction in nil TransactionCtx")
			}

			if err = db.store.BeginTransaction(); err != nil {
				return
			}

			db.rwmu.Lock()
			db.beginTransaction()
			db.cc = pc
			db.tnl++
			tnlb = db.tnl
			db.rw = true
			return
		case commitStmt:
			defer db.muUnlock()
			return nil, tnla, tnlb, errCommitNotInTransaction
		case rollbackStmt:
			defer db.muUnlock()
			return nil, tnla, tnlb, errRollbackNotInTransaction
		default:
			if s.isUpdating() {
				db.muUnlock()
				return nil, tnla, tnlb, fmt.Errorf("attempt to update the DB outside of a transaction")
			}

			db.rwmu.RLock() // can safely grab before Unlock
			db.muUnlock()
			defer db.rwmu.RUnlock()
			rs, err = s.exec(&execCtx{db, arg}) // R/O tctx
			return rs, tnla, tnlb, err
		}
	default: // case true:
		switch s.(type) {
		case beginTransactionStmt:
			defer db.muUnlock()

			if pc == nil {
				return nil, tnla, tnlb, errBeginTransNoCtx
			}

			if pc != db.cc {
				for db.rw {
					ch := make(chan struct{}, 1)
					db.queue = append(db.queue, ch)
					db.mu.Unlock()
					<-ch
					db.mu.Lock()
				}

				db.rw = true
				db.rwmu.Lock()

			}

			if err = db.store.BeginTransaction(); err != nil {
				return
			}

			db.beginTransaction()
			db.cc = pc
			db.tnl++
			tnlb = db.tnl
			return
		case commitStmt:
			defer db.muUnlock()
			if pc != db.cc {
				return nil, tnla, tnlb, fmt.Errorf("invalid passed transaction context")
			}

			db.commit()
			err = db.store.Commit()
			db.tnl--
			tnlb = db.tnl
			if db.tnl != 0 {
				return
			}

			db.cc = nil
			db.rw = false
			db.rwmu.Unlock()
			return
		case rollbackStmt:
			defer db.muUnlock()
			defer func() { pc.LastInsertID = db.root.lastInsertID }()
			if pc != db.cc {
				return nil, tnla, tnlb, fmt.Errorf("invalid passed transaction context")
			}

			db.rollback()
			err = db.store.Rollback()
			db.tnl--
			tnlb = db.tnl
			if db.tnl != 0 {
				return
			}

			db.cc = nil
			db.rw = false
			db.rwmu.Unlock()
			return
		default:
			if pc == nil {
				if s.isUpdating() {
					db.muUnlock()
					return nil, tnla, tnlb, fmt.Errorf("attempt to update the DB outside of a transaction")
				}

				db.muUnlock() // must Unlock before RLock
				db.rwmu.RLock()
				defer db.rwmu.RUnlock()
				rs, err = s.exec(&execCtx{db, arg})
				return rs, tnla, tnlb, err
			}

			defer db.muUnlock()
			defer func() { pc.LastInsertID = db.root.lastInsertID }()
			if pc != db.cc {
				return nil, tnla, tnlb, fmt.Errorf("invalid passed transaction context")
			}

			rs, err = s.exec(&execCtx{db, arg})
			return rs, tnla, tnlb, err
		}
	}
}

// Flush ends the transaction collecting window, if applicable. IOW, if the DB
// is dirty, it schedules a 2PC (WAL + DB file) commit on the next outer most
// DB.Commit or performs it synchronously if there's currently no open
// transaction.
//
// The collecting window is an implementation detail and future versions of
// Flush may become a no operation while keeping the operation semantics.
func (db *DB) Flush() (err error) {
	return nil
}

// Close will close the DB. Successful Close is idempotent.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.muUnlock()
	if db.store == nil {
		return nil
	}

	if db.tnl != 0 {
		return fmt.Errorf("cannot close DB while open transaction exist")
	}

	err := db.store.Close()
	db.root, db.store = nil, nil
	return err
}

func (db *DB) do(r recordset, f func(data []interface{}) (bool, error)) (err error) {
	db.mu.Lock()
	switch db.rw {
	case false:
		db.rwmu.RLock() // can safely grab before Unlock
		db.muUnlock()
		defer db.rwmu.RUnlock()
	default: // case true:
		if r.tx == nil {
			db.muUnlock() // must Unlock before RLock
			db.rwmu.RLock()
			defer db.rwmu.RUnlock()
			break
		}

		defer db.muUnlock()
		if r.tx != db.cc {
			return fmt.Errorf("invalid passed transaction context")
		}
	}

	return r.do(r.ctx, func(id interface{}, data []interface{}) (bool, error) {
		if err = expand(data); err != nil {
			return false, err
		}

		return f(data)
	})
}

func (db *DB) beginTransaction() { //TODO Rewrite, must use much smaller undo info!
	root := *db.root
	root.parent = db.root
	root.tables = make(map[string]*table, len(db.root.tables))
	var tprev *table
	for t := db.root.thead; t != nil; t = t.tnext {
		t2 := t.clone()
		root.tables[t2.name] = t2
		t2.tprev = tprev
		switch {
		case tprev == nil:
			root.thead = t2
		default:
			tprev.tnext = t2
		}
		tprev = t2
	}
	db.root = &root
}

func (db *DB) rollback() {
	db.root = db.root.parent
}

func (db *DB) commit() {
	db.root.parent = db.root.parent.parent
}

// Type represents a QL type (bigint, int, string, ...)
type Type int

// Values of ColumnInfo.Type.
const (
	BigInt     Type = qBigInt
	BigRat          = qBigRat
	Blob            = qBlob
	Bool            = qBool
	Complex128      = qComplex128
	Complex64       = qComplex64
	Duration        = qDuration
	Float32         = qFloat32
	Float64         = qFloat64
	Int16           = qInt16
	Int32           = qInt32
	Int64           = qInt64
	Int8            = qInt8
	String          = qString
	Time            = qTime
	Uint16          = qUint16
	Uint32          = qUint32
	Uint64          = qUint64
	Uint8           = qUint8
)

// String implements fmt.Stringer.
func (t Type) String() string {
	return typeStr(int(t))
}

// ColumnInfo provides meta data describing a table column.
type ColumnInfo struct {
	Name       string // Column name.
	Type       Type   // Column type (BigInt, BigRat, ...).
	NotNull    bool   // Column cannot be NULL.
	Constraint string // Constraint expression, if any.
	Default    string // Default expression, if any.
}

// TableInfo provides meta data describing a DB table.
type TableInfo struct {
	// Table name.
	Name string

	// Table schema. Columns are listed in the order in which they appear
	// in the schema.
	Columns []ColumnInfo
}

// IndexInfo provides meta data describing a DB index.  It corresponds to the
// statement
//
//	CREATE INDEX Name ON Table (Column);
type IndexInfo struct {
	Name           string   // Index name
	Table          string   // Table name.
	Column         string   // Column name.
	Unique         bool     // Whether the index is unique.
	ExpressionList []string // Index expression list.
}

// DbInfo provides meta data describing a DB.
type DbInfo struct {
	Name    string      // DB name.
	Tables  []TableInfo // Tables in the DB.
	Indices []IndexInfo // Indices in the DB.
}

func (db *DB) info() (r *DbInfo, err error) {
	_, hasColumn2 := db.root.tables["__Column2"]
	r = &DbInfo{Name: db.Name()}
	for nm, t := range db.root.tables {
		ti := TableInfo{Name: nm}
		m := map[string]*ColumnInfo{}
		if hasColumn2 {
			rs, err := selectColumn2.l[0].exec(&execCtx{db: db, arg: []interface{}{nm}})
			if err != nil {
				return nil, err
			}

			if err := rs.(recordset).do(
				&execCtx{db: db, arg: []interface{}{nm}},
				func(id interface{}, data []interface{}) (bool, error) {
					ci := &ColumnInfo{NotNull: data[1].(bool), Constraint: data[2].(string), Default: data[3].(string)}
					m[data[0].(string)] = ci
					return true, nil
				},
			); err != nil {
				return nil, err
			}
		}
		for _, c := range t.cols {
			ci := ColumnInfo{Name: c.name, Type: Type(c.typ)}
			if c2 := m[c.name]; c2 != nil {
				ci.NotNull = c2.NotNull
				ci.Constraint = c2.Constraint
				ci.Default = c2.Default
			}
			ti.Columns = append(ti.Columns, ci)
		}
		r.Tables = append(r.Tables, ti)
		for i, x := range t.indices {
			if x == nil {
				continue
			}

			var cn string
			switch {
			case i == 0:
				cn = "id()"
			default:
				cn = t.cols0[i-1].name
			}
			r.Indices = append(r.Indices, IndexInfo{x.name, nm, cn, x.unique, []string{cn}})
		}
		var a []string
		for k := range t.indices2 {
			a = append(a, k)
		}
		for _, k := range a {
			x := t.indices2[k]
			a = a[:0]
			for _, e := range x.exprList {
				a = append(a, e.String())
			}
			r.Indices = append(r.Indices, IndexInfo{k, nm, "", x.unique, a})
		}
	}
	return
}

// Info provides meta data describing a DB or an error if any. It locks the DB
// to obtain the result.
func (db *DB) Info() (r *DbInfo, err error) {
	db.mu.Lock()
	defer db.muUnlock()
	return db.info()
}

type constraint struct {
	expr expression // If expr == nil: constraint is 'NOT NULL'
}

func (c *constraint) clone() *constraint {
	if c == nil {
		return nil
	}

	var e expression
	if c.expr != nil {
		e, _ = c.expr.clone(nil)
	}
	return &constraint{e}
}

type joinRset struct {
	sources []interface{}
	typ     int
	on      expression
}

func (r *joinRset) String() string {
	a := make([]string, len(r.sources))
	for i, pair0 := range r.sources {
		pair := pair0.([]interface{})
		altName := pair[1].(string)
		switch x := pair[0].(type) {
		case string: // table name
			switch {
			case altName == "":
				a[i] = x
			default:
				a[i] = fmt.Sprintf("%s AS %s", x, altName)
			}
		case *selectStmt:
			switch {
			case altName == "":
				a[i] = fmt.Sprintf("(%s)", x)
			default:
				a[i] = fmt.Sprintf("(%s) AS %s", x, altName)
			}
		default:
			panic("internal error 054")
		}
	}
	n := len(a)
	a2 := a[:n-1]
	j := a[n-1]
	var s string
	switch r.typ {
	case crossJoin:
		return strings.Join(a, ", ")
	case leftJoin:
		s = strings.Join(a2, ",") + " LEFT"
	case rightJoin:
		s = strings.Join(a2, ",") + " RIGHT"
	case fullJoin:
		s = strings.Join(a2, ",") + " FULL"
	}
	s += " OUTER JOIN " + j + " ON " + r.on.String()
	return s
}

func (r *joinRset) plan(ctx *execCtx) (plan, error) {
	rsets := make([]plan, len(r.sources))
	names := make([]string, len(r.sources))
	var err error
	m := map[string]bool{}
	var fields []string
	for i, v := range r.sources {
		pair := v.([]interface{})
		src := pair[0]
		nm := pair[1].(string)
		if s, ok := src.(string); ok {
			src = tableRset(s)
			if nm == "" {
				nm = s
			}
		}
		if m[nm] {
			return nil, fmt.Errorf("%s: duplicate name %s", r.String(), nm)
		}

		if nm != "" {
			m[nm] = true
		}
		names[i] = nm
		var q plan
		switch x := src.(type) {
		case rset:
			if q, err = x.plan(ctx); err != nil {
				return nil, err
			}
		case plan:
			q = x
		default:
			panic("internal error 008")
		}

		switch {
		case len(r.sources) == 1:
			fields = q.fieldNames()
		default:
			for _, f := range q.fieldNames() {
				if strings.Contains(f, ".") {
					return nil, fmt.Errorf("cannot join on recordset with already qualified field names (use the AS clause): %s", f)
				}

				if f != "" && nm != "" {
					f = fmt.Sprintf("%s.%s", nm, f)
				}
				if nm == "" {
					f = ""
				}
				fields = append(fields, f)
			}
		}
		rsets[i] = q
	}

	if len(rsets) == 1 {
		return rsets[0], nil
	}

	right := len(rsets[len(rsets)-1].fieldNames())
	switch r.typ {
	case crossJoin:
		return &crossJoinDefaultPlan{rsets: rsets, names: names, fields: fields}, nil
	case leftJoin:
		return &leftJoinDefaultPlan{rsets: rsets, names: names, fields: fields, on: r.on, right: right}, nil
	case rightJoin:
		return &rightJoinDefaultPlan{leftJoinDefaultPlan{rsets: rsets, names: names, fields: fields, on: r.on, right: right}}, nil
	case fullJoin:
		return &fullJoinDefaultPlan{leftJoinDefaultPlan{rsets: rsets, names: names, fields: fields, on: r.on, right: right}}, nil
	default:
		panic("internal error 010")
	}
}

type fld struct {
	expr expression
	name string
}
