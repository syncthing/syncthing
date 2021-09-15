// Copyright (C) 2020 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/lib/pq"

	"github.com/syncthing/syncthing/lib/ur/contract"
)

func migrate(db *sql.DB) error {
	var count uint64
	log.Println("Checking old table row count, this might take a while...")
	if err := db.QueryRow(`SELECT COUNT(1) FROM Reports`).Scan(&count); err != nil || count == 0 {
		// err != nil most likely means table does not exist.
		return nil
	}
	log.Printf("Found %d records, will perform migration.", count)

	tx, err := db.Begin()
	if err != nil {
		log.Println("sql:", err)
		return err
	}
	defer tx.Rollback()

	// These must be lower case, because we don't quote them when creating, so postgres creates them lower case.
	// Yet pg.CopyIn quotes them, which makes them case sensitive.
	stmt, err := tx.Prepare(pq.CopyIn("reportsjson", "received", "report"))
	if err != nil {
		log.Println("sql:", err)
		return err
	}

	// Custom types used in the old struct.
	var rep contract.Report
	var rescanIntvs pq.Int64Array
	var fsWatcherDelay pq.Int64Array
	pullOrder := make(IntMap)
	fileSystemType := make(IntMap)
	themes := make(IntMap)
	transportStats := make(IntMap)

	rows, err := db.Query(`SELECT ` + strings.Join(rep.FieldNames(), ", ") + `, FolderFsWatcherDelays, RescanIntvs, FolderPullOrder, FolderFilesystemType, GUITheme, Transport FROM Reports`)
	if err != nil {
		log.Println("sql:", err)
		return err
	}
	defer rows.Close()

	var done uint64
	pct := count / 100

	for rows.Next() {
		err := rows.Scan(append(rep.FieldPointers(), &fsWatcherDelay, &rescanIntvs, &pullOrder, &fileSystemType, &themes, &transportStats)...)
		if err != nil {
			log.Println("sql scan:", err)
			return err
		}
		// Patch up parts that used to use custom types
		rep.RescanIntvs = make([]int, len(rescanIntvs))
		for i := range rescanIntvs {
			rep.RescanIntvs[i] = int(rescanIntvs[i])
		}
		rep.FolderUsesV3.FsWatcherDelays = make([]float32, len(fsWatcherDelay))
		for i := range fsWatcherDelay {
			rep.FolderUsesV3.FsWatcherDelays[i] = float32(fsWatcherDelay[i])
		}
		rep.FolderUsesV3.PullOrder = pullOrder
		rep.FolderUsesV3.FilesystemType = fileSystemType
		rep.GUIStats.Theme = themes
		rep.TransportStats = transportStats

		_, err = stmt.Exec(rep.Received, rep)
		if err != nil {
			log.Println("sql insert:", err)
			return err
		}
		done++
		if done%pct == 0 {
			log.Printf("Migration progress %d/%d (%d%%)", done, count, (100*done)/count)
		}
	}

	// Tell the driver bulk copy is finished
	_, err = stmt.Exec()
	if err != nil {
		log.Println("sql stmt exec:", err)
		return err
	}

	err = stmt.Close()
	if err != nil {
		log.Println("sql stmt close:", err)
		return err
	}

	_, err = tx.Exec("DROP TABLE Reports")
	if err != nil {
		log.Println("sql drop:", err)
		return err
	}

	err = tx.Commit()
	if err != nil {
		log.Println("sql commit:", err)
		return err
	}
	return nil
}

type IntMap map[string]int

func (p IntMap) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *IntMap) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("Type assertion .([]byte) failed.")
	}

	var i map[string]int
	err := json.Unmarshal(source, &i)
	if err != nil {
		return err
	}

	*p = i
	return nil
}
