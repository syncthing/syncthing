package pq

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"
)

func TestCopyInStmt(t *testing.T) {
	var stmt string
	stmt = CopyIn("table name")
	if stmt != `COPY "table name" () FROM STDIN` {
		t.Fatal(stmt)
	}

	stmt = CopyIn("table name", "column 1", "column 2")
	if stmt != `COPY "table name" ("column 1", "column 2") FROM STDIN` {
		t.Fatal(stmt)
	}

	stmt = CopyIn(`table " name """`, `co"lumn""`)
	if stmt != `COPY "table "" name """"""" ("co""lumn""""") FROM STDIN` {
		t.Fatal(stmt)
	}
}

func TestCopyInSchemaStmt(t *testing.T) {
	var stmt string
	stmt = CopyInSchema("schema name", "table name")
	if stmt != `COPY "schema name"."table name" () FROM STDIN` {
		t.Fatal(stmt)
	}

	stmt = CopyInSchema("schema name", "table name", "column 1", "column 2")
	if stmt != `COPY "schema name"."table name" ("column 1", "column 2") FROM STDIN` {
		t.Fatal(stmt)
	}

	stmt = CopyInSchema(`schema " name """`, `table " name """`, `co"lumn""`)
	if stmt != `COPY "schema "" name """"""".`+
		`"table "" name """"""" ("co""lumn""""") FROM STDIN` {
		t.Fatal(stmt)
	}
}

func TestCopyInMultipleValues(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (a int, b varchar)")
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "a", "b"))
	if err != nil {
		t.Fatal(err)
	}

	longString := strings.Repeat("#", 500)

	for i := 0; i < 500; i++ {
		_, err = stmt.Exec(int64(i), longString)
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		t.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	var num int
	err = txn.QueryRow("SELECT COUNT(*) FROM temp").Scan(&num)
	if err != nil {
		t.Fatal(err)
	}

	if num != 500 {
		t.Fatalf("expected 500 items, not %d", num)
	}
}

func TestCopyInRaiseStmtTrigger(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	if getServerVersion(t, db) < 90000 {
		var exists int
		err := db.QueryRow("SELECT 1 FROM pg_language WHERE lanname = 'plpgsql'").Scan(&exists)
		if err == sql.ErrNoRows {
			t.Skip("language PL/PgSQL does not exist; skipping TestCopyInRaiseStmtTrigger")
		} else if err != nil {
			t.Fatal(err)
		}
	}

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (a int, b varchar)")
	if err != nil {
		t.Fatal(err)
	}

	_, err = txn.Exec(`
			CREATE OR REPLACE FUNCTION pg_temp.temptest()
			RETURNS trigger AS 
			$BODY$ begin
				raise notice 'Hello world';
				return new;
			end $BODY$
			LANGUAGE plpgsql`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = txn.Exec(`
			CREATE TRIGGER temptest_trigger
			BEFORE INSERT
			ON temp 
			FOR EACH ROW
			EXECUTE PROCEDURE pg_temp.temptest()`)
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "a", "b"))
	if err != nil {
		t.Fatal(err)
	}

	longString := strings.Repeat("#", 500)

	_, err = stmt.Exec(int64(1), longString)
	if err != nil {
		t.Fatal(err)
	}

	_, err = stmt.Exec()
	if err != nil {
		t.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	var num int
	err = txn.QueryRow("SELECT COUNT(*) FROM temp").Scan(&num)
	if err != nil {
		t.Fatal(err)
	}

	if num != 1 {
		t.Fatalf("expected 1 items, not %d", num)
	}
}

