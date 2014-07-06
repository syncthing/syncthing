package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/calmh/syncthing/files"
	"github.com/calmh/syncthing/protocol"
	"github.com/calmh/syncthing/scanner"
	"github.com/syndtr/goleveldb/leveldb"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	repo := flag.String("repo", "default", "Repository ID")
	node := flag.String("node", "", "Node ID (blank for global)")
	flag.Parse()

	db, err := leveldb.OpenFile(flag.Arg(0), nil)
	if err != nil {
		log.Fatal(err)
	}

	fs := files.NewSet(*repo, db)

	if *node == "" {
		log.Printf("*** Global index for repo %q", *repo)
		fs.WithGlobal(func(f scanner.File) bool {
			fmt.Println(f)
			fmt.Println("\t", fs.Availability(f.Name))
			return true
		})
	} else {
		n, err := protocol.NodeIDFromString(*node)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("*** Have index for repo %q node %q", *repo, n)
		fs.WithHave(n, func(f scanner.File) bool {
			fmt.Println(f)
			return true
		})
	}
}
