package pq

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq/oid"
)

func TestScanTimestamp(t *testing.T) {
	var nt NullTime
	tn := time.Now()
	nt.Scan(tn)
	if !nt.Valid {
		t.Errorf("Expected Valid=false")
	}
	if nt.Time != tn {
		t.Errorf("Time value mismatch")
	}
}

func TestScanNilTimestamp(t *testing.T) {
	var nt NullTime
	nt.Scan(nil)
	if nt.Valid {
		t.Errorf("Expected Valid=false")
	}
}

var timeTests = []struct {
	str     string
	timeval time.Time
}{
	{"22001-02-03", time.Date(22001, time.February, 3, 0, 0, 0, 0, time.FixedZone("", 0))},
	{"2001-02-03", time.Date(2001, time.February, 3, 0, 0, 0, 0, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06", time.Date(2001, time.February, 3, 4, 5, 6, 0, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.000001", time.Date(2001, time.February, 3, 4, 5, 6, 1000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.00001", time.Date(2001, time.February, 3, 4, 5, 6, 10000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.0001", time.Date(2001, time.February, 3, 4, 5, 6, 100000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.001", time.Date(2001, time.February, 3, 4, 5, 6, 1000000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.01", time.Date(2001, time.February, 3, 4, 5, 6, 10000000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.1", time.Date(2001, time.February, 3, 4, 5, 6, 100000000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.12", time.Date(2001, time.February, 3, 4, 5, 6, 120000000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.123", time.Date(2001, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.1234", time.Date(2001, time.February, 3, 4, 5, 6, 123400000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.12345", time.Date(2001, time.February, 3, 4, 5, 6, 123450000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.123456", time.Date(2001, time.February, 3, 4, 5, 6, 123456000, time.FixedZone("", 0))},
	{"2001-02-03 04:05:06.123-07", time.Date(2001, time.February, 3, 4, 5, 6, 123000000,
		time.FixedZone("", -7*60*60))},
	{"2001-02-03 04:05:06-07", time.Date(2001, time.February, 3, 4, 5, 6, 0,
		time.FixedZone("", -7*60*60))},
	{"2001-02-03 04:05:06-07:42", time.Date(2001, time.February, 3, 4, 5, 6, 0,
		time.FixedZone("", -(7*60*60+42*60)))},
	{"2001-02-03 04:05:06-07:30:09", time.Date(2001, time.February, 3, 4, 5, 6, 0,
		time.FixedZone("", -(7*60*60+30*60+9)))},
	{"2001-02-03 04:05:06+07", time.Date(2001, time.February, 3, 4, 5, 6, 0,
		time.FixedZone("", 7*60*60))},
	{"0011-02-03 04:05:06 BC", time.Date(-10, time.February, 3, 4, 5, 6, 0, time.FixedZone("", 0))},
	{"0011-02-03 04:05:06.123 BC", time.Date(-10, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0))},
	{"0011-02-03 04:05:06.123-07 BC", time.Date(-10, time.February, 3, 4, 5, 6, 123000000,
		time.FixedZone("", -7*60*60))},
	{"0001-02-03 04:05:06.123", time.Date(1, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0))},
	{"0001-02-03 04:05:06.123 BC", time.Date(1, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0)).AddDate(-1, 0, 0)},
	{"0001-02-03 04:05:06.123 BC", time.Date(0, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0))},
	{"0002-02-03 04:05:06.123 BC", time.Date(0, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0)).AddDate(-1, 0, 0)},
	{"0002-02-03 04:05:06.123 BC", time.Date(-1, time.February, 3, 4, 5, 6, 123000000, time.FixedZone("", 0))},
	{"12345-02-03 04:05:06.1", time.Date(12345, time.February, 3, 4, 5, 6, 100000000, time.FixedZone("", 0))},
	{"123456-02-03 04:05:06.1", time.Date(123456, time.February, 3, 4, 5, 6, 100000000, time.FixedZone("", 0))},
}

// Helper function for the two tests below
func tryParse(str string) (t time.Time, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
			return
		}
	}()
	i := parseTs(nil, str)
	t, ok := i.(time.Time)
	if !ok {
		err = fmt.Errorf("Not a time.Time type, got %#v", i)
	}
	return
}

