package main

import (
	"github.com/AudriusButkevicius/cli"
	"sort"
	"time"
)

type ByAlphabet []cli.Command

func (a ByAlphabet) Len() int           { return len(a) }
func (a ByAlphabet) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAlphabet) Less(i, j int) bool { return a[i].Name < a[j].Name }

var cliCommands []cli.Command

func main() {
	app := cli.NewApp()
	app.Name = "syncthing-cli"
	app.Author = "Audrius Butkevičius"
	app.Email = "audrius.butkevicius@gmail.com"
	app.Compiled = time.Now()
	app.Usage = "Syncthing command line interface"
	app.Version = "0.1"
	app.HideHelp = true

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "endpoint, e",
			Value:  "http://127.0.0.1:8080",
			Usage:  "End point to connect to",
			EnvVar: "STENDPOINT",
		},
		cli.StringFlag{
			Name:   "apikey, k",
			Value:  "",
			Usage:  "API Key",
			EnvVar: "STAPIKEY",
		},
		cli.StringFlag{
			Name:   "username, u",
			Value:  "",
			Usage:  "Username",
			EnvVar: "STUSERNAME",
		},
		cli.StringFlag{
			Name:   "password, p",
			Value:  "",
			Usage:  "Password",
			EnvVar: "STPASSWORD",
		},
	}

	sort.Sort(ByAlphabet(cliCommands))
	app.Commands = cliCommands
	app.RunAndExitOnError()
}
