// Copyright (c) 2014 ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ql

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

var (
	oN        = flag.Int("N", 0, "")
	oM        = flag.Int("M", 0, "")
	oFastFail = flag.Bool("fastFail", false, "")
	oSlow     = flag.Bool("slow", false, "Do not wrap storage tests in a single outer transaction, write everything to disk file. Very slow.")
)

var testdata []string

func init() {
	tests, err := ioutil.ReadFile("testdata.ql")
	if err != nil {
		log.Panic(err)
	}

	a := bytes.Split(tests, []byte("\n-- "))
	pre := []byte("-- ")
	pres := []byte("S ")
	for _, v := range a[1:] {
		switch {
		case bytes.HasPrefix(v, pres):
			v = append(pre, v...)
			v = append([]byte(sample), v...)
		default:
			v = append(pre, v...)
		}
		testdata = append(testdata, string(v))
	}
}

func typeof(v interface{}) (r int) { //NTYPE
	switch v.(type) {
	case bool:
		return qBool
	case complex64:
		return qComplex64
	case complex128:
		return qComplex128
	case float32:
		return qFloat32
	case float64:
		return qFloat64
	case int8:
		return qInt8
	case int16:
		return qInt16
	case int32:
		return qInt32
	case int64:
		return qInt64
	case string:
		return qString
	case uint8:
		return qUint8
	case uint16:
		return qUint16
	case uint32:
		return qUint32
	case uint64:
		return qUint64
	}
	return
}

func stypeof(nm string, val interface{}) string {
	if t := typeof(val); t != 0 {
		return fmt.Sprintf("%c%s", t, nm)
	}

	switch val.(type) {
	case idealComplex:
		return fmt.Sprintf("c%s", nm)
	case idealFloat:
		return fmt.Sprintf("f%s", nm)
	case idealInt:
		return fmt.Sprintf("l%s", nm)
	case idealRune:
		return fmt.Sprintf("k%s", nm)
	case idealUint:
		return fmt.Sprintf("x%s", nm)
	default:
		return fmt.Sprintf("?%s", nm)
	}
}

func dumpCols(cols []*col) string {
	a := []string{}
	for _, col := range cols {
		a = append(a, fmt.Sprintf("%d:%s %s", col.index, col.name, typeStr(col.typ)))
	}
	return strings.Join(a, ",")
}

func dumpFlds(flds []*fld) string {
	a := []string{}
	for _, fld := range flds {
		a = append(a, fmt.Sprintf("%s AS %s", fld.expr, fld.name))
	}
	return strings.Join(a, ",")
}

func recSetDump(rs Recordset) (s string, err error) {
	recset := rs.(recordset)
	p := recset.plan
	a0 := append([]string(nil), p.fieldNames()...)
	for i, v := range a0 {
		a0[i] = fmt.Sprintf("%q", v)
	}
	a := []string{strings.Join(a0, ", ")}
	if err := p.do(recset.ctx, func(id interface{}, data []interface{}) (bool, error) {
		if err = expand(data); err != nil {
			return false, err
		}

		a = append(a, fmt.Sprintf("%v", data))
		return true, nil
	}); err != nil {
		return "", err
	}
	return strings.Join(a, "\n"), nil
}

// http://en.wikipedia.org/wiki/Join_(SQL)#Sample_tables
const sample = `
     BEGIN TRANSACTION;
		CREATE TABLE department (
			DepartmentID   int,
			DepartmentName string,
		);

		INSERT INTO department VALUES
			(31, "Sales"),
			(33, "Engineering"),
			(34, "Clerical"),
			(35, "Marketing"),
		;

		CREATE TABLE employee (
			LastName     string,
			DepartmentID int,
		);

		INSERT INTO employee VALUES
			("Rafferty", 31),
			("Jones", 33),
			("Heisenberg", 33),
			("Robinson", 34),
			("Smith", 34),
			("Williams", NULL),
		;
     COMMIT;
`