// Test that parsing the string results in the expected value.
func TestParseTs(t *testing.T) {
	for i, tt := range timeTests {
		val, err := tryParse(tt.str)
		if err != nil {
			t.Errorf("%d: got error: %v", i, err)
		} else if val.String() != tt.timeval.String() {
			t.Errorf("%d: expected to parse %q into %q; got %q",
				i, tt.str, tt.timeval, val)
		}
	}
}

// Now test that sending the value into the database and parsing it back
// returns the same time.Time value.
func TestEncodeAndParseTs(t *testing.T) {
	db, err := openTestConnConninfo("timezone='Etc/UTC'")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i, tt := range timeTests {
		var dbstr string
		err = db.QueryRow("SELECT ($1::timestamptz)::text", tt.timeval).Scan(&dbstr)
		if err != nil {
			t.Errorf("%d: could not send value %q to the database: %s", i, tt.timeval, err)
			continue
		}

		val, err := tryParse(dbstr)
		if err != nil {
			t.Errorf("%d: could not parse value %q: %s", i, dbstr, err)
			continue
		}
		val = val.In(tt.timeval.Location())
		if val.String() != tt.timeval.String() {
			t.Errorf("%d: expected to parse %q into %q; got %q", i, dbstr, tt.timeval, val)
		}
	}
}

