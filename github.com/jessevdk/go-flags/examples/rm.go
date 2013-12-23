package main

import (
	"fmt"
)

type RmCommand struct {
	Force bool `short:"f" long:"force" description:"Force removal of files"`
}

var rmCommand RmCommand

func (x *RmCommand) Execute(args []string) error {
	fmt.Printf("Removing (force=%v): %#v\n", x.Force, args)
	return nil
}

func init() {
	parser.AddCommand("rm",
		"Remove a file",
		"The rm command removes a file to the repository. Use -f to force removal of files.",
		&rmCommand)
}