func explained(db *DB, s stmt, tctx *TCtx) (string, error) {
	src := "explain " + s.String()
	rs, _, err := db.Run(tctx, src, int64(30))
	if err != nil {
		return "", err
	}

	rows, err := rs[0].Rows(-1, 0)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(rows[0][0].(string), "┌") {
		return "", nil
	}

	var a []string
	for _, v := range rows {
		a = append(a, v[0].(string))
	}
	return strings.Join(a, "\n"), nil
}

// Test provides a testing facility for alternative storage implementations.
// The s.setup should return a freshly created and empty storage. Removing the
// store from the system is the responsibility of the caller. The test only
// guarantees not to panic on recoverable errors and return an error instead.
// Test errors are not returned but reported to t.
func test(t *testing.T, s testDB) (panicked error) {
	defer func() {
		if e := recover(); e != nil {
			switch x := e.(type) {
			case error:
				panicked = x
			default:
				panicked = fmt.Errorf("%v", e)
			}
		}
		if panicked != nil {
			t.Errorf("PANIC: %v\n%s", panicked, debug.Stack())
		}
	}()

	db, err := s.setup()
	if err != nil {
		t.Error(err)
		return
	}

	tctx := NewRWCtx()
	if !*oSlow {
		if _, _, err := db.Execute(tctx, txBegin); err != nil {
			t.Error(err)
			return nil
		}
	}

	if err = s.mark(); err != nil {
		t.Error(err)
		return
	}

	defer func() {
		x := tctx
		if *oSlow {
			x = nil
		}
		if err = s.teardown(x); err != nil {
			t.Error(err)
		}
	}()

	chk := func(test int, err error, expErr string, re *regexp.Regexp) (ok bool) {
		s := err.Error()
		if re == nil {
			t.Error("FAIL: ", test, s)
			return false
		}

		if !re.MatchString(s) {
			t.Error("FAIL: ", test, "error doesn't match:", s, "expected", expErr)
			return false
		}

		return true
	}

	var logf *os.File
	hasLogf := false
	noErrors := true
	if _, ok := s.(*memTestDB); ok {
		if logf, err = ioutil.TempFile("", "ql-test-log-"); err != nil {
			t.Error(err)
			return nil
		}

		hasLogf = true
	} else {
		if logf, err = os.Create(os.DevNull); err != nil {
			t.Error(err)
			return nil
		}
	}

	defer func() {
		if hasLogf && noErrors {
			func() {
				if _, err := logf.Seek(0, 0); err != nil {
					t.Error(err)
					return
				}

				dst, err := os.Create("testdata.log")
				if err != nil {
					t.Error(err)
					return
				}

				if _, err := io.Copy(dst, logf); err != nil {
					t.Error(err)
					return
				}

				if err := dst.Close(); err != nil {
					t.Error(err)
				}
			}()
		}

		nm := logf.Name()
		if err := logf.Close(); err != nil {
			t.Error(err)
		}

		if hasLogf {
			if err := os.Remove(nm); err != nil {
				t.Error(err)
			}
		}
	}()

	log := bufio.NewWriter(logf)

	defer func() {
		if err := log.Flush(); err != nil {
			t.Error(err)
		}
	}()

	max := len(testdata)
	if n := *oM; n != 0 && n < max {
		max = n
	}
	for itest, test := range testdata[*oN:max] {
		//dbg("------------------------------------------------------------- ( itest %d ) ----", itest)
		var re *regexp.Regexp
		a := strings.Split(test+"|", "|")
		q, rset := a[0], strings.TrimSpace(a[1])
		var expErr string
		if len(a) < 3 {
			t.Error(itest, "internal error 066")
			return
		}

		if expErr = a[2]; expErr != "" {
			re = regexp.MustCompile("(?i:" + strings.TrimSpace(expErr) + ")")
		}

		q = strings.Replace(q, "&or;", "|", -1)
		q = strings.Replace(q, "&oror;", "||", -1)
		list, err := Compile(q)
		if err != nil {
			if !chk(itest, err, expErr, re) && *oFastFail {
				return
			}

			continue
		}

		for _, s := range list.l {
			if err := testMentionedColumns(s); err != nil {
				t.Error(itest, err)
				return
			}
		}

		s1 := list.String()
		list1, err := Compile(s1)
		if err != nil {
			t.Errorf("recreated source does not compile: %v\n---- orig\n%s\n---- recreated\n%s", err, q, s1)
			if *oFastFail {
				return
			}

			continue
		}

		s2 := list1.String()
		if g, e := s2, s1; g != e {
			t.Errorf("recreated source is not idempotent\n---- orig\n%s\n---- recreated1\n%s\n---- recreated2\n%s", q, s1, s2)
			if *oFastFail {
				return
			}

			continue
		}

		if !func() (ok bool) {
			tnl0 := db.tnl
			defer func() {
				s3 := list.String()
				if g, e := s1, s3; g != e {
					t.Errorf("#%d: execution mutates compiled statement list\n---- orig\n%s----new\n%s", itest, g, e)
				}

				if !ok {
					noErrors = false
				}

				if noErrors {
					hdr := false
					for _, v := range list.l {
						s, err := explained(db, v, tctx)
						if err != nil {
							t.Error(err)
							return
						}

						if !strings.HasPrefix(s, "┌") {
							continue
						}

						if !hdr {
							fmt.Fprintf(log, "---- %v\n", itest)
							hdr = true
						}
						fmt.Fprintf(log, "%s\n", v)
						fmt.Fprintf(log, "%s\n\n", s)
					}
				}

				tnl := db.tnl
				if tnl != tnl0 {
					panic(fmt.Errorf("internal error 057: tnl0 %v, tnl %v", tnl0, tnl))
				}

				nfo, err := db.Info()
				if err != nil {
					dbg("", err)
					panic(err)
				}

				for _, idx := range nfo.Indices {
					//dbg("#%d: cleanup index %s", itest, idx.Name)
					if _, _, err = db.run(tctx, fmt.Sprintf(`
						BEGIN TRANSACTION;
							DROP INDEX %s;
						COMMIT;
						`,
						idx.Name)); err != nil {
						t.Errorf("#%d: cleanup DROP INDEX %s: %v", itest, idx.Name, err)
						ok = false
					}
				}
				for _, tab := range nfo.Tables {
					//dbg("#%d: cleanup table %s", itest, tab.Name)
					if _, _, err = db.run(tctx, fmt.Sprintf(`
						BEGIN TRANSACTION;
							DROP table %s;
						COMMIT;
						`,
						tab.Name)); err != nil {
						t.Errorf("#%d: cleanup DROP TABLE %s: %v", itest, tab.Name, err)
						ok = false
					}
				}
				db.hasIndex2 = 0
			}()

			if err = s.mark(); err != nil {
				t.Error(err)
				return
			}

			rs, _, err := db.Execute(tctx, list, int64(30))
			if err != nil {
				return chk(itest, err, expErr, re)
			}

			if rs == nil {
				t.Errorf("FAIL: %d: expected non nil Recordset or error %q", itest, expErr)
				return
			}

			g, err := recSetDump(rs[len(rs)-1])
			if err != nil {
				return chk(itest, err, expErr, re)
			}

			if expErr != "" {
				t.Errorf("FAIL: %d: expected error %q", itest, expErr)
				return
			}

			a = strings.Split(rset, "\n")
			for i, v := range a {
				a[i] = strings.TrimSpace(v)
			}
			e := strings.Join(a, "\n")
			if g != e {
				t.Errorf("FAIL: test # %d\n%s\n---- g\n%s\n---- e\n%s\n----", itest, q, g, e)
				return
			}

			return true
		}() && *oFastFail {
			return
		}
	}
	return
}
