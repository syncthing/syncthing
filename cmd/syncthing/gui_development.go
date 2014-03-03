//+build guidev

package main

import "github.com/codegangsta/martini"

func embeddedStatic() interface{} {
	return martini.Static("gui")
}
