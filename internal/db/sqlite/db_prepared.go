package sqlite

import "github.com/jmoiron/sqlx"

type txPreparedStmts struct {
	*sqlx.Tx
	stmts map[string]*sqlx.Stmt
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
		return nil, err
	}
	p.stmts[query] = stmt
	return stmt, nil
}

func (p *txPreparedStmts) Commit() error {
	for _, s := range p.stmts {
		s.Close()
	}
	return p.Tx.Commit()
}

func (p *txPreparedStmts) Rollback() error {
	for _, s := range p.stmts {
		s.Close()
	}
	return p.Tx.Rollback()
}
