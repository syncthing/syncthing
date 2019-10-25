package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/syncthing/syncthing/lib/db/backend"
)

func main() {
	from := flag.String("from", "leveldb", "Source format (leveldb or badger)")
	to := flag.String("to", "badger", "Destination format (leveldb or badger)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Println("Give path to source database, and optionally format parameters")
		os.Exit(2)
	}

	var fromDB backend.Backend
	var toDB backend.Backend
	var err error

	switch *from {
	case "leveldb":
		fromDB, err = backend.OpenLevelDBRO(flag.Arg(0))
	case "badger":
		fromDB, err = backend.OpenBadger(flag.Arg(0))
	}
	if err != nil {
		fmt.Println("Opening source:", err)
		os.Exit(1)
	}

	switch *to {
	case "leveldb":
		toDB, err = backend.OpenLevelDB(flag.Arg(0)+".to-leveldb", backend.TuningAuto)
	case "badger":
		toDB, err = backend.OpenBadger(flag.Arg(0) + ".to-badger")
	}
	if err != nil {
		fmt.Println("Opening destination:", err)
		os.Exit(1)
	}

	srcIt, err := fromDB.NewPrefixIterator(nil)
	if err != nil {
		fmt.Println("Iterating source:", err)
		os.Exit(1)
	}
	dstTx, err := toDB.NewWriteTransaction()
	if err != nil {
		fmt.Println("Destination transaction:", err)
		os.Exit(1)
	}
	for srcIt.Next() {
		if err := dstTx.Put(srcIt.Key(), srcIt.Value()); err != nil {
			fmt.Println("Destination put:", err)
			os.Exit(1)
		}
	}
	if srcIt.Error() != nil {
		fmt.Println("Iterating source:", err)
		os.Exit(1)
	}
	srcIt.Release()
	fromDB.Close()

	if dstTx.Commit() != nil {
		fmt.Println("Destination commit:", err)
		os.Exit(1)
	}
	toDB.Close()
}
