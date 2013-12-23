package main

import (
	"fmt"
)

type AddCommand struct {
	All bool `short:"a" long:"all" description:"Add all files"`
}

var addCommand AddCommand

func (x *AddCommand) Execute(args []string) error {
	fmt.Printf("Adding (all=%v): %#v\n", x.All, args)
	return nil
}

func init() {
	parser.AddCommand("add",
		"Add a file",
		"The add command adds a file to the repository. Use -a to add all files.",
		&addCommand)
}
