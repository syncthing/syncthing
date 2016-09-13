// Copyright (c) 2014 ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ql

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cznic/strutil"
)

// NOTE: all stmt implementations must be safe for concurrent use by multiple
// goroutines.  If the exec method requires any execution domain local data,
// they must be held out of the implementing instance.
var (
	_ stmt = (*alterTableAddStmt)(nil)
	_ stmt = (*alterTableDropColumnStmt)(nil)
	_ stmt = (*createIndexStmt)(nil)
	_ stmt = (*createTableStmt)(nil)
	_ stmt = (*deleteStmt)(nil) //TODO optimizer plan
	_ stmt = (*dropIndexStmt)(nil)
	_ stmt = (*dropTableStmt)(nil)
	_ stmt = (*explainStmt)(nil)
	_ stmt = (*insertIntoStmt)(nil)
	_ stmt = (*selectStmt)(nil)
	_ stmt = (*truncateTableStmt)(nil)
	_ stmt = (*updateStmt)(nil) //TODO optimizer plan
	_ stmt = beginTransactionStmt{}
	_ stmt = commitStmt{}
	_ stmt = rollbackStmt{}
)

var (
	createColumn2 = mustCompile(`
		create table if not exists __Column2 (
			TableName string,
			Name string,
			NotNull bool,
			ConstraintExpr string,
			DefaultExpr string,
		);
		create index if not exists __Column2TableName on __Column2(TableName);
	`)

	insertColumn2 = mustCompile(`insert into __Column2 values($1, $2, $3, $4, $5)`)

	selectColumn2 = MustCompile(`
		select Name, NotNull, ConstraintExpr, DefaultExpr
		from __Column2
		where TableName == $1
	`)

	deleteColumn2 = mustCompile(`
		delete from __Column2
		where TableName == $1 && Name == $2
	`)

	createIndex2 = mustCompile(`
		// Index register 2.
		create table if not exists __Index2(
			TableName string,
			IndexName string,
			IsUnique  bool,
			IsSimple  bool,   // Just a column name or id().
			Root      int64,  // BTree handle
		);

		// Expressions for given index. Compared in order of id(__Index2_Expr).
		create table if not exists __Index2_Expr(
			Index2_ID int,
			Expr      string,
		);

		create index if not exists __xIndex2_TableName on __Index2(TableName);
		create unique index if not exists __xIndex2_IndexName on __Index2(IndexName);
		create index if not exists __xIndex2_ID on __Index2(id());
		create index if not exists __xIndex2_Expr_Index2_ID on __Index2_Expr(Index2_ID);
`)

	insertIndex2     = mustCompile("insert into __Index2 values($1, $2, $3, $4, $5)")
	insertIndex2Expr = mustCompile("insert into __Index2_Expr values($1, $2)")

	deleteIndex2ByIndexName = mustCompile(`
		delete from __Index2_Expr
		where Index2_ID in (
			select id() from __Index2 where IndexName == $1;
		);	

		delete from __Index2
		where IndexName == $1;
`)
	deleteIndex2ByTableName = mustCompile(`
		delete from __Index2_Expr
		where Index2_ID in (
			select id() from __Index2 where TableName == $1;
		);	

		delete from __Index2
		where TableName == $1;
`)
)

type stmt interface {
	// never invoked for
	// - beginTransactionStmt
	// - commitStmt
	// - rollbackStmt
	exec(ctx *execCtx) (Recordset, error)

	explain(ctx *execCtx, w strutil.Formatter)

	// return value ignored for
	// - beginTransactionStmt
	// - commitStmt
	// - rollbackStmt
	isUpdating() bool
	String() string
}

type execCtx struct { //LATER +shared temp
	db  *DB
	arg []interface{}
}

type explainStmt struct {
	s stmt
}

func (s *explainStmt) explain(ctx *execCtx, w strutil.Formatter) {
	for {
		x, ok := s.s.(*explainStmt)
		if !ok {
			s.s.explain(ctx, w)
			return
		}

		s = x
	}
}

func (s *explainStmt) String() string {
	return "EXPLAIN " + s.s.String()
}