var formatTimeTests = []struct {
	time     time.Time
	expected string
}{
	{time.Time{}, "0001-01-01T00:00:00Z"},
	{time.Date(2001, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 0)), "2001-02-03T04:05:06.123456789Z"},
	{time.Date(2001, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 2*60*60)), "2001-02-03T04:05:06.123456789+02:00"},
	{time.Date(2001, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", -6*60*60)), "2001-02-03T04:05:06.123456789-06:00"},
	{time.Date(2001, time.February, 3, 4, 5, 6, 0, time.FixedZone("", -(7*60*60+30*60+9))), "2001-02-03T04:05:06-07:30:09"},

	{time.Date(1, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 0)), "0001-02-03T04:05:06.123456789Z"},
	{time.Date(1, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 2*60*60)), "0001-02-03T04:05:06.123456789+02:00"},
	{time.Date(1, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", -6*60*60)), "0001-02-03T04:05:06.123456789-06:00"},

	{time.Date(0, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 0)), "0001-02-03T04:05:06.123456789Z BC"},
	{time.Date(0, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", 2*60*60)), "0001-02-03T04:05:06.123456789+02:00 BC"},
	{time.Date(0, time.February, 3, 4, 5, 6, 123456789, time.FixedZone("", -6*60*60)), "0001-02-03T04:05:06.123456789-06:00 BC"},

	{time.Date(1, time.February, 3, 4, 5, 6, 0, time.FixedZone("", -(7*60*60+30*60+9))), "0001-02-03T04:05:06-07:30:09"},
	{time.Date(0, time.February, 3, 4, 5, 6, 0, time.FixedZone("", -(7*60*60+30*60+9))), "0001-02-03T04:05:06-07:30:09 BC"},
}

func TestFormatTs(t *testing.T) {
	for i, tt := range formatTimeTests {
		val := string(formatTs(tt.time))
		if val != tt.expected {
			t.Errorf("%d: incorrect time format %q, want %q", i, val, tt.expected)
		}
	}
}

func TestTimestampWithTimeZone(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	// try several different locations, all included in Go's zoneinfo.zip
	for _, locName := range []string{
		"UTC",
		"America/Chicago",
		"America/New_York",
		"Australia/Darwin",
		"Australia/Perth",
	} {
		loc, err := time.LoadLocation(locName)
		if err != nil {
			t.Logf("Could not load time zone %s - skipping", locName)
			continue
		}

		// Postgres timestamps have a resolution of 1 microsecond, so don't
		// use the full range of the Nanosecond argument
		refTime := time.Date(2012, 11, 6, 10, 23, 42, 123456000, loc)

		for _, pgTimeZone := range []string{"US/Eastern", "Australia/Darwin"} {
			// Switch Postgres's timezone to test different output timestamp formats
			_, err = tx.Exec(fmt.Sprintf("set time zone '%s'", pgTimeZone))
			if err != nil {
				t.Fatal(err)
			}

			var gotTime time.Time
			row := tx.QueryRow("select $1::timestamp with time zone", refTime)
			err = row.Scan(&gotTime)
			if err != nil {
				t.Fatal(err)
			}

			if !refTime.Equal(gotTime) {
				t.Errorf("timestamps not equal: %s != %s", refTime, gotTime)
			}

			// check that the time zone is set correctly based on TimeZone
			pgLoc, err := time.LoadLocation(pgTimeZone)
			if err != nil {
				t.Logf("Could not load time zone %s - skipping", pgLoc)
				continue
			}
			translated := refTime.In(pgLoc)
			if translated.String() != gotTime.String() {
				t.Errorf("timestamps not equal: %s != %s", translated, gotTime)
			}
		}
	}
}

func TestTimestampWithOutTimezone(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	test := func(ts, pgts string) {
		r, err := db.Query("SELECT $1::timestamp", pgts)
		if err != nil {
			t.Fatalf("Could not run query: %v", err)
		}

		n := r.Next()

		if n != true {
			t.Fatal("Expected at least one row")
		}

		var result time.Time
		err = r.Scan(&result)
		if err != nil {
			t.Fatalf("Did not expect error scanning row: %v", err)
		}

		expected, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t.Fatalf("Could not parse test time literal: %v", err)
		}

		if !result.Equal(expected) {
			t.Fatalf("Expected time to match %v: got mismatch %v",
				expected, result)
		}

		n = r.Next()
		if n != false {
			t.Fatal("Expected only one row")
		}
	}

	test("2000-01-01T00:00:00Z", "2000-01-01T00:00:00")

	// Test higher precision time
	test("2013-01-04T20:14:58.80033Z", "2013-01-04 20:14:58.80033")
}

func TestInfinityTimestamp(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()
	var err error
	var resultT time.Time

	expectedError := fmt.Errorf(`sql: Scan error on column index 0: unsupported driver -> Scan pair: []uint8 -> *time.Time`)
	type testCases []struct {
		Query       string
		Param       string
		ExpectedErr error
		ExpectedVal interface{}
	}
	tc := testCases{
		{"SELECT $1::timestamp", "-infinity", expectedError, "-infinity"},
		{"SELECT $1::timestamptz", "-infinity", expectedError, "-infinity"},
		{"SELECT $1::timestamp", "infinity", expectedError, "infinity"},
		{"SELECT $1::timestamptz", "infinity", expectedError, "infinity"},
	}
	// try to assert []byte to time.Time
	for _, q := range tc {
		err = db.QueryRow(q.Query, q.Param).Scan(&resultT)
		if err.Error() != q.ExpectedErr.Error() {
			t.Errorf("Scanning -/+infinity, expected error, %q, got %q", q.ExpectedErr, err)
		}
	}
	// yield []byte
	for _, q := range tc {
		var resultI interface{}
		err = db.QueryRow(q.Query, q.Param).Scan(&resultI)
		if err != nil {
			t.Errorf("Scanning -/+infinity, expected no error, got %q", err)
		}
		result, ok := resultI.([]byte)
		if !ok {
			t.Errorf("Scanning -/+infinity, expected []byte, got %#v", resultI)
		}
		if string(result) != q.ExpectedVal {
			t.Errorf("Scanning -/+infinity, expected %q, got %q", q.ExpectedVal, result)
		}
	}

	y1500 := time.Date(1500, time.January, 1, 0, 0, 0, 0, time.UTC)
	y2500 := time.Date(2500, time.January, 1, 0, 0, 0, 0, time.UTC)
	EnableInfinityTs(y1500, y2500)

	err = db.QueryRow("SELECT $1::timestamp", "infinity").Scan(&resultT)
	if err != nil {
		t.Errorf("Scanning infinity, expected no error, got %q", err)
	}
	if !resultT.Equal(y2500) {
		t.Errorf("Scanning infinity, expected %q, got %q", y2500, resultT)
	}

	err = db.QueryRow("SELECT $1::timestamptz", "infinity").Scan(&resultT)
	if err != nil {
		t.Errorf("Scanning infinity, expected no error, got %q", err)
	}
	if !resultT.Equal(y2500) {
		t.Errorf("Scanning Infinity, expected time %q, got %q", y2500, resultT.String())
	}

	err = db.QueryRow("SELECT $1::timestamp", "-infinity").Scan(&resultT)
	if err != nil {
		t.Errorf("Scanning -infinity, expected no error, got %q", err)
	}
	if !resultT.Equal(y1500) {
		t.Errorf("Scanning -infinity, expected time %q, got %q", y1500, resultT.String())
	}

	err = db.QueryRow("SELECT $1::timestamptz", "-infinity").Scan(&resultT)
	if err != nil {
		t.Errorf("Scanning -infinity, expected no error, got %q", err)
	}
	if !resultT.Equal(y1500) {
		t.Errorf("Scanning -infinity, expected time %q, got %q", y1500, resultT.String())
	}

	y_1500 := time.Date(-1500, time.January, 1, 0, 0, 0, 0, time.UTC)
	y11500 := time.Date(11500, time.January, 1, 0, 0, 0, 0, time.UTC)
	var s string
	err = db.QueryRow("SELECT $1::timestamp::text", y_1500).Scan(&s)
	if err != nil {
		t.Errorf("Encoding -infinity, expected no error, got %q", err)
	}
	if s != "-infinity" {
		t.Errorf("Encoding -infinity, expected %q, got %q", "-infinity", s)
	}
	err = db.QueryRow("SELECT $1::timestamptz::text", y_1500).Scan(&s)
	if err != nil {
		t.Errorf("Encoding -infinity, expected no error, got %q", err)
	}
	if s != "-infinity" {
		t.Errorf("Encoding -infinity, expected %q, got %q", "-infinity", s)
	}

	err = db.QueryRow("SELECT $1::timestamp::text", y11500).Scan(&s)
	if err != nil {
		t.Errorf("Encoding infinity, expected no error, got %q", err)
	}
	if s != "infinity" {
		t.Errorf("Encoding infinity, expected %q, got %q", "infinity", s)
	}
	err = db.QueryRow("SELECT $1::timestamptz::text", y11500).Scan(&s)
	if err != nil {
		t.Errorf("Encoding infinity, expected no error, got %q", err)
	}
	if s != "infinity" {
		t.Errorf("Encoding infinity, expected %q, got %q", "infinity", s)
	}

	disableInfinityTs()

	var panicErrorString string
	func() {
		defer func() {
			panicErrorString, _ = recover().(string)
		}()
		EnableInfinityTs(y2500, y1500)
	}()
	if panicErrorString != infinityTsNegativeMustBeSmaller {
		t.Errorf("Expected error, %q, got %q", infinityTsNegativeMustBeSmaller, panicErrorString)
	}
}

func TestStringWithNul(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	hello0world := string("hello\x00world")
	_, err := db.Query("SELECT $1::text", &hello0world)
	if err == nil {
		t.Fatal("Postgres accepts a string with nul in it; " +
			"injection attacks may be plausible")
	}
}

func TestByteaToText(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	b := []byte("hello world")
	row := db.QueryRow("SELECT $1::text", b)

	var result []byte
	err := row.Scan(&result)
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != string(b) {
		t.Fatalf("expected %v but got %v", b, result)
	}
}

func TestTextToBytea(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	b := "hello world"
	row := db.QueryRow("SELECT $1::bytea", b)

	var result []byte
	err := row.Scan(&result)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(result, []byte(b)) {
		t.Fatalf("expected %v but got %v", b, result)
	}
}

func TestByteaOutputFormatEncoding(t *testing.T) {
	input := []byte("\\x\x00\x01\x02\xFF\xFEabcdefg0123")
	want := []byte("\\x5c78000102fffe6162636465666730313233")
	got := encode(&parameterStatus{serverVersion: 90000}, input, oid.T_bytea)
	if !bytes.Equal(want, got) {
		t.Errorf("invalid hex bytea output, got %v but expected %v", got, want)
	}

	want = []byte("\\\\x\\000\\001\\002\\377\\376abcdefg0123")
	got = encode(&parameterStatus{serverVersion: 84000}, input, oid.T_bytea)
	if !bytes.Equal(want, got) {
		t.Errorf("invalid escape bytea output, got %v but expected %v", got, want)
	}
}

func TestByteaOutputFormats(t *testing.T) {
	db := openTestConn(t)
	defer db.Close()

	if getServerVersion(t, db) < 90000 {
		// skip
		return
	}

	testByteaOutputFormat := func(f string) {
		expectedData := []byte("\x5c\x78\x00\xff\x61\x62\x63\x01\x08")
		sqlQuery := "SELECT decode('5c7800ff6162630108', 'hex')"

		var data []byte

		// use a txn to avoid relying on getting the same connection
		txn, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer txn.Rollback()

		_, err = txn.Exec("SET LOCAL bytea_output TO " + f)
		if err != nil {
			t.Fatal(err)
		}
		// use Query; QueryRow would hide the actual error
		rows, err := txn.Query(sqlQuery)
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			if rows.Err() != nil {
				t.Fatal(rows.Err())
			}
			t.Fatal("shouldn't happen")
		}
		err = rows.Scan(&data)
		if err != nil {
			t.Fatal(err)
		}
		err = rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, expectedData) {
			t.Errorf("unexpected bytea value %v for format %s; expected %v", data, f, expectedData)
		}
	}

	testByteaOutputFormat("hex")
	testByteaOutputFormat("escape")
}

