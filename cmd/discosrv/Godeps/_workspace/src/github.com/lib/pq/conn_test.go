package pq

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

type Fatalistic interface {
	Fatal(args ...interface{})
}

func openTestConnConninfo(conninfo string) (*sql.DB, error) {
	defaultTo := func(envvar string, value string) {
		if os.Getenv(envvar) == "" {
			os.Setenv(envvar, value)
		}
	}
	defaultTo("PGDATABASE", "pqgotest")
	defaultTo("PGSSLMODE", "disable")
	defaultTo("PGCONNECT_TIMEOUT", "20")
	return sql.Open("postgres", conninfo)
}

func openTestConn(t Fatalistic) *sql.DB {
	conn, err := openTestConnConninfo("")
	if err != nil {
		t.Fatal(err)
	}

	return conn
}

func getServerVersion(t *testing.T, db *sql.DB) int {
	var version int
	err := db.QueryRow("SHOW server_version_num").Scan(&version)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func TestReconnect(t *testing.T) {
	db1 := openTestConn(t)
	defer db1.Close()
	tx, err := db1.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var pid1 int
	err = tx.QueryRow("SELECT pg_backend_pid()").Scan(&pid1)
	if err != nil {
		t.Fatal(err)
	}
	db2 := openTestConn(t)
	defer db2.Close()
	_, err = db2.Exec("SELECT pg_terminate_backend($1)", pid1)
	if err != nil {
		t.Fatal(err)
	}
	// The rollback will probably "fail" because we just killed
	// its connection above
	_ = tx.Rollback()

	const expected int = 42
	var result int
	err = db1.QueryRow(fmt.Sprintf("SELECT %d", expected)).Scan(&result)
	if err != nil {
		t.Fatal(err)
	}
	if result != expected {
		t.Errorf("got %v; expected %v", result, expected)
	}
}

func TestCommitInFailedTransaction(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := txn.Query("SELECT error")
	if err == nil {
		rows.Close()
		t.Fatal("expected failure")
	}
	err = txn.Commit()
	if err != ErrInFailedTransaction {
		t.Fatalf("expected ErrInFailedTransaction; got %#v", err)
	}
}

func TestOpenURL(t *testing.T) {
	testURL := func(url string) {
		db, err := openTestConnConninfo(url)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		// database/sql might not call our Open at all unless we do something with
		// the connection
		txn, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		txn.Rollback()
	}
	testURL("postgres://")
	testURL("postgresql://")
}

func TestExec(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Exec("CREATE TEMP TABLE temp (a int)")
	if err != nil {
		t.Fatal(err)
	}

	r, err := db.Exec("INSERT INTO temp VALUES (1)")
	if err != nil {
		t.Fatal(err)
	}

	if n, _ := r.RowsAffected(); n != 1 {
		t.Fatalf("expected 1 row affected, not %d", n)
	}

	r, err = db.Exec("INSERT INTO temp VALUES ($1), ($2), ($3)", 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}

	if n, _ := r.RowsAffected(); n != 3 {
		t.Fatalf("expected 3 rows affected, not %d", n)
	}

	// SELECT doesn't send the number of returned rows in the command tag
	// before 9.0
	if getServerVersion(t, db) >= 90000 {
		r, err = db.Exec("SELECT g FROM generate_series(1, 2) g")
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := r.RowsAffected(); n != 2 {
			t.Fatalf("expected 2 rows affected, not %d", n)
		}

		r, err = db.Exec("SELECT g FROM generate_series(1, $1) g", 3)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := r.RowsAffected(); n != 3 {
			t.Fatalf("expected 3 rows affected, not %d", n)
		}
	}
}

func TestStatment(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	st, err := db.Prepare("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}

	st1, err := db.Prepare("SELECT 2")
	if err != nil {
		t.Fatal(err)
	}

	r, err := st.Query()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if !r.Next() {
		t.Fatal("expected row")
	}

	var i int
	err = r.Scan(&i)
	if err != nil {
		t.Fatal(err)
	}

	if i != 1 {
		t.Fatalf("expected 1, got %d", i)
	}

	// st1

	r1, err := st1.Query()
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()

	if !r1.Next() {
		if r.Err() != nil {
			t.Fatal(r1.Err())
		}
		t.Fatal("expected row")
	}

	err = r1.Scan(&i)
	if err != nil {
		t.Fatal(err)
	}

	if i != 2 {
		t.Fatalf("expected 2, got %d", i)
	}
}

func TestRowsCloseBeforeDone(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	r, err := db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}

	err = r.Close()
	if err != nil {
		t.Fatal(err)
	}

	if r.Next() {
		t.Fatal("unexpected row")
	}

	if r.Err() != nil {
		t.Fatal(r.Err())
	}
}

func TestParameterCountMismatch(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	var notused int
	err := db.QueryRow("SELECT false", 1).Scan(&notused)
	if err == nil {
		t.Fatal("expected err")
	}
	// make sure we clean up correctly
	err = db.QueryRow("SELECT 1").Scan(&notused)
	if err != nil {
		t.Fatal(err)
	}

	err = db.QueryRow("SELECT $1").Scan(&notused)
	if err == nil {
		t.Fatal("expected err")
	}
	// make sure we clean up correctly
	err = db.QueryRow("SELECT 1").Scan(&notused)
	if err != nil {
		t.Fatal(err)
	}
}

// Test that EmptyQueryResponses are handled correctly.
func TestEmptyQuery(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Exec("")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("")
	if err != nil {
		t.Fatal(err)
	}
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 0 {
		t.Fatalf("unexpected number of columns %d in response to an empty query", len(cols))
	}
	if rows.Next() {
		t.Fatal("unexpected row")
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}

	stmt, err := db.Prepare("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = stmt.Exec()
	if err != nil {
		t.Fatal(err)
	}
	rows, err = stmt.Query()
	if err != nil {
		t.Fatal(err)
	}
	cols, err = rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 0 {
		t.Fatalf("unexpected number of columns %d in response to an empty query", len(cols))
	}
	if rows.Next() {
		t.Fatal("unexpected row")
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
}

func TestEncodeDecode(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	q := `
		SELECT
			E'\\000\\001\\002'::bytea,
			'foobar'::text,
			NULL::integer,
			'2000-1-1 01:02:03.04-7'::timestamptz,
			0::boolean,
			123,
			3.14::float8
		WHERE
			    E'\\000\\001\\002'::bytea = $1
			AND 'foobar'::text = $2
			AND $3::integer is NULL
	`
	// AND '2000-1-1 12:00:00.000000-7'::timestamp = $3

	exp1 := []byte{0, 1, 2}
	exp2 := "foobar"

	r, err := db.Query(q, exp1, exp2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if !r.Next() {
		if r.Err() != nil {
			t.Fatal(r.Err())
		}
		t.Fatal("expected row")
	}

	var got1 []byte
	var got2 string
	var got3 = sql.NullInt64{Valid: true}
	var got4 time.Time
	var got5, got6, got7 interface{}

	err = r.Scan(&got1, &got2, &got3, &got4, &got5, &got6, &got7)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(exp1, got1) {
		t.Errorf("expected %q byte: %q", exp1, got1)
	}

	if !reflect.DeepEqual(exp2, got2) {
		t.Errorf("expected %q byte: %q", exp2, got2)
	}

	if got3.Valid {
		t.Fatal("expected invalid")
	}

	if got4.Year() != 2000 {
		t.Fatal("wrong year")
	}

	if got5 != false {
		t.Fatalf("expected false, got %q", got5)
	}

	if got6 != int64(123) {
		t.Fatalf("expected 123, got %d", got6)
	}

	if got7 != float64(3.14) {
		t.Fatalf("expected 3.14, got %f", got7)
	}
}

func TestNoData(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	st, err := db.Prepare("SELECT 1 WHERE true = false")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	r, err := st.Query()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if r.Next() {
		if r.Err() != nil {
			t.Fatal(r.Err())
		}
		t.Fatal("unexpected row")
	}

	_, err = db.Query("SELECT * FROM nonexistenttable WHERE age=$1", 20)
	if err == nil {
		t.Fatal("Should have raised an error on non existent table")
	}

	_, err = db.Query("SELECT * FROM nonexistenttable")
	if err == nil {
		t.Fatal("Should have raised an error on non existent table")
	}
}

func TestErrorDuringStartup(t *testing.T) {
	// Don't use the normal connection setup, this is intended to
	// blow up in the startup packet from a non-existent user.
	db, err := openTestConnConninfo("user=thisuserreallydoesntexist")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Begin()
	if err == nil {
		t.Fatal("expected error")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected Error, got %#v", err)
	} else if e.Code.Name() != "invalid_authorization_specification" && e.Code.Name() != "invalid_password" {
		t.Fatalf("expected invalid_authorization_specification or invalid_password, got %s (%+v)", e.Code.Name(), err)
	}
}

func TestBadConn(t *testing.T) {
	var err error

	cn := conn{}
	func() {
		defer cn.errRecover(&err)
		panic(io.EOF)
	}()
	if err != driver.ErrBadConn {
		t.Fatalf("expected driver.ErrBadConn, got: %#v", err)
	}
	if !cn.bad {
		t.Fatalf("expected cn.bad")
	}

	cn = conn{}
	func() {
		defer cn.errRecover(&err)
		e := &Error{Severity: Efatal}
		panic(e)
	}()
	if err != driver.ErrBadConn {
		t.Fatalf("expected driver.ErrBadConn, got: %#v", err)
	}
	if !cn.bad {
		t.Fatalf("expected cn.bad")
	}
}

func TestErrorOnExec(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMPORARY TABLE foo(f1 int PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}

	_, err = txn.Exec("INSERT INTO foo VALUES (0), (0)")
	if err == nil {
		t.Fatal("Should have raised error")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected Error, got %#v", err)
	} else if e.Code.Name() != "unique_violation" {
		t.Fatalf("expected unique_violation, got %s (%+v)", e.Code.Name(), err)
	}
}

func TestErrorOnQuery(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMPORARY TABLE foo(f1 int PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}

	_, err = txn.Query("INSERT INTO foo VALUES (0), (0)")
	if err == nil {
		t.Fatal("Should have raised error")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected Error, got %#v", err)
	} else if e.Code.Name() != "unique_violation" {
		t.Fatalf("expected unique_violation, got %s (%+v)", e.Code.Name(), err)
	}
}

func TestErrorOnQueryRowSimpleQuery(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	_, err = txn.Exec("CREATE TEMPORARY TABLE foo(f1 int PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}

	var v int
	err = txn.QueryRow("INSERT INTO foo VALUES (0), (0)").Scan(&v)
	if err == nil {
		t.Fatal("Should have raised error")
	}

	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected Error, got %#v", err)
	} else if e.Code.Name() != "unique_violation" {
		t.Fatalf("expected unique_violation, got %s (%+v)", e.Code.Name(), err)
	}
}

// Test the QueryRow bug workarounds in stmt.exec() and simpleQuery()
func TestQueryRowBugWorkaround(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	// stmt.exec()
	_, err := db.Exec("CREATE TEMP TABLE notnulltemp (a varchar(10) not null)")
	if err != nil {
		t.Fatal(err)
	}

	var a string
	err = db.QueryRow("INSERT INTO notnulltemp(a) values($1) RETURNING a", nil).Scan(&a)
	if err == sql.ErrNoRows {
		t.Fatalf("expected constraint violation error; got: %v", err)
	}
	pge, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error; got: %#v", err)
	}
	if pge.Code.Name() != "not_null_violation" {
		t.Fatalf("expected not_null_violation; got: %s (%+v)", pge.Code.Name(), err)
	}

	// Test workaround in simpleQuery()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("unexpected error %s in Begin", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec("SET LOCAL check_function_bodies TO FALSE")
	if err != nil {
		t.Fatalf("could not disable check_function_bodies: %s", err)
	}
	_, err = tx.Exec(`
CREATE OR REPLACE FUNCTION bad_function()
RETURNS integer
-- hack to prevent the function from being inlined
SET check_function_bodies TO TRUE
AS $$
	SELECT text 'bad'
$$ LANGUAGE sql`)
	if err != nil {
		t.Fatalf("could not create function: %s", err)
	}

	err = tx.QueryRow("SELECT * FROM bad_function()").Scan(&a)
	if err == nil {
		t.Fatalf("expected error")
	}
	pge, ok = err.(*Error)
	if !ok {
		t.Fatalf("expected *Error; got: %#v", err)
	}
	if pge.Code.Name() != "invalid_function_definition" {
		t.Fatalf("expected invalid_function_definition; got: %s (%+v)", pge.Code.Name(), err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Fatalf("unexpected error %s in Rollback", err)
	}

	// Also test that simpleQuery()'s workaround works when the query fails
	// after a row has been received.
	rows, err := db.Query(`
select
	(select generate_series(1, ss.i))
from (select gs.i
      from generate_series(1, 2) gs(i)
      order by gs.i limit 2) ss`)
	if err != nil {
		t.Fatalf("query failed: %s", err)
	}
	if !rows.Next() {
		t.Fatalf("expected at least one result row; got %s", rows.Err())
	}
	var i int
	err = rows.Scan(&i)
	if err != nil {
		t.Fatalf("rows.Scan() failed: %s", err)
	}
	if i != 1 {
		t.Fatalf("unexpected value for i: %d", i)
	}
	if rows.Next() {
		t.Fatalf("unexpected row")
	}
	pge, ok = rows.Err().(*Error)
	if !ok {
		t.Fatalf("expected *Error; got: %#v", err)
	}
	if pge.Code.Name() != "cardinality_violation" {
		t.Fatalf("expected cardinality_violation; got: %s (%+v)", pge.Code.Name(), rows.Err())
	}
}

func TestSimpleQuery(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	r, err := db.Query("select 1")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if !r.Next() {
		t.Fatal("expected row")
	}
}

func TestBindError(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Exec("create temp table test (i integer)")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Query("select * from test where i=$1", "hhh")
	if err == nil {
		t.Fatal("expected an error")
	}

	// Should not get error here
	r, err := db.Query("select * from test where i=$1", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
}

func TestParseErrorInExtendedQuery(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	rows, err := db.Query("PARSE_ERROR $1", 1)
	if err == nil {
		t.Fatal("expected error")
	}

	rows, err = db.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

// TestReturning tests that an INSERT query using the RETURNING clause returns a row.
func TestReturning(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Exec("CREATE TEMP TABLE distributors (did integer default 0, dname text)")
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query("INSERT INTO distributors (did, dname) VALUES (DEFAULT, 'XYZ Widgets') " +
		"RETURNING did;")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("no rows")
	}
	var did int
	err = rows.Scan(&did)
	if err != nil {
		t.Fatal(err)
	}
	if did != 0 {
		t.Fatalf("bad value for did: got %d, want %d", did, 0)
	}

	if rows.Next() {
		t.Fatal("unexpected next row")
	}
	err = rows.Err()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIssue186(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	// Exec() a query which returns results
	_, err := db.Exec("VALUES (1), (2), (3)")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec("VALUES ($1), ($2), ($3)", 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Query() a query which doesn't return any results
	txn, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer txn.Rollback()

	rows, err := txn.Query("CREATE TEMP TABLE foo(f1 int)")
	if err != nil {
		t.Fatal(err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}

	// small trick to get NoData from a parameterized query
	_, err = txn.Exec("CREATE RULE nodata AS ON INSERT TO foo DO INSTEAD NOTHING")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = txn.Query("INSERT INTO foo VALUES ($1)", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestIssue196(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	row := db.QueryRow("SELECT float4 '0.10000122' = $1, float8 '35.03554004971999' = $2",
		float32(0.10000122), float64(35.03554004971999))

	var float4match, float8match bool
	err := row.Scan(&float4match, &float8match)
	if err != nil {
		t.Fatal(err)
	}
	if !float4match {
		t.Errorf("Expected float4 fidelity to be maintained; got no match")
	}
	if !float8match {
		t.Errorf("Expected float8 fidelity to be maintained; got no match")
	}
}

// Test that any CommandComplete messages sent before the query results are
// ignored.
func TestIssue282(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	var search_path string
	err := db.QueryRow(`
		SET LOCAL search_path TO pg_catalog;
		SET LOCAL search_path TO pg_catalog;
		SHOW search_path`).Scan(&search_path)
	if err != nil {
		t.Fatal(err)
	}
	if search_path != "pg_catalog" {
		t.Fatalf("unexpected search_path %s", search_path)
	}
}

func TestReadFloatPrecision(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	row := db.QueryRow("SELECT float4 '0.10000122', float8 '35.03554004971999'")
	var float4val float32
	var float8val float64
	err := row.Scan(&float4val, &float8val)
	if err != nil {
		t.Fatal(err)
	}
	if float4val != float32(0.10000122) {
		t.Errorf("Expected float4 fidelity to be maintained; got no match")
	}
	if float8val != float64(35.03554004971999) {
		t.Errorf("Expected float8 fidelity to be maintained; got no match")
	}
}

func TestXactMultiStmt(t *testing.T) {
	// minified test case based on bug reports from
	// pico303@gmail.com and rangelspam@gmail.com
	t.Skip("Skipping failing test")
	db := openTestConn(t)
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Commit()

	rows, err := tx.Query("select 1")
	if err != nil {
		t.Fatal(err)
	}

	if rows.Next() {
		var val int32
		if err = rows.Scan(&val); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("Expected at least one row in first query in xact")
	}

	rows2, err := tx.Query("select 2")
	if err != nil {
		t.Fatal(err)
	}

	if rows2.Next() {
		var val2 int32
		if err := rows2.Scan(&val2); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("Expected at least one row in second query in xact")
	}

	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	if err = rows2.Err(); err != nil {
		t.Fatal(err)
	}

	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

var envParseTests = []struct {
	Expected map[string]string
	Env      []string
}{
	{
		Env:      []string{"PGDATABASE=hello", "PGUSER=goodbye"},
		Expected: map[string]string{"dbname": "hello", "user": "goodbye"},
	},
	{
		Env:      []string{"PGDATESTYLE=ISO, MDY"},
		Expected: map[string]string{"datestyle": "ISO, MDY"},
	},
	{
		Env:      []string{"PGCONNECT_TIMEOUT=30"},
		Expected: map[string]string{"connect_timeout": "30"},
	},
}

func TestParseEnviron(t *testing.T) {
	for i, tt := range envParseTests {
		results := parseEnviron(tt.Env)
		if !reflect.DeepEqual(tt.Expected, results) {
			t.Errorf("%d: Expected: %#v Got: %#v", i, tt.Expected, results)
		}
	}
}

func TestParseComplete(t *testing.T) {
	tpc := func(commandTag string, command string, affectedRows int64, shouldFail bool) {
		defer func() {
			if p := recover(); p != nil {
				if !shouldFail {
					t.Error(p)
				}
			}
		}()
		cn := &conn{}
		res, c := cn.parseComplete(commandTag)
		if c != command {
			t.Errorf("Expected %v, got %v", command, c)
		}
		n, err := res.RowsAffected()
		if err != nil {
			t.Fatal(err)
		}
		if n != affectedRows {
			t.Errorf("Expected %d, got %d", affectedRows, n)
		}
	}

	tpc("ALTER TABLE", "ALTER TABLE", 0, false)
	tpc("INSERT 0 1", "INSERT", 1, false)
	tpc("UPDATE 100", "UPDATE", 100, false)
	tpc("SELECT 100", "SELECT", 100, false)
	tpc("FETCH 100", "FETCH", 100, false)
	// allow COPY (and others) without row count
	tpc("COPY", "COPY", 0, false)
	// don't fail on command tags we don't recognize
	tpc("UNKNOWNCOMMANDTAG", "UNKNOWNCOMMANDTAG", 0, false)

	// failure cases
	tpc("INSERT 1", "", 0, true)   // missing oid
	tpc("UPDATE 0 1", "", 0, true) // too many numbers
	tpc("SELECT foo", "", 0, true) // invalid row count
}

func TestExecerInterface(t *testing.T) {
	// Gin up a straw man private struct just for the type check
	cn := &conn{c: nil}
	var cni interface{} = cn

	_, ok := cni.(driver.Execer)
	if !ok {
		t.Fatal("Driver doesn't implement Execer")
	}
}

func TestNullAfterNonNull(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	r, err := db.Query("SELECT 9::integer UNION SELECT NULL::integer")
	if err != nil {
		t.Fatal(err)
	}

	var n sql.NullInt64

	if !r.Next() {
		if r.Err() != nil {
			t.Fatal(err)
		}
		t.Fatal("expected row")
	}

	if err := r.Scan(&n); err != nil {
		t.Fatal(err)
	}

	if n.Int64 != 9 {
		t.Fatalf("expected 2, not %d", n.Int64)
	}

	if !r.Next() {
		if r.Err() != nil {
			t.Fatal(err)
		}
		t.Fatal("expected row")
	}

	if err := r.Scan(&n); err != nil {
		t.Fatal(err)
	}

	if n.Valid {
		t.Fatal("expected n to be invalid")
	}

	if n.Int64 != 0 {
		t.Fatalf("expected n to 2, not %d", n.Int64)
	}
}

func Test64BitErrorChecking(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			t.Fatal("panic due to 0xFFFFFFFF != -1 " +
				"when int is 64 bits")
		}
	}()

	db := openTestConn(t)
	defer db.Close()

	r, err := db.Query(`SELECT *
FROM (VALUES (0::integer, NULL::text), (1, 'test string')) AS t;`)

	if err != nil {
		t.Fatal(err)
	}

	defer r.Close()

	for r.Next() {
	}
}

func TestCommit(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Exec("CREATE TEMP TABLE temp (a int)")
	if err != nil {
		t.Fatal(err)
	}
	sqlInsert := "INSERT INTO temp VALUES (1)"
	sqlSelect := "SELECT * FROM temp"
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(sqlInsert)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.Commit()
	if err != nil {
		t.Fatal(err)
	}
	var i int
	err = db.QueryRow(sqlSelect).Scan(&i)
	if err != nil {
		t.Fatal(err)
	}
	if i != 1 {
		t.Fatalf("expected 1, got %d", i)
	}
}

func TestErrorClass(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	_, err := db.Query("SELECT int 'notint'")
	if err == nil {
		t.Fatal("expected error")
	}
	pge, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *pq.Error, got %#+v", err)
	}
	if pge.Code.Class() != "22" {
		t.Fatalf("expected class 28, got %v", pge.Code.Class())
	}
	if pge.Code.Class().Name() != "data_exception" {
		t.Fatalf("expected data_exception, got %v", pge.Code.Class().Name())
	}
}

func TestParseOpts(t *testing.T) {
	tests := []struct {
		in       string
		expected values
		valid    bool
	}{
		{"dbname=hello user=goodbye", values{"dbname": "hello", "user": "goodbye"}, true},
		{"dbname=hello user=goodbye  ", values{"dbname": "hello", "user": "goodbye"}, true},
		{"dbname = hello user=goodbye", values{"dbname": "hello", "user": "goodbye"}, true},
		{"dbname=hello user =goodbye", values{"dbname": "hello", "user": "goodbye"}, true},
		{"dbname=hello user= goodbye", values{"dbname": "hello", "user": "goodbye"}, true},
		{"host=localhost password='correct horse battery staple'", values{"host": "localhost", "password": "correct horse battery staple"}, true},
		{"dbname=データベース password=パスワード", values{"dbname": "データベース", "password": "パスワード"}, true},
		{"dbname=hello user=''", values{"dbname": "hello", "user": ""}, true},
		{"user='' dbname=hello", values{"dbname": "hello", "user": ""}, true},
		// The last option value is an empty string if there's no non-whitespace after its =
		{"dbname=hello user=   ", values{"dbname": "hello", "user": ""}, true},

		// The parser ignores spaces after = and interprets the next set of non-whitespace characters as the value.
		{"user= password=foo", values{"user": "password=foo"}, true},

		// Backslash escapes next char
		{`user=a\ \'\\b`, values{"user": `a '\b`}, true},
		{`user='a \'b'`, values{"user": `a 'b`}, true},

		// Incomplete escape
		{`user=x\`, values{}, false},

		// No '=' after the key
		{"postgre://marko@internet", values{}, false},
		{"dbname user=goodbye", values{}, false},
		{"user=foo blah", values{}, false},
		{"user=foo blah   ", values{}, false},

		// Unterminated quoted value
		{"dbname=hello user='unterminated", values{}, false},
	}

	for _, test := range tests {
		o := make(values)
		err := parseOpts(test.in, o)

		switch {
		case err != nil && test.valid:
			t.Errorf("%q got unexpected error: %s", test.in, err)
		case err == nil && test.valid && !reflect.DeepEqual(test.expected, o):
			t.Errorf("%q got: %#v want: %#v", test.in, o, test.expected)
		case err == nil && !test.valid:
			t.Errorf("%q expected an error", test.in)
		}
	}
}

func TestRuntimeParameters(t *testing.T) {
	type RuntimeTestResult int
	const (
		ResultUnknown RuntimeTestResult = iota
		ResultSuccess
		ResultError // other error
	)

	tests := []struct {
		conninfo        string
		param           string
		expected        string
		expectedOutcome RuntimeTestResult
	}{
		// invalid parameter
		{"DOESNOTEXIST=foo", "", "", ResultError},
		// we can only work with a specific value for these two
		{"client_encoding=SQL_ASCII", "", "", ResultError},
		{"datestyle='ISO, YDM'", "", "", ResultError},
		// "options" should work exactly as it does in libpq
		{"options='-c search_path=pqgotest'", "search_path", "pqgotest", ResultSuccess},
		// pq should override client_encoding in this case
		{"options='-c client_encoding=SQL_ASCII'", "client_encoding", "UTF8", ResultSuccess},
		// allow client_encoding to be set explicitly
		{"client_encoding=UTF8", "client_encoding", "UTF8", ResultSuccess},
		// test a runtime parameter not supported by libpq
		{"work_mem='139kB'", "work_mem", "139kB", ResultSuccess},
		// test fallback_application_name
		{"application_name=foo fallback_application_name=bar", "application_name", "foo", ResultSuccess},
		{"application_name='' fallback_application_name=bar", "application_name", "", ResultSuccess},
		{"fallback_application_name=bar", "application_name", "bar", ResultSuccess},
	}

	for _, test := range tests {
		db, err := openTestConnConninfo(test.conninfo)
		if err != nil {
			t.Fatal(err)
		}

		// application_name didn't exist before 9.0
		if test.param == "application_name" && getServerVersion(t, db) < 90000 {
			db.Close()
			continue
		}

		tryGetParameterValue := func() (value string, outcome RuntimeTestResult) {
			defer db.Close()
			row := db.QueryRow("SELECT current_setting($1)", test.param)
			err = row.Scan(&value)
			if err != nil {
				return "", ResultError
			}
			return value, ResultSuccess
		}

		value, outcome := tryGetParameterValue()
		if outcome != test.expectedOutcome && outcome == ResultError {
			t.Fatalf("%v: unexpected error: %v", test.conninfo, err)
		}
		if outcome != test.expectedOutcome {
			t.Fatalf("unexpected outcome %v (was expecting %v) for conninfo \"%s\"",
				outcome, test.expectedOutcome, test.conninfo)
		}
		if value != test.expected {
			t.Fatalf("bad value for %s: got %s, want %s with conninfo \"%s\"",
				test.param, value, test.expected, test.conninfo)
		}
	}
}

func TestIsUTF8(t *testing.T) {
	var cases = []struct {
		name string
		want bool
	}{
		{"unicode", true},
		{"utf-8", true},
		{"utf_8", true},
		{"UTF-8", true},
		{"UTF8", true},
		{"utf8", true},
		{"u n ic_ode", true},
		{"ut_f%8", true},
		{"ubf8", false},
		{"punycode", false},
	}

	for _, test := range cases {
		if g := isUTF8(test.name); g != test.want {
			t.Errorf("isUTF8(%q) = %v want %v", test.name, g, test.want)
		}
	}
}

func TestQuoteIdentifier(t *testing.T) {
	var cases = []struct {
		input string
		want  string
	}{
		{`foo`, `"foo"`},
		{`foo bar baz`, `"foo bar baz"`},
		{`foo"bar`, `"foo""bar"`},
		{"foo\x00bar", `"foo"`},
		{"\x00foo", `""`},
	}

	for _, test := range cases {
		got := QuoteIdentifier(test.input)
		if got != test.want {
			t.Errorf("QuoteIdentifier(%q) = %v want %v", test.input, got, test.want)
		}
	}
}
