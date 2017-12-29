// Copyright 2014 The ql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command ql is a utility to explore a database, prototype a schema or test
// drive a query, etc.
//
// Installation:
//
//	$ go get github.com/cznic/ql/ql
//
// Usage:
//
//	ql [-db name] [-schema regexp] [-tables regexp] [-fld] statement_list
//
// Options:
//
//	-db name	Name of the database to use. Defaults to "ql.db".
//			If the DB file does not exists it is created automatically.
//
//	-schema re	If re != "" show the CREATE statements of matching tables and exit.
//
//	-tables re	If re != "" show the matching table names and exit.
//
//	-fld		First row of a query result set will show field names.
//
//	statement_list	QL statements to execute.
//			If no non flag arguments are present, ql reads from stdin.
//			The list is wrapped into an automatic transaction.
//
//	-t		Report and measure time to execute, including creating/opening and closing the DB.
//
// Example:
//
//	$ ql 'create table t (i int, s string)'
//	$ ql << EOF
//	> insert into t values
//	> (1, "a"),
//	> (2, "b"),
//	> (3, "c"),
//	> EOF
//	$ ql 'select * from t'
//	3, "c"
//	2, "b"
//	1, "a"
//	$ ql -fld 'select * from t where i != 2 order by s'
//	"i", "s"
//	1, "a"
//	3, "c"
//	$
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cznic/ql"
)

func str(data []interface{}) string {
	a := make([]string, len(data))
	for i, v := range data {
		switch x := v.(type) {
		case string:
			a[i] = fmt.Sprintf("%q", x)
		default:
			a[i] = fmt.Sprint(x)
		}
	}
	return strings.Join(a, ", ")
}

func main() {
	if err := do(); err != nil {
		log.Fatal(err)
	}
}

type config struct {
	db          string
	flds        bool
	schema      string
	tables      string
	time        bool
	help        bool
	interactive bool
}

func (c *config) parse() {
	db := flag.String("db", "ql.db", "The DB file to open. It'll be created if missing.")
	flds := flag.Bool("fld", false, "Show recordset's field names.")
	schema := flag.String("schema", "", "If non empty, show the CREATE statements of matching tables and exit.")
	tables := flag.String("tables", "", "If non empty, list matching table names and exit.")
	time := flag.Bool("t", false, "Measure and report time to execute the statement(s) including DB create/open/close.")
	help := flag.Bool("h", false, "Shows this help text.")
	interactive := flag.Bool("i", false, "runs in interactive mode")
	flag.Parse()
	c.flds = *flds
	c.db = *db
	c.schema = *schema
	c.tables = *tables
	c.time = *time
	c.help = *help
	c.interactive = *interactive
}

func do() (err error) {
	cfg := &config{}
	cfg.parse()
	if cfg.help {
		flag.PrintDefaults()
		return nil
	}
	if flag.NArg() == 0 && !cfg.interactive {

		// Somehow we expect input to the ql tool.
		// This will block trying to read input from stdin
		b, err := ioutil.ReadAll(os.Stdin)
		if err != nil || len(b) == 0 {
			flag.PrintDefaults()
			return nil
		}
		db, err := ql.OpenFile(cfg.db, &ql.Options{CanCreate: true})
		if err != nil {
			return err
		}
		defer func() {
			ec := db.Close()
			switch {
			case ec != nil && err != nil:
				log.Println(ec)
			case ec != nil:
				err = ec
			}
		}()
		return run(cfg, bufio.NewWriter(os.Stdout), string(b), db)
	}
	db, err := ql.OpenFile(cfg.db, &ql.Options{CanCreate: true})
	if err != nil {
		return err
	}

	defer func() {
		ec := db.Close()
		switch {
		case ec != nil && err != nil:
			log.Println(ec)
		case ec != nil:
			err = ec
		}
	}()
	r := bufio.NewReader(os.Stdin)
	o := bufio.NewWriter(os.Stdout)
	if cfg.interactive {
		for {
			o.WriteString("ql> ")
			o.Flush()
			src, err := readSrc(cfg.interactive, r)
			if err != nil {
				return err
			}
			err = run(cfg, o, src, db)
			if err != nil {
				fmt.Fprintln(o, err)
				o.Flush()
			}
		}
		return nil
	}
	src, err := readSrc(cfg.interactive, r)
	if err != nil {
		return err
	}
	return run(cfg, o, src, db)
}