func TestCopyInTypes(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (num INTEGER, text VARCHAR, blob BYTEA, nothing VARCHAR)")
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "num", "text", "blob", "nothing"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = stmt.Exec(int64(1234567890), "Héllö\n ☃!\r\t\\", []byte{0, 255, 9, 10, 13}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = stmt.Exec()
	if err != nil {
		t.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}

	var num int
	var text string
	var blob []byte
	var nothing sql.NullString

	err = txn.QueryRow("SELECT * FROM temp").Scan(&num, &text, &blob, &nothing)
	if err != nil {
		t.Fatal(err)
	}

	if num != 1234567890 {
		t.Fatal("unexpected result", num)
	}
	if text != "Héllö\n ☃!\r\t\\" {
		t.Fatal("unexpected result", text)
	}
	if bytes.Compare(blob, []byte{0, 255, 9, 10, 13}) != 0 {
		t.Fatal("unexpected result", blob)
	}
	if nothing.Valid {
		t.Fatal("unexpected result", nothing.String)
	}
}

func TestCopyInWrongType(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (num INTEGER)")
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "num"))
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()

	_, err = stmt.Exec("Héllö\n ☃!\r\t\\")
	if err != nil {
		t.Fatal(err)
	}

	_, err = stmt.Exec()
	if err == nil {
		t.Fatal("expected error")
	}
	if pge := err.(*Error); pge.Code.Name() != "invalid_text_representation" {
		t.Fatalf("expected 'invalid input syntax for integer' error, got %s (%+v)", pge.Code.Name(), pge)
	}
}

func TestCopyOutsideOfTxnError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Prepare(CopyIn("temp", "num"))
	if err == nil {
		t.Fatal("COPY outside of transaction did not return an error")
	}
	if err != errCopyNotSupportedOutsideTxn {
		t.Fatalf("expected %s, got %s", err, err.Error())
	}
}

func TestCopyInBinaryError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (num INTEGER)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = txn.Prepare("COPY temp (num) FROM STDIN WITH binary")
	if err != errBinaryCopyNotSupported {
		t.Fatalf("expected %s, got %+v", errBinaryCopyNotSupported, err)
	}
	// check that the protocol is in a valid state
	err = txn.Rollback()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCopyFromError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (num INTEGER)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = txn.Prepare("COPY temp (num) TO STDOUT")
	if err != errCopyToNotSupported {
		t.Fatalf("expected %s, got %+v", errCopyToNotSupported, err)
	}
	// check that the protocol is in a valid state
	err = txn.Rollback()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCopySyntaxError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Prepare("COPY ")
	if err == nil {
		t.Fatal("expected error")
	}
	if pge := err.(*Error); pge.Code.Name() != "syntax_error" {
		t.Fatalf("expected syntax error, got %s (%+v)", pge.Code.Name(), pge)
	}
	// check that the protocol is in a valid state
	err = txn.Rollback()
	if err != nil {
		t.Fatal(err)
	}
}

// Tests for connection errors in copyin.resploop()
func TestCopyRespLoopConnectionError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	var pid int
	err = txn.QueryRow("SELECT pg_backend_pid()").Scan(&pid)
	if err != nil {
		t.Fatal(err)
	}

	_, err = txn.Exec("CREATE TEMP TABLE temp (a int)")
	if err != nil {
		t.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "a"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("SELECT pg_terminate_backend($1)", pid)
	if err != nil {
		t.Fatal(err)
	}

	if getServerVersion(t, db) < 90500 {
		// We have to try and send something over, since postgres before
		// version 9.5 won't process SIGTERMs while it's waiting for
		// CopyData/CopyEnd messages; see tcop/postgres.c.
		_, err = stmt.Exec(1)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = stmt.Exec()
	if err == nil {
		t.Fatalf("expected error")
	}
	pge, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *pq.Error, got %+#v", err)
	} else if pge.Code.Name() != "admin_shutdown" {
		t.Fatalf("expected admin_shutdown, got %s", pge.Code.Name())
	}

	err = stmt.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkCopyIn(b *testing.B) {
	db := openTestConn(b)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMP TABLE temp (a int, b varchar)")
	if err != nil {
		b.Fatal(err)
	}

	stmt, err := txn.Prepare(CopyIn("temp", "a", "b"))
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		_, err = stmt.Exec(int64(i), "hello world!")
		if err != nil {
			b.Fatal(err)
		}
	}

	_, err = stmt.Exec()
	if err != nil {
		b.Fatal(err)
	}

	err = stmt.Close()
	if err != nil {
		b.Fatal(err)
	}

	var num int
	err = txn.QueryRow("SELECT COUNT(*) FROM temp").Scan(&num)
	if err != nil {
		b.Fatal(err)
	}

	if num != b.N {
		b.Fatalf("expected %d items, not %d", b.N, num)
	}
}