func (*explainStmt) isUpdating() bool { return false }

func (s *explainStmt) exec(ctx *execCtx) (_ Recordset, err error) {
	return recordset{ctx, &explainDefaultPlan{s.s}, ctx.db.cc}, nil
}

type updateStmt struct {
	tableName string
	list      []assignment
	where     expression
}

func (s *updateStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *updateStmt) String() string {
	u := fmt.Sprintf("UPDATE %s", s.tableName)
	a := make([]string, len(s.list))
	for i, v := range s.list {
		a[i] = v.String()
	}
	w := ""
	if s.where != nil {
		w = fmt.Sprintf(" WHERE %s", s.where)
	}
	return fmt.Sprintf("%s %s%s;", u, strings.Join(a, ", "), w)
}

func (s *updateStmt) exec(ctx *execCtx) (_ Recordset, err error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("UPDATE: table %s does not exist", s.tableName)
	}

	tcols := make([]*col, len(s.list))
	for i, asgn := range s.list {
		col := findCol(t.cols, asgn.colName)
		if col == nil {
			return nil, fmt.Errorf("UPDATE: unknown column %s", asgn.colName)
		}
		tcols[i] = col
	}

	m := map[interface{}]interface{}{}
	var nh int64
	expr := s.where
	blobCols := t.blobCols()
	cc := ctx.db.cc
	var old []interface{}
	var touched []bool
	if t.hasIndices() {
		old = make([]interface{}, len(t.cols0))
		touched = make([]bool, len(t.cols0))
	}
	for h := t.head; h != 0; h = nh {
		// Read can return lazily expanded chunks
		data, err := t.store.Read(nil, h, t.cols...)
		if err != nil {
			return nil, err
		}

		nh = data[0].(int64)
		for _, col := range t.cols {
			m[col.name] = data[2+col.index]
		}
		id := data[1].(int64)
		m["$id"] = id
		if expr != nil {
			val, err := s.where.eval(ctx, m)
			if err != nil {
				return nil, err
			}

			if val == nil {
				continue
			}

			x, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("invalid WHERE expression %s (value of type %T)", val, val)
			}

			if !x {
				continue
			}
		}

		// hit
		for _, ix := range t.indices2 {
			vlist, err := ix.eval(ctx, t.cols, id, data[2:])
			if err != nil {
				return nil, err
			}

			if err := ix.x.Delete(vlist, h); err != nil {
				return nil, err
			}
		}
		for i, asgn := range s.list {
			val, err := asgn.expr.eval(ctx, m)
			if err != nil {
				return nil, err
			}

			colIndex := tcols[i].index
			if t.hasIndices() {
				old[colIndex] = data[2+colIndex]
				touched[colIndex] = true
			}
			data[2+colIndex] = val
		}
		if err = typeCheck(data[2:], t.cols); err != nil {
			return nil, err
		}

		if err = t.checkConstraintsAndDefaults(ctx, data[2:], m); err != nil {
			return nil, err
		}

		for i, v := range t.indices {
			if i == 0 { // id() N/A
				continue
			}

			if v == nil || !touched[i-1] {
				continue
			}

			if err = v.x.Delete([]interface{}{old[i-1]}, h); err != nil {
				return nil, err
			}
		}

		if err = t.store.UpdateRow(h, blobCols, data...); err != nil { //LATER detect which blobs are actually affected
			return nil, err
		}

		for i, v := range t.indices {
			if i == 0 { // id() N/A
				continue
			}

			if v == nil || !touched[i-1] {
				continue
			}

			if err = v.x.Create([]interface{}{data[2+i-1]}, h); err != nil {
				return nil, err
			}
		}
		for _, ix := range t.indices2 {
			vlist, err := ix.eval(ctx, t.cols, id, data[2:])
			if err != nil {
				return nil, err
			}

			if err := ix.x.Create(vlist, h); err != nil {
				return nil, err
			}
		}

		cc.RowsAffected++
	}
	return
}

func (s *updateStmt) isUpdating() bool { return true }

type deleteStmt struct {
	tableName string
	where     expression
}

