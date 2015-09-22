package pq

// This file contains SSL tests

import (
	_ "crypto/sha256"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func maybeSkipSSLTests(t *testing.T) {
	// Require some special variables for testing certificates
	if os.Getenv("PQSSLCERTTEST_PATH") == "" {
		t.Skip("PQSSLCERTTEST_PATH not set, skipping SSL tests")
	}

	value := os.Getenv("PQGOSSLTESTS")
	if value == "" || value == "0" {
		t.Skip("PQGOSSLTESTS not enabled, skipping SSL tests")
	} else if value != "1" {
		t.Fatalf("unexpected value %q for PQGOSSLTESTS", value)
	}
}

func openSSLConn(t *testing.T, conninfo string) (*sql.DB, error) {
	db, err := openTestConnConninfo(conninfo)
	if err != nil {
		// should never fail
		t.Fatal(err)
	}
	// Do something with the connection to see whether it's working or not.
	tx, err := db.Begin()
	if err == nil {
		return db, tx.Rollback()
	}
	_ = db.Close()
	return nil, err
}

func checkSSLSetup(t *testing.T, conninfo string) {
	db, err := openSSLConn(t, conninfo)
	if err == nil {
		db.Close()
		t.Fatalf("expected error with conninfo=%q", conninfo)
	}
}

// Connect over SSL and run a simple query to test the basics
func TestSSLConnection(t *testing.T) {
	maybeSkipSSLTests(t)
	// Environment sanity check: should fail without SSL
	checkSSLSetup(t, "sslmode=disable user=pqgossltest")

	db, err := openSSLConn(t, "sslmode=require user=pqgossltest")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

// Test sslmode=verify-full
func TestSSLVerifyFull(t *testing.T) {
	maybeSkipSSLTests(t)
	// Environment sanity check: should fail without SSL
	checkSSLSetup(t, "sslmode=disable user=pqgossltest")

	// Not OK according to the system CA
	_, err := openSSLConn(t, "host=postgres sslmode=verify-full user=pqgossltest")
	if err == nil {
		t.Fatal("expected error")
	}
	_, ok := err.(x509.UnknownAuthorityError)
	if !ok {
		t.Fatalf("expected x509.UnknownAuthorityError, got %#+v", err)
	}

	rootCertPath := filepath.Join(os.Getenv("PQSSLCERTTEST_PATH"), "root.crt")
	rootCert := "sslrootcert=" + rootCertPath + " "
	// No match on Common Name
	_, err = openSSLConn(t, rootCert+"host=127.0.0.1 sslmode=verify-full user=pqgossltest")
	if err == nil {
		t.Fatal("expected error")
	}
	_, ok = err.(x509.HostnameError)
	if !ok {
		t.Fatalf("expected x509.HostnameError, got %#+v", err)
	}
	// OK
	_, err = openSSLConn(t, rootCert+"host=postgres sslmode=verify-full user=pqgossltest")
	if err != nil {
		t.Fatal(err)
	}
}

// Test sslmode=verify-ca
func TestSSLVerifyCA(t *testing.T) {
	maybeSkipSSLTests(t)
	// Environment sanity check: should fail without SSL
	checkSSLSetup(t, "sslmode=disable user=pqgossltest")

	// Not OK according to the system CA
	_, err := openSSLConn(t, "host=postgres sslmode=verify-ca user=pqgossltest")
	if err == nil {
		t.Fatal("expected error")
	}
	_, ok := err.(x509.UnknownAuthorityError)
	if !ok {
		t.Fatalf("expected x509.UnknownAuthorityError, got %#+v", err)
	}

	rootCertPath := filepath.Join(os.Getenv("PQSSLCERTTEST_PATH"), "root.crt")
	rootCert := "sslrootcert=" + rootCertPath + " "
	// No match on Common Name, but that's OK
	_, err = openSSLConn(t, rootCert+"host=127.0.0.1 sslmode=verify-ca user=pqgossltest")
	if err != nil {
		t.Fatal(err)
	}
	// Everything OK
	_, err = openSSLConn(t, rootCert+"host=postgres sslmode=verify-ca user=pqgossltest")
	if err != nil {
		t.Fatal(err)
	}
}

func getCertConninfo(t *testing.T, source string) string {
	var sslkey string
	var sslcert string

	certpath := os.Getenv("PQSSLCERTTEST_PATH")

	switch source {
	case "missingkey":
		sslkey = "/tmp/filedoesnotexist"
		sslcert = filepath.Join(certpath, "postgresql.crt")
	case "missingcert":
		sslkey = filepath.Join(certpath, "postgresql.key")
		sslcert = "/tmp/filedoesnotexist"
	case "certtwice":
		sslkey = filepath.Join(certpath, "postgresql.crt")
		sslcert = filepath.Join(certpath, "postgresql.crt")
	case "valid":
		sslkey = filepath.Join(certpath, "postgresql.key")
		sslcert = filepath.Join(certpath, "postgresql.crt")
	default:
		t.Fatalf("invalid source %q", source)
	}
	return fmt.Sprintf("sslmode=require user=pqgosslcert sslkey=%s sslcert=%s", sslkey, sslcert)
}

// Authenticate over SSL using client certificates
func TestSSLClientCertificates(t *testing.T) {
	maybeSkipSSLTests(t)
	// Environment sanity check: should fail without SSL
	checkSSLSetup(t, "sslmode=disable user=pqgossltest")

	// Should also fail without a valid certificate
	db, err := openSSLConn(t, "sslmode=require user=pqgosslcert")
	if err == nil {
		db.Close()
		t.Fatal("expected error")
	}
	pge, ok := err.(*Error)
	if !ok {
		t.Fatal("expected pq.Error")
	}
	if pge.Code.Name() != "invalid_authorization_specification" {
		t.Fatalf("unexpected error code %q", pge.Code.Name())
	}

	// Should work
	db, err = openSSLConn(t, getCertConninfo(t, "valid"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

// Test errors with ssl certificates
func TestSSLClientCertificatesMissingFiles(t *testing.T) {
	maybeSkipSSLTests(t)
	// Environment sanity check: should fail without SSL
	checkSSLSetup(t, "sslmode=disable user=pqgossltest")

	// Key missing, should fail
	_, err := openSSLConn(t, getCertConninfo(t, "missingkey"))
	if err == nil {
		t.Fatal("expected error")
	}
	// should be a PathError
	_, ok := err.(*os.PathError)
	if !ok {
		t.Fatalf("expected PathError, got %#+v", err)
	}

	// Cert missing, should fail
	_, err = openSSLConn(t, getCertConninfo(t, "missingcert"))
	if err == nil {
		t.Fatal("expected error")
	}
	// should be a PathError
	_, ok = err.(*os.PathError)
	if !ok {
		t.Fatalf("expected PathError, got %#+v", err)
	}

	// Key has wrong permissions, should fail
	_, err = openSSLConn(t, getCertConninfo(t, "certtwice"))
	if err == nil {
		t.Fatal("expected error")
	}
	if err != ErrSSLKeyHasWorldPermissions {
		t.Fatalf("expected ErrSSLKeyHasWorldPermissions, got %#+v", err)
	}
}
