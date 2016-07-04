// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"log"
	"time"
)

type cleansrv struct {
	intv time.Duration
	db   *sql.DB
	prep map[string]*sql.Stmt
}

func (s *cleansrv) Serve() {
	for {
		time.Sleep(next(s.intv))

		err := s.cleanOldEntries()
		if err != nil {
			log.Println("Clean:", err)
		}
	}
}

func (s *cleansrv) Stop() {
	panic("stop unimplemented")
}

func (s *cleansrv) cleanOldEntries() (err error) {
	var tx *sql.Tx
	tx, err = s.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	res, err := tx.Stmt(s.prep["cleanAddress"]).Exec()
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		log.Printf("Clean: %d old addresses", rows)
	}

	res, err = tx.Stmt(s.prep["cleanDevice"]).Exec()
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		log.Printf("Clean: %d old devices", rows)
	}

	var devs, addrs int
	row := tx.Stmt(s.prep["countDevice"]).QueryRow()
	if err = row.Scan(&devs); err != nil {
		return err
	}
	row = tx.Stmt(s.prep["countAddress"]).QueryRow()
	if err = row.Scan(&addrs); err != nil {
		return err
	}

	log.Printf("Database: %d devices, %d addresses", devs, addrs)
	return nil
}