func (s *deleteStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *deleteStmt) String() string {
	switch {
	case s.where == nil:
		return fmt.Sprintf("DELETE FROM %s;", s.tableName)
	default:
		return fmt.Sprintf("DELETE FROM %s WHERE %s;", s.tableName, s.where)
	}
}

func (s *deleteStmt) exec(ctx *execCtx) (_ Recordset, err error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("DELETE FROM: table %s does not exist", s.tableName)
	}

	m := map[interface{}]interface{}{}
	var ph, h, nh int64
	var data []interface{}
	blobCols := t.blobCols()
	cc := ctx.db.cc
	for h = t.head; h != 0; ph, h = h, nh {
		for i, v := range data {
			c, ok := v.(chunk)
			if !ok {
				continue
			}

			data[i] = c.b
		}
		// Read can return lazily expanded chunks
		data, err = t.store.Read(nil, h, t.cols...)
		if err != nil {
			return nil, err
		}

		nh = data[0].(int64)
		for _, col := range t.cols {
			m[col.name] = data[2+col.index]
		}
		id := data[1].(int64)
		m["$id"] = id
		val, err := s.where.eval(ctx, m)
		if err != nil {
			return nil, err
		}

		if val == nil {
			continue
		}

		x, ok := val.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid WHERE expression %s (value of type %T)", val, val)
		}

		if !x {
			continue
		}

		// hit
		for i, v := range t.indices {
			if v == nil {
				continue
			}

			// overflow chunks left in place
			if err = v.x.Delete([]interface{}{data[i+1]}, h); err != nil {
				return nil, err
			}
		}
		for _, ix := range t.indices2 {
			vlist, err := ix.eval(ctx, t.cols, id, data[2:])
			if err != nil {
				return nil, err
			}

			if err := ix.x.Delete(vlist, h); err != nil {
				return nil, err
			}
		}

		// overflow chunks freed here
		if err = t.store.Delete(h, blobCols...); err != nil {
			return nil, err
		}

		cc.RowsAffected++
		switch {
		case ph == 0 && nh == 0: // "only"
			fallthrough
		case ph == 0 && nh != 0: // "first"
			if err = t.store.Update(t.hhead, nh); err != nil {
				return nil, err
			}

			t.head, h = nh, 0
		case ph != 0 && nh == 0: // "last"
			fallthrough
		case ph != 0 && nh != 0: // "inner"
			pdata, err := t.store.Read(nil, ph, t.cols...)
			if err != nil {
				return nil, err
			}

			for i, v := range pdata {
				if x, ok := v.(chunk); ok {
					pdata[i] = x.b
				}
			}
			pdata[0] = nh
			if err = t.store.Update(ph, pdata...); err != nil {
				return nil, err
			}

			h = ph
		}
	}

	return
}

func (s *deleteStmt) isUpdating() bool { return true }

type truncateTableStmt struct {
	tableName string
}

func (s *truncateTableStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *truncateTableStmt) String() string { return fmt.Sprintf("TRUNCATE TABLE %s;", s.tableName) }

func (s *truncateTableStmt) exec(ctx *execCtx) (Recordset, error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("TRUNCATE TABLE: table %s does not exist", s.tableName)
	}

	return nil, t.truncate()
}

func (s *truncateTableStmt) isUpdating() bool { return true }

type dropIndexStmt struct {
	ifExists  bool
	indexName string
}

func (s *dropIndexStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *dropIndexStmt) String() string { return fmt.Sprintf("DROP INDEX %s;", s.indexName) }

func (s *dropIndexStmt) exec(ctx *execCtx) (Recordset, error) {
	t, x := ctx.db.root.findIndexByName(s.indexName)
	if x == nil {
		if s.ifExists {
			return nil, nil
		}

		return nil, fmt.Errorf("DROP INDEX: index %s does not exist", s.indexName)
	}

	if ctx.db.hasAllIndex2() {
		if err := ctx.db.deleteIndex2ByIndexName(s.indexName); err != nil {
			return nil, err
		}
	}

	switch ix := x.(type) {
	case *indexedCol:
		for i, v := range t.indices {
			if v == nil || v.name != s.indexName {
				continue
			}

			return nil, t.dropIndex(i)
		}
	case *index2:
		delete(t.indices2, s.indexName)
		return nil, ix.x.Drop()
	}

	panic("internal error 058")
}