func TestAppendEncodedText(t *testing.T) {
	var buf []byte

	buf = appendEncodedText(&parameterStatus{serverVersion: 90000}, buf, int64(10))
	buf = append(buf, '\t')
	buf = appendEncodedText(&parameterStatus{serverVersion: 90000}, buf, 42.0000000001)
	buf = append(buf, '\t')
	buf = appendEncodedText(&parameterStatus{serverVersion: 90000}, buf, "hello\tworld")
	buf = append(buf, '\t')
	buf = appendEncodedText(&parameterStatus{serverVersion: 90000}, buf, []byte{0, 128, 255})

	if string(buf) != "10\t42.0000000001\thello\\tworld\t\\\\x0080ff" {
		t.Fatal(string(buf))
	}
}

func TestAppendEscapedText(t *testing.T) {
	if esc := appendEscapedText(nil, "hallo\tescape"); string(esc) != "hallo\\tescape" {
		t.Fatal(string(esc))
	}
	if esc := appendEscapedText(nil, "hallo\\tescape\n"); string(esc) != "hallo\\\\tescape\\n" {
		t.Fatal(string(esc))
	}
	if esc := appendEscapedText(nil, "\n\r\t\f"); string(esc) != "\\n\\r\\t\f" {
		t.Fatal(string(esc))
	}
}

func TestAppendEscapedTextExistingBuffer(t *testing.T) {
	var buf []byte
	buf = []byte("123\t")
	if esc := appendEscapedText(buf, "hallo\tescape"); string(esc) != "123\thallo\\tescape" {
		t.Fatal(string(esc))
	}
	buf = []byte("123\t")
	if esc := appendEscapedText(buf, "hallo\\tescape\n"); string(esc) != "123\thallo\\\\tescape\\n" {
		t.Fatal(string(esc))
	}
	buf = []byte("123\t")
	if esc := appendEscapedText(buf, "\n\r\t\f"); string(esc) != "123\t\\n\\r\\t\f" {
		t.Fatal(string(esc))
	}
}

func BenchmarkAppendEscapedText(b *testing.B) {
	longString := ""
	for i := 0; i < 100; i++ {
		longString += "123456789\n"
	}
	for i := 0; i < b.N; i++ {
		appendEscapedText(nil, longString)
	}
}

func BenchmarkAppendEscapedTextNoEscape(b *testing.B) {
	longString := ""
	for i := 0; i < 100; i++ {
		longString += "1234567890"
	}
	for i := 0; i < b.N; i++ {
		appendEscapedText(nil, longString)
	}
}