func readSrc(i bool, in *bufio.Reader) (string, error) {
	if i {
		return in.ReadString('\n')
	}
	var src string
	switch n := flag.NArg(); n {
	case 0:
		b, err := ioutil.ReadAll(in)
		if err != nil {
			return "", err
		}

		src = string(b)
	default:
		a := make([]string, n)
		for i := range a {
			a[i] = flag.Arg(i)
		}
		src = strings.Join(a, " ")
	}
	return src, nil
}

func run(cfg *config, o *bufio.Writer, src string, db *ql.DB) (err error) {
	defer o.Flush()
	if cfg.interactive {
		src = strings.TrimSpace(src)
		if strings.HasPrefix(src, "\\") ||
			strings.HasPrefix(src, ".") {
			switch src {
			case "\\clear", ".clear":
				switch runtime.GOOS {
				case "darwin", "linux":
					fmt.Fprintln(o, "\033[H\033[2J")
				default:
					fmt.Fprintln(o, "clear not supported in this system")
				}
				return nil
			case "\\q", "\\exit", ".q", ".exit":
				// we make sure to close the database before exiting
				db.Close()
				os.Exit(1)
			}
		}

	}

	t0 := time.Now()
	if cfg.time {
		defer func() {
			fmt.Fprintf(os.Stderr, "%s\n", time.Since(t0))
		}()
	}
	if pat := cfg.schema; pat != "" {
		re, err := regexp.Compile(pat)
		if err != nil {
			return err
		}

		nfo, err := db.Info()
		if err != nil {
			return err
		}

		r := []string{}
		for _, ti := range nfo.Tables {
			if !re.MatchString(ti.Name) {
				continue
			}

			a := []string{}
			for _, ci := range ti.Columns {
				a = append(a, fmt.Sprintf("%s %s", ci.Name, ci.Type))
			}
			r = append(r, fmt.Sprintf("CREATE TABLE %s (%s);", ti.Name, strings.Join(a, ", ")))
		}
		sort.Strings(r)
		if len(r) != 0 {
			fmt.Fprintln(o, strings.Join(r, "\n"))
		}
		return nil
	}

	if pat := cfg.tables; pat != "" {
		re, err := regexp.Compile(pat)
		if err != nil {
			return err
		}

		nfo, err := db.Info()
		if err != nil {
			return err
		}

		r := []string{}
		for _, ti := range nfo.Tables {
			if !re.MatchString(ti.Name) {
				continue
			}

			r = append(r, ti.Name)
		}
		sort.Strings(r)
		if len(r) != 0 {
			fmt.Fprintln(o, strings.Join(r, "\n"))
		}
		return nil
	}

	src = strings.TrimSpace(src)

	commit := "COMMIT;"
	if !strings.HasSuffix(src, ";") {
		commit = "; " + commit
	}
	src = "BEGIN TRANSACTION; " + src + commit
	l, err := ql.Compile(src)
	if err != nil {
		log.Println(src)
		return err
	}

	rs, i, err := db.Execute(ql.NewRWCtx(), l)
	if err != nil {
		a := strings.Split(strings.TrimSpace(fmt.Sprint(l)), "\n")
		return fmt.Errorf("%v: %s", err, a[i])
	}

	if len(rs) == 0 {
		return
	}

	switch {
	case l.IsExplainStmt():
		return rs[len(rs)-1].Do(cfg.flds, func(data []interface{}) (bool, error) {
			fmt.Fprintln(o, data[0])
			return true, nil
		})
	default:
		for _, rst := range rs {
			err = rst.Do(cfg.flds, func(data []interface{}) (bool, error) {
				fmt.Fprintln(o, str(data))
				return true, nil
			})
			o.Flush()
			if err != nil {
				return
			}
		}
		return
	}
}