func (s *dropIndexStmt) isUpdating() bool { return true }

type dropTableStmt struct {
	ifExists  bool
	tableName string
}

func (s *dropTableStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *dropTableStmt) String() string { return fmt.Sprintf("DROP TABLE %s;", s.tableName) }

func (s *dropTableStmt) exec(ctx *execCtx) (Recordset, error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		if s.ifExists {
			return nil, nil
		}

		return nil, fmt.Errorf("DROP TABLE: table %s does not exist", s.tableName)
	}

	if ctx.db.hasAllIndex2() {
		if err := ctx.db.deleteIndex2ByTableName(s.tableName); err != nil {
			return nil, err
		}
	}

	return nil, ctx.db.root.dropTable(t)
}

func (s *dropTableStmt) isUpdating() bool { return true }

type alterTableDropColumnStmt struct {
	tableName, colName string
}

func (s *alterTableDropColumnStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *alterTableDropColumnStmt) String() string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", s.tableName, s.colName)
}

func (s *alterTableDropColumnStmt) exec(ctx *execCtx) (Recordset, error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("ALTER TABLE: table %s does not exist", s.tableName)
	}

	cols := t.cols
	for _, c := range cols {
		if c.name == s.colName {
			if len(cols) == 1 {
				return nil, fmt.Errorf("ALTER TABLE %s DROP COLUMN: cannot drop the only column: %s", s.tableName, s.colName)
			}

			if _, ok := ctx.db.root.tables["__Column2"]; ok {
				if _, err := deleteColumn2.l[0].exec(&execCtx{db: ctx.db, arg: []interface{}{s.tableName, c.name}}); err != nil {
					return nil, err
				}
			}

			c.name = ""
			t.cols0[c.index].name = ""
			if t.hasIndices() {
				if len(t.indices) != 0 {
					if v := t.indices[c.index+1]; v != nil {
						if err := t.dropIndex(c.index + 1); err != nil {
							return nil, err
						}

						if ctx.db.hasAllIndex2() {
							if err := ctx.db.deleteIndex2ByIndexName(v.name); err != nil {
								return nil, err
							}
						}
					}
				}

				for nm, ix := range t.indices2 {
					for _, e := range ix.exprList {
						m := mentionedColumns(e)
						if _, ok := m[s.colName]; ok {
							if err := ctx.db.deleteIndex2ByIndexName(nm); err != nil {
								return nil, err
							}

							if err := ix.x.Drop(); err != nil {
								return nil, err
							}

							delete(t.indices2, nm)
							break
						}
					}
				}
			}
			if err := t.constraintsAndDefaults(ctx); err != nil {
				return nil, err
			}

			return nil, t.updated()
		}
	}

	return nil, fmt.Errorf("ALTER TABLE %s DROP COLUMN: column %s does not exist", s.tableName, s.colName)
}

func (s *alterTableDropColumnStmt) isUpdating() bool { return true }

type alterTableAddStmt struct {
	tableName string
	c         *col
}

func (s *alterTableAddStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *alterTableAddStmt) String() string {
	r := fmt.Sprintf("ALTER TABLE %s ADD %s %s", s.tableName, s.c.name, typeStr(s.c.typ))
	c := s.c
	if x := c.constraint; x != nil { //TODO add (*col).String()
		switch e := x.expr; {
		case e != nil:
			r += " " + e.String()
		default:
			r += " NOT NULL"
		}
	}
	if x := c.dflt; x != nil {
		r += " DEFAULT " + x.String()
	}
	return r + ";"
}

