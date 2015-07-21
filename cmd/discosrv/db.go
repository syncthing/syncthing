// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"fmt"
)

type setupFunc func(db *sql.DB) error
type compileFunc func(db *sql.DB) (map[string]*sql.Stmt, error)

var (
	setupFuncs   = make(map[string]setupFunc)
	compileFuncs = make(map[string]compileFunc)
)

func register(name string, setup setupFunc, compile compileFunc) {
	setupFuncs[name] = setup
	compileFuncs[name] = compile
}

func setup(backend string, db *sql.DB) (map[string]*sql.Stmt, error) {
	setup, ok := setupFuncs[backend]
	if !ok {
		return nil, fmt.Errorf("Unsupported backend")
	}
	if err := setup(db); err != nil {
		return nil, err
	}
	return compileFuncs[backend](db)
}
