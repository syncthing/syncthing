package main

import (
	"compress/gzip"
	"flag"
	"log"
	"os"

	"github.com/calmh/syncthing/protocol"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	showBlocks := flag.Bool("b", false, "Show blocks")
	flag.Parse()
	name := flag.Arg(0)

	idxf, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	defer idxf.Close()

	gzr, err := gzip.NewReader(idxf)
	if err != nil {
		log.Fatal(err)
	}
	defer gzr.Close()

	var im protocol.IndexMessage
	err = im.DecodeXDR(gzr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Repo: %q, Files: %d", im.Repository, len(im.Files))
	for _, file := range im.Files {
		del := file.Flags&protocol.FlagDeleted != 0
		inv := file.Flags&protocol.FlagInvalid != 0
		dir := file.Flags&protocol.FlagDirectory != 0
		prm := file.Flags & 0777
		log.Printf("File: %q, Del: %v, Inv: %v, Dir: %v, Perm: 0%03o, Modified: %d, Blocks: %d",
			file.Name, del, inv, dir, prm, file.Modified, len(file.Blocks))
		if *showBlocks {
			for _, block := range file.Blocks {
				log.Printf("   Size: %6d, Hash: %x", block.Size, block.Hash)
			}
		}
	}
}
