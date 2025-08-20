// Copyright (C) 2025 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package sqlite

import "github.com/jmoiron/sqlx"

type txPreparedStmts struct {
	*sqlx.Tx
	stmts map[string]*sqlx.Stmt
	namedStmts map[string]*sqlx.NamedStmt
}

func (p *txPreparedStmts) Preparex(query string) (*sqlx.Stmt, error) {
	if p.stmts == nil {
		p.stmts = make(map[string]*sqlx.Stmt)
	}
	stmt, ok := p.stmts[query]
	if ok {
		return stmt, nil
	}
	stmt, err := p.Tx.Preparex(query)
	if err != nil {
		return nil, wrap(err)
	}
	p.stmts[query] = stmt
	return stmt, nil
}

func (p *txPreparedStmts) PrepareNamed(query string) (*sqlx.NamedStmt, error) {
	if p.namedStmts == nil {
		p.namedStmts = make(map[string]*sqlx.NamedStmt)
	}
	stmt, ok := p.namedStmts[query]
	if ok {
		return stmt, nil
	}
	stmt, err := p.Tx.PrepareNamed(query)
	if err != nil {
		return nil, wrap(err)
	}
	p.namedStmts[query] = stmt
	return stmt, nil
}

func (p *txPreparedStmts) Commit() error {
	p.closeStmts()
	return p.Tx.Commit()
}

func (p *txPreparedStmts) Rollback() error {
	p.closeStmts()
	return p.Tx.Rollback()
}

func (p *txPreparedStmts) closeStmts() {
	for _, s := range p.stmts {
		s.Close()
	}
	for _, s := range p.namedStmts {
		s.Close()
	}
}
