// Copyright (C) 2014-2015 Jakob Borg and Contributors (see the CONTRIBUTORS file).

package main

import (
	"database/sql"

	_ "github.com/lib/pq"
)

func init() {
	register("postgres", postgresSetup, postgresCompile)
}

func postgresSetup(db *sql.DB) error {
	var err error

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Devices (
		DeviceID CHAR(63) NOT NULL PRIMARY KEY,
		Seen TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return err
	}

	row := db.QueryRow(`SELECT 'DevicesDeviceIDIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX DevicesDeviceIDIndex ON Devices (DeviceID)`)
	}
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'DevicesSeenIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX DevicesSeenIndex ON Devices (Seen)`)
	}
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Addresses (
		DeviceID CHAR(63) NOT NULL,
		Seen TIMESTAMP NOT NULL,
		Address VARCHAR(256) NOT NULL
	)`)
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'AddressesDeviceIDSeenIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX AddressesDeviceIDSeenIndex ON Addresses (DeviceID, Seen)`)
	}
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'AddressesDeviceIDAddressIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX AddressesDeviceIDAddressIndex ON Addresses (DeviceID, Address)`)
	}
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS Relays (
		DeviceID CHAR(63) NOT NULL,
		Seen TIMESTAMP NOT NULL,
		Address VARCHAR(256) NOT NULL,
		Latency INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'RelaysDeviceIDSeenIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX RelaysDeviceIDSeenIndex ON Relays (DeviceID, Seen)`)
	}
	if err != nil {
		return err
	}

	row = db.QueryRow(`SELECT 'RelaysDeviceIDAddressIndex'::regclass`)
	if err := row.Scan(nil); err != nil {
		_, err = db.Exec(`CREATE INDEX RelaysDeviceIDAddressIndex ON Relays (DeviceID, Address)`)
	}
	if err != nil {
		return err
	}

	return nil
}

func postgresCompile(db *sql.DB) (map[string]*sql.Stmt, error) {
	stmts := map[string]string{
		"cleanAddress":  "DELETE FROM Addresses WHERE Seen < now() - '2 hour'::INTERVAL",
		"cleanRelay":    "DELETE FROM Relays WHERE Seen < now() - '2 hour'::INTERVAL",
		"cleanDevice":   "DELETE FROM Devices WHERE Seen < now() - '24 hour'::INTERVAL",
		"countAddress":  "SELECT count(*) FROM Addresses",
		"countDevice":   "SELECT count(*) FROM Devices",
		"countRelay":    "SELECT count(*) FROM Relays",
		"insertAddress": "INSERT INTO Addresses (DeviceID, Seen, Address) VALUES ($1, now(), $2)",
		"insertRelay":   "INSERT INTO Relays (DeviceID, Seen, Address, Latency) VALUES ($1, now(), $2, $3)",
		"insertDevice":  "INSERT INTO Devices (DeviceID, Seen) VALUES ($1, now())",
		"selectAddress": "SELECT Address FROM Addresses WHERE DeviceID=$1 AND Seen > now() - '1 hour'::INTERVAL ORDER BY random() LIMIT 16",
		"selectRelay":   "SELECT Address, Latency FROM Relays WHERE DeviceID=$1 AND Seen > now() - '1 hour'::INTERVAL ORDER BY random() LIMIT 16",
		"updateAddress": "UPDATE Addresses SET Seen=now() WHERE DeviceID=$1 AND Address=$2",
		"updateDevice":  "UPDATE Devices SET Seen=now() WHERE DeviceID=$1",
		"deleteRelay":   "DELETE FROM Relays WHERE DeviceID=$1",
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
