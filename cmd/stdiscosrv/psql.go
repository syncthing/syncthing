// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func init() {
	register("postgres", postgresSetup, postgresCompile)
}

func postgresSetup(db *sql.DB) error {
	var err error

	db.SetMaxIdleConns(4)
	db.SetMaxOpenConns(8)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Devices (
		DeviceID CHAR(63) NOT NULL PRIMARY KEY,
		Seen TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return err
	}

	var tmp string
	row := db.QueryRow(`SELECT 'DevicesDeviceIDIndex'::regclass`)
	if err = row.Scan(&tmp); err != nil {
		_, err = db.Exec(`CREATE INDEX DevicesDeviceIDIndex ON Devices (DeviceID)`)
	}
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'DevicesSeenIndex'::regclass`)
	if err = row.Scan(&tmp); err != nil {
		_, err = db.Exec(`CREATE INDEX DevicesSeenIndex ON Devices (Seen)`)
	}
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Addresses (
		DeviceID CHAR(63) NOT NULL,
		Seen TIMESTAMP NOT NULL,
		Address VARCHAR(2048) NOT NULL
	)`)
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'AddressesDeviceIDSeenIndex'::regclass`)
	if err = row.Scan(&tmp); err != nil {
		_, err = db.Exec(`CREATE INDEX AddressesDeviceIDSeenIndex ON Addresses (DeviceID, Seen)`)
	}
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'AddressesDeviceIDAddressIndex'::regclass`)
	if err = row.Scan(&tmp); err != nil {
		_, err = db.Exec(`CREATE INDEX AddressesDeviceIDAddressIndex ON Addresses (DeviceID, Address)`)
	}
	if err != nil {
		return err
	}

	return nil
}

func postgresCompile(db *sql.DB) (map[string]*sql.Stmt, error) {
	stmts := map[string]string{
		"cleanAddress":  "DELETE FROM Addresses WHERE Seen < now() - '2 hour'::INTERVAL",
		"cleanDevice":   fmt.Sprintf("DELETE FROM Devices WHERE Seen < now() - '%d hour'::INTERVAL", maxDeviceAge/3600),
		"countAddress":  "SELECT count(*) FROM Addresses",
		"countDevice":   "SELECT count(*) FROM Devices",
		"insertAddress": "INSERT INTO Addresses (DeviceID, Seen, Address) VALUES ($1, now(), $2)",
		"insertDevice":  "INSERT INTO Devices (DeviceID, Seen) VALUES ($1, now())",
		"selectAddress": "SELECT Address FROM Addresses WHERE DeviceID=$1 AND Seen > now() - '1 hour'::INTERVAL ORDER BY random() LIMIT 16",
		"selectDevice":  "SELECT Seen FROM Devices WHERE DeviceID=$1",
		"updateAddress": "UPDATE Addresses SET Seen=now() WHERE DeviceID=$1 AND Address=$2",
		"updateDevice":  "UPDATE Devices SET Seen=now() WHERE DeviceID=$1",
	}

	res := make(map[string]*sql.Stmt, len(stmts))
	for key, stmt := range stmts {
		prep, err := db.Prepare(stmt)
		if err != nil {
			return nil, err
		}
		res[key] = prep
	}
	return res, nil
}