func (s *alterTableAddStmt) exec(ctx *execCtx) (Recordset, error) {
	t, ok := ctx.db.root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("ALTER TABLE: table %s does not exist", s.tableName)
	}

	hasRecords := t.head != 0
	c := s.c
	if c.constraint != nil && hasRecords {
		return nil, fmt.Errorf("ALTER TABLE %s ADD %s: cannot add constrained column to table with existing data", s.tableName, c.name)
	}

	cols := t.cols
	for _, c := range cols {
		nm := c.name
		if nm == s.c.name {
			return nil, fmt.Errorf("ALTER TABLE %s ADD: column %s exists", s.tableName, nm)
		}
	}

	if len(t.indices) != 0 {
		t.indices = append(t.indices, nil)
		t.xroots = append(t.xroots, 0)
		if err := t.store.Update(t.hxroots, t.xroots...); err != nil {
			return nil, err
		}
	}

	if c.constraint != nil || c.dflt != nil {
		for _, s := range createColumn2.l {
			_, err := s.exec(&execCtx{db: ctx.db})
			if err != nil {
				return nil, err
			}
		}
		notNull := c.constraint != nil && c.constraint.expr == nil
		var co, d string
		if c.constraint != nil && c.constraint.expr != nil {
			co = c.constraint.expr.String()
		}
		if e := c.dflt; e != nil {
			d = e.String()
		}
		if _, err := insertColumn2.l[0].exec(&execCtx{db: ctx.db, arg: []interface{}{s.tableName, c.name, notNull, co, d}}); err != nil {
			return nil, err
		}
	}

	t.cols0 = append(t.cols0, s.c)
	if err := t.constraintsAndDefaults(ctx); err != nil {
		return nil, err
	}

	return nil, t.updated()
}

func (s *alterTableAddStmt) isUpdating() bool { return true }

type selectStmt struct {
	distinct      bool
	flds          []*fld
	from          *joinRset
	group         *groupByRset
	hasAggregates bool
	limit         *limitRset
	offset        *offsetRset
	order         *orderByRset
	where         *whereRset
}

func (s *selectStmt) explain(ctx *execCtx, w strutil.Formatter) {
	p, err := s.plan(ctx)
	if err != nil {
		w.Format("ERROR: %v\n", err)
		return
	}

	p.explain(w)
}

func (s *selectStmt) String() string {
	var b bytes.Buffer
	b.WriteString("SELECT")
	if s.distinct {
		b.WriteString(" DISTINCT")
	}
	switch {
	case len(s.flds) == 0:
		b.WriteString(" *")
	default:
		a := make([]string, len(s.flds))
		for i, v := range s.flds {
			s := v.expr.String()
			if v.name != "" && v.name != s {
				s += " AS " + v.name
			}
			a[i] = s
		}
		b.WriteString(" " + strings.Join(a, ", "))
	}
	b.WriteString(" FROM ")
	b.WriteString(s.from.String())
	if s.where != nil {
		b.WriteString(" WHERE ")
		b.WriteString(s.where.expr.String())
	}
	if s.group != nil {
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(s.group.colNames, ", "))
	}
	if s.order != nil {
		b.WriteString(" ORDER BY ")
		b.WriteString(s.order.String())
	}
	if s.limit != nil {
		b.WriteString(" LIMIT ")
		b.WriteString(s.limit.expr.String())
	}
	if s.offset != nil {
		b.WriteString(" OFFSET ")
		b.WriteString(s.offset.expr.String())
	}
	b.WriteRune(';')
	return b.String()
}

