// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/cznic/ql"
)

func init() {
	ql.RegisterDriver()
	register("ql", qlSetup, qlCompile)
}

func qlSetup(db *sql.DB) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return
	}

	defer func() {
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS Devices (
		DeviceID STRING NOT NULL,
		Seen TIME NOT NULL
	)`)
	if err != nil {
		return
	}

	if _, err = tx.Exec(`CREATE INDEX IF NOT EXISTS DevicesDeviceIDIndex ON Devices (DeviceID)`); err != nil {
		return
	}

	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS Addresses (
		DeviceID STRING NOT NULL,
		Seen TIME NOT NULL,
		Address STRING NOT NULL,
	)`)
	if err != nil {
		return
	}

	_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS AddressesDeviceIDAddressIndex ON Addresses (DeviceID, Address)`)
	return
}

func qlCompile(db *sql.DB) (map[string]*sql.Stmt, error) {
	stmts := map[string]string{
		"cleanAddress":  `DELETE FROM Addresses WHERE Seen < now() - duration("2h")`,
		"cleanDevice":   fmt.Sprintf(`DELETE FROM Devices WHERE Seen < now() - duration("%dh")`, maxDeviceAge/3600),
		"countAddress":  "SELECT count(*) FROM Addresses",
		"countDevice":   "SELECT count(*) FROM Devices",
		"insertAddress": "INSERT INTO Addresses (DeviceID, Seen, Address) VALUES ($1, now(), $2)",
		"insertDevice":  "INSERT INTO Devices (DeviceID, Seen) VALUES ($1, now())",
		"selectAddress": `SELECT Address from Addresses WHERE DeviceID==$1 AND Seen > now() - duration("1h") LIMIT 16`,
		"selectDevice":  "SELECT Seen FROM Devices WHERE DeviceID==$1",
		"updateAddress": "UPDATE Addresses Seen=now() WHERE DeviceID==$1 AND Address==$2",
		"updateDevice":  "UPDATE Devices Seen=now() WHERE DeviceID==$1",
	}

	res := make(map[string]*sql.Stmt, len(stmts))
	for key, stmt := range stmts {
		prep, err := db.Prepare(stmt)
		if err != nil {
			log.Println("Failed to compile", stmt)
			return nil, err
		}
		res[key] = prep
	}
	return res, nil
}
