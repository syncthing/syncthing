// +build go1.8

package ql

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

const prefix = "$"

func (c *driverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	query, vals, err := replaceNamed(query, args)
	if err != nil {
		return nil, err
	}

	return c.Exec(query, vals)
}

func replaceNamed(query string, args []driver.NamedValue) (string, []driver.Value, error) {
	toks, err := tokenize(query)
	if err != nil {
		return "", nil, err
	}

	a := make([]driver.Value, len(args))
	m := map[string]int{}
	for _, v := range args {
		m[v.Name] = v.Ordinal
		a[v.Ordinal-1] = v.Value
	}
	for i, v := range toks {
		if len(v) > 1 && strings.HasPrefix(v, prefix) {
			if v[1] >= '1' && v[1] <= '9' {
				continue
			}

			nm := v[1:]
			k, ok := m[nm]
			if !ok {
				return query, nil, fmt.Errorf("unknown named parameter %s", nm)
			}

			toks[i] = fmt.Sprintf("$%d", k)
		}
	}
	return strings.Join(toks, " "), a, nil
}

func (c *driverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	query, vals, err := replaceNamed(query, args)
	if err != nil {
		return nil, err
	}

	return c.Query(query, vals)
}

func (c *driverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	query, err := filterNamedArgs(query)
	if err != nil {
		return nil, err
	}

	return c.Prepare(query)
}

func filterNamedArgs(query string) (string, error) {
	toks, err := tokenize(query)
	if err != nil {
		return "", err
	}

	n := 0
	for _, v := range toks {
		if len(v) > 1 && strings.HasPrefix(v, prefix) && v[1] >= '1' && v[1] <= '9' {
			m, err := strconv.ParseUint(v[1:], 10, 31)
			if err != nil {
				return "", err
			}

			if int(m) > n {
				n = int(m)
			}
		}
	}
	for i, v := range toks {
		if len(v) > 1 && strings.HasPrefix(v, prefix) {
			if v[1] >= '1' && v[1] <= '9' {
				continue
			}

			n++
			toks[i] = fmt.Sprintf("$%d", n)
		}
	}
	return strings.Join(toks, " "), nil
}

func (s *driverStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	a := make([]driver.Value, len(args))
	for k, v := range args {
		a[k] = v.Value
	}
	return s.Exec(a)
}

func (s *driverStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	a := make([]driver.Value, len(args))
	for k, v := range args {
		a[k] = v.Value
	}
	return s.Query(a)
}

func tokenize(s string) (r []string, _ error) {
	lx, err := newLexer(s)
	if err != nil {
		return nil, err
	}

	var lval yySymType
	for lx.Lex(&lval) != 0 {
		s := string(lx.TokenBytes(nil))
		if s != "" {
			switch s[len(s)-1] {
			case '"':
				s = "\"" + s
			case '`':
				s = "`" + s
			}
		}
		r = append(r, s)
	}
	return r, nil
}