func (s *selectStmt) plan(ctx *execCtx) (plan, error) { //LATER overlapping goroutines/pipelines
	r, err := s.from.plan(ctx)
	if err != nil {
		return nil, err
	}

	if w := s.where; w != nil {
		if r, err = (&whereRset{expr: w.expr, src: r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	switch {
	case !s.hasAggregates && s.group == nil: // nop
	case !s.hasAggregates && s.group != nil:
		if r, err = (&groupByRset{colNames: s.group.colNames, src: r}).plan(ctx); err != nil {
			return nil, err
		}
	case s.hasAggregates && s.group == nil:
		if r, err = (&groupByRset{src: r}).plan(ctx); err != nil {
			return nil, err
		}
	case s.hasAggregates && s.group != nil:
		if r, err = (&groupByRset{colNames: s.group.colNames, src: r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	if r, err = (&selectRset{flds: s.flds, src: r}).plan(ctx); err != nil {
		return nil, err
	}

	if s.distinct {
		if r, err = (&distinctRset{src: r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	if s := s.order; s != nil {
		if r, err = (&orderByRset{asc: s.asc, by: s.by, src: r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	if s := s.offset; s != nil {
		if r, err = (&offsetRset{s.expr, r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	if s := s.limit; s != nil {
		if r, err = (&limitRset{s.expr, r}).plan(ctx); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (s *selectStmt) exec(ctx *execCtx) (rs Recordset, err error) {
	r, err := s.plan(ctx)
	if err != nil {
		return nil, err
	}

	return recordset{ctx, r, nil}, nil
}

func (s *selectStmt) isUpdating() bool { return false }

type insertIntoStmt struct {
	colNames  []string
	lists     [][]expression
	sel       *selectStmt
	tableName string
}

func (s *insertIntoStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *insertIntoStmt) String() string {
	cn := ""
	if len(s.colNames) != 0 {
		cn = fmt.Sprintf(" (%s)", strings.Join(s.colNames, ", "))
	}
	switch {
	case s.sel != nil:
		return fmt.Sprintf("INSERT INTO %s%s %s;", s.tableName, cn, s.sel)
	default:
		a := make([]string, len(s.lists))
		for i, v := range s.lists {
			b := make([]string, len(v))
			for i, v := range v {
				b[i] = v.String()
			}
			a[i] = fmt.Sprintf("(%s)", strings.Join(b, ", "))
		}
		return fmt.Sprintf("INSERT INTO %s%s VALUES %s;", s.tableName, cn, strings.Join(a, ", "))
	}
}

func (s *insertIntoStmt) execSelect(t *table, cols []*col, ctx *execCtx) (_ Recordset, err error) {
	//TODO missing rs column number eq check
	r, err := s.sel.plan(ctx)
	if err != nil {
		return nil, err
	}

	h := t.head
	data0 := make([]interface{}, len(t.cols0)+2)
	cc := ctx.db.cc
	m := map[interface{}]interface{}{}
	if err = r.do(ctx, func(_ interface{}, data []interface{}) (more bool, err error) {
		for i, d := range data {
			data0[cols[i].index+2] = d
		}
		if err = typeCheck(data0[2:], cols); err != nil {
			return
		}

		if err = t.checkConstraintsAndDefaults(ctx, data0[2:], m); err != nil {
			return false, err
		}

		id, err := t.store.ID()
		if err != nil {
			return false, err
		}

		data0[0] = h
		data0[1] = id

		// Any overflow chunks are written here.
		if h, err = t.store.Create(data0...); err != nil {
			return false, err
		}

		for i, v := range t.indices {
			if v == nil {
				continue
			}

			// Any overflow chunks are shared with the BTree key
			if err = v.x.Create([]interface{}{data0[i+1]}, h); err != nil {
				return false, err
			}
		}
		for _, ix := range t.indices2 {
			vlist, err := ix.eval(ctx, t.cols, id, data0[2:])
			if err != nil {
				return false, err
			}

			if err := ix.x.Create(vlist, h); err != nil {
				return false, err
			}
		}

		cc.RowsAffected++
		ctx.db.root.lastInsertID = id
		return true, nil
	}); err != nil {
		return nil, err
	}

	t.head = h
	return nil, t.store.Update(t.hhead, h)
}

func (s *insertIntoStmt) exec(ctx *execCtx) (_ Recordset, err error) {
	root := ctx.db.root
	t, ok := root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("INSERT INTO %s: table does not exist", s.tableName)
	}

	var cols []*col
	switch len(s.colNames) {
	case 0:
		cols = t.cols
	default:
		for _, colName := range s.colNames {
			if col := findCol(t.cols, colName); col != nil {
				cols = append(cols, col)
				continue
			}

			return nil, fmt.Errorf("INSERT INTO %s: unknown column %s", s.tableName, colName)
		}
	}

	if s.sel != nil {
		return s.execSelect(t, cols, ctx)
	}

	for _, list := range s.lists {
		if g, e := len(list), len(cols); g != e {
			return nil, fmt.Errorf("INSERT INTO %s: expected %d value(s), have %d", s.tableName, e, g)
		}
	}

	cc := ctx.db.cc
	r := make([]interface{}, len(t.cols0))
	m := map[interface{}]interface{}{}
	for _, list := range s.lists {
		for i, expr := range list {
			val, err := expr.eval(ctx, m)
			if err != nil {
				return nil, err
			}

			r[cols[i].index] = val
		}
		if err = typeCheck(r, cols); err != nil {
			return nil, err
		}

		if err = t.checkConstraintsAndDefaults(ctx, r, m); err != nil {
			return nil, err
		}

		id, err := t.addRecord(ctx, r)
		if err != nil {
			return nil, err
		}

		cc.RowsAffected++
		root.lastInsertID = id
	}
	return nil, nil
}

func (s *insertIntoStmt) isUpdating() bool { return true }

type beginTransactionStmt struct{}

func (s beginTransactionStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (beginTransactionStmt) String() string { return "BEGIN TRANSACTION;" }
func (beginTransactionStmt) exec(*execCtx) (Recordset, error) {
	panic("internal error 059")
}
func (beginTransactionStmt) isUpdating() bool {
	panic("internal error 060")
}

type commitStmt struct{}

func (s commitStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (commitStmt) String() string { return "COMMIT;" }
func (commitStmt) exec(*execCtx) (Recordset, error) {
	panic("internal error 061")
}
func (commitStmt) isUpdating() bool {
	panic("internal error 062")
}

type rollbackStmt struct{}

func (s rollbackStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (rollbackStmt) String() string { return "ROLLBACK;" }
func (rollbackStmt) exec(*execCtx) (Recordset, error) {
	panic("internal error 063")
}
func (rollbackStmt) isUpdating() bool {
	panic("internal error 064")
}

type createIndexStmt struct {
	colName     string // alt. "id()" for simple index on id()
	ifNotExists bool
	indexName   string
	tableName   string
	unique      bool
	exprList    []expression
}

func (s *createIndexStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *createIndexStmt) isSimpleIndex() bool { return s.colName != "" }

func (s *createIndexStmt) String() string {
	u := ""
	if s.unique {
		u = "UNIQUE "
	}
	e := ""
	if s.ifNotExists {
		e = "IF NOT EXISTS "
	}
	expr := s.colName
	if !s.isSimpleIndex() {
		var a []string
		for _, v := range s.exprList {
			a = append(a, v.String())
		}
		expr = strings.Join(a, ", ")
	}
	return fmt.Sprintf("CREATE %sINDEX %s%s ON %s (%s);", u, e, s.indexName, s.tableName, expr)
}

func (s *createIndexStmt) exec(ctx *execCtx) (Recordset, error) {
	root := ctx.db.root
	if t, i := root.findIndexByName(s.indexName); i != nil {
		if s.ifNotExists {
			return nil, nil
		}

		return nil, fmt.Errorf("CREATE INDEX: table %s already has an index named %s", t.name, s.indexName)
	}

	if root.tables[s.indexName] != nil {
		return nil, fmt.Errorf("CREATE INDEX: index name collision with existing table: %s", s.indexName)
	}

	t, ok := root.tables[s.tableName]
	if !ok {
		return nil, fmt.Errorf("CREATE INDEX: table does not exist %s", s.tableName)
	}

	if findCol(t.cols, s.indexName) != nil {
		return nil, fmt.Errorf("CREATE INDEX: index name collision with existing column: %s", s.indexName)
	}

	var h int64
	var err error
	switch {
	case s.isSimpleIndex():
		colIndex := -1
		if s.colName != "id()" {
			c := findCol(t.cols, s.colName)
			if c == nil {
				return nil, fmt.Errorf("CREATE INDEX: column does not exist: %s", s.colName)
			}

			colIndex = c.index
		}

		if h, err = t.addIndex(s.unique, s.indexName, colIndex); err != nil {
			return nil, fmt.Errorf("CREATE INDEX: %v", err)
		}

		if err = t.updated(); err != nil {
			return nil, err
		}
	default:
		for _, e := range s.exprList {
			m := mentionedColumns(e)
			for colName := range m {
				c := findCol(t.cols, colName)
				if c == nil {
					return nil, fmt.Errorf("CREATE INDEX: column does not exist: %s", colName)
				}
			}
		}
		if h, err = t.addIndex2(ctx, s.unique, s.indexName, s.exprList); err != nil {
			return nil, fmt.Errorf("CREATE INDEX: %v", err)
		}
	}

	switch ctx.db.hasIndex2 {
	case 0:
		if err := ctx.db.createIndex2(); err != nil {
			return nil, err
		}

		if s.isSimpleIndex() {
			return nil, nil
		}
	case 1:
		return nil, nil
	case 2:
		if s.isSimpleIndex() {
			return nil, ctx.db.insertIndex2(s.tableName, s.indexName, []string{s.colName}, s.unique, true, h)
		}
	default:
		panic("internal error 011")
	}

	exprList := make([]string, 0, len(s.exprList))
	for _, e := range s.exprList {
		exprList = append(exprList, e.String())
	}
	return nil, ctx.db.insertIndex2(s.tableName, s.indexName, exprList, s.unique, false, h)
}

func (s *createIndexStmt) isUpdating() bool { return true }

type createTableStmt struct {
	ifNotExists bool
	tableName   string
	cols        []*col
}

func (s *createTableStmt) explain(ctx *execCtx, w strutil.Formatter) {
	w.Format("%s\n", s)
}

func (s *createTableStmt) String() string {
	a := make([]string, len(s.cols))
	for i, v := range s.cols {
		var c, d string
		if x := v.constraint; x != nil {
			switch e := x.expr; {
			case e != nil:
				c = " " + e.String()
			default:
				c = " NOT NULL"
			}
		}
		if x := v.dflt; x != nil {
			d = " DEFAULT " + x.String()
		}
		a[i] = fmt.Sprintf("%s %s%s%s", v.name, typeStr(v.typ), c, d)
	}
	e := ""
	if s.ifNotExists {
		e = "IF NOT EXISTS "
	}
	return fmt.Sprintf("CREATE TABLE %s%s (%s);", e, s.tableName, strings.Join(a, ", "))
}

func (s *createTableStmt) exec(ctx *execCtx) (_ Recordset, err error) {
	var cols []*col
	for _, v := range s.cols {
		cols = append(cols, v.clone())
	}
	root := ctx.db.root
	if _, ok := root.tables[s.tableName]; ok {
		if s.ifNotExists {
			return nil, nil
		}

		return nil, fmt.Errorf("CREATE TABLE: table exists %s", s.tableName)
	}

	if t, x := root.findIndexByName(s.tableName); x != nil {
		return nil, fmt.Errorf("CREATE TABLE: table %s has index %s", t.name, s.tableName)
	}

	m := map[string]bool{}
	mustCreateColumn2 := true
	for i, c := range cols {
		nm := c.name
		if m[nm] {
			return nil, fmt.Errorf("CREATE TABLE: duplicate column %s", nm)
		}

		m[nm] = true
		c.index = i
		if c.constraint != nil || c.dflt != nil {
			if mustCreateColumn2 {
				for _, stmt := range createColumn2.l {
					_, err := stmt.exec(&execCtx{db: ctx.db})
					if err != nil {
						return nil, err
					}
				}
			}

			mustCreateColumn2 = false
			notNull := c.constraint != nil && c.constraint.expr == nil
			var co, d string
			if c.constraint != nil && c.constraint.expr != nil {
				co = c.constraint.expr.String()
			}
			if e := c.dflt; e != nil {
				d = e.String()
			}
			if _, err := insertColumn2.l[0].exec(&execCtx{db: ctx.db, arg: []interface{}{s.tableName, c.name, notNull, co, d}}); err != nil {
				return nil, err
			}
		}
	}
	t, err := root.createTable(s.tableName, cols)
	if err != nil {
		return nil, err
	}

	return nil, t.constraintsAndDefaults(ctx)
}

func (s *createTableStmt) isUpdating() bool { return true }
